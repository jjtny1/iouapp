import { useEffect, useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api, type Bill } from "../api";
import { useAuth } from "../auth";
import { formatMoney } from "../money";

export default function Home() {
  const { user, setUser } = useAuth();
  const navigate = useNavigate();
  const [wallet, setWallet] = useState(user?.wallet_address ?? "");
  const [status, setStatus] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [bills, setBills] = useState<Bill[]>([]);
  const [creating, setCreating] = useState(false);

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

  return (
    <main className="app">
      <h1>splitit</h1>
      <p>
        Signed in as <strong>{user.email}</strong>
      </p>

      <button onClick={newBill} disabled={creating}>
        {creating ? "Creating…" : "New bill"}
      </button>

      <h2>Your bills</h2>
      {bills.length === 0 ? (
        <p className="status">No bills yet.</p>
      ) : (
        <ul className="bill-list">
          {bills.map((b) => (
            <li key={b.id}>
              <Link to={`/bills/${b.id}`}>
                {b.restaurant || "Untitled bill"} — {b.status}
                {" · "}
                {formatMoney(
                  b.items.reduce((s, it) => s + it.price_cents, 0) +
                    b.tax_cents +
                    b.tip_cents,
                  b.currency,
                )}
              </Link>
            </li>
          ))}
        </ul>
      )}

      <form onSubmit={saveWallet}>
        <label htmlFor="wallet">Wallet address</label>
        <input
          id="wallet"
          type="text"
          placeholder="0x…"
          value={wallet}
          onChange={(e) => setWallet(e.target.value)}
        />
        <button type="submit" disabled={busy}>
          {busy ? "Saving…" : "Save wallet"}
        </button>
      </form>
      {status && <p className="status">{status}</p>}
      <button className="link-button" onClick={logout}>
        Log out
      </button>
    </main>
  );
}
