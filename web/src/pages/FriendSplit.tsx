import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useParams } from "react-router-dom";
import {
  api,
  type Bill,
  type BillSummary,
  type PaymentChallenge,
} from "../api";
import { formatMoney } from "../money";
import { Avatar, AvatarStack, Brand, Icon, PaperApp } from "../ui";

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
  const [challenge, setChallenge] = useState<PaymentChallenge | null>(null);
  const [loadingChallenge, setLoadingChallenge] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [txRef, setTxRef] = useState<string | null>(null);

  useEffect(() => {
    if (!token) return;
    api
      .billByToken(token)
      .then((b) => {
        setBill(b);
        const stored = localStorage.getItem(tokenKey(b.id));
        if (stored) setParticipantToken(stored);
        const storedId = localStorage.getItem(idKey(b.id));
        if (storedId) setParticipantId(storedId);
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

  useEffect(() => {
    // refresh() is async — setState runs after the await, not synchronously.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    if (bill && participantToken) void refresh();
  }, [bill, participantToken, refresh]);

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
    setLoadingChallenge(true);
    try {
      const res = await api.pay(bill.id, participantToken);
      if (res.kind === "paid") {
        setTxRef(res.payment.tx_ref);
        await refresh();
        setPayOpen(false);
      } else {
        setChallenge(res.challenge);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "could not start payment");
      setPayOpen(false);
    } finally {
      setLoadingChallenge(false);
    }
  }

  async function confirmPay() {
    if (!bill || !participantToken || !challenge) return;
    setConfirming(true);
    setError(null);
    try {
      const payment = await api.confirmPayment(
        bill.id,
        participantToken,
        challenge.payment_id,
        "mock-proof",
      );
      setTxRef(payment.tx_ref);
      setChallenge(null);
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
  const paidTxRef = txRef ?? myParticipant?.tx_ref ?? null;
  const owes = myShare?.total_cents ?? 0;
  const fmt = (c: number) => formatMoney(c, bill.currency);
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
              {paidTxRef && (
                <>
                  <hr className="dash" style={{ margin: "10px 0" }} />
                  <div className="row row-between">
                    <span
                      className="mono muted"
                      style={{ fontSize: 10, letterSpacing: "0.06em" }}
                    >
                      TX REF
                    </span>
                    <span
                      className="mono muted truncate"
                      style={{ fontSize: 10, maxWidth: 160 }}
                    >
                      {paidTxRef}
                    </span>
                  </div>
                </>
              )}
            </div>
          </div>

          <p className="body muted center" style={{ fontSize: 12 }}>
            That's it — you can close this page.
          </p>
        </div>
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

      {/* Pay sheet */}
      {payOpen && (
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
                <span
                  className="mono"
                  style={{ fontSize: 19, fontWeight: 600 }}
                >
                  {fmt(owes)}
                </span>
              </div>
            </div>

            <div
              className="mt-4"
              style={{
                background: "rgba(232,193,74,.20)",
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
                  background: "var(--accent)",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  color: "var(--ink)",
                }}
              >
                <Icon.Wallet size={16} />
              </div>
              <div style={{ flex: 1, minWidth: 0 }}>
                <p style={{ margin: 0, fontSize: 13, fontWeight: 500 }}>
                  {challenge
                    ? `${challenge.currency} · ${challenge.network}`
                    : "Preparing payment…"}
                </p>
                {challenge && (
                  <p
                    className="mono muted truncate"
                    style={{ margin: "2px 0 0", fontSize: 10.5 }}
                  >
                    {challenge.recipient}
                  </p>
                )}
              </div>
            </div>

            {error && (
              <p className="body danger mt-3" style={{ fontSize: 12 }}>
                {error}
              </p>
            )}

            <button
              className="btn btn-block mt-4"
              style={{ padding: "14px 18px" }}
              disabled={loadingChallenge || confirming || !challenge}
              onClick={confirmPay}
            >
              {confirming ? (
                <>
                  <span className="spinner" /> Confirming…
                </>
              ) : loadingChallenge ? (
                <>
                  <span className="spinner" /> Preparing…
                </>
              ) : (
                <>
                  Pay {fmt(owes)} <Icon.Arrow size={12} />
                </>
              )}
            </button>
            <p
              className="body muted center"
              style={{ fontSize: 11, marginTop: 10 }}
            >
              Simulated payment — no real funds will move.
            </p>
          </div>
        </>
      )}
    </PaperApp>
  );
}
