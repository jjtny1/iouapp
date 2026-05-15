import { useEffect, useState, type ChangeEvent } from "react";
import { Link, useParams } from "react-router-dom";
import { api, type Bill, type BillSummary } from "../api";
import { useAuth } from "../auth";
import { prepareReceiptImage } from "../image";
import { formatMoney } from "../money";

interface DraftItem {
  name: string;
  priceDollars: string;
}

function centsToDollars(cents: number): string {
  return (cents / 100).toFixed(2);
}

function dollarsToCents(value: string): number {
  const n = Math.round(parseFloat(value) * 100);
  return Number.isFinite(n) ? n : 0;
}

export default function BillEditor() {
  const { id } = useParams<{ id: string }>();
  const { user } = useAuth();
  const [bill, setBill] = useState<Bill | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [parsing, setParsing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [copied, setCopied] = useState(false);

  const [restaurant, setRestaurant] = useState("");
  const [currency, setCurrency] = useState("USD");
  const [items, setItems] = useState<DraftItem[]>([]);
  const [taxDollars, setTaxDollars] = useState("0.00");
  const [tipDollars, setTipDollars] = useState("0.00");
  const [summary, setSummary] = useState<BillSummary | null>(null);

  function loadFromBill(b: Bill) {
    setBill(b);
    setRestaurant(b.restaurant);
    setCurrency(b.currency);
    setTaxDollars(centsToDollars(b.tax_cents));
    setTipDollars(centsToDollars(b.tip_cents));
    setItems(
      b.items.map((it) => ({
        name: it.name,
        priceDollars: centsToDollars(it.price_cents),
      })),
    );
  }

  useEffect(() => {
    if (!id) return;
    api
      .getBill(id)
      .then(loadFromBill)
      .catch((err) =>
        setError(err instanceof Error ? err.message : "could not load bill"),
      )
      .finally(() => setLoading(false));
  }, [id]);

  useEffect(() => {
    if (!id) return;
    api
      .summary(id)
      .then(setSummary)
      .catch(() => setSummary(null));
  }, [id]);

  async function onUpload(e: ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file || !id) return;
    setError(null);
    setParsing(true);
    try {
      const prepared = await prepareReceiptImage(file);
      const updated = await api.uploadReceipt(id, prepared);
      loadFromBill(updated);
    } catch (err) {
      console.error("[receipt upload]", err);
      setError(err instanceof Error ? err.message : "upload failed");
    } finally {
      setParsing(false);
    }
  }

  async function onSave() {
    if (!id) return;
    setError(null);
    setSaving(true);
    try {
      const updated = await api.updateBill(id, {
        restaurant,
        currency,
        tax_cents: dollarsToCents(taxDollars),
        tip_cents: dollarsToCents(tipDollars),
        items: items.map((it) => ({
          name: it.name,
          price_cents: dollarsToCents(it.priceDollars),
        })),
      });
      loadFromBill(updated);
    } catch (err) {
      setError(err instanceof Error ? err.message : "save failed");
    } finally {
      setSaving(false);
    }
  }

  function updateItem(index: number, patch: Partial<DraftItem>) {
    setItems((prev) =>
      prev.map((it, i) => (i === index ? { ...it, ...patch } : it)),
    );
  }

  function addItem() {
    setItems((prev) => [...prev, { name: "", priceDollars: "0.00" }]);
  }

  function removeItem(index: number) {
    setItems((prev) => prev.filter((_, i) => i !== index));
  }

  async function copyShareLink() {
    if (!bill?.share_url) return;
    try {
      await navigator.clipboard.writeText(bill.share_url);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopied(false);
    }
  }

  if (loading) {
    return (
      <main className="app">
        <h1>IOU</h1>
        <p>Loading…</p>
      </main>
    );
  }

  if (!bill) {
    return (
      <main className="app">
        <h1>IOU</h1>
        <p className="error">{error ?? "Bill not found."}</p>
        <Link to="/">Back</Link>
      </main>
    );
  }

  const subtotalCents = items.reduce(
    (sum, it) => sum + dollarsToCents(it.priceDollars),
    0,
  );
  const totalCents =
    subtotalCents + dollarsToCents(taxDollars) + dollarsToCents(tipDollars);

  return (
    <main className="app">
      <h1>IOU</h1>
      <Link to="/">← Back</Link>

      {bill.share_url && (
        <p className="status">
          Share link:{" "}
          <a href={bill.share_url} target="_blank" rel="noreferrer">
            {bill.share_url}
          </a>{" "}
          <button className="link-button" onClick={copyShareLink}>
            {copied ? "Copied!" : "Copy"}
          </button>
        </p>
      )}

      {error && <p className="error">{error}</p>}

      {items.length === 0 ? (
        <section>
          <h2>Upload a receipt</h2>
          {parsing ? (
            <p className="status">Parsing receipt…</p>
          ) : (
            <input
              type="file"
              accept="image/*,.heic,.heif"
              onChange={onUpload}
            />
          )}
        </section>
      ) : (
        <section className="bill-editor">
          <label htmlFor="restaurant">Restaurant</label>
          <input
            id="restaurant"
            type="text"
            value={restaurant}
            onChange={(e) => setRestaurant(e.target.value)}
          />

          <label htmlFor="currency">Currency (ISO 4217 code)</label>
          <input
            id="currency"
            type="text"
            maxLength={3}
            placeholder="USD"
            value={currency}
            onChange={(e) =>
              setCurrency(e.target.value.toUpperCase().replace(/[^A-Z]/g, ""))
            }
          />

          <h2>Items</h2>
          {items.map((it, i) => (
            <div className="item-row" key={i}>
              <input
                type="text"
                placeholder="Item name"
                value={it.name}
                onChange={(e) => updateItem(i, { name: e.target.value })}
              />
              <input
                type="number"
                step="0.01"
                min="0"
                placeholder="0.00"
                value={it.priceDollars}
                onChange={(e) =>
                  updateItem(i, { priceDollars: e.target.value })
                }
              />
              <button className="link-button" onClick={() => removeItem(i)}>
                Remove
              </button>
            </div>
          ))}
          <button onClick={addItem}>Add item</button>

          <label htmlFor="tax">Tax ({currency})</label>
          <input
            id="tax"
            type="number"
            step="0.01"
            min="0"
            value={taxDollars}
            onChange={(e) => setTaxDollars(e.target.value)}
          />

          <label htmlFor="tip">Tip ({currency})</label>
          <input
            id="tip"
            type="number"
            step="0.01"
            min="0"
            value={tipDollars}
            onChange={(e) => setTipDollars(e.target.value)}
          />

          <p className="status">
            Subtotal: {formatMoney(subtotalCents, currency)}
            <br />
            Total: {formatMoney(totalCents, currency)}
          </p>

          <button onClick={onSave} disabled={saving}>
            {saving ? "Saving…" : "Save"}
          </button>
        </section>
      )}

      {summary && summary.participants.length > 0 && (
        <section className="my-share">
          <h2>Who owes what</h2>
          {!user?.wallet_address && (
            <p className="error">
              Friends can't pay until you set a payout wallet address.{" "}
              <Link to="/">Set it on your home page.</Link>
            </p>
          )}
          <ul className="bill-list">
            {summary.participants.map((p) => {
              const share = summary.split.participants.find(
                (s) => s.participant_id === p.id,
              );
              return (
                <li key={p.id}>
                  {p.display_name} —{" "}
                  {formatMoney(share?.total_cents ?? 0, currency)}
                  {p.payment_status === "paid" ? (
                    <span className="status">
                      {" · Paid ✓"}
                      {p.tx_ref ? ` (${p.tx_ref})` : ""}
                    </span>
                  ) : p.payment_status === "pending" ? (
                    <span className="status">{" · Pending"}</span>
                  ) : null}
                </li>
              );
            })}
          </ul>
          {summary.split.unclaimed_cents > 0 && (
            <p className="status">
              Unclaimed: {formatMoney(summary.split.unclaimed_cents, currency)}
            </p>
          )}
          <p className="status">
            Grand total:{" "}
            {formatMoney(summary.split.grand_total_cents, currency)}
          </p>
        </section>
      )}
    </main>
  );
}
