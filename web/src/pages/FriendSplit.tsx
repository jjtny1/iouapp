import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useParams } from "react-router-dom";
import { api, type Bill, type BillSummary, type PaymentIntent } from "../api";
import { formatMoney } from "../money";
import { Avatar, AvatarStack, Brand, Icon, PaperApp, QrCode } from "../ui";

// Phones get a venmo:// deep link that opens the Venmo app; desktops, which
// have no app, get a QR code to scan with their phone instead.
const isMobile =
  typeof navigator !== "undefined" &&
  /iphone|ipad|ipod|android/i.test(navigator.userAgent);

function tokenKey(billId: string): string {
  return `iou:participant:${billId}`;
}
function idKey(billId: string): string {
  return `iou:participant-id:${billId}`;
}

export default function FriendSplit() {
  const { token } = useParams<{ token: string }>();
  const [bill, setBill] = useState<Bill | null>(null);
  const [participantToken, setParticipantToken] = useState<string | null>(null);
  const [participantId, setParticipantId] = useState<string | null>(null);
  const [summary, setSummary] = useState<BillSummary | null>(null);
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [joining, setJoining] = useState(false);
  const [payOpen, setPayOpen] = useState(false);
  const [intent, setIntent] = useState<PaymentIntent | null>(null);
  const [loadingIntent, setLoadingIntent] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [handedOff, setHandedOff] = useState(false);

  useEffect(() => {
    if (!token) return;
    api
      .billByToken(token)
      .then((b) => {
        setBill(b);
        // A host-split bill is a shared roster: every visitor should land on
        // the identity picker and choose who they are, so its pick is never
        // persisted. Only the self-claim flow restores a saved participant.
        if (b.split_mode !== "host") {
          const stored = localStorage.getItem(tokenKey(b.id));
          if (stored) setParticipantToken(stored);
          const storedId = localStorage.getItem(idKey(b.id));
          if (storedId) setParticipantId(storedId);
        }
      })
      .catch((err) =>
        setError(err instanceof Error ? err.message : "could not load bill"),
      )
      .finally(() => setLoading(false));
  }, [token]);

  const refresh = useCallback(async () => {
    if (!bill || !token) return;
    try {
      setSummary(await api.summary(bill.id, token));
    } catch (err) {
      setError(err instanceof Error ? err.message : "could not load summary");
    }
  }, [bill, token]);

  const isHostSplit = bill?.split_mode === "host";

  useEffect(() => {
    // refresh() is async — setState runs after the await, not synchronously.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    if (bill && (participantToken || isHostSplit)) void refresh();
  }, [bill, participantToken, isHostSplit, refresh]);

  async function onJoin(e: FormEvent) {
    e.preventDefault();
    if (!bill || !token) return;
    const trimmed = name.trim();
    if (!trimmed) return;
    setJoining(true);
    setError(null);
    try {
      const res = await api.joinBill(bill.id, trimmed, token);
      localStorage.setItem(tokenKey(bill.id), res.participant_token);
      localStorage.setItem(idKey(bill.id), res.participant.id);
      setParticipantToken(res.participant_token);
      setParticipantId(res.participant.id);
    } catch (err) {
      setError(err instanceof Error ? err.message : "could not join");
    } finally {
      setJoining(false);
    }
  }

  // pickIdentity is the host-split equivalent of joining: the friend taps
  // their pre-created name and we adopt that participant's token for the
  // existing payment flow. No new participant is created. The choice is
  // session-only — deliberately not persisted — so reopening the link returns
  // to the picker and a different person (or the same one) can pick.
  function pickIdentity(pickedId: string, pToken: string) {
    setParticipantToken(pToken);
    setParticipantId(pickedId);
  }

  // clearIdentity drops the current host-split pick and returns to the picker.
  function clearIdentity() {
    setParticipantToken(null);
    setParticipantId(null);
    setError(null);
  }

  async function toggleItem(itemId: string) {
    if (!bill || !participantToken || !summary || !participantId) return;
    const current = new Set(
      Object.entries(summary.claims)
        .filter(([, ids]) => ids.includes(participantId))
        .map(([cid]) => cid),
    );
    if (current.has(itemId)) current.delete(itemId);
    else current.add(itemId);
    try {
      setSummary(await api.setClaims(bill.id, participantToken, [...current]));
    } catch (err) {
      setError(err instanceof Error ? err.message : "could not update");
    }
  }

  async function openPay() {
    if (!bill || !participantToken) return;
    setPayOpen(true);
    setError(null);
    setHandedOff(false);
    setLoadingIntent(true);
    try {
      const res = await api.pay(bill.id, participantToken);
      if (res.status === "paid") {
        await refresh();
        setPayOpen(false);
      } else {
        setIntent(res);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "could not start payment");
      setPayOpen(false);
    } finally {
      setLoadingIntent(false);
    }
  }

  // openVenmo hands the friend off to the Venmo app (phones) or venmo.com
  // (desktop), prefilled with the host's handle and the amount owed.
  function openVenmo() {
    if (!intent) return;
    setHandedOff(true);
    if (isMobile) {
      window.location.href = intent.app_url;
    } else {
      window.open(intent.web_url, "_blank", "noopener");
    }
  }

  async function confirmPay() {
    if (!bill || !participantToken || !intent) return;
    setConfirming(true);
    setError(null);
    try {
      await api.confirmPayment(bill.id, participantToken, intent.payment_id);
      setIntent(null);
      await refresh();
      setPayOpen(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "could not confirm");
    } finally {
      setConfirming(false);
    }
  }

  /* ── Loading / not-found ─────────────────────────────────────────── */
  if (loading) {
    return (
      <PaperApp>
        <div
          className="page-center"
          style={{ alignItems: "center", justifyContent: "center" }}
        >
          <Brand size={56} />
          <p className="eyebrow mt-6">Loading…</p>
        </div>
      </PaperApp>
    );
  }

  if (!bill) {
    return (
      <PaperApp>
        <div
          className="page-center"
          style={{ alignItems: "center", justifyContent: "center" }}
        >
          <Brand size={56} />
          <p className="body danger mt-6 center">
            {error ?? "This tab couldn't be found."}
          </p>
        </div>
      </PaperApp>
    );
  }

  /* ── Host-split identity picker ──────────────────────────────────── */
  // For a host-managed split the host already assigned items; the friend
  // just taps which person they are, then pays. No self-claiming.
  if (isHostSplit && !participantToken) {
    if (!summary) {
      return (
        <PaperApp>
          <div
            className="page-center"
            style={{ alignItems: "center", justifyContent: "center" }}
          >
            <Brand size={56} />
            <p className="eyebrow mt-6">Loading the split…</p>
          </div>
        </PaperApp>
      );
    }
    const fmtH = (c: number) => formatMoney(c, bill.currency);
    const pickable = summary.participants.filter(
      (p) => p.host_managed && !p.is_host,
    );
    return (
      <PaperApp>
        <div className="page-center">
          <p className="eyebrow center">{bill.restaurant || "The tab"}</p>
          <p
            className="center"
            style={{
              margin: "10px 0 0",
              fontFamily: "var(--serif)",
              fontStyle: "italic",
              fontSize: 36,
              lineHeight: 1.05,
              letterSpacing: "-0.01em",
              color: "var(--ink)",
            }}
          >
            Which one are you?
          </p>
          <p className="mono muted center mt-3" style={{ fontSize: 11 }}>
            The host already split this — tap your name to pay.
          </p>

          {error && (
            <p className="body danger center mt-3" style={{ fontSize: 13 }}>
              {error}
            </p>
          )}

          <div className="col mt-6" style={{ paddingBottom: 14 }}>
            {pickable.length === 0 ? (
              <p className="body muted center" style={{ fontSize: 13 }}>
                No one to pick yet — check back once the host finishes.
              </p>
            ) : (
              pickable.map((p) => {
                const share = summary.split.participants.find(
                  (s) => s.participant_id === p.id,
                );
                const paid = p.payment_status === "paid";
                return (
                  <button
                    key={p.id}
                    className="party-row"
                    disabled={!p.participant_token}
                    onClick={() =>
                      p.participant_token &&
                      pickIdentity(p.id, p.participant_token)
                    }
                    style={{
                      width: "100%",
                      textAlign: "left",
                      background: "transparent",
                      cursor: p.participant_token ? "pointer" : "default",
                    }}
                  >
                    <Avatar name={p.display_name} seed={p.id} size="md" />
                    <div className="flex1">
                      <p style={{ margin: 0, fontSize: 14 }}>
                        {p.display_name}
                      </p>
                      <p
                        className="mono muted"
                        style={{ margin: 0, fontSize: 11 }}
                      >
                        {paid ? "paid ✓" : "tap to pay"}
                      </p>
                    </div>
                    <span className="mono" style={{ fontSize: 13 }}>
                      {fmtH(share?.total_cents ?? 0)}
                    </span>
                    <Icon.Arrow size={13} />
                  </button>
                );
              })
            )}
          </div>
        </div>
      </PaperApp>
    );
  }

  /* ── Join ────────────────────────────────────────────────────────── */
  if (!participantToken) {
    return (
      <PaperApp>
        <div className="page-center">
          <p className="eyebrow center">You're invited to split</p>
          <p
            className="center"
            style={{
              margin: "10px 0 0",
              fontFamily: "var(--serif)",
              fontStyle: "italic",
              fontSize: 36,
              lineHeight: 1.05,
              letterSpacing: "-0.01em",
              color: "var(--ink)",
            }}
          >
            {bill.restaurant || "Split the bill"}
          </p>
          <p className="mono muted center mt-3" style={{ fontSize: 11 }}>
            {bill.items.length} items · pick what you ordered
          </p>

          {error && (
            <p className="body danger center mt-3" style={{ fontSize: 13 }}>
              {error}
            </p>
          )}

          <form
            onSubmit={onJoin}
            className="col gap-3"
            style={{ marginTop: "auto", paddingBottom: 14 }}
          >
            <label className="eyebrow" htmlFor="name">
              Your name
            </label>
            <input
              id="name"
              className="input"
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. Maya"
            />
            <button
              type="submit"
              className="btn btn-block mt-2"
              disabled={joining}
            >
              {joining ? "Joining…" : "Join the tab"} <Icon.Arrow size={12} />
            </button>
            <p
              className="body muted center"
              style={{ fontSize: 11, marginTop: 6 }}
            >
              No account needed.
            </p>
          </form>
        </div>
      </PaperApp>
    );
  }

  if (!summary) {
    return (
      <PaperApp>
        <div
          className="page-center"
          style={{ alignItems: "center", justifyContent: "center" }}
        >
          <Brand size={56} />
          <p className="eyebrow mt-6">Loading items…</p>
        </div>
      </PaperApp>
    );
  }

  const myId = participantId;
  const myParticipant = myId
    ? summary.participants.find((p) => p.id === myId)
    : undefined;
  const myShare = myId
    ? summary.split.participants.find((s) => s.participant_id === myId)
    : undefined;
  const myName = myParticipant?.display_name || name || "you";
  const firstName = myName.split(/\s+/)[0];
  const isPaid = myParticipant?.payment_status === "paid";
  const owes = myShare?.total_cents ?? 0;
  const fmt = (c: number) => formatMoney(c, bill.currency);
  // The "you owe" total folds prorated tax, tip and service on top of the
  // claimed items — spell each part out with its own amount so the number
  // isn't a mystery.
  const myItemCents = myShare?.item_subtotal_cents ?? 0;
  const myTaxCents = myShare?.tax_cents ?? 0;
  const myTipCents = myShare?.tip_cents ?? 0;
  const myServiceCents = myShare?.service_cents ?? 0;
  const extrasCents = myTaxCents + myTipCents + myServiceCents;
  const owedBreakdown = [
    `${fmt(myItemCents)} items`,
    myTaxCents > 0 ? `${fmt(myTaxCents)} tax` : null,
    myTipCents > 0 ? `${fmt(myTipCents)} tip` : null,
    myServiceCents > 0 ? `${fmt(myServiceCents)} service` : null,
  ]
    .filter(Boolean)
    .join(" + ");
  const nameOf = (pid: string) =>
    summary.participants.find((p) => p.id === pid)?.display_name ?? "?";
  const serviceTotal = summary.split.service_charge_cents;
  const serviceDiners =
    bill.service_charge_headcount > 0
      ? bill.service_charge_headcount
      : summary.participants.length;

  /* ── Paid ────────────────────────────────────────────────────────── */
  if (isPaid) {
    return (
      <PaperApp>
        <div
          className="page-center"
          style={{ paddingTop: 22, paddingBottom: 28 }}
        >
          <div className="row row-between">
            <Brand size={26} />
            <span className="eyebrow">{bill.restaurant || "Tab"}</span>
          </div>

          <div
            style={{
              flex: 1,
              display: "flex",
              flexDirection: "column",
              alignItems: "center",
              justifyContent: "center",
              textAlign: "center",
            }}
          >
            <div className="bigcheck">
              <Icon.CheckBig size={48} />
            </div>
            <p className="eyebrow mt-4">Settled up</p>
            <h2 className="h-section mt-2" style={{ fontSize: 30 }}>
              You're square,
              <br />
              {firstName}.
            </h2>

            <div className="card mt-4" style={{ width: "100%" }}>
              <div className="row row-between">
                <span className="mono muted" style={{ fontSize: 11 }}>
                  Items
                </span>
                <span className="mono" style={{ fontSize: 13 }}>
                  {fmt(myShare?.item_subtotal_cents ?? 0)}
                </span>
              </div>
              <div className="row row-between mt-2">
                <span className="mono muted" style={{ fontSize: 11 }}>
                  Tax + Tip
                </span>
                <span className="mono" style={{ fontSize: 13 }}>
                  {fmt((myShare?.tax_cents ?? 0) + (myShare?.tip_cents ?? 0))}
                </span>
              </div>
              {(myShare?.service_cents ?? 0) > 0 && (
                <div className="row row-between mt-2">
                  <span className="mono muted" style={{ fontSize: 11 }}>
                    Service
                  </span>
                  <span className="mono" style={{ fontSize: 13 }}>
                    {fmt(myShare?.service_cents ?? 0)}
                  </span>
                </div>
              )}
              <hr className="dash" style={{ margin: "10px 0" }} />
              <div className="row row-between">
                <span
                  style={{
                    fontFamily: "var(--serif)",
                    fontStyle: "italic",
                    fontSize: 18,
                  }}
                >
                  Total
                </span>
                <span
                  className="mono"
                  style={{ fontSize: 18, fontWeight: 600 }}
                >
                  {fmt(owes)}
                </span>
              </div>
            </div>
          </div>

          <p className="body muted center" style={{ fontSize: 12 }}>
            Paid in Venmo — you can close this page.
          </p>
          {isHostSplit && (
            <button
              className="link-btn"
              onClick={clearIdentity}
              style={{ marginTop: 10, alignSelf: "center" }}
            >
              Not {firstName}? Pick someone else
            </button>
          )}
        </div>
      </PaperApp>
    );
  }

  /* ── Pay sheet (shared by both flows) ────────────────────────────── */
  const paySheet = payOpen && (
    <>
      <div
        className="sheet-backdrop"
        onClick={() => !confirming && setPayOpen(false)}
      />
      <div className="sheet">
        <div className="sheet-handle" />
        <div className="row row-between">
          <span className="eyebrow">Settle the tab</span>
          <button
            onClick={() => !confirming && setPayOpen(false)}
            style={{
              background: "transparent",
              border: 0,
              color: "var(--muted)",
              fontSize: 16,
              cursor: "pointer",
            }}
          >
            ✕
          </button>
        </div>
        <p className="h-section mt-2">Settle up.</p>

        <div className="card mt-4">
          <div className="row row-between">
            <span className="body muted" style={{ fontSize: 13 }}>
              Your items
            </span>
            <span className="mono" style={{ fontSize: 13 }}>
              {fmt(myShare?.item_subtotal_cents ?? 0)}
            </span>
          </div>
          <div className="row row-between mt-2">
            <span className="body muted" style={{ fontSize: 13 }}>
              Tax + tip
            </span>
            <span className="mono" style={{ fontSize: 13 }}>
              {fmt((myShare?.tax_cents ?? 0) + (myShare?.tip_cents ?? 0))}
            </span>
          </div>
          {(myShare?.service_cents ?? 0) > 0 && (
            <div className="row row-between mt-2">
              <span className="body muted" style={{ fontSize: 13 }}>
                Service charge
              </span>
              <span className="mono" style={{ fontSize: 13 }}>
                {fmt(myShare?.service_cents ?? 0)}
              </span>
            </div>
          )}
          <hr className="dash" style={{ margin: "10px 0" }} />
          <div className="row row-between">
            <span
              style={{
                fontFamily: "var(--serif)",
                fontStyle: "italic",
                fontSize: 20,
              }}
            >
              Total
            </span>
            <span className="mono" style={{ fontSize: 19, fontWeight: 600 }}>
              {fmt(owes)}
            </span>
          </div>
        </div>

        {loadingIntent ? (
          <p className="body muted center mt-4" style={{ fontSize: 13 }}>
            <span className="spinner" /> Preparing your Venmo payment…
          </p>
        ) : intent ? (
          <>
            {/* Venmo recipient */}
            <div
              className="mt-4"
              style={{
                background: "rgba(61,149,206,.12)",
                borderRadius: 12,
                padding: "12px 14px",
                display: "flex",
                alignItems: "center",
                gap: 10,
              }}
            >
              <div
                style={{
                  width: 32,
                  height: 32,
                  borderRadius: 8,
                  background: "#3D95CE",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  color: "#fff",
                  fontWeight: 700,
                  fontSize: 17,
                }}
              >
                V
              </div>
              <div style={{ flex: 1, minWidth: 0 }}>
                <p style={{ margin: 0, fontSize: 13, fontWeight: 500 }}>
                  Pay @{intent.venmo_handle} on Venmo
                </p>
                <p
                  className="mono muted truncate"
                  style={{ margin: "2px 0 0", fontSize: 10.5 }}
                >
                  {fmt(owes)} · {intent.note}
                </p>
              </div>
            </div>

            {isMobile ? (
              <>
                <button
                  className="btn btn-block mt-4"
                  style={{ padding: "14px 18px" }}
                  onClick={openVenmo}
                >
                  Open Venmo <Icon.Arrow size={12} />
                </button>
                <p
                  className="body muted center"
                  style={{ fontSize: 11, marginTop: 8 }}
                >
                  Venmo opens prefilled with {fmt(owes)} to @
                  {intent.venmo_handle}.
                </p>
              </>
            ) : (
              <div
                className="mt-4"
                style={{
                  display: "flex",
                  flexDirection: "column",
                  alignItems: "center",
                }}
              >
                {/* The QR encodes the venmo:// app link so a phone
                    camera opens it straight in the Venmo app; the web
                    link below is the fallback for paying on the desktop
                    itself, which has no app. */}
                <QrCode value={intent.app_url} />
                <p
                  className="body muted center"
                  style={{ fontSize: 11, marginTop: 8 }}
                >
                  Scan with your phone's camera to pay {fmt(owes)} in the Venmo
                  app — or{" "}
                  <a
                    href={intent.web_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    onClick={() => setHandedOff(true)}
                  >
                    open Venmo on the web
                  </a>
                  .
                </p>
              </div>
            )}

            {error && (
              <p className="body danger mt-3" style={{ fontSize: 12 }}>
                {error}
              </p>
            )}

            <hr className="dash" style={{ margin: "16px 0 12px" }} />
            <p
              className="body muted center"
              style={{ fontSize: 12, marginBottom: 8 }}
            >
              {handedOff
                ? "All done in Venmo? Mark yourself settled."
                : "Already sent it in Venmo?"}
            </p>
            <button
              className={`btn btn-block${handedOff ? "" : " btn-ghost"}`}
              disabled={confirming}
              onClick={confirmPay}
            >
              {confirming ? (
                <>
                  <span className="spinner" /> Saving…
                </>
              ) : (
                <>
                  <Icon.Check size={12} /> I've paid {fmt(owes)}
                </>
              )}
            </button>
          </>
        ) : (
          error && (
            <p className="body danger mt-3" style={{ fontSize: 12 }}>
              {error}
            </p>
          )
        )}
      </div>
    </>
  );

  /* ── Host-split: pick-your-name flow has no claiming, just pay ───── */
  if (isHostSplit) {
    const myClaimedItems = myId
      ? summary.items.filter((it) =>
          (summary.claims[it.id] ?? []).includes(myId),
        )
      : [];
    return (
      <PaperApp>
        <div className="page" style={{ paddingBottom: 8 }}>
          <div className="row row-between">
            <Brand size={26} />
            <Avatar name={myName} seed={myId ?? myName} />
          </div>

          <p className="eyebrow mt-4">
            {bill.restaurant || "The tab"} · {summary.participants.length}{" "}
            splitting
          </p>
          <h2 className="h-section mt-1">Here's your share, {firstName}.</h2>
          <p className="body muted mt-2">
            The host already split this tab — just settle up below.
          </p>
          <button
            className="link-btn"
            onClick={clearIdentity}
            style={{ marginTop: 8 }}
          >
            Not {firstName}? Pick someone else
          </button>

          {error && (
            <p className="body danger mt-3" style={{ fontSize: 13 }}>
              {error}
            </p>
          )}

          <div className="card mt-4">
            <p className="eyebrow">Assigned to you</p>
            {myClaimedItems.length === 0 ? (
              <p className="body muted mt-2" style={{ fontSize: 13 }}>
                No items were assigned to you — your share covers tax, tip and
                any service charge.
              </p>
            ) : (
              <div className="col mt-2">
                {myClaimedItems.map((it) => {
                  const claimers = summary.claims[it.id] ?? [];
                  const ea =
                    claimers.length > 0
                      ? Math.round(it.price_cents / claimers.length)
                      : it.price_cents;
                  return (
                    <div key={it.id} className="row row-between">
                      <span style={{ fontSize: 14 }}>
                        {it.name || "Item"}
                        {claimers.length > 1 && (
                          <span
                            className="mono muted"
                            style={{ fontSize: 10, marginLeft: 6 }}
                          >
                            split {claimers.length} ways
                          </span>
                        )}
                      </span>
                      <span className="mono" style={{ fontSize: 13 }}>
                        {fmt(ea)}
                      </span>
                    </div>
                  );
                })}
              </div>
            )}
          </div>

          {serviceTotal > 0 && (
            <div className="card mt-3">
              <p className="eyebrow">Service charge</p>
              <p className="body muted mt-2" style={{ fontSize: 12 }}>
                This tab adds a{" "}
                {bill.service_charge_kind === "percent"
                  ? `${bill.service_charge_rate_bps / 100}% `
                  : ""}
                service charge of {fmt(serviceTotal)} — folded into your share
                below.
              </p>
            </div>
          )}

          <div className="totalbar">
            <div>
              <p className="label">You owe</p>
              <p className="amt">{fmt(owes)}</p>
              {owes > 0 && extrasCents > 0 && (
                <p className="sub">{owedBreakdown}</p>
              )}
            </div>
            <button
              className="btn btn-accent"
              disabled={owes <= 0}
              onClick={openPay}
            >
              Pay <Icon.Arrow size={12} />
            </button>
          </div>
        </div>
        {paySheet}
      </PaperApp>
    );
  }

  /* ── Claim (+ pay sheet) ─────────────────────────────────────────── */
  const anyClaim = myId
    ? Object.values(summary.claims).some((ids) => ids.includes(myId))
    : false;

  return (
    <PaperApp>
      <div className="page" style={{ paddingBottom: 8 }}>
        <div className="row row-between">
          <Brand size={26} />
          <Avatar name={myName} seed={myId ?? myName} />
        </div>

        <p className="eyebrow mt-4">
          {bill.restaurant || "The tab"} · {summary.participants.length}{" "}
          splitting
        </p>
        <h2 className="h-section mt-1">What did you get, {firstName}?</h2>
        <p className="body muted mt-2">
          Tap each item. Shared things split evenly.
        </p>

        {error && (
          <p className="body danger mt-3" style={{ fontSize: 13 }}>
            {error}
          </p>
        )}

        {serviceTotal > 0 && (
          <div className="card mt-3">
            <p className="eyebrow">Service charge</p>
            <p className="body muted mt-2" style={{ fontSize: 12 }}>
              This tab adds a{" "}
              {bill.service_charge_kind === "percent"
                ? `${bill.service_charge_rate_bps / 100}% `
                : ""}
              service charge of {fmt(serviceTotal)} — not an item you claim.{" "}
              {bill.service_charge_kind === "percent"
                ? "It's split across what each person ordered."
                : `Split evenly between ${serviceDiners} diner${
                    serviceDiners === 1 ? "" : "s"
                  }.`}
            </p>
          </div>
        )}

        <div className="col mt-4">
          {summary.items.map((it) => {
            const claimers = summary.claims[it.id] ?? [];
            const mine = myId ? claimers.includes(myId) : false;
            const others = claimers
              .filter((c) => c !== myId)
              .map((c) => ({ id: c, name: nameOf(c) }));
            const ea =
              claimers.length > 0
                ? Math.round(it.price_cents / claimers.length)
                : it.price_cents;
            return (
              <button
                key={it.id}
                className={`claim-item${mine ? " mine" : ""}`}
                onClick={() => toggleItem(it.id)}
              >
                <span className={`claim-box${mine ? " checked" : ""}`}>
                  <Icon.Check size={12} />
                </span>
                <div>
                  <p style={{ margin: 0, fontSize: 14, color: "var(--ink)" }}>
                    {it.name || "Item"}
                  </p>
                  {others.length > 0 && (
                    <div className="row gap-1 mt-1">
                      <AvatarStack people={others} size="xs" />
                      {claimers.length > 1 && (
                        <span className="mono muted" style={{ fontSize: 10 }}>
                          split {claimers.length} ways
                        </span>
                      )}
                    </div>
                  )}
                </div>
                <div style={{ textAlign: "right" }}>
                  <p
                    className="mono"
                    style={{
                      margin: 0,
                      fontSize: 13,
                      color: "var(--ink)",
                      fontWeight: mine ? 600 : 400,
                    }}
                  >
                    {fmt(it.price_cents)}
                  </p>
                  {mine && claimers.length > 1 && (
                    <p
                      className="mono"
                      style={{
                        margin: "2px 0 0",
                        fontSize: 10,
                        color: "var(--accent-deep)",
                      }}
                    >
                      you: {fmt(ea)}
                    </p>
                  )}
                </div>
              </button>
            );
          })}
        </div>

        <div className="totalbar">
          <div>
            <p className="label">You owe</p>
            <p className="amt">{fmt(owes)}</p>
            {owes > 0 && extrasCents > 0 && (
              <p className="sub">{owedBreakdown}</p>
            )}
          </div>
          <button
            className="btn btn-accent"
            disabled={!anyClaim || owes <= 0}
            onClick={openPay}
          >
            Pay <Icon.Arrow size={12} />
          </button>
        </div>
      </div>

      {paySheet}
    </PaperApp>
  );
}
