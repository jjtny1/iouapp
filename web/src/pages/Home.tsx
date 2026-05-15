import { useEffect, useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { api, type Bill } from "../api";
import { useAuth } from "../auth";
import { formatMoney } from "../money";
import { Avatar, Brand, Icon, PaperApp } from "../ui";

// billTotal is the bill's full amount for the list summary: items, tax,
// tip and the service charge (percent prorates the item subtotal, fixed is
// a flat amount, none adds nothing).
function billTotal(b: Bill): number {
  const subtotal = b.items.reduce((s, it) => s + it.price_cents, 0);
  const service =
    b.service_charge_kind === "percent"
      ? Math.round((b.service_charge_rate_bps * subtotal) / 10000)
      : b.service_charge_kind === "fixed"
        ? b.service_charge_cents
        : 0;
  return subtotal + b.tax_cents + b.tip_cents + service;
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

// TabRow renders one saved tab on Home — an "card" for open tabs, a compact
// "line" for settled ones. A dim trash handle reveals an inline confirm panel
// that slides over the row; deletion runs only after the host confirms.
function TabRow({
  bill,
  variant,
  onOpen,
  onDelete,
}: {
  bill: Bill;
  variant: "card" | "line";
  onOpen: () => void;
  onDelete: () => Promise<void>;
}) {
  const [confirming, setConfirming] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [failed, setFailed] = useState(false);

  async function confirmDelete() {
    setDeleting(true);
    setFailed(false);
    try {
      await onDelete();
      // On success the parent drops this row, unmounting the component.
    } catch {
      setFailed(true);
      setDeleting(false);
    }
  }

  return (
    <div className={`tab tab-${variant}`}>
      <button
        className="tab-face"
        onClick={onOpen}
        tabIndex={confirming ? -1 : 0}
      >
        {variant === "card" ? (
          <div
            className="row row-between gap-3"
            style={{ alignItems: "flex-start" }}
          >
            <div className="flex1">
              <p className="h-card truncate">
                {bill.restaurant || "Untitled tab"}
              </p>
              <p
                className="mono"
                style={{
                  margin: "4px 0 0",
                  fontSize: 11,
                  color: "var(--muted)",
                }}
              >
                {formatDate(bill.created_at)} · {bill.status}
              </p>
            </div>
            <span
              className="mono"
              style={{ fontSize: 14, fontWeight: 600, flexShrink: 0 }}
            >
              {formatMoney(billTotal(bill), bill.currency)}
            </span>
          </div>
        ) : (
          <div className="row row-between">
            <div>
              <p style={{ margin: 0, fontSize: 14 }}>
                {bill.restaurant || "Untitled tab"}
              </p>
              <p
                className="mono muted"
                style={{ margin: "2px 0 0", fontSize: 11 }}
              >
                {formatDate(bill.created_at)}
              </p>
            </div>
            <span className="mono muted" style={{ fontSize: 13 }}>
              {formatMoney(billTotal(bill), bill.currency)}
            </span>
          </div>
        )}
      </button>

      <button
        className="tab-del"
        onClick={() => setConfirming(true)}
        aria-label={`Delete ${bill.restaurant || "untitled tab"}`}
        tabIndex={confirming ? -1 : 0}
      >
        <Icon.Trash size={16} />
      </button>

      {confirming && (
        <div
          className="tab-confirm"
          role="alertdialog"
          aria-label="Confirm delete"
        >
          <span className={`tab-confirm-msg${failed ? " is-error" : ""}`}>
            {failed ? "Couldn't delete — retry?" : "Delete this tab?"}
          </span>
          <div className="tab-confirm-actions">
            <button
              className="tab-confirm-btn tab-confirm-keep"
              onClick={() => setConfirming(false)}
              disabled={deleting}
            >
              Keep
            </button>
            <button
              className="tab-confirm-btn tab-confirm-del"
              onClick={confirmDelete}
              disabled={deleting}
              autoFocus
            >
              {deleting ? "Deleting…" : "Delete"}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

export default function Home() {
  const { user, setUser } = useAuth();
  const navigate = useNavigate();
  const [handle, setHandle] = useState(user?.venmo_handle ?? "");
  const [status, setStatus] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [bills, setBills] = useState<Bill[]>([]);
  const [creating, setCreating] = useState(false);
  const [showVenmo, setShowVenmo] = useState(false);

  useEffect(() => {
    api
      .listBills()
      .then(setBills)
      .catch(() => setBills([]));
  }, []);

  if (!user) return null;

  async function saveVenmoHandle(e: FormEvent) {
    e.preventDefault();
    setStatus(null);
    setBusy(true);
    try {
      const updated = await api.updateVenmoHandle(handle);
      setUser(updated);
      setHandle(updated.venmo_handle ?? "");
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

  // deleteBill removes a tab server-side, then drops it from the list. Any
  // failure is rethrown so the row's confirm panel can surface a retry.
  async function deleteBill(id: string) {
    await api.deleteBill(id);
    setBills((prev) => prev.filter((b) => b.id !== id));
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
                <TabRow
                  key={b.id}
                  bill={b}
                  variant="card"
                  onOpen={() => navigate(`/bills/${b.id}`)}
                  onDelete={() => deleteBill(b.id)}
                />
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
                <TabRow
                  key={b.id}
                  bill={b}
                  variant="line"
                  onOpen={() => navigate(`/bills/${b.id}`)}
                  onDelete={() => deleteBill(b.id)}
                />
              ))}
            </div>
          </>
        )}

        {/* Venmo handle — friends settle their share straight to it */}
        <div className="mt-8">
          <button
            className="row row-between"
            onClick={() => setShowVenmo((v) => !v)}
            style={{
              background: "transparent",
              border: 0,
              padding: 0,
              width: "100%",
              cursor: "pointer",
            }}
          >
            <span className="eyebrow">Venmo handle</span>
            <span className="eyebrow muted">
              {user.venmo_handle ? "set ✓" : "not set"}
            </span>
          </button>
          {showVenmo && (
            <form onSubmit={saveVenmoHandle} className="col gap-2 mt-3 fade-up">
              <p className="body muted" style={{ fontSize: 12 }}>
                Where your friends send their share. New tabs reuse it.
              </p>
              <input
                className="input input-mono"
                type="text"
                placeholder="@your-venmo"
                value={handle}
                onChange={(e) => setHandle(e.target.value)}
              />
              <button
                type="submit"
                className="btn btn-ghost btn-sm"
                disabled={busy}
              >
                <Icon.Wallet size={14} /> {busy ? "Saving…" : "Save handle"}
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
