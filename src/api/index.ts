// HTTP API client — drop-in replacement for the wailsjs/go/main/App bindings.
// Function names and signatures are intentionally identical so atom files only
// need their import path changed.
import { get, post, put, patch, del, setToken, clearToken } from "./client";

// ---- Auth ----

export interface CurrentUser {
  id: string;
  email: string;
  displayName: string;
  role: "admin" | "user";
  isActive: number;
  authProvider: "local" | "oidc";
}

export interface UserRecord extends CurrentUser {
  createdAt: string;
  lastLoginAt: number | null;
}

export const Login = async (email: string, password: string): Promise<{ token: string; user: CurrentUser }> => {
  const res = await fetch("/api/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error ?? res.statusText);
  }
  const data = await res.json();
  setToken(data.token);
  return data;
};

export const Logout = () => {
  clearToken();
  return Promise.resolve();
};

export const GetMe = () => get<CurrentUser>("/auth/me");

// GET /api/auth/oidc/enabled is intentionally not routed through the shared
// fetch wrapper's 401 handling (this is a public, unauthenticated endpoint) —
// a plain fetch keeps it simple and avoids any token-related side effects.
export const GetOidcEnabled = async (): Promise<boolean> => {
  try {
    const res = await fetch("/api/auth/oidc/enabled");
    if (!res.ok) return false;
    const data = await res.json();
    return Boolean(data.enabled);
  } catch {
    return false;
  }
};

// ---- Users (admin only) ----

export const ListUsers = (search?: string) =>
  get<UserRecord[]>(`/users${search ? `?search=${encodeURIComponent(search)}` : ""}`);
export const GetUser = (id: string) => get<UserRecord>(`/users/${id}`);
export const CreateUser = (req: { email: string; password: string; displayName: string; role: string }) =>
  post<UserRecord>("/users", req);
export const UpdateUser = (id: string, req: { displayName?: string; role?: string; isActive?: number; password?: string }) =>
  put<UserRecord>(`/users/${id}`, req);
export const DeleteUser = (id: string) => del<void>(`/users/${id}`);

// ---- Utility ----

export const GetVersion = () =>
  get<{ version: string }>("/version").then((r) => r.version);

export const OpenURL = (url: string) => {
  window.open(url, "_blank", "noopener,noreferrer");
};

export const SaveFile = (defaultName: string, contents: Blob | Uint8Array) => {
  const blob = contents instanceof Blob ? contents : new Blob([contents as BlobPart], { type: "application/octet-stream" });
  const href = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = href;
  a.download = defaultName;
  a.rel = "noopener";
  // The anchor must be in the DOM for the download to fire reliably (Safari/Firefox).
  document.body.appendChild(a);
  a.click();
  // Defer cleanup: revoking the object URL synchronously after click() can race the
  // browser's blob read, producing an empty/failed download that never reaches disk.
  setTimeout(() => {
    URL.revokeObjectURL(href);
    a.remove();
  }, 1000);
  return Promise.resolve(undefined);
};

export interface BackupEntry {
  name: string;
  size: number;
  createdAt: string;
}

export interface BackupConfig {
  enabled: boolean;
  scheduleHour: number;
  retentionDays: number;
}

export const ListBackups = () => get<BackupEntry[]>("/backups");

export const TriggerBackup = async (): Promise<string> => {
  const res = await fetch("/api/backups", { method: "POST" });
  if (!res.ok) throw new Error("Backup failed");
  const blob = await res.blob();
  const disposition = res.headers.get("Content-Disposition") ?? "";
  const match = disposition.match(/filename="([^"]+)"/);
  const filename = match?.[1] ?? "fatura-backup.db";
  const href = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = href;
  a.download = filename;
  a.rel = "noopener";
  document.body.appendChild(a);
  a.click();
  setTimeout(() => {
    URL.revokeObjectURL(href);
    a.remove();
  }, 1000);
  return filename;
};

export const RestoreNamedBackup = (name: string): Promise<{ message: string }> =>
  post<{ message: string }>(`/backups/${encodeURIComponent(name)}/restore`, {});

export const RestoreDatabase = async (file: File): Promise<string> => {
  const form = new FormData();
  form.append("database", file);
  const res = await fetch("/api/restore", { method: "POST", body: form });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error ?? res.statusText);
  }
  const data = await res.json();
  return data.message as string;
};

export const GetBackupConfig = () => get<BackupConfig>("/backup/config");
export const SetBackupConfig = (cfg: BackupConfig) => put<BackupConfig>("/backup/config", cfg);

// ---- Organizations ----

export const GetOrganizations = () => get<any[]>("/organizations");
export const GetOrganization = (id: string) => get<any>(`/organizations/${id}`);
export const CreateOrganization = (req: any) => post<any>("/organizations", req);
export const UpdateOrganization = (id: string, req: any) => put<any>(`/organizations/${id}`, req);
export const DeleteOrganization = (id: string) =>
  del<{ deleted: boolean }>(`/organizations/${id}`).then((r) => r.deleted);
export type OrganizationUsageCount = {
  clients: number;
  invoices: number;
  products: number;
  orders: number;
  deliveries: number;
  taxRates: number;
};
export const GetOrganizationUsageCount = (id: string) =>
  get<OrganizationUsageCount>(`/organizations/${id}/usage-count`);

// ---- Clients ----

export const GetClients = (organizationId: string) =>
  get<any[]>(`/organizations/${organizationId}/clients`);
export const GetClient = (id: string) => get<any>(`/clients/${id}`);
export const CreateClient = (req: any) => post<any>("/clients", req);
export const UpdateClient = (id: string, req: any) => put<any>(`/clients/${id}`, req);
export const DeleteClient = (id: string) =>
  del<{ deleted: boolean }>(`/clients/${id}`).then((r) => r.deleted);
export const GetClientInvoiceCount = (id: string) =>
  get<{ count: number }>(`/clients/${id}/invoice-count`).then((r) => r.count);

// ---- Invoices ----

export const GetInvoices = (organizationId: string) =>
  get<any[]>(`/organizations/${organizationId}/invoices`);
export const GetInvoice = (id: string) => get<any>(`/invoices/${id}`);
export const GetInvoiceLineItems = (id: string) => get<any[]>(`/invoices/${id}/line-items`);
export const CreateInvoice = (req: any) => post<any>("/invoices", req);
export const UpdateInvoice = (id: string, req: any) => put<any>(`/invoices/${id}`, req);
export const UpdateInvoiceState = (id: string, state: string) =>
  patch<any>(`/invoices/${id}/state`, { state });
export const DeleteInvoice = (id: string) =>
  del<{ deleted: boolean }>(`/invoices/${id}`).then((r) => r.deleted);

// ---- Tax Rates ----

export const GetTaxRates = (organizationId: string) =>
  get<any[]>(`/organizations/${organizationId}/tax-rates`);
export const GetTaxRate = (id: string) => get<any>(`/tax-rates/${id}`);
export const CreateTaxRate = (req: any) => post<any>("/tax-rates", req);
export const UpdateTaxRate = (id: string, req: any) => put<any>(`/tax-rates/${id}`, req);
export const DeleteTaxRate = (id: string) =>
  del<{ deleted: boolean }>(`/tax-rates/${id}`).then((r) => r.deleted);
export const GetTaxRateUsageCount = (id: string) =>
  get<{ count: number }>(`/tax-rates/${id}/usage-count`).then((r) => r.count);

// ---- Products ----

export const GetProducts = (organizationId: string) =>
  get<any[]>(`/organizations/${organizationId}/products`);
export const GetProduct = (id: string) => get<any>(`/products/${id}`);
export const CreateProduct = (req: any) => post<any>("/products", req);
export const UpdateProduct = (id: string, req: any) => put<any>(`/products/${id}`, req);
export const DeleteProduct = (id: string) =>
  del<{ deleted: boolean }>(`/products/${id}`).then((r) => r.deleted);
export const GetProductStockMovements = (id: string) =>
  get<any[]>(`/products/${id}/stock-movements`);

// ---- Stock Movements ----

export const GetStockMovements = (organizationId: string) =>
  get<any[]>(`/organizations/${organizationId}/stock-movements`);
export const CreateStockMovement = (req: any) => post<any>("/stock-movements", req);
export const DeleteStockMovement = (id: string) =>
  del<{ deleted: boolean }>(`/stock-movements/${id}`).then((r) => r.deleted);

// ---- Orders ----

export const GetOrders = (organizationId: string) =>
  get<any[]>(`/organizations/${organizationId}/orders`);
export const GetOrder = (id: string) => get<any>(`/orders/${id}`);
export const GetOrderLineItems = (id: string) => get<any[]>(`/orders/${id}/line-items`);
export const GetOrderDeliveredQuantities = (id: string) =>
  get<Record<string, number>>(`/orders/${id}/delivered-quantities`);
export const CreateOrder = (req: any) => post<any>("/orders", req);
export const UpdateOrder = (id: string, req: any) => put<any>(`/orders/${id}`, req);
export const UpdateOrderStatus = (id: string, status: string) =>
  patch<any>(`/orders/${id}/status`, { status });
export const DeleteOrder = (id: string) =>
  del<{ deleted: boolean }>(`/orders/${id}`).then((r) => r.deleted);

// ---- Outbound Deliveries ----

export const GetDeliveries = (organizationId: string) =>
  get<any[]>(`/organizations/${organizationId}/deliveries`);
export const GetNextDeliveryNumber = (organizationId: string) =>
  get<{ number: string }>(`/organizations/${organizationId}/deliveries/next-number`).then((r) => r.number);
export const GetDelivery = (id: string) => get<any>(`/deliveries/${id}`);
export const GetDeliveryLineItems = (id: string) => get<any[]>(`/deliveries/${id}/line-items`);
export const CreateDelivery = (req: any) => post<any>("/deliveries", req);
export const UpdateDelivery = (id: string, req: any) => put<any>(`/deliveries/${id}`, req);
export const UpdateDeliveryStatus = (id: string, status: string) =>
  patch<any>(`/deliveries/${id}/status`, { status });
export const DeleteDelivery = (id: string) =>
  del<{ success: boolean }>(`/deliveries/${id}`).then((r) => r.success);

