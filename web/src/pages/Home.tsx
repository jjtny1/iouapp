import { useEffect, useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { api, type Bill } from "../api";
import { useAuth } from "../auth";
import { formatMoney } from "../money";
import { Avatar, Brand, Icon, PaperApp } from "../ui";

function billTotal(b: Bill): number {
  return (
    b.items.reduce((s, it) => s + it.price_cents, 0) + b.tax_cents + b.tip_cents
  );
}

function formatDate(createdAt: number): string {
  // created_at is a unix timestamp; tolerate seconds or milliseconds.
  const ms = createdAt < 1e12 ? createdAt * 1000 : createdAt;
  const d = new Date(ms);
  if (Number.isNaN(d.getTime())) return "";
  const today = new Date();
  if (d.toDateString() === today.toDateString()) return "Tonight";
  return d.toLocaleDateString(undefined, {
    weekday: "short",
    month: "short",
    day: "numeric",
  });
}

function isSettled(b: Bill): boolean {
  return b.status.toLowerCase() === "settled";
}

export default function Home() {
  const { user, setUser } = useAuth();
  const navigate = useNavigate();
  const [wallet, setWallet] = useState(user?.wallet_address ?? "");
  const [status, setStatus] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [bills, setBills] = useState<Bill[]>([]);
  const [creating, setCreating] = useState(false);
  const [showWallet, setShowWallet] = useState(false);

  useEffect(() => {
    api
      .listBills()
      .then(setBills)
      .catch(() => setBills([]));
  }, []);

  if (!user) return null;

  async function saveWallet(e: FormEvent) {
    e.preventDefault();
    setStatus(null);
    setBusy(true);
    try {
      const updated = await api.updateWallet(wallet);
      setUser(updated);
      setStatus("Saved.");
    } catch (err) {
      setStatus(err instanceof Error ? err.message : "save failed");
    } finally {
      setBusy(false);
    }
  }

  async function logout() {
    await api.logout().catch(() => undefined);
    setUser(null);
  }

  async function newBill() {
    setCreating(true);
    try {
      const bill = await api.createBill();
      navigate(`/bills/${bill.id}`);
    } catch (err) {
      setStatus(err instanceof Error ? err.message : "could not create bill");
      setCreating(false);
    }
  }

  const open = bills.filter((b) => !isSettled(b));
  const settled = bills.filter(isSettled);

  return (
    <PaperApp>
      <div className="page">
        <div className="row row-between">
          <Brand size={32} />
          <Avatar name={user.email} seed={user.id} size="md" />
        </div>

        <h2 className="h-section mt-6">Your tabs.</h2>
        <p className="body muted mt-1">Open a new one or split last night's.</p>

        <button
          className="btn btn-block mt-4"
          onClick={newBill}
          disabled={creating}
        >
          <Icon.Plus size={16} /> {creating ? "Creating…" : "New tab"}
        </button>

        {bills.length === 0 && (
          <p className="body muted center mt-6" style={{ fontSize: 13 }}>
            No tabs yet — snap a receipt to start one.
          </p>
        )}

        {open.length > 0 && (
          <>
            <div className="row row-between mt-6 mb-2">
              <span className="eyebrow">Open</span>
              <span className="eyebrow muted">{open.length}</span>
            </div>
            <div className="col gap-2">
              {open.map((b) => (
                <button
                  key={b.id}
                  className="tap-card"
                  onClick={() => navigate(`/bills/${b.id}`)}
                >
                  <div
                    className="row row-between gap-3"
                    style={{ alignItems: "flex-start" }}
                  >
                    <div className="flex1">
                      <p className="h-card truncate">
                        {b.restaurant || "Untitled tab"}
                      </p>
                      <p
                        className="mono"
                        style={{
                          margin: "4px 0 0",
                          fontSize: 11,
                          color: "var(--muted)",
                        }}
                      >
                        {formatDate(b.created_at)} · {b.status}
                      </p>
                    </div>
                    <span
                      className="mono"
                      style={{
                        fontSize: 14,
                        fontWeight: 600,
                        flexShrink: 0,
                      }}
                    >
                      {formatMoney(billTotal(b), b.currency)}
                    </span>
                  </div>
                </button>
              ))}
            </div>
          </>
        )}

        {settled.length > 0 && (
          <>
            <div className="row row-between mt-6 mb-2">
              <span className="eyebrow">Settled</span>
              <span className="eyebrow muted">{settled.length}</span>
            </div>
            <div>
              {settled.map((b) => (
                <button
                  key={b.id}
                  className="bill-line"
                  onClick={() => navigate(`/bills/${b.id}`)}
                  style={{
                    background: "transparent",
                    border: 0,
                    borderBottom: "1px dashed var(--line)",
                    width: "100%",
                    textAlign: "left",
                    cursor: "pointer",
                  }}
                >
                  <div className="row row-between">
                    <div>
                      <p style={{ margin: 0, fontSize: 14 }}>
                        {b.restaurant || "Untitled tab"}
                      </p>
                      <p
                        className="mono muted"
                        style={{ margin: "2px 0 0", fontSize: 11 }}
                      >
                        {formatDate(b.created_at)}
                      </p>
                    </div>
                    <span className="mono muted" style={{ fontSize: 13 }}>
                      {formatMoney(billTotal(b), b.currency)}
                    </span>
                  </div>
                </button>
              ))}
            </div>
          </>
        )}

        {/* Payout wallet — friends settle to this address */}
        <div className="mt-8">
          <button
            className="row row-between"
            onClick={() => setShowWallet((v) => !v)}
            style={{
              background: "transparent",
              border: 0,
              padding: 0,
              width: "100%",
              cursor: "pointer",
            }}
          >
            <span className="eyebrow">Payout wallet</span>
            <span className="eyebrow muted">
              {user.wallet_address ? "set ✓" : "not set"}
            </span>
          </button>
          {showWallet && (
            <form onSubmit={saveWallet} className="col gap-2 mt-3 fade-up">
              <p className="body muted" style={{ fontSize: 12 }}>
                Where your friends' payments land.
              </p>
              <input
                className="input input-mono"
                type="text"
                placeholder="0x…"
                value={wallet}
                onChange={(e) => setWallet(e.target.value)}
              />
              <button
                type="submit"
                className="btn btn-ghost btn-sm"
                disabled={busy}
              >
                <Icon.Wallet size={14} /> {busy ? "Saving…" : "Save wallet"}
              </button>
              {status && (
                <p className="body muted" style={{ fontSize: 12 }}>
                  {status}
                </p>
              )}
            </form>
          )}
        </div>

        <div className="center mt-8">
          <button className="link-btn" onClick={logout}>
            Sign out
          </button>
        </div>
      </div>
    </PaperApp>
  );
}
