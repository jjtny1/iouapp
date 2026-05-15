import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useParams } from "react-router-dom";
import {
  api,
  type Bill,
  type BillSummary,
  type PaymentChallenge,
} from "../api";
import { formatMoney } from "../money";

function tokenKey(billId: string): string {
  return `splitit:participant:${billId}`;
}

function idKey(billId: string): string {
  return `splitit:participant-id:${billId}`;
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
  const [challenge, setChallenge] = useState<PaymentChallenge | null>(null);
  const [paying, setPaying] = useState(false);
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
      const s = await api.summary(bill.id, token);
      setSummary(s);
    } catch (err) {
      setError(err instanceof Error ? err.message : "could not load summary");
    }
  }, [bill, token]);

  useEffect(() => {
    if (bill && participantToken) refresh();
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
    if (!bill || !participantToken || !summary) return;
    const myId = resolveParticipantId();
    if (!myId) return;
    const current = new Set(
      Object.entries(summary.claims)
        .filter(([, ids]) => ids.includes(myId))
        .map(([id]) => id),
    );
    if (current.has(itemId)) current.delete(itemId);
    else current.add(itemId);
    try {
      const s = await api.setClaims(bill.id, participantToken, [...current]);
      setSummary(s);
    } catch (err) {
      setError(err instanceof Error ? err.message : "could not update");
    }
  }

  function resolveParticipantId(): string | null {
    if (participantId) return participantId;
    return null;
  }

  async function startPayment() {
    if (!bill || !participantToken) return;
    setError(null);
    setPaying(true);
    try {
      const res = await api.pay(bill.id, participantToken);
      if (res.kind === "paid") {
        setTxRef(res.payment.tx_ref);
        await refresh();
      } else {
        setChallenge(res.challenge);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "could not start payment");
    } finally {
      setPaying(false);
    }
  }

  async function confirmPayment() {
    if (!bill || !participantToken || !challenge) return;
    setError(null);
    setConfirming(true);
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
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "could not confirm payment",
      );
    } finally {
      setConfirming(false);
    }
  }

  if (loading) {
    return (
      <main className="app">
        <h1>splitit</h1>
        <p>Loading…</p>
      </main>
    );
  }

  if (!bill) {
    return (
      <main className="app">
        <h1>splitit</h1>
        <p className="error">{error ?? "Bill not found."}</p>
      </main>
    );
  }

  if (!participantToken) {
    return (
      <main className="app">
        <h1>splitit</h1>
        <h2>{bill.restaurant || "Split the bill"}</h2>
        <p className="status">Enter your name to claim your items.</p>
        {error && <p className="error">{error}</p>}
        <form onSubmit={onJoin}>
          <label htmlFor="name">Your name</label>
          <input
            id="name"
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. Alex"
          />
          <button type="submit" disabled={joining}>
            {joining ? "Joining…" : "Join"}
          </button>
        </form>
      </main>
    );
  }

  if (!summary) {
    return (
      <main className="app">
        <h1>splitit</h1>
        <p>Loading items…</p>
        {error && <p className="error">{error}</p>}
      </main>
    );
  }

  const myId = resolveParticipantId();
  const mine = myId
    ? summary.split.participants.find((p) => p.participant_id === myId)
    : undefined;
  const myParticipant = myId
    ? summary.participants.find((p) => p.id === myId)
    : undefined;
  const isPaid = myParticipant?.payment_status === "paid";
  const paidTxRef = txRef ?? myParticipant?.tx_ref ?? null;
  const owesAmount = mine?.total_cents ?? 0;
  const fmt = (cents: number) => formatMoney(cents, bill.currency);

  return (
    <main className="app">
      <h1>splitit</h1>
      <h2>{bill.restaurant || "Split the bill"}</h2>
      <p className="status">
        Tap the items you ordered. Shared items split evenly.
      </p>
      {error && <p className="error">{error}</p>}

      <ul className="bill-list">
        {summary.items.map((it) => {
          const claimers = summary.claims[it.id] ?? [];
          const mineClaimed = myId ? claimers.includes(myId) : false;
          const total = it.price_cents * it.qty;
          return (
            <li key={it.id}>
              <label className="claim-row">
                <input
                  type="checkbox"
                  checked={mineClaimed}
                  onChange={() => toggleItem(it.id)}
                />
                <span className="claim-name">
                  {it.name || "Item"}
                  {it.qty > 1 ? ` ×${it.qty}` : ""}
                </span>
                <span className="claim-price">
                  {fmt(total)}
                  {claimers.length > 0 && (
                    <em>
                      {" "}
                      ({claimers.length} sharing ·{" "}
                      {fmt(Math.floor(total / claimers.length))} ea)
                    </em>
                  )}
                </span>
              </label>
            </li>
          );
        })}
      </ul>

      <section className="my-share">
        <h2>Your share</h2>
        {mine ? (
          <p className="status">
            Items: {fmt(mine.item_subtotal_cents)}
            <br />
            Tax: {fmt(mine.tax_cents)}
            <br />
            Tip: {fmt(mine.tip_cents)}
            <br />
            <strong>Total: {fmt(mine.total_cents)}</strong>
          </p>
        ) : (
          <p className="status">Claim items to see your total.</p>
        )}
      </section>

      <section className="pay-section">
        {isPaid ? (
          <p className="pay-paid">
            Paid ✓
            {paidTxRef && (
              <>
                <br />
                <span className="status">tx: {paidTxRef}</span>
              </>
            )}
          </p>
        ) : challenge ? (
          <div className="pay-panel">
            <p className="pay-mock-note">
              Simulated payment — no real funds will move.
            </p>
            <p className="status">
              Amount: {formatMoney(challenge.amount_cents, challenge.currency)}
              <br />
              Network: {challenge.network}
              <br />
              Recipient: <code>{challenge.recipient}</code>
            </p>
            <button onClick={confirmPayment} disabled={confirming}>
              {confirming ? "Confirming…" : "Confirm payment"}
            </button>
          </div>
        ) : owesAmount > 0 ? (
          <button onClick={startPayment} disabled={paying}>
            {paying ? "Starting…" : `Pay ${fmt(owesAmount)}`}
          </button>
        ) : (
          <button disabled title="Claim items first">
            Pay
          </button>
        )}
      </section>
    </main>
  );
}
