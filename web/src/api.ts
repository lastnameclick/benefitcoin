// Tiny typed API client. Access token lives in memory; the refresh token is
// persisted so a reload can transparently re-authenticate. A 401 triggers one
// refresh-and-retry. Every mutating request carries a fresh Idempotency-Key.

export type Role = "operator" | "holder";

export interface Branding {
  product_name: string;
  site_name: string;
  coin_name: string;
  coin_name_plural: string;
  coin_code: string;
}

export const DEFAULT_BRANDING: Branding = {
  product_name: "BenefitCoins",
  site_name: "BenefitCoins",
  coin_name: "coin",
  coin_name_plural: "coins",
  coin_code: "BNC",
};

export interface TokenResponse {
  access_token: string;
  expires_at: string;
  refresh_token: string;
  role: Role;
  customer_id: string;
  tenant_id: string;
  household: string;
  username: string;
}

export interface Balance {
  current_minor: number;
  available_minor: number;
  awaiting_approval_minor: number;
  current: string;
  available: string;
  awaiting_approval: string;
  currency: string;
}

export interface Account {
  id: string;
  customer_id?: string;
  kind: string;
  tb_account_id: string;
  currency: string;
  name: string;
  status: string;
  balance?: Balance;
}

export interface Task {
  id: string;
  name: string;
  description: string;
  value_minor: number;
  active: boolean;
}

export type TxType = "earn" | "redeem" | "adjust_credit" | "adjust_debit";

export interface Transaction {
  id: string;
  type: TxType;
  status: "pending" | "settled" | "voided";
  account_id: string;
  amount_minor: number;
  memo: string;
  task_id?: string;
  effective_at?: string;
  details?: Record<string, unknown>;
  tb_pending_transfer_id: string;
  tb_post_transfer_id?: string;
  created_at: string;
  decided_at?: string;
}

// Whether a transaction increases (credit) or decreases (debit) the balance.
export function isCredit(t: Transaction): boolean {
  return t.type === "earn" || t.type === "adjust_credit";
}

export function txLabel(t: Transaction): string {
  return { earn: "Earn", redeem: "Redeem", adjust_credit: "Credit", adjust_debit: "Debit" }[t.type];
}

export interface Customer {
  id: string;
  type: string;
  display_name: string;
  status: string;
  username?: string;
}

export interface AuditEvent {
  id: string;
  actor_identity_id?: string;
  action: string;
  entity_type: string;
  entity_id: string;
  metadata: Record<string, unknown>;
  created_at: string;
}

const REFRESH_KEY = "cpal_refresh";
let accessToken: string | null = null;

export function setAccessToken(t: string | null) {
  accessToken = t;
}
export function getRefreshToken(): string | null {
  return localStorage.getItem(REFRESH_KEY);
}
export function setRefreshToken(t: string | null) {
  if (t) localStorage.setItem(REFRESH_KEY, t);
  else localStorage.removeItem(REFRESH_KEY);
}

export class ApiError extends Error {
  code: string;
  status: number;
  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

function uuid(): string {
  return crypto.randomUUID();
}

async function tryRefresh(): Promise<boolean> {
  const rt = getRefreshToken();
  if (!rt) return false;
  const res = await fetch("/api/v1/auth/refresh", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ refresh_token: rt }),
  });
  if (!res.ok) {
    setRefreshToken(null);
    setAccessToken(null);
    return false;
  }
  const data: TokenResponse = await res.json();
  setAccessToken(data.access_token);
  setRefreshToken(data.refresh_token);
  return true;
}

async function request<T>(method: string, path: string, body?: unknown, retry = true): Promise<T> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (accessToken) headers.Authorization = `Bearer ${accessToken}`;
  if (method !== "GET") headers["Idempotency-Key"] = uuid();

  const res = await fetch(`/api/v1${path}`, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });

  if (res.status === 401 && retry) {
    if (await tryRefresh()) return request<T>(method, path, body, false);
  }

  if (res.status === 204) return undefined as T;

  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) {
    const err = data?.error ?? { code: "error", message: res.statusText };
    throw new ApiError(res.status, err.code, err.message);
  }
  return data as T;
}

export interface SignupInput {
  household_name: string;
  display_name: string;
  email: string;
  password: string;
}

export const api = {
  getConfig: () => request<Branding>("GET", "/config"),
  signup: (input: SignupInput) => request<TokenResponse>("POST", "/auth/signup", input),
  login: (username: string, password: string) =>
    request<TokenResponse>("POST", "/auth/login", { username, password }),
  refresh: tryRefresh,
  logout: async () => {
    const rt = getRefreshToken();
    if (rt) await request("POST", "/auth/logout", { refresh_token: rt }).catch(() => {});
    setAccessToken(null);
    setRefreshToken(null);
  },

  listAccounts: () => request<{ accounts: Account[] }>("GET", "/accounts"),
  getAccount: (id: string) => request<Account>("GET", `/accounts/${id}`),
  getBalance: (id: string) => request<Balance>("GET", `/accounts/${id}/balance`),
  accountTransactions: (id: string) =>
    request<{ transactions: Transaction[] }>("GET", `/accounts/${id}/transactions`),

  listTasks: () => request<{ tasks: Task[] }>("GET", "/tasks"),
  createTask: (name: string, description: string, value: string) =>
    request<Task>("POST", "/tasks", { name, description, value }),
  updateTask: (id: string, patch: Partial<{ name: string; description: string; value: string; active: boolean }>) =>
    request<Task>("PATCH", `/tasks/${id}`, patch),

  submitEarning: (accountId: string, taskId: string) =>
    request<Transaction>("POST", `/accounts/${accountId}/earnings`, { task_id: taskId }),
  requestRedemption: (accountId: string) =>
    request<Transaction>("POST", `/accounts/${accountId}/redemptions`, {}),
  adjust: (
    accountId: string,
    body: { direction: "credit" | "debit"; amount: string; reason: string; occurred_at?: string; details?: Record<string, unknown> },
  ) => request<Transaction>("POST", `/accounts/${accountId}/adjustments`, body),

  listCustomers: () => request<{ customers: Customer[] }>("GET", "/customers"),
  createCustomer: (type: string, display_name: string, username: string, password: string) =>
    request<Customer & { account?: Account }>("POST", "/customers", {
      type,
      display_name,
      username,
      password,
    }),

  pendingTransactions: () =>
    request<{ transactions: Transaction[] }>("GET", "/transactions?status=pending"),
  listTransactions: (status?: string) =>
    request<{ transactions: Transaction[] }>("GET", `/transactions${status ? `?status=${status}` : ""}`),
  settle: (id: string) => request<Transaction>("POST", `/transactions/${id}/settle`, {}),
  void: (id: string) => request<Transaction>("POST", `/transactions/${id}/void`, {}),

  audit: () => request<{ events: AuditEvent[] }>("GET", "/audit"),
};

// Format minor units (1000 = 1 coin) as a coin string.
export function coins(minor: number): string {
  const neg = minor < 0;
  const v = Math.abs(minor);
  const whole = Math.floor(v / 1000);
  const frac = v % 1000;
  let s = String(whole);
  if (frac !== 0) s += "." + String(frac).padStart(3, "0").replace(/0+$/, "");
  return (neg ? "-" : "") + s;
}
