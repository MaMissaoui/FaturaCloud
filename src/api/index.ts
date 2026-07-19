// HTTP API client — drop-in replacement for the wailsjs/go/main/App bindings.
// Function names and signatures are intentionally identical so atom files only
// need their import path changed.
import { get, post, put, patch, del, CSRF_HEADER } from "./client";
import type {
  Client, Invoice, InvoiceLineItem, Product, TaxRate, Organization,
  Order, OrderLineItem, Delivery, DeliveryLineItem, StockMovement,
} from "src/types/models";

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

// Login authenticates and — on success — the server sets the httpOnly session
// cookie via Set-Cookie. This is a bespoke fetch (not the shared wrapper), so
// the CSRF header and credentials are attached explicitly. No token is returned
// in the body; only the user record.
export const Login = async (email: string, password: string): Promise<{ user: CurrentUser }> => {
  const res = await fetch("/api/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json", [CSRF_HEADER]: "1" },
    credentials: "same-origin",
    body: JSON.stringify({ email, password }),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error ?? res.statusText);
  }
  return res.json();
};

// Logout must hit the server so it can expire the httpOnly cookie — JavaScript
// can't clear it. Routed through the wrapper so it carries the CSRF header.
export const Logout = () => post<{ message: string }>("/auth/logout", {});

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
  const res = await fetch("/api/backups", {
    method: "POST",
    headers: { [CSRF_HEADER]: "1" },
    credentials: "same-origin",
  });
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
  const res = await fetch("/api/restore", {
    method: "POST",
    headers: { [CSRF_HEADER]: "1" },
    credentials: "same-origin",
    body: form,
  });
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

export const GetOrganizations = () => get<Organization[]>("/organizations");
export const GetOrganization = (id: string) => get<Organization>(`/organizations/${id}`);
export const CreateOrganization = (req: Partial<Organization>) => post<Organization>("/organizations", req);
export const UpdateOrganization = (id: string, req: Partial<Organization>) => put<Organization>(`/organizations/${id}`, req);
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
  get<Client[]>(`/organizations/${organizationId}/clients`);
export const GetClient = (id: string) => get<Client>(`/clients/${id}`);
export const CreateClient = (req: Partial<Client>) => post<Client>("/clients", req);
export const UpdateClient = (id: string, req: Partial<Client>) => put<Client>(`/clients/${id}`, req);
export const DeleteClient = (id: string) =>
  del<{ deleted: boolean }>(`/clients/${id}`).then((r) => r.deleted);
export const GetClientInvoiceCount = (id: string) =>
  get<{ count: number }>(`/clients/${id}/invoice-count`).then((r) => r.count);

// ---- Invoices ----

export const GetInvoices = (organizationId: string) =>
  get<Invoice[]>(`/organizations/${organizationId}/invoices`);
export const GetInvoice = (id: string) => get<Invoice>(`/invoices/${id}`);
export const GetInvoiceLineItems = (id: string) => get<InvoiceLineItem[]>(`/invoices/${id}/line-items`);
export const CreateInvoice = (req: unknown) => post<Invoice>("/invoices", req);
export const UpdateInvoice = (id: string, req: unknown) => put<Invoice>(`/invoices/${id}`, req);
export const UpdateInvoiceState = (id: string, state: string) =>
  patch<Invoice>(`/invoices/${id}/state`, { state });
export const DeleteInvoice = (id: string) =>
  del<{ deleted: boolean }>(`/invoices/${id}`).then((r) => r.deleted);

// ---- Tax Rates ----

export const GetTaxRates = (organizationId: string) =>
  get<TaxRate[]>(`/organizations/${organizationId}/tax-rates`);
export const GetTaxRate = (id: string) => get<TaxRate>(`/tax-rates/${id}`);
export const CreateTaxRate = (req: Partial<TaxRate>) => post<TaxRate>("/tax-rates", req);
export const UpdateTaxRate = (id: string, req: Partial<TaxRate>) => put<TaxRate>(`/tax-rates/${id}`, req);
export const DeleteTaxRate = (id: string) =>
  del<{ deleted: boolean }>(`/tax-rates/${id}`).then((r) => r.deleted);
export const GetTaxRateUsageCount = (id: string) =>
  get<{ count: number }>(`/tax-rates/${id}/usage-count`).then((r) => r.count);

// ---- Products ----

export const GetProducts = (organizationId: string) =>
  get<Product[]>(`/organizations/${organizationId}/products`);
export const GetProduct = (id: string) => get<Product>(`/products/${id}`);
export const CreateProduct = (req: Partial<Product>) => post<Product>("/products", req);
export const UpdateProduct = (id: string, req: Partial<Product>) => put<Product>(`/products/${id}`, req);
export const DeleteProduct = (id: string) =>
  del<{ deleted: boolean }>(`/products/${id}`).then((r) => r.deleted);
export const GetProductStockMovements = (id: string) =>
  get<StockMovement[]>(`/products/${id}/stock-movements`);

// ---- Stock Movements ----

export const GetStockMovements = (organizationId: string) =>
  get<StockMovement[]>(`/organizations/${organizationId}/stock-movements`);
export const CreateStockMovement = (req: Partial<StockMovement>) => post<StockMovement>("/stock-movements", req);
export const DeleteStockMovement = (id: string) =>
  del<{ deleted: boolean }>(`/stock-movements/${id}`).then((r) => r.deleted);

// ---- Orders ----

export const GetOrders = (organizationId: string) =>
  get<Order[]>(`/organizations/${organizationId}/orders`);
export const GetOrder = (id: string) => get<Order>(`/orders/${id}`);
export const GetOrderLineItems = (id: string) => get<OrderLineItem[]>(`/orders/${id}/line-items`);
export const GetOrderDeliveredQuantities = (id: string) =>
  get<Record<string, number>>(`/orders/${id}/delivered-quantities`);
export const CreateOrder = (req: unknown) => post<Order>("/orders", req);
export const UpdateOrder = (id: string, req: unknown) => put<Order>(`/orders/${id}`, req);
export const UpdateOrderStatus = (id: string, status: string) =>
  patch<Order>(`/orders/${id}/status`, { status });
export const DeleteOrder = (id: string) =>
  del<{ deleted: boolean }>(`/orders/${id}`).then((r) => r.deleted);

// ---- Outbound Deliveries ----

export const GetDeliveries = (organizationId: string) =>
  get<Delivery[]>(`/organizations/${organizationId}/deliveries`);
export const GetNextDeliveryNumber = (organizationId: string) =>
  get<{ number: string }>(`/organizations/${organizationId}/deliveries/next-number`).then((r) => r.number);
export const GetDelivery = (id: string) => get<Delivery>(`/deliveries/${id}`);
export const GetDeliveryLineItems = (id: string) => get<DeliveryLineItem[]>(`/deliveries/${id}/line-items`);
export const CreateDelivery = (req: unknown) => post<Delivery>("/deliveries", req);
export const UpdateDelivery = (id: string, req: unknown) => put<Delivery>(`/deliveries/${id}`, req);
export const UpdateDeliveryStatus = (id: string, status: string) =>
  patch<Delivery>(`/deliveries/${id}/status`, { status });
export const DeleteDelivery = (id: string) =>
  del<{ success: boolean }>(`/deliveries/${id}`).then((r) => r.success);

