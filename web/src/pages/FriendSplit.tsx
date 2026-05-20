import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { useParams } from "react-router-dom";
import { api, type Bill, type BillSummary, type PaymentIntent } from "../api";
import { useAuth } from "../auth";
import { formatMoney } from "../money";
import { Avatar, AvatarStack, Brand, Icon, PaperApp, QrCode } from "../ui";

// Phones get a venmo:// deep link that opens the Venmo app; desktops, which
// have no app, get a QR code to scan with their phone instead.
const isMobile =
  typeof navigator !== "undefined" &&
  /iphone|ipad|ipod|android/i.test(navigator.userAgent);

// MAX_SHARE caps how many ways one dish can be declared shared; it mirrors the
// server's clamp so the stepper never offers an amount the API would reject.
const MAX_SHARE = 20;

function tokenKey(billId: string): string {
  return `iou:participant:${billId}`;
}
function idKey(billId: string): string {
  return `iou:participant-id:${billId}`;
}

export default function FriendSplit() {
  const { token } = useParams<{ token: string }>();
  const { user } = useAuth();
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
  // Once paid, the friend lands on a terminal "settled up" screen. showSplit
  // lets them step back to their own split view (via the IOU wordmark) so the
  // page isn't a dead end — feedback was that there was no way out of it.
  const [showSplit, setShowSplit] = useState(false);
  // pendingClaims holds the user's most recent tap intent during an in-flight
  // save, so the checkboxes and total bar reflect the tap immediately instead
  // of waiting for the API round-trip. Cleared when the latest save's
  // response arrives. saveReqRef sequences concurrent saves so an older
  // response can't clobber a newer one if they return out of order.
  const [pendingClaims, setPendingClaims] = useState<Map<
    string,
    number
  > | null>(null);
  const saveReqRef = useRef(0);

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

  // Once the bill and auth state are known, a signed-in friend restores their
  // identity from their account — the participant linked to them when they
  // joined (claim) or picked an identity (host-split) while logged in. This is
  // what makes a tab opened from Home work on any device, not just the one
  // they joined on. localStorage restore (above) covers the same device for
  // signed-out friends; this covers the account across devices. A 404 means
  // they have no linked participant — fall through to the join/picker UI.
  useEffect(() => {
    if (!bill || !user || participantToken) return;
    let cancelled = false;
    api
      .myParticipant(bill.id)
      .then((mine) => {
        if (cancelled) return;
        setParticipantToken(mine.participant_token);
        setParticipantId(mine.participant.id);
        // Persist for the claim flow so a later offline visit restores too;
        // a host-split pick stays unpersisted (it is a shared roster).
        if (bill.split_mode !== "host") {
          localStorage.setItem(tokenKey(bill.id), mine.participant_token);
          localStorage.setItem(idKey(bill.id), mine.participant.id);
        }
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [bill, user, participantToken]);

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
    // If the friend is signed in, link this identity to their account so the
    // tab appears on their Home. Best-effort: a failure never blocks paying,
    // and a participant already linked to someone else is left untouched.
    if (user && bill && token) {
      api.linkIdentity(bill.id, pickedId, token).catch(() => {});
    }
  }

  // clearIdentity drops the current host-split pick and returns to the picker.
  function clearIdentity() {
    setParticipantToken(null);
    setParticipantId(null);
    setError(null);
  }

  // myClaims reads this friend's current claims as a map of
  // item_id -> share_count (the headcount they declared for sharing it).
  // Prefers pendingClaims (the tap the user just made) so the UI doesn't
  // wait for the API round-trip — the live summary is the fallback once the
  // latest save's response has landed.
  function myClaims(): Map<string, number> {
    if (pendingClaims) return new Map(pendingClaims);
    const m = new Map<string, number>();
    if (!summary || !participantId) return m;
    for (const [itemId, entries] of Object.entries(summary.claims)) {
      const mine = entries.find((e) => e.participant_id === participantId);
      if (mine) m.set(itemId, mine.share_count);
    }
    return m;
  }

  // saveClaims posts the friend's whole claim set and stores the fresh
  // summary. Holds the tap's claims in pendingClaims while the API is in
  // flight so the UI shows the new state immediately. Only the response of
  // the most recent save is applied — older responses arriving out of order
  // are dropped so they can't overwrite a newer summary.
  async function saveClaims(claims: Map<string, number>) {
    if (!bill || !participantToken) return;
    const myReq = ++saveReqRef.current;
    setPendingClaims(claims);
    try {
      const next = await api.setClaims(
        bill.id,
        participantToken,
        [...claims].map(([item_id, share_count]) => ({
          item_id,
          share_count,
        })),
      );
      if (myReq === saveReqRef.current) {
        setSummary(next);
        setPendingClaims(null);
      }
    } catch (err) {
      if (myReq === saveReqRef.current) {
        setError(err instanceof Error ? err.message : "could not update");
        setPendingClaims(null);
      }
    }
  }

  // toggleItem claims an item (as a whole item, share_count 1) or drops it.
  async function toggleItem(itemId: string) {
    const claims = myClaims();
    if (claims.has(itemId)) claims.delete(itemId);
    else claims.set(itemId, 1);
    await saveClaims(claims);
  }

  // setShareCount changes how many ways a claimed dish is split.
  async function setShareCount(itemId: string, count: number) {
    const claims = myClaims();
    if (!claims.has(itemId)) return;
    claims.set(itemId, Math.min(MAX_SHARE, Math.max(1, count)));
    await saveClaims(claims);
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
    const shareOf = (pid: string) =>
      summary.split.participants.find((s) => s.participant_id === pid);
    // "total split" recap — the sum the listed friends owe the host.
    const listedTotal = pickable.reduce(
      (sum, p) => sum + (shareOf(p.id)?.total_cents ?? 0),
      0,
    );
    return (
      <PaperApp>
        <div className="page-center">
          <p className="eyebrow" style={{ textAlign: "center" }}>
            {bill.restaurant || "The tab"}
          </p>
          <h2
            className="h-section mt-3"
            style={{ textAlign: "center", fontSize: 38, lineHeight: 1.0 }}
          >
            Which one
            <br />
            are you?
          </h2>
          <p
            className="body muted mt-3"
            style={{
              textAlign: "center",
              fontSize: 13,
              maxWidth: 240,
              margin: "12px auto 0",
            }}
          >
            The host already split this. Tap your name to pay.
          </p>

          {error && (
            <p className="body danger center mt-3" style={{ fontSize: 13 }}>
              {error}
            </p>
          )}

          {pickable.length === 0 ? (
            <p className="body muted center mt-6" style={{ fontSize: 13 }}>
              No one to pick yet — check back once the host finishes.
            </p>
          ) : (
            <>
              {/* Single soft paper card with dashed perforations between
                  names — a receipt's tab list. */}
              <div
                className="mt-6"
                style={{
                  background: "var(--paper)",
                  borderRadius: 14,
                  border: "1px solid rgba(31,61,43,.10)",
                  overflow: "hidden",
                  boxShadow: "0 8px 24px -14px rgba(31,61,43,.16)",
                }}
              >
                {pickable.map((p, i) => {
                  const share = shareOf(p.id);
                  const paid = p.payment_status === "paid";
                  const disabled = paid || !p.participant_token;
                  const isLast = i === pickable.length - 1;
                  return (
                    <button
                      key={p.id}
                      disabled={disabled}
                      onClick={() =>
                        p.participant_token &&
                        pickIdentity(p.id, p.participant_token)
                      }
                      style={{
                        appearance: "none",
                        border: 0,
                        background: "transparent",
                        width: "100%",
                        padding: "14px 16px",
                        borderBottom: isLast
                          ? "0"
                          : "1px dashed rgba(31,61,43,.18)",
                        display: "grid",
                        gridTemplateColumns: "auto 1fr auto auto",
                        gap: 12,
                        alignItems: "center",
                        textAlign: "left",
                        cursor: disabled ? "default" : "pointer",
                        opacity: paid ? 0.5 : 1,
                        transition: "background .15s ease",
                        fontFamily: "inherit",
                        color: "inherit",
                      }}
                      onMouseEnter={(e) => {
                        if (!disabled)
                          e.currentTarget.style.background =
                            "rgba(31,61,43,.04)";
                      }}
                      onMouseLeave={(e) => {
                        e.currentTarget.style.background = "transparent";
                      }}
                    >
                      <Avatar name={p.display_name} seed={p.id} size="md" />
                      <div style={{ minWidth: 0 }}>
                        <p
                          style={{
                            margin: 0,
                            fontSize: 15,
                            fontWeight: 500,
                            color: "var(--ink)",
                            textTransform: "lowercase",
                            textDecoration: paid ? "line-through" : "none",
                            textDecorationColor: "rgba(31,61,43,.4)",
                            textDecorationThickness: "1px",
                          }}
                        >
                          {p.display_name}
                        </p>
                        <p
                          className="mono"
                          style={{
                            margin: "3px 0 0",
                            fontSize: 10.5,
                            color: paid ? "var(--muted)" : "var(--accent-deep)",
                            letterSpacing: "0.04em",
                            whiteSpace: "nowrap",
                          }}
                        >
                          {paid ? "paid ✓" : "tap to pay"}
                        </p>
                      </div>
                      <span
                        className="mono"
                        style={{
                          fontSize: 14,
                          color: "var(--ink)",
                          fontWeight: paid ? 400 : 600,
                          whiteSpace: "nowrap",
                        }}
                      >
                        {fmtH(share?.total_cents ?? 0)}
                      </span>
                      <span style={{ display: "flex", opacity: 0.6 }}>
                        <Icon.Arrow size={14} />
                      </span>
                    </button>
                  );
                })}
              </div>

              {/* Footer: total recap pinned to the bottom of the frame. */}
              <p
                className="mono"
                style={{
                  textAlign: "center",
                  fontSize: 11,
                  color: "var(--muted)",
                  letterSpacing: "0.04em",
                  marginTop: "auto",
                  paddingTop: 18,
                }}
              >
                total split ·{" "}
                <span style={{ color: "var(--ink)" }}>{fmtH(listedTotal)}</span>
              </p>
            </>
          )}
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
  const fmt = (c: number) => formatMoney(c, bill.currency);

  // While a claim save is in flight, derive the visible totals from
  // pendingClaims so the bar reflects the tap immediately. The math mirrors
  // the server's proration (tax/tip and a percent service charge scale with
  // the item subtotal), so the value differs from the eventual server value
  // by at most a cent — the largest-remainder pennies snap into place when
  // the response lands. A fixed service charge stays on the live summary's
  // value since it splits by headcount, not by claims.
  const optimistic = pendingClaims
    ? optimisticShare(
        bill,
        summary,
        myId,
        pendingClaims,
        myShare?.service_cents ?? 0,
      )
    : null;
  const owes = optimistic?.total ?? myShare?.total_cents ?? 0;
  // The "you owe" total folds prorated tax, tip and service on top of the
  // claimed items — spell each part out with its own amount so the number
  // isn't a mystery.
  const myItemCents = optimistic?.items ?? myShare?.item_subtotal_cents ?? 0;
  const myTaxCents = optimistic?.tax ?? myShare?.tax_cents ?? 0;
  const myTipCents = optimistic?.tip ?? myShare?.tip_cents ?? 0;
  const myServiceCents = optimistic?.service ?? myShare?.service_cents ?? 0;
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
  if (isPaid && !showSplit) {
    return (
      <PaperApp>
        <div
          className="page-center"
          style={{ paddingTop: 22, paddingBottom: 28 }}
        >
          <div className="row row-between">
            <Brand size={26} onClick={() => setShowSplit(true)} />
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
          (summary.claims[it.id] ?? []).some((e) => e.participant_id === myId),
        )
      : [];
    return (
      <PaperApp>
        <div className="page" style={{ paddingBottom: 8 }}>
          <div className="row row-between">
            <Brand
              size={26}
              onClick={isPaid ? () => setShowSplit(false) : undefined}
            />
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
    ? Object.values(summary.claims).some((entries) =>
        entries.some((e) => e.participant_id === myId),
      )
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
          Tap what you ordered. Shared a dish? Set how many ways with ＋.
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
            // While a save is in flight, MY claim on this item comes from
            // pendingClaims (the tap I just made); others' claims stay as
            // the live summary shows them. This keeps the checkbox in sync
            // with the user's intent without waiting for the API.
            const liveClaimers = summary.claims[it.id] ?? [];
            const myPending = pendingClaims?.get(it.id);
            const others = liveClaimers
              .filter((c) => c.participant_id !== myId)
              .map((c) => ({
                id: c.participant_id,
                name: nameOf(c.participant_id),
              }));
            let mine: boolean;
            let myCount: number;
            if (pendingClaims) {
              mine = myPending !== undefined;
              myCount = myPending ?? 1;
            } else {
              const mineEntry = myId
                ? liveClaimers.find((c) => c.participant_id === myId)
                : undefined;
              mine = !!mineEntry;
              myCount = mineEntry?.share_count ?? 1;
            }
            const claimerCount = others.length + (mine ? 1 : 0);
            // My effective denominator is never below the number of claimers,
            // so this matches the share the server computes.
            const denom = Math.max(myCount, claimerCount);
            const youPay = Math.round(it.price_cents / denom);
            return (
              <div key={it.id} className={`claim-row${mine ? " mine" : ""}`}>
                <button
                  className="claim-item"
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
                        <span className="mono muted" style={{ fontSize: 10 }}>
                          {others.length === 1
                            ? `with ${others[0].name}`
                            : `with ${others.length} others`}
                        </span>
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
                    {mine && denom > 1 && (
                      <p
                        className="mono"
                        style={{
                          margin: "2px 0 0",
                          fontSize: 10,
                          color: "var(--accent-deep)",
                        }}
                      >
                        you: {fmt(youPay)}
                      </p>
                    )}
                  </div>
                </button>
                {mine && (
                  <div className="share-stepper">
                    <span className="share-stepper-label">
                      {denom > 1
                        ? `Split ${denom} ways`
                        : "Just you — tap ＋ to share"}
                    </span>
                    <div
                      className="stepper"
                      role="group"
                      aria-label="how many ways shared"
                    >
                      <button
                        type="button"
                        className="step-btn"
                        disabled={myCount <= 1}
                        aria-label="fewer people"
                        onClick={() => setShareCount(it.id, myCount - 1)}
                      >
                        −
                      </button>
                      <span className="step-val mono">{myCount}</span>
                      <button
                        type="button"
                        className="step-btn"
                        disabled={myCount >= MAX_SHARE}
                        aria-label="more people"
                        onClick={() => setShareCount(it.id, myCount + 1)}
                      >
                        +
                      </button>
                    </div>
                  </div>
                )}
              </div>
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

// optimisticShare projects what the friend's share will look like after the
// pending claims hit the server. Mirrors internal/split.Compute's proration
// (tax/tip and a percent service charge scale with the friend's item
// subtotal) so the value differs from the eventual server value by at most a
// largest-remainder cent. A fixed service charge stays on liveServiceCents —
// it splits by headcount, not by claims.
function optimisticShare(
  bill: Bill,
  summary: BillSummary,
  myId: string | null,
  claims: Map<string, number>,
  liveServiceCents: number,
): { total: number; items: number; tax: number; tip: number; service: number } {
  let myItems = 0;
  let totalSubtotal = 0;
  for (const it of bill.items) {
    totalSubtotal += it.price_cents;
    const myShareCount = claims.get(it.id);
    if (myShareCount === undefined) continue;
    const others = (summary.claims[it.id] ?? []).filter(
      (e) => e.participant_id !== myId,
    ).length;
    const denom = Math.max(myShareCount, others + 1);
    myItems += Math.round(it.price_cents / denom);
  }
  const ratio = totalSubtotal > 0 ? myItems / totalSubtotal : 0;
  const tax = Math.round(ratio * bill.tax_cents);
  const tip = Math.round(ratio * bill.tip_cents);
  let service = liveServiceCents;
  if (
    bill.service_charge_kind === "percent" &&
    bill.service_charge_rate_bps > 0
  ) {
    const serviceTotal = Math.round(
      (bill.service_charge_rate_bps * totalSubtotal) / 10000,
    );
    service = Math.round(ratio * serviceTotal);
  }
  return {
    total: myItems + tax + tip + service,
    items: myItems,
    tax,
    tip,
    service,
  };
}
