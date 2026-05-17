export interface User {
  id: string;
  email: string;
  venmo_handle: string | null;
}

export interface BillItem {
  id: string;
  name: string;
  price_cents: number;
  position: number;
}

// ServiceChargeKind controls how a mandatory service charge is split:
// "percent" prorates over what each person ordered, "fixed" splits evenly
// across the diner headcount, "none" means there is no service charge.
export type ServiceChargeKind = "none" | "percent" | "fixed";

// split_mode is how a bill is divvied up: "claim" is the default — friends
// open the share link and self-claim items; "host" means the host already
// split it (via the audio-split flow) and friends only pick their name + pay.
export type SplitMode = "claim" | "host";

export interface Bill {
  id: string;
  restaurant: string;
  currency: string;
  tax_cents: number;
  tip_cents: number;
  service_charge_kind: ServiceChargeKind;
  service_charge_rate_bps: number;
  service_charge_cents: number;
  service_charge_headcount: number;
  split_mode: SplitMode;
  items: BillItem[];
  created_at: number;
  friend_token?: string;
  share_url?: string;
}

export interface BillUpdate {
  restaurant: string;
  currency: string;
  tax_cents: number;
  tip_cents: number;
  service_charge_kind: ServiceChargeKind;
  service_charge_rate_bps: number;
  service_charge_cents: number;
  service_charge_headcount: number;
  // id is the existing item's id, echoed back so the server updates that row
  // in place and any claims on it survive the edit; omit it for a new item.
  items: { id?: string; name: string; price_cents: number }[];
}

export type PaymentStatus = "none" | "pending" | "paid";

export interface Participant {
  id: string;
  display_name: string;
  payment_status: PaymentStatus;
  // host_managed marks a participant the host created via the audio split
  // (rather than someone who joined themselves). is_host flags the host's own
  // participant row. participant_token is the per-participant token a friend
  // uses to pay — present only on participants of a split_mode === "host" bill.
  host_managed?: boolean;
  is_host?: boolean;
  participant_token?: string;
}

// PaymentIntent is what the server hands a friend to settle in Venmo: the
// host's handle, the amount owed, and ready-made deep links (app_url opens
// the Venmo app; web_url opens venmo.com and is also encoded into a QR code).
export interface PaymentIntent {
  payment_id: string;
  status: PaymentStatus;
  amount_cents: number;
  currency: string;
  venmo_handle: string;
  note: string;
  app_url: string;
  web_url: string;
}

export interface Payment {
  id: string;
  participant_id: string;
  amount_cents: number;
  currency: string;
  status: "pending" | "paid";
  recipient: string;
}

export interface ParticipantShare {
  participant_id: string;
  item_subtotal_cents: number;
  tax_cents: number;
  tip_cents: number;
  service_cents: number;
  total_cents: number;
}

export interface SplitResult {
  participants: ParticipantShare[];
  unclaimed_cents: number;
  service_charge_cents: number;
  grand_total_cents: number;
}

// ClaimEntry is one friend's claim on an item. share_count is the headcount
// they declared for sharing the dish: they pay 1/share_count of it. Their
// share never drops below 1/(number of claimers), so an item is never
// over-collected even if the declared counts disagree.
export interface ClaimEntry {
  participant_id: string;
  share_count: number;
}

export interface BillSummary {
  bill: Bill;
  items: BillItem[];
  participants: Participant[];
  claims: Record<string, ClaimEntry[]>;
  split: SplitResult;
}

// AutoSplitResult is what POST /auto-split returns: a full bill summary
// (with the host-created participants + claims) plus the prompt the AI worked
// from (a transcript of the audio, or the host's typed text used verbatim)
// and Claude's notes about how it assigned items.
export interface AutoSplitResult extends BillSummary {
  prompt: string;
  notes: string;
}

// JoinedTab is one bill the signed-in user joined as a friend (not as host).
// owed_cents is their own share from the live split; payment_status is their
// pay state on it. friend_token reopens the share link straight to their view.
export interface JoinedTab {
  bill_id: string;
  restaurant: string;
  currency: string;
  created_at: number;
  friend_token: string;
  split_mode: SplitMode;
  participant_id: string;
  display_name: string;
  owed_cents: number;
  payment_status: PaymentStatus;
}

// API_BASE prefixes every request path. It is empty for the web build (the Go
// server serves the SPA, so `/api` is same-origin) and set to the live backend
// for the native iOS build — see VITE_API_BASE in vite-env.d.ts.
const API_BASE = import.meta.env.VITE_API_BASE ?? "";

/** apiUrl resolves an `/api/...` path against the configured backend. */
export function apiUrl(path: string): string {
  return API_BASE + path;
}

// The native iOS build is cross-origin with the API and receives no session
// cookie, so it authenticates with a bearer token instead. The web build is
// same-origin and uses the HttpOnly session cookie, so it stores no token.
// IS_NATIVE keys off VITE_API_BASE, which only the iOS build sets.
const IS_NATIVE = API_BASE !== "";
const TOKEN_KEY = "iou_session_token";

/** setAuthToken persists (or clears) the native app's session token. It is a
 *  no-op on the web build, which authenticates with the session cookie. */
export function setAuthToken(token: string | null): void {
  if (!IS_NATIVE) return;
  if (token) localStorage.setItem(TOKEN_KEY, token);
  else localStorage.removeItem(TOKEN_KEY);
}

/** authHeaders returns the bearer Authorization header for the native app,
 *  or an empty object on the web build / when signed out. */
function authHeaders(): Record<string, string> {
  const t = IS_NATIVE ? localStorage.getItem(TOKEN_KEY) : null;
  return t ? { Authorization: `Bearer ${t}` } : {};
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(apiUrl(path), {
    credentials: "same-origin",
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...authHeaders(),
      ...init?.headers,
    },
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error((body as { error?: string }).error ?? "request failed");
  }
  return body as T;
}

export const api = {
  me: () => request<User>("/api/auth/me"),
  requestLink: (email: string) =>
    request<{ message: string; link?: string }>("/api/auth/request", {
      method: "POST",
      body: JSON.stringify({ email }),
    }),
  // verify exchanges a magic-link token for a session. On the native build it
  // also stashes the session token the response carries, so later requests
  // can send it as a bearer token (the web build relies on the cookie).
  verify: async (token: string): Promise<User> => {
    const u = await request<User & { token?: string }>("/api/auth/verify", {
      method: "POST",
      body: JSON.stringify({ token }),
    });
    if (u.token) setAuthToken(u.token);
    return u;
  },
  logout: async (): Promise<{ message: string }> => {
    try {
      return await request<{ message: string }>("/api/auth/logout", {
        method: "POST",
      });
    } finally {
      setAuthToken(null);
    }
  },
  updateVenmoHandle: (venmo_handle: string) =>
    request<User>("/api/users/me", {
      method: "PATCH",
      body: JSON.stringify({ venmo_handle }),
    }),
  createBill: () => request<Bill>("/api/bills", { method: "POST" }),
  listBills: () => request<Bill[]>("/api/bills"),
  // joinedTabs lists bills the signed-in user joined as a friend (not as
  // host), for the "tabs you joined" section on Home.
  joinedTabs: () => request<JoinedTab[]>("/api/bills/joined"),
  getBill: (id: string, token?: string) =>
    request<Bill>(
      `/api/bills/${id}${token ? `?t=${encodeURIComponent(token)}` : ""}`,
    ),
  uploadReceipt: async (id: string, file: File): Promise<Bill> => {
    const form = new FormData();
    form.append("receipt", file);
    const res = await fetch(apiUrl(`/api/bills/${id}/receipt`), {
      method: "POST",
      credentials: "same-origin",
      headers: authHeaders(),
      body: form,
    });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) {
      throw new Error((body as { error?: string }).error ?? "upload failed");
    }
    return body as Bill;
  },
  // autoSplit sends the host's description of how the bill splits — either an
  // audio recording or a typed text prompt — to the server, which transcribes
  // audio (typed text is used verbatim), uses Claude to assign items to named
  // people, creates participants + claims, and flips the bill to "host" split
  // mode. Multipart, like uploadReceipt — no Content-Type header.
  autoSplit: async (
    id: string,
    hostName: string,
    input: { audio: File } | { text: string },
  ): Promise<AutoSplitResult> => {
    const form = new FormData();
    form.append("host_name", hostName);
    if ("audio" in input) form.append("audio", input.audio);
    else form.append("text", input.text);
    const res = await fetch(apiUrl(`/api/bills/${id}/auto-split`), {
      method: "POST",
      credentials: "same-origin",
      headers: authHeaders(),
      body: form,
    });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) {
      throw new Error(
        (body as { error?: string }).error ?? "auto-split failed",
      );
    }
    return body as AutoSplitResult;
  },
  updateBill: (id: string, update: BillUpdate) =>
    request<Bill>(`/api/bills/${id}`, {
      method: "PATCH",
      body: JSON.stringify(update),
    }),
  // deleteBill removes a tab the signed-in user hosts. The server replies
  // 204 No Content, so there is no body to parse on success.
  deleteBill: async (id: string): Promise<void> => {
    const res = await fetch(apiUrl(`/api/bills/${id}`), {
      method: "DELETE",
      credentials: "same-origin",
      headers: authHeaders(),
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(
        (body as { error?: string }).error ?? "could not delete tab",
      );
    }
  },
  billByToken: (token: string) =>
    request<Bill>(`/api/by-token/${encodeURIComponent(token)}`),
  joinBill: (id: string, display_name: string, t: string) =>
    request<{ participant: Participant; participant_token: string }>(
      `/api/bills/${id}/participants`,
      {
        method: "POST",
        body: JSON.stringify({ display_name, t }),
      },
    ),
  // linkIdentity ties a host-split participant to the signed-in user's
  // account when they pick that identity, so the tab shows on their Home.
  // Best-effort: callers fire it without blocking the pay flow.
  linkIdentity: (id: string, participant_id: string, t: string) =>
    request<{ status: string }>(
      `/api/bills/${id}/participants/${participant_id}/link`,
      {
        method: "POST",
        body: JSON.stringify({ t }),
      },
    ),
  // myParticipant returns the signed-in user's existing participant on a bill,
  // if they joined (or picked an identity) while logged in — so FriendSplit can
  // restore their identity from their account on any device. It rejects with a
  // 404 when they have none; callers treat that as "not joined yet".
  myParticipant: (id: string) =>
    request<{ participant: Participant; participant_token: string }>(
      `/api/bills/${id}/my-participant`,
    ),
  // setClaims replaces a friend's claims. Each claim carries a share_count —
  // the headcount for a shared dish (1 means the friend takes the whole item).
  setClaims: (
    id: string,
    participant_token: string,
    claims: { item_id: string; share_count: number }[],
  ) =>
    request<BillSummary>(`/api/bills/${id}/claims`, {
      method: "PUT",
      body: JSON.stringify({ participant_token, claims }),
    }),
  summary: (id: string, token?: string) =>
    request<BillSummary>(
      `/api/bills/${id}/summary${
        token ? `?t=${encodeURIComponent(token)}` : ""
      }`,
    ),
  // pay prepares a Venmo payment and returns the intent the friend needs to
  // hand off to Venmo. If the friend has already paid, the intent comes back
  // with status "paid".
  pay: (id: string, participant_token: string) =>
    request<PaymentIntent>(`/api/bills/${id}/pay`, {
      method: "POST",
      body: JSON.stringify({ participant_token }),
    }),
  // confirmPayment records the friend's self-report that they paid in Venmo.
  confirmPayment: (id: string, participant_token: string, payment_id: string) =>
    request<Payment>(`/api/bills/${id}/pay/confirm`, {
      method: "POST",
      body: JSON.stringify({ participant_token, payment_id }),
    }),
  // markPayment lets the host confirm (paid=true) or undo (paid=false) a
  // friend's payment; it returns the refreshed bill summary.
  markPayment: (id: string, participant_id: string, paid: boolean) =>
    request<BillSummary>(`/api/bills/${id}/payments/${participant_id}`, {
      method: "POST",
      body: JSON.stringify({ paid }),
    }),
  payments: (id: string, token?: string) =>
    request<Payment[]>(
      `/api/bills/${id}/payments${
        token ? `?t=${encodeURIComponent(token)}` : ""
      }`,
    ),
};
