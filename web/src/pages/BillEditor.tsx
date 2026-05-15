import {
  useEffect,
  useRef,
  useState,
  type ChangeEvent,
  type DragEvent,
} from "react";
import { useNavigate, useParams } from "react-router-dom";
import {
  api,
  type Bill,
  type BillSummary,
  type ServiceChargeKind,
} from "../api";
import { useAuth } from "../auth";
import { prepareReceiptImage } from "../image";
import { formatMoney } from "../money";
import { Avatar, Brand, Icon, PaperApp, QrCode, ReceiptZig } from "../ui";

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

/* ── Parsing animation — shown while the receipt upload is in flight ── */
function ParsingView() {
  const [step, setStep] = useState(0);
  const messages = ["Reading receipt", "Finding items", "Pulling totals"];
  useEffect(() => {
    const t = setInterval(() => setStep((s) => (s + 1) % messages.length), 900);
    return () => clearInterval(t);
  }, [messages.length]);

  return (
    <div
      className="page"
      style={{ minHeight: 460, display: "flex", flexDirection: "column" }}
    >
      <div className="row row-between">
        <span className="muted" style={{ fontSize: 13, opacity: 0.5 }}>
          · · ·
        </span>
        <span className="eyebrow">Step 2 / 3</span>
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
        <div
          style={{
            width: "100%",
            maxWidth: 220,
            background: "var(--paper)",
            borderRadius: "4px 4px 0 0",
            padding: "20px 18px",
            position: "relative",
            overflow: "hidden",
            boxShadow: "0 8px 24px -10px rgba(31,61,43,.20)",
          }}
        >
          <div className="scan-line" />
          <p
            style={{
              margin: "0 0 10px",
              fontFamily: "var(--serif)",
              fontStyle: "italic",
              fontSize: 18,
              textAlign: "center",
              borderBottom: "1px dashed var(--line-dashed)",
              paddingBottom: 10,
            }}
          >
            Receipt
          </p>
          {[1, 2, 3, 4, 5].map((i) => (
            <div
              key={i}
              className="row row-between"
              style={{
                padding: "6px 0",
                borderBottom: "1px dashed var(--line-dashed)",
              }}
            >
              <div
                className="shimmer"
                style={{
                  height: 8,
                  width: 60 + i * 8,
                  animationDelay: `${i * 0.15}s`,
                }}
              />
              <div
                className="shimmer"
                style={{ height: 8, width: 30, animationDelay: `${i * 0.15}s` }}
              />
            </div>
          ))}
          <ReceiptZig />
        </div>
        <p
          className="mono"
          style={{ margin: "24px 0 0", fontSize: 12, letterSpacing: "0.04em" }}
        >
          {messages[step]}
          <span style={{ opacity: 0.5 }}>…</span>
        </p>
      </div>
    </div>
  );
}

export default function BillEditor() {
  const { id } = useParams<{ id: string }>();
  const { user, setUser } = useAuth();
  const navigate = useNavigate();
  const fileRef = useRef<HTMLInputElement>(null);

  const [bill, setBill] = useState<Bill | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [parsing, setParsing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [toast, setToast] = useState<string | null>(null);
  const [dragging, setDragging] = useState(false);
  const [manual, setManual] = useState(false);

  const [restaurant, setRestaurant] = useState("");
  const [currency, setCurrency] = useState("USD");
  const [items, setItems] = useState<DraftItem[]>([]);
  const [taxDollars, setTaxDollars] = useState("0.00");
  const [tipDollars, setTipDollars] = useState("0.00");
  const [scKind, setScKind] = useState<ServiceChargeKind>("none");
  const [scRatePercent, setScRatePercent] = useState("0");
  const [scFixedDollars, setScFixedDollars] = useState("0.00");
  const [scHeadcount, setScHeadcount] = useState("");
  const [summary, setSummary] = useState<BillSummary | null>(null);
  const [venmoHandle, setVenmoHandle] = useState(user?.venmo_handle ?? "");
  const [savingHandle, setSavingHandle] = useState(false);

  function loadFromBill(b: Bill) {
    setBill(b);
    setRestaurant(b.restaurant);
    setCurrency(b.currency);
    setTaxDollars(centsToDollars(b.tax_cents));
    setTipDollars(centsToDollars(b.tip_cents));
    setScKind(b.service_charge_kind);
    setScRatePercent(String(b.service_charge_rate_bps / 100));
    setScFixedDollars(centsToDollars(b.service_charge_cents));
    setScHeadcount(
      b.service_charge_headcount > 0 ? String(b.service_charge_headcount) : "",
    );
    setItems(
      b.items.map((it) => ({
        name: it.name,
        priceDollars: centsToDollars(it.price_cents),
      })),
    );
  }

  function showToast(msg: string) {
    setToast(msg);
    setTimeout(() => setToast(null), 1800);
  }

  function refreshSummary() {
    if (!id) return;
    api
      .summary(id)
      .then(setSummary)
      .catch(() => setSummary(null));
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
    refreshSummary();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  async function handleFile(file: File | undefined) {
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

  function onUpload(e: ChangeEvent<HTMLInputElement>) {
    void handleFile(e.target.files?.[0]);
  }

  function onDrop(e: DragEvent) {
    e.preventDefault();
    setDragging(false);
    void handleFile(e.dataTransfer.files?.[0]);
  }

  // Service charge entry: a percent rate (stored as basis points) or a fixed
  // amount; scHeadcount blank means "split across everyone who joined" (0).
  const ratePercentNum = parseFloat(scRatePercent);
  const scRateBps = Number.isFinite(ratePercentNum)
    ? Math.max(0, Math.round(ratePercentNum * 100))
    : 0;
  const scFixedCents = dollarsToCents(scFixedDollars);
  const scHeadcountNum =
    scHeadcount.trim() === ""
      ? 0
      : Math.max(0, Math.trunc(Number(scHeadcount)) || 0);

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
        service_charge_kind: scKind,
        service_charge_rate_bps: scKind === "percent" ? scRateBps : 0,
        service_charge_cents: scKind === "fixed" ? scFixedCents : 0,
        service_charge_headcount: scKind === "fixed" ? scHeadcountNum : 0,
        items: items.map((it) => ({
          name: it.name,
          price_cents: dollarsToCents(it.priceDollars),
        })),
      });
      loadFromBill(updated);
      refreshSummary();
      showToast("Tab saved");
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
      showToast("Link copied");
    } catch {
      showToast("Couldn't copy");
    }
  }

  // saveVenmoHandle stores the host's Venmo handle on their account, so every
  // new tab reuses it and friends know where to send their share.
  async function saveVenmoHandle() {
    setError(null);
    setSavingHandle(true);
    try {
      const updated = await api.updateVenmoHandle(venmoHandle);
      setUser(updated);
      setVenmoHandle(updated.venmo_handle ?? "");
      showToast("Venmo handle saved");
    } catch (err) {
      setError(err instanceof Error ? err.message : "could not save handle");
    } finally {
      setSavingHandle(false);
    }
  }

  // togglePaid lets the host confirm or undo a friend's payment.
  async function togglePaid(participantId: string, paid: boolean) {
    if (!id) return;
    setError(null);
    try {
      setSummary(await api.markPayment(id, participantId, paid));
    } catch (err) {
      setError(err instanceof Error ? err.message : "could not update payment");
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
        <div className="page">
          <button className="back-btn" onClick={() => navigate("/")}>
            <Icon.ArrowLeft size={14} /> Tabs
          </button>
          <p className="body danger mt-6">{error ?? "Tab not found."}</p>
        </div>
      </PaperApp>
    );
  }

  /* ── Parsing ─────────────────────────────────────────────────────── */
  if (parsing) {
    return (
      <PaperApp>
        <ParsingView />
      </PaperApp>
    );
  }

  /* ── Upload (no items yet, manual mode not chosen) ───────────────── */
  if (items.length === 0 && !manual) {
    return (
      <PaperApp>
        <div className="page">
          <div className="row row-between">
            <button className="back-btn" onClick={() => navigate("/")}>
              <Icon.ArrowLeft size={14} /> Back
            </button>
            <span className="eyebrow">Step 1 / 3</span>
          </div>

          <h2 className="h-section mt-6" style={{ fontSize: 28 }}>
            Snap the receipt.
          </h2>
          <p className="body muted mt-2">
            We'll read the items so you don't have to type them in.
          </p>

          {error && (
            <p className="body danger mt-3" style={{ fontSize: 13 }}>
              {error}
            </p>
          )}

          <input
            ref={fileRef}
            type="file"
            accept="image/*,.heic,.heif"
            onChange={onUpload}
            style={{ display: "none" }}
          />
          <div
            className={`dropzone mt-6${dragging ? " dragging" : ""}`}
            style={{ height: 260, justifyContent: "center" }}
            onClick={() => fileRef.current?.click()}
            onDragOver={(e) => {
              e.preventDefault();
              setDragging(true);
            }}
            onDragLeave={() => setDragging(false)}
            onDrop={onDrop}
          >
            <div className="dropzone-orb">
              <Icon.Camera size={28} />
            </div>
            <p style={{ margin: 0, fontSize: 15, fontWeight: 500 }}>
              {dragging ? "Drop it!" : "Drop a photo"}
            </p>
            <p className="body muted" style={{ fontSize: 12, margin: 0 }}>
              or tap to choose one
            </p>
            <span className="pill mt-2">PNG · JPG · HEIC</span>
          </div>

          <p className="body muted center mt-4" style={{ fontSize: 12 }}>
            or
          </p>
          <button
            className="btn btn-ghost btn-block"
            onClick={() => {
              setManual(true);
              addItem();
            }}
          >
            Enter items manually
          </button>
        </div>
      </PaperApp>
    );
  }

  /* ── Editor + Share ──────────────────────────────────────────────── */
  const subtotalCents = items.reduce(
    (sum, it) => sum + dollarsToCents(it.priceDollars),
    0,
  );
  const serviceCents =
    scKind === "percent"
      ? Math.round((scRateBps * subtotalCents) / 10000)
      : scKind === "fixed"
        ? scFixedCents
        : 0;
  const totalCents =
    subtotalCents +
    dollarsToCents(taxDollars) +
    dollarsToCents(tipDollars) +
    serviceCents;
  const fmt = (c: number) => formatMoney(c, currency);
  const joined = summary?.participants ?? [];

  return (
    <PaperApp>
      <div className="page">
        <div className="row row-between">
          <button className="back-btn" onClick={() => navigate("/")}>
            <Icon.ArrowLeft size={14} /> Tabs
          </button>
          <span className="eyebrow">Step 3 / 3</span>
        </div>

        {error && (
          <p className="body danger mt-3" style={{ fontSize: 13 }}>
            {error}
          </p>
        )}

        {/* Editable receipt */}
        <div className="receipt mt-3">
          <div className="receipt-head">
            <input
              className="receipt-input rname"
              value={restaurant}
              placeholder="Restaurant"
              onChange={(e) => setRestaurant(e.target.value)}
              style={{
                fontFamily: "var(--serif)",
                fontStyle: "italic",
                fontSize: 24,
                textAlign: "center",
              }}
            />
            <div
              className="row gap-1"
              style={{ justifyContent: "center", marginTop: 4 }}
            >
              <span className="rmeta">Currency</span>
              <input
                className="receipt-input rmeta"
                value={currency}
                maxLength={3}
                onChange={(e) =>
                  setCurrency(
                    e.target.value.toUpperCase().replace(/[^A-Z]/g, ""),
                  )
                }
                style={{
                  width: 42,
                  textAlign: "center",
                  fontFamily: "var(--mono)",
                  fontSize: 10,
                  letterSpacing: "0.08em",
                }}
              />
            </div>
          </div>

          {items.map((it, i) => (
            <div
              key={i}
              className="receipt-row"
              style={{ gridTemplateColumns: "1fr auto auto" }}
            >
              <input
                className="receipt-input"
                placeholder="Item"
                value={it.name}
                onChange={(e) => updateItem(i, { name: e.target.value })}
              />
              <input
                className="receipt-input receipt-input-price"
                type="number"
                step="0.01"
                min="0"
                value={it.priceDollars}
                onChange={(e) =>
                  updateItem(i, { priceDollars: e.target.value })
                }
              />
              <button
                onClick={() => removeItem(i)}
                title="Remove item"
                style={{
                  background: "transparent",
                  border: 0,
                  color: "var(--muted)",
                  padding: "0 0 0 8px",
                  fontSize: 16,
                  lineHeight: 1,
                  cursor: "pointer",
                }}
              >
                ×
              </button>
            </div>
          ))}

          <button
            onClick={addItem}
            className="link-btn"
            style={{ margin: "12px 0 4px" }}
          >
            <Icon.Plus size={12} /> Add item
          </button>

          <div className="receipt-totals">
            <div className="line">
              <span>Subtotal</span>
              <span>{fmt(subtotalCents)}</span>
            </div>
            <div className="line">
              <span>Tax</span>
              <input
                className="receipt-input receipt-input-price"
                type="number"
                step="0.01"
                min="0"
                value={taxDollars}
                onChange={(e) => setTaxDollars(e.target.value)}
                style={{ fontSize: 12, color: "var(--muted)" }}
              />
            </div>
            <div className="line">
              <span>Tip</span>
              <input
                className="receipt-input receipt-input-price"
                type="number"
                step="0.01"
                min="0"
                value={tipDollars}
                onChange={(e) => setTipDollars(e.target.value)}
                style={{ fontSize: 12, color: "var(--muted)" }}
              />
            </div>
            {scKind !== "none" && (
              <div className="line">
                <span>Service</span>
                <span>{fmt(serviceCents)}</span>
              </div>
            )}
            <div className="line grand">
              <span>Total</span>
              <span>{fmt(totalCents)}</span>
            </div>
          </div>
          <ReceiptZig />
        </div>

        {/* Service charge — appears only when the receipt had one detected.
            Setting the type to None and saving removes it. */}
        {bill.service_charge_kind !== "none" && (
          <div className="card mt-4">
            <p className="eyebrow">Service charge</p>
            <p className="body muted mt-2" style={{ fontSize: 12 }}>
              A mandatory restaurant fee from the receipt. It's split
              automatically — never claimed as an item.
            </p>
            <select
              className="input mt-3"
              value={scKind}
              onChange={(e) => setScKind(e.target.value as ServiceChargeKind)}
            >
              <option value="none">None — remove it</option>
              <option value="percent">Percentage of the bill</option>
              <option value="fixed">Fixed amount</option>
            </select>

            {scKind === "percent" && (
              <div className="col gap-1 mt-3">
                <label className="eyebrow">Rate (%)</label>
                <input
                  className="input input-mono"
                  type="number"
                  step="0.01"
                  min="0"
                  value={scRatePercent}
                  onChange={(e) => setScRatePercent(e.target.value)}
                />
                <p className="body muted" style={{ fontSize: 11 }}>
                  {fmt(serviceCents)} — split in proportion to what each person
                  ordered.
                </p>
              </div>
            )}

            {scKind === "fixed" && (
              <div className="col gap-1 mt-3">
                <label className="eyebrow">Amount ({currency})</label>
                <input
                  className="input input-mono"
                  type="number"
                  step="0.01"
                  min="0"
                  value={scFixedDollars}
                  onChange={(e) => setScFixedDollars(e.target.value)}
                />
                <label className="eyebrow" style={{ marginTop: 8 }}>
                  Number of diners
                </label>
                <input
                  className="input input-mono"
                  type="number"
                  step="1"
                  min="0"
                  placeholder="blank = everyone who joins"
                  value={scHeadcount}
                  onChange={(e) => setScHeadcount(e.target.value)}
                />
                <p className="body muted" style={{ fontSize: 11 }}>
                  Split evenly. Leave the headcount blank to divide it among
                  everyone who joins; set it higher if some diners aren't using
                  the app — their shares then show as unclaimed.
                </p>
              </div>
            )}
          </div>
        )}

        <button
          className="btn btn-accent btn-block mt-4"
          onClick={onSave}
          disabled={saving}
        >
          {saving ? "Saving…" : "Save tab"}
        </button>

        {/* Venmo handle — friends pay their share straight to it */}
        <div className="card mt-8">
          <div className="row row-between">
            <span className="eyebrow">Your Venmo</span>
            <span className="eyebrow muted">
              {user?.venmo_handle ? "set ✓" : "needed"}
            </span>
          </div>
          <p className="body muted mt-2" style={{ fontSize: 12 }}>
            {user?.venmo_handle
              ? "Friends pay their share straight to this handle."
              : "Set it once — every new tab reuses it automatically."}
          </p>
          <div className="row gap-2 mt-3">
            <input
              className="input input-mono"
              style={{ flex: 1 }}
              type="text"
              placeholder="@your-venmo"
              value={venmoHandle}
              onChange={(e) => setVenmoHandle(e.target.value)}
            />
            <button
              className="btn btn-ghost btn-sm"
              onClick={saveVenmoHandle}
              disabled={savingHandle}
            >
              {savingHandle ? "Saving…" : "Save"}
            </button>
          </div>
        </div>

        {/* Share */}
        <h2 className="h-section mt-8">Send it round.</h2>
        <p className="body muted mt-2">
          Anyone with the link can claim their items.
        </p>

        {bill.share_url ? (
          <>
            <div
              className="mt-6"
              style={{ display: "flex", justifyContent: "center" }}
            >
              <QrCode value={bill.share_url} />
            </div>
            <div
              className="mt-4"
              style={{
                background: "var(--paper)",
                border: "1px dashed var(--line-dashed)",
                borderRadius: 10,
                padding: "12px 14px",
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                gap: 10,
              }}
            >
              <span className="mono truncate" style={{ fontSize: 12 }}>
                {bill.share_url}
              </span>
              <button className="link-btn" onClick={copyShareLink}>
                <Icon.Copy size={12} /> Copy
              </button>
            </div>
          </>
        ) : (
          <p className="body muted mt-4" style={{ fontSize: 13 }}>
            Save the tab to generate a share link.
          </p>
        )}

        {/* Joined */}
        <div className="row row-between mt-6 mb-2">
          <span className="eyebrow">Joined</span>
          <span className="eyebrow muted">{joined.length}</span>
        </div>
        {joined.length === 0 ? (
          <p className="body muted" style={{ fontSize: 13 }}>
            No one's joined yet — share the link above.
          </p>
        ) : (
          <div className="col">
            {joined.map((p) => {
              const share = summary?.split.participants.find(
                (s) => s.participant_id === p.id,
              );
              const isPaid = p.payment_status === "paid";
              return (
                <div key={p.id} className="party-row">
                  <Avatar name={p.display_name} seed={p.id} size="md" />
                  <div className="flex1">
                    <p style={{ margin: 0, fontSize: 14 }}>{p.display_name}</p>
                    <p
                      className="mono muted"
                      style={{ margin: 0, fontSize: 11 }}
                    >
                      {isPaid
                        ? "paid ✓"
                        : p.payment_status === "pending"
                          ? "paying…"
                          : "claiming…"}
                    </p>
                  </div>
                  <span className="mono" style={{ fontSize: 13 }}>
                    {fmt(share?.total_cents ?? 0)}
                  </span>
                  <button
                    className="btn btn-sm"
                    onClick={() => togglePaid(p.id, !isPaid)}
                    title={isPaid ? "Tap to mark unpaid" : "Tap to mark paid"}
                    style={{
                      flexShrink: 0,
                      padding: "5px 11px",
                      background: isPaid ? "var(--accent)" : "transparent",
                      color: isPaid ? "var(--ink)" : "var(--muted)",
                      border: isPaid ? "0" : "1px solid var(--line)",
                    }}
                  >
                    {isPaid ? "Paid ✓" : "Mark paid"}
                  </button>
                </div>
              );
            })}
          </div>
        )}

        {summary && joined.length > 0 && (
          <>
            {!user?.venmo_handle && (
              <p className="body danger mt-4" style={{ fontSize: 12 }}>
                Friends can't pay until you add your Venmo handle above.
              </p>
            )}
            <hr className="dash mt-4" />
            <div className="row row-between mt-4">
              <span className="eyebrow">Grand total</span>
              <span className="mono" style={{ fontSize: 14, fontWeight: 600 }}>
                {fmt(summary.split.grand_total_cents)}
              </span>
            </div>
            {summary.split.unclaimed_cents > 0 && (
              <div className="row row-between mt-1">
                <span className="eyebrow muted">Unclaimed</span>
                <span className="mono muted" style={{ fontSize: 13 }}>
                  {fmt(summary.split.unclaimed_cents)}
                </span>
              </div>
            )}
          </>
        )}
      </div>
      {toast && <div className="toast">{toast}</div>}
    </PaperApp>
  );
}
