// Shared UI primitives for the IOU "Paper" design — icons, avatars, the paper
// column shell, the brand wordmark, receipt zig-zag edge and a live QR code.
import { useEffect, useState, type CSSProperties, type ReactNode } from "react";
import QRCode from "qrcode";

/* ─── Icons ─────────────────────────────────────────────────────────── */
interface IconProps {
  size?: number;
  className?: string;
  style?: CSSProperties;
}

// eslint-disable-next-line react-refresh/only-export-components -- icon registry of SVG components
export const Icon = {
  Plus: ({ size = 18, ...p }: IconProps) => (
    <svg
      viewBox="0 0 24 24"
      width={size}
      height={size}
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      {...p}
    >
      <path d="M12 5v14M5 12h14" />
    </svg>
  ),
  Pencil: ({ size = 16, ...p }: IconProps) => (
    <svg
      viewBox="0 0 24 24"
      width={size}
      height={size}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...p}
    >
      <path d="M12 20h9" />
      <path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z" />
    </svg>
  ),
  Camera: ({ size = 22, ...p }: IconProps) => (
    <svg
      viewBox="0 0 24 24"
      width={size}
      height={size}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...p}
    >
      <path d="M3 7h3l2-3h8l2 3h3v12H3z" />
      <circle cx="12" cy="13" r="3.6" />
    </svg>
  ),
  Check: ({ size = 14, ...p }: IconProps) => (
    <svg
      viewBox="0 0 24 24"
      width={size}
      height={size}
      fill="none"
      stroke="currentColor"
      strokeWidth="2.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...p}
    >
      <path d="M5 12l4.5 4.5L19 7" />
    </svg>
  ),
  CheckBig: ({ size = 44, ...p }: IconProps) => (
    <svg
      viewBox="0 0 64 64"
      width={size}
      height={size}
      fill="none"
      stroke="currentColor"
      strokeWidth="3.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...p}
    >
      <path d="M18 33l9 9 19-21" />
    </svg>
  ),
  Copy: ({ size = 14, ...p }: IconProps) => (
    <svg
      viewBox="0 0 24 24"
      width={size}
      height={size}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...p}
    >
      <rect x="8" y="8" width="13" height="13" rx="2" />
      <path d="M16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3" />
    </svg>
  ),
  Share: ({ size = 16, ...p }: IconProps) => (
    <svg
      viewBox="0 0 24 24"
      width={size}
      height={size}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...p}
    >
      <path d="M12 16V4M7 9l5-5 5 5M5 14v5a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-5" />
    </svg>
  ),
  Mail: ({ size = 16, ...p }: IconProps) => (
    <svg
      viewBox="0 0 24 24"
      width={size}
      height={size}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...p}
    >
      <rect x="3" y="5" width="18" height="14" rx="2" />
      <path d="M3 7l9 7 9-7" />
    </svg>
  ),
  Arrow: ({ size = 14, ...p }: IconProps) => (
    <svg
      viewBox="0 0 24 24"
      width={size}
      height={size}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...p}
    >
      <path d="M5 12h14M13 5l7 7-7 7" />
    </svg>
  ),
  ArrowLeft: ({ size = 14, ...p }: IconProps) => (
    <svg
      viewBox="0 0 24 24"
      width={size}
      height={size}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...p}
    >
      <path d="M19 12H5M11 5l-7 7 7 7" />
    </svg>
  ),
  Trash: ({ size = 16, ...p }: IconProps) => (
    <svg
      viewBox="0 0 24 24"
      width={size}
      height={size}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...p}
    >
      <path d="M4 7h16M10 11v6M14 11v6" />
      <path d="M6 7l1 13a1 1 0 0 0 1 1h8a1 1 0 0 0 1-1l1-13" />
      <path d="M9 7V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v3" />
    </svg>
  ),
  Mic: ({ size = 22, ...p }: IconProps) => (
    <svg
      viewBox="0 0 24 24"
      width={size}
      height={size}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...p}
    >
      <rect x="9" y="3" width="6" height="11" rx="3" />
      <path d="M5 11a7 7 0 0 0 14 0M12 18v3" />
    </svg>
  ),
  ChevronDown: ({ size = 14, ...p }: IconProps) => (
    <svg
      viewBox="0 0 24 24"
      width={size}
      height={size}
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...p}
    >
      <path d="M6 9l6 6 6-6" />
    </svg>
  ),
  Wallet: ({ size = 16, ...p }: IconProps) => (
    <svg
      viewBox="0 0 24 24"
      width={size}
      height={size}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      {...p}
    >
      <path d="M3 7a2 2 0 0 1 2-2h13a2 2 0 0 1 2 2v0H3z" />
      <path d="M3 7v10a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-3h-4a2 2 0 0 1 0-4h4V9" />
      <circle cx="16" cy="12" r="1" fill="currentColor" />
    </svg>
  ),
};

/* ─── Brand wordmark ────────────────────────────────────────────────── */
// Pass onClick to make the wordmark a tappable way out of a page — used as a
// "back to your split" affordance on the friend's settled-up screen.
export function Brand({
  size = 32,
  onClick,
}: {
  size?: number;
  onClick?: () => void;
}) {
  const style: CSSProperties = { fontSize: size, lineHeight: 1 };
  if (onClick) {
    return (
      <button
        type="button"
        className="brand-mark brand-mark-btn"
        style={style}
        onClick={onClick}
        aria-label="Back to your split"
      >
        IOU
      </button>
    );
  }
  return (
    <span className="brand-mark" style={style}>
      IOU
    </span>
  );
}

/* ─── Paper column shell ────────────────────────────────────────────── */
export function PaperApp({ children }: { children: ReactNode }) {
  return (
    <div className="paper-app">
      <div className="paper-scroll">{children}</div>
    </div>
  );
}

/* ─── Avatars ───────────────────────────────────────────────────────── */
function initialsOf(name: string): string {
  return (name || "?")
    .trim()
    .split(/\s+/)
    .slice(0, 2)
    .map((p) => p[0])
    .join("")
    .toUpperCase();
}

// Deterministic 1..6 colour bucket from any stable id/name.
function colorFor(seed: string): number {
  let h = 0;
  for (let i = 0; i < seed.length; i++) h = (h * 31 + seed.charCodeAt(i)) | 0;
  return (Math.abs(h) % 6) + 1;
}

type AvSize = "" | "xs" | "sm" | "md" | "lg";

export function Avatar({
  name,
  seed,
  size = "",
}: {
  name: string;
  seed?: string;
  size?: AvSize;
}) {
  const color = colorFor(seed ?? name);
  return (
    <span className={`av${size ? " av-" + size : ""} av-${color}`}>
      {initialsOf(name)}
    </span>
  );
}

export function AvatarStack({
  people,
  size = "xs",
}: {
  people: { id?: string; name: string }[];
  size?: AvSize;
}) {
  return (
    <span className="av-stack">
      {people.map((p, i) => (
        <Avatar
          key={p.id ?? i}
          name={p.name}
          seed={p.id ?? p.name}
          size={size}
        />
      ))}
    </span>
  );
}

/* ─── Receipt zig-zag bottom edge ───────────────────────────────────── */
export function ReceiptZig() {
  const pts = ["0,0", "320,0", "320,12"];
  const N = 40;
  for (let i = N; i >= 0; i--) {
    const x = (i / N) * 320;
    const y = i % 2 === 0 ? 12 : 4;
    pts.push(`${x.toFixed(1)},${y}`);
  }
  return (
    <svg
      className="receipt-zig"
      viewBox="0 0 320 12"
      preserveAspectRatio="none"
    >
      <polygon
        points={pts.join(" ")}
        fill="#fffdf6"
        stroke="rgba(31,61,43,.18)"
        strokeWidth="1"
      />
    </svg>
  );
}

/* ─── Live QR code ──────────────────────────────────────────────────── */
export function QrCode({ value }: { value: string }) {
  const [svg, setSvg] = useState<string>("");
  useEffect(() => {
    let alive = true;
    QRCode.toString(value, {
      type: "svg",
      margin: 0,
      color: { dark: "#1f3d2b", light: "#fffdf600" },
    })
      .then((s) => {
        if (alive) setSvg(s);
      })
      .catch(() => undefined);
    return () => {
      alive = false;
    };
  }, [value]);
  return (
    <div className="qr" aria-label="QR code for the share link">
      <div
        style={{ width: "100%", height: "100%" }}
        dangerouslySetInnerHTML={{ __html: svg }}
      />
    </div>
  );
}
