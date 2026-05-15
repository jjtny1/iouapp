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
  status: string;
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
  status?: string;
  items: { name: string; price_cents: number }[];
}

export type PaymentStatus = "none" | "pending" | "paid";

export interface Participant {
  id: string;
  display_name: string;
  payment_status: PaymentStatus;
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

export interface BillSummary {
  bill: Bill;
  items: BillItem[];
  participants: Participant[];
  claims: Record<string, string[]>;
  split: SplitResult;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    ...init,
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
  verify: (token: string) =>
    request<User>("/api/auth/verify", {
      method: "POST",
      body: JSON.stringify({ token }),
    }),
  logout: () =>
    request<{ message: string }>("/api/auth/logout", { method: "POST" }),
  updateVenmoHandle: (venmo_handle: string) =>
    request<User>("/api/users/me", {
      method: "PATCH",
      body: JSON.stringify({ venmo_handle }),
    }),
  createBill: () => request<Bill>("/api/bills", { method: "POST" }),
  listBills: () => request<Bill[]>("/api/bills"),
  getBill: (id: string, token?: string) =>
    request<Bill>(
      `/api/bills/${id}${token ? `?t=${encodeURIComponent(token)}` : ""}`,
    ),
  uploadReceipt: async (id: string, file: File): Promise<Bill> => {
    const form = new FormData();
    form.append("receipt", file);
    const res = await fetch(`/api/bills/${id}/receipt`, {
      method: "POST",
      credentials: "same-origin",
      body: form,
    });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) {
      throw new Error((body as { error?: string }).error ?? "upload failed");
    }
    return body as Bill;
  },
  updateBill: (id: string, update: BillUpdate) =>
    request<Bill>(`/api/bills/${id}`, {
      method: "PATCH",
      body: JSON.stringify(update),
    }),
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
  setClaims: (id: string, participant_token: string, item_ids: string[]) =>
    request<BillSummary>(`/api/bills/${id}/claims`, {
      method: "PUT",
      body: JSON.stringify({ participant_token, item_ids }),
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
