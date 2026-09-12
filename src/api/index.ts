// HTTP API client — drop-in replacement for the wailsjs/go/main/App bindings.
// Function names and signatures are intentionally identical so atom files only
// need their import path changed.
import { get, post, put, patch, del, CSRF_HEADER } from "./client";
import type {
  Client,
  Vendor,
  Import,
  ImportSummary,
  Invoice,
  InvoiceLineItem,
  Product,
  TaxRate,
  PaymentTerm,
  Organization,
  PurchaseOrder,
  PurchaseOrderLineItem,
  InboundDelivery,
  InboundDeliveryLineItem,
  IncomingInvoice,
  IncomingInvoiceLineItem,
  MatchLine,
  Order,
  OrderLineItem,
  Delivery,
  DeliveryLineItem,
  StockMovement,
  SerialNumber,
  Account,
  Journal,
  FiscalYear,
  FiscalPeriod,
  JournalEntry,
  JournalLine,
  TrialBalanceRow,
  ProfitAndLoss,
  BalanceSheet,
  Payment,
  PaymentApplication,
  CreatePaymentRequest,
} from "src/types/models";

// ---- Auth ----

export interface CurrentUser {
  id: string;
  email: string;
  displayName: string;
  role: "admin" | "user";
  isPlatformAdmin: number;
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
export const CreateUser = (req: {
  email: string;
  password: string;
  displayName: string;
  role: string;
}) => post<UserRecord>("/users", req);
export const UpdateUser = (
  id: string,
  req: { displayName?: string; role?: string; isActive?: number; password?: string },
) => put<UserRecord>(`/users/${id}`, req);
export const DeleteUser = (id: string) => del<void>(`/users/${id}`);

// ---- Utility ----

export const GetVersion = () => get<{ version: string }>("/version").then((r) => r.version);

export const OpenURL = (url: string) => {
  window.open(url, "_blank", "noopener,noreferrer");
};

export const SaveFile = (defaultName: string, contents: Blob | Uint8Array) => {
  const blob =
    contents instanceof Blob
      ? contents
      : new Blob([contents as BlobPart], { type: "application/octet-stream" });
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
export const CreateOrganization = (req: Partial<Organization>) =>
  post<Organization>("/organizations", req);
export const UpdateOrganization = (id: string, req: Partial<Organization>) =>
  put<Organization>(`/organizations/${id}`, req);
export const DeleteOrganization = (id: string) =>
  del<{ deleted: boolean }>(`/organizations/${id}`).then((r) => r.deleted);
export type OrganizationUsageCount = {
  clients: number;
  vendors: number;
  invoices: number;
  products: number;
  orders: number;
  deliveries: number;
  taxRates: number;
  purchaseOrders: number;
  inboundDeliveries: number;
  incomingInvoices: number;
  stockMovements: number;
};
export const GetOrganizationUsageCount = (id: string) =>
  get<OrganizationUsageCount>(`/organizations/${id}/usage-count`);

// Wipes selected record collections for an organization without deleting the
// organization itself. Requesting master data always wipes transactional
// data too — see db/reset.go for why the two can't be separated. Returns how
// many rows of each kind were actually removed.
export const ResetOrganizationData = (
  id: string,
  req: { resetMasterData: boolean; resetTransactionalData: boolean },
) => post<OrganizationUsageCount>(`/organizations/${id}/reset`, req);

// The logo lives outside the Organization JSON (excluded server-side via
// json:"-") — GET /logo is a raw image response, not JSON, so this fetches
// it directly rather than going through the shared `get` wrapper.
export const GetOrganizationLogoDataUri = async (id: string): Promise<string | null> => {
  const res = await fetch(`/api/organizations/${id}/logo`, { credentials: "same-origin" });
  if (!res.ok) return null;
  const blob = await res.blob();
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(blob);
  });
};

export const UploadOrganizationLogo = async (id: string, file: File): Promise<void> => {
  const form = new FormData();
  form.append("file", file);
  const res = await fetch(`/api/organizations/${id}/logo`, {
    method: "POST",
    headers: { [CSRF_HEADER]: "1" },
    credentials: "same-origin",
    body: form,
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error ?? res.statusText);
  }
};

export const DeleteOrganizationLogo = (id: string) => del<void>(`/organizations/${id}/logo`);

// ---- Organization Members ----
// Per-organization membership/roles — who can access this organization and
// at what role. Managing membership is itself an org-admin action.

export interface OrganizationMember {
  id: string;
  organizationId: string;
  userId: string;
  role: "admin" | "user";
  createdAt: string;
  email: string;
  displayName: string;
  isActive: number;
}

export const GetOrganizationMembers = (organizationId: string) =>
  get<OrganizationMember[]>(`/organizations/${organizationId}/members`);

// Not org-admin gated — any authenticated user may ask their own role in an
// organization, which is how the frontend decides whether to show
// org-admin-only actions for the currently selected organization.
export const GetMyOrganizationRole = (organizationId: string) =>
  get<{ role: "admin" | "user" | ""; isMember: boolean }>(
    `/organizations/${organizationId}/my-role`,
  );

// Batch counterpart to GetMyOrganizationRole (issue #147) — every
// organization the caller belongs to, in one request, keyed by organization
// id. An organization absent from the map means "not a member," same as
// GetMyOrganizationRole's isMember: false.
export const GetMyOrganizationRoles = () =>
  get<Record<string, "admin" | "user">>("/organizations/my-roles");

// Grants an existing user account access by email — an org admin has no
// route to list every platform user account to find an id by, so email is
// what they'd actually have on hand to invite someone with.
export const AddOrganizationMember = (
  organizationId: string,
  req: { email: string; role: "admin" | "user" },
) => post<OrganizationMember>(`/organizations/${organizationId}/members`, req);

export const UpdateOrganizationMemberRole = (
  organizationId: string,
  userId: string,
  role: "admin" | "user",
) => put<{ updated: boolean }>(`/organizations/${organizationId}/members/${userId}`, { role });

export const RemoveOrganizationMember = (organizationId: string, userId: string) =>
  del<void>(`/organizations/${organizationId}/members/${userId}`);

// ---- Document Templates (issue #115) ----
// Per-org, per-document-type Excel export template overrides. GET returns
// the org's uploaded override, or the embedded default if none was
// uploaded — either way a real, downloadable .xlsx.

export interface DocumentTemplateMeta {
  id: string;
  organizationId: string;
  documentType: string;
  filename: string;
  createdAt: string;
  updatedAt: string;
}

// Lists which document types currently have a custom override (content
// excluded) — what the Settings page uses to show "Custom" vs "Default" per
// card without downloading a template just to check.
export const ListDocumentTemplates = (orgId: string) =>
  get<DocumentTemplateMeta[]>(`/organizations/${orgId}/document-templates`);

export const GetDocumentTemplate = async (orgId: string, documentType: string): Promise<Blob> => {
  const res = await fetch(`/api/organizations/${orgId}/document-templates/${documentType}`, {
    credentials: "same-origin",
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error ?? res.statusText);
  }
  return res.blob();
};

export const DownloadDocumentTemplate = async (
  orgId: string,
  documentType: string,
  defaultFilename: string,
): Promise<void> => {
  const blob = await GetDocumentTemplate(orgId, documentType);
  await SaveFile(defaultFilename, blob);
};

// Mirrors UploadOrganizationLogo's shape (raw fetch, manual CSRF header) —
// content is validated server-side by actually opening it via excelize
// before it's accepted, a stronger bar than the logo's content-type sniff
// since this file is later opened and filled, not just displayed.
export const UploadDocumentTemplate = async (
  orgId: string,
  documentType: string,
  file: File,
): Promise<void> => {
  const form = new FormData();
  form.append("file", file);
  const res = await fetch(`/api/organizations/${orgId}/document-templates/${documentType}`, {
    method: "POST",
    headers: { [CSRF_HEADER]: "1" },
    credentials: "same-origin",
    body: form,
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error ?? res.statusText);
  }
};

export const DeleteDocumentTemplate = (orgId: string, documentType: string) =>
  del<void>(`/organizations/${orgId}/document-templates/${documentType}`);

// Page orientation (landscape/portrait) override for one document type's
// export. orientation is "" when the org has never set one — the fill
// engine then leaves the template's own authored page setup untouched
// (every embedded default renders portrait with no override).
export interface DocumentTemplateOrientation {
  orientation: "" | "portrait" | "landscape";
}

export const GetDocumentTemplateOrientation = (orgId: string, documentType: string) =>
  get<DocumentTemplateOrientation>(
    `/organizations/${orgId}/document-templates/${documentType}/orientation`,
  );

export const UpdateDocumentTemplateOrientation = (
  orgId: string,
  documentType: string,
  orientation: "portrait" | "landscape",
) =>
  put<DocumentTemplateOrientation>(
    `/organizations/${orgId}/document-templates/${documentType}/orientation`,
    {
      orientation,
    },
  );

export const DeleteDocumentTemplateOrientation = (orgId: string, documentType: string) =>
  del<void>(`/organizations/${orgId}/document-templates/${documentType}/orientation`);

// ---- Clients ----

export const GetClients = (organizationId: string) =>
  get<Client[]>(`/organizations/${organizationId}/clients`);
export const GetClient = (id: string) => get<Client>(`/clients/${id}`);
export const CreateClient = (req: Partial<Client>) => post<Client>("/clients", req);
export const UpdateClient = (id: string, req: Partial<Client>) =>
  put<Client>(`/clients/${id}`, req);
export const DeleteClient = (id: string) =>
  del<{ deleted: boolean }>(`/clients/${id}`).then((r) => r.deleted);
export const GetClientInvoiceCount = (id: string) =>
  get<{ count: number }>(`/clients/${id}/invoice-count`).then((r) => r.count);

// ---- Invoices ----

export const GetInvoices = (organizationId: string) =>
  get<Invoice[]>(`/organizations/${organizationId}/invoices`);
export const GetInvoice = (id: string) => get<Invoice>(`/invoices/${id}`);
export const GetInvoiceLineItems = (id: string) =>
  get<InvoiceLineItem[]>(`/invoices/${id}/line-items`);
export const CreateInvoice = (req: unknown) => post<Invoice>("/invoices", req);
export const UpdateInvoice = (id: string, req: unknown) => put<Invoice>(`/invoices/${id}`, req);
export const UpdateInvoiceState = (id: string, state: string) =>
  patch<Invoice>(`/invoices/${id}/state`, { state });
export const DeleteInvoice = (id: string) =>
  del<{ deleted: boolean }>(`/invoices/${id}`).then((r) => r.deleted);

// Downloads the invoice as an EN 16931 UBL XML document — XRechnung for a
// German buyer, generic EN 16931 core for everyone else (see
// resolveEInvoiceProfile in db/einvoice.go).
export const DownloadInvoiceEInvoice = async (id: string): Promise<void> => {
  const res = await fetch(`/api/invoices/${id}/e-invoice`, { credentials: "same-origin" });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error ?? res.statusText);
  }
  const blob = await res.blob();
  const disposition = res.headers.get("Content-Disposition") ?? "";
  const filename = disposition.match(/filename="([^"]+)"/)?.[1] ?? `${id}-e-invoice.xml`;
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
};

// downloadDocumentExport is the shared engine behind every ExportXDocument
// function below (invoice, purchase order, …) — fetches a filled Excel/PDF
// export and triggers a browser download. fallbackFilename only applies if
// the server didn't set a Content-Disposition filename (it always does
// today; this is just a defensive default).
const downloadDocumentExport = async (url: string, fallbackFilename: string): Promise<void> => {
  const res = await fetch(url, { credentials: "same-origin" });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error ?? res.statusText);
  }
  const blob = await res.blob();
  const disposition = res.headers.get("Content-Disposition") ?? "";
  const filename = disposition.match(/filename="([^"]+)"/)?.[1] ?? fallbackFilename;
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
};

// Exports an invoice through the org's custom Excel template (an uploaded
// override, or the embedded default) filled with this invoice's persisted
// data — as opposed to the default/tunisia PDF button, which renders live
// unsaved form values client-side via @react-pdf/renderer.
export const ExportInvoiceDocument = (id: string, format: "xlsx" | "pdf") =>
  downloadDocumentExport(`/api/invoices/${id}/export?format=${format}`, `invoice-${id}.${format}`);

// Exports a purchase order the same way — see db/xlsx_export_purchase_order.go.
export const ExportPurchaseOrderDocument = (id: string, format: "xlsx" | "pdf") =>
  downloadDocumentExport(
    `/api/purchase-orders/${id}/export?format=${format}`,
    `purchase-order-${id}.${format}`,
  );

// Exports a sales order the same way — see db/xlsx_export_order.go.
export const ExportOrderDocument = (id: string, format: "xlsx" | "pdf") =>
  downloadDocumentExport(`/api/orders/${id}/export?format=${format}`, `order-${id}.${format}`);

// Exports an incoming invoice (vendor bill) the same way — see
// db/xlsx_export_incoming_invoice.go.
export const ExportIncomingInvoiceDocument = (id: string, format: "xlsx" | "pdf") =>
  downloadDocumentExport(
    `/api/incoming-invoices/${id}/export?format=${format}`,
    `incoming-invoice-${id}.${format}`,
  );

// Exports an outbound delivery (delivery note) the same way — see
// db/xlsx_export_delivery.go.
export const ExportDeliveryDocument = (id: string, format: "xlsx" | "pdf") =>
  downloadDocumentExport(
    `/api/deliveries/${id}/export?format=${format}`,
    `delivery-${id}.${format}`,
  );

// Exports an inbound delivery (goods receipt) the same way — see
// db/xlsx_export_inbound_delivery.go.
export const ExportInboundDeliveryDocument = (id: string, format: "xlsx" | "pdf") =>
  downloadDocumentExport(
    `/api/inbound-deliveries/${id}/export?format=${format}`,
    `goods-receipt-${id}.${format}`,
  );

// ---- Tax Rates ----

export const GetTaxRates = (organizationId: string) =>
  get<TaxRate[]>(`/organizations/${organizationId}/tax-rates`);
export const GetTaxRate = (id: string) => get<TaxRate>(`/tax-rates/${id}`);
export const CreateTaxRate = (req: Partial<TaxRate>) => post<TaxRate>("/tax-rates", req);
export const UpdateTaxRate = (id: string, req: Partial<TaxRate>) =>
  put<TaxRate>(`/tax-rates/${id}`, req);
export const DeleteTaxRate = (id: string) =>
  del<{ deleted: boolean }>(`/tax-rates/${id}`).then((r) => r.deleted);
export const GetTaxRateUsageCount = (id: string) =>
  get<{ count: number }>(`/tax-rates/${id}/usage-count`).then((r) => r.count);

// ---- Payment Terms ----

export const GetPaymentTerms = (organizationId: string) =>
  get<PaymentTerm[]>(`/organizations/${organizationId}/payment-terms`);
export const CreatePaymentTerm = (req: Partial<PaymentTerm>) =>
  post<PaymentTerm>("/payment-terms", req);
export const UpdatePaymentTerm = (id: string, req: Partial<PaymentTerm>) =>
  put<PaymentTerm>(`/payment-terms/${id}`, req);
export const DeletePaymentTerm = (id: string) =>
  del<{ deleted: boolean }>(`/payment-terms/${id}`).then((r) => r.deleted);

// ---- Countries ----

export const GetActiveCountries = () => get<string[]>("/countries/active");
export const SetCountryActive = (code: string, active: boolean) =>
  patch<{ code: string; active: boolean }>(`/countries/${code}`, { active });

// ---- Products ----

// The Go handler always answers with this envelope now (products can be
// paginated), so every caller — including the ones below that pass no
// params and therefore get everything, same as before — reads through it.
export interface Page<T> {
  data: T[];
  total: number;
}

export const GetProducts = (
  organizationId: string,
  params?: {
    search?: string;
    limit?: number;
    offset?: number;
    sort?: string;
    order?: "asc" | "desc";
  },
) => {
  const qs = new URLSearchParams();
  if (params?.search) qs.set("search", params.search);
  if (params?.limit) qs.set("limit", String(params.limit));
  if (params?.offset) qs.set("offset", String(params.offset));
  if (params?.sort) qs.set("sort", params.sort);
  if (params?.order) qs.set("order", params.order);
  const suffix = qs.toString() ? `?${qs}` : "";
  return get<Page<Product>>(`/organizations/${organizationId}/products${suffix}`);
};
export const GetProduct = (id: string) => get<Product>(`/products/${id}`);
export const CreateProduct = (req: Partial<Product>) => post<Product>("/products", req);
export const UpdateProduct = (id: string, req: Partial<Product>) =>
  put<Product>(`/products/${id}`, req);
export const DeleteProduct = (id: string) =>
  del<{ deleted: boolean }>(`/products/${id}`).then((r) => r.deleted);
export const GetProductStockMovements = (id: string) =>
  get<StockMovement[]>(`/products/${id}/stock-movements`);
export const GetProductSerialNumbers = (id: string) =>
  get<SerialNumber[]>(`/products/${id}/serial-numbers`);

// ---- Stock Movements ----

export const GetStockMovements = (
  organizationId: string,
  params?: {
    productId?: string;
    category?: string;
    type?: string;
    reference?: string;
    limit?: number;
    offset?: number;
    sort?: string;
    order?: "asc" | "desc";
  },
) => {
  const qs = new URLSearchParams();
  if (params?.productId) qs.set("productId", params.productId);
  if (params?.category) qs.set("category", params.category);
  if (params?.type) qs.set("type", params.type);
  if (params?.reference) qs.set("reference", params.reference);
  if (params?.limit) qs.set("limit", String(params.limit));
  if (params?.offset) qs.set("offset", String(params.offset));
  if (params?.sort) qs.set("sort", params.sort);
  if (params?.order) qs.set("order", params.order);
  const suffix = qs.toString() ? `?${qs}` : "";
  return get<Page<StockMovement>>(`/organizations/${organizationId}/stock-movements${suffix}`);
};
// serialNumbers is only meaningful for a serialized product: new/returning
// serials for type "in", existing in-stock serials to remove for type "out".
// The server returns every movement row the request produced (more than one
// for a serialized product's fan-out, exactly one otherwise) plus the
// refreshed product, since a per-row stockQuantity delta the caller could
// apply locally no longer exists once a request can post multiple rows.
export const CreateStockMovement = (req: Partial<StockMovement> & { serialNumbers?: string[] }) =>
  post<{ movements: StockMovement[]; product: Product }>("/stock-movements", req);
export const DeleteStockMovement = (id: string) =>
  del<{ deleted: boolean }>(`/stock-movements/${id}`).then((r) => r.deleted);

// ---- Dashboard ----

// "Revenue" is defined consistently the same way in every field below: sent
// or paid invoices only (mirrors db/dashboard.go's revenueStates).
export interface MonthlyRevenue {
  month: string; // "2026-01"
  revenue: number;
}

export interface OutstandingInvoiceSummary {
  id: string;
  number: string;
  clientName: string;
  dueDate: number | null;
  // currency/foreignTotal (F116, multi-currency reporting) carry the same
  // remaining balance in the invoice's own currency, unconverted — total is
  // always the functional-currency (organization.currency) equivalent.
  currency: string;
  foreignTotal: number;
  total: number;
  daysOverdue: number;
}

export interface OutstandingSummary {
  total: number;
  current: number;
  days1To30: number;
  days31To60: number;
  days61To90: number;
  days90Plus: number;
  invoices: OutstandingInvoiceSummary[]; // most overdue first
}

export interface StockValuationItem {
  productId: string;
  name: string;
  quantity: number;
  value: number;
}

export interface StockValuation {
  total: number;
  items: StockValuationItem[]; // top 10 by value
}

export interface ClientRevenue {
  clientId: string;
  name: string;
  revenue: number;
}

export interface ProductRevenue {
  productId: string;
  name: string;
  revenue: number;
}

export interface DashboardData {
  revenueByMonth: MonthlyRevenue[];
  outstanding: OutstandingSummary;
  stockValuation: StockValuation;
  topClients: ClientRevenue[];
  topProducts: ProductRevenue[];
}

export const GetDashboard = (organizationId: string, months?: number) => {
  const suffix = months ? `?months=${months}` : "";
  return get<DashboardData>(`/organizations/${organizationId}/dashboard${suffix}`);
};

// ---- Exchange rate prefill ----

export interface LastExchangeRate {
  rate: number | null;
  date: number | null;
}

// Prefill convenience only — the rate the user actually saves is always
// confirmed by them, never auto-applied (see db/exchange_rate.go).
export const GetLastExchangeRate = (organizationId: string, currency: string) =>
  get<LastExchangeRate>(
    `/organizations/${organizationId}/exchange-rate?currency=${encodeURIComponent(currency)}`,
  );

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
  get<{ number: string }>(`/organizations/${organizationId}/deliveries/next-number`).then(
    (r) => r.number,
  );
export const GetDelivery = (id: string) => get<Delivery>(`/deliveries/${id}`);
export const GetDeliveryLineItems = (id: string) =>
  get<DeliveryLineItem[]>(`/deliveries/${id}/line-items`);
export const CreateDelivery = (req: unknown) => post<Delivery>("/deliveries", req);
export const UpdateDelivery = (id: string, req: unknown) => put<Delivery>(`/deliveries/${id}`, req);
// serialNumbers, keyed by line-item id, is required for any line whose
// product is serialized when transitioning draft -> shipped.
export const UpdateDeliveryStatus = (
  id: string,
  status: string,
  serialNumbers?: Record<string, string[]>,
) => patch<Delivery>(`/deliveries/${id}/status`, { status, serialNumbers });
export const DeleteDelivery = (id: string) =>
  del<{ success: boolean }>(`/deliveries/${id}`).then((r) => r.success);

// ---- Vendors ----

export const GetVendors = (organizationId: string) =>
  get<Vendor[]>(`/organizations/${organizationId}/vendors`);
export const GetVendor = (id: string) => get<Vendor>(`/vendors/${id}`);
export const CreateVendor = (req: Partial<Vendor>) => post<Vendor>("/vendors", req);
export const UpdateVendor = (id: string, req: Partial<Vendor>) =>
  put<Vendor>(`/vendors/${id}`, req);
export const DeleteVendor = (id: string) =>
  del<{ deleted: boolean }>(`/vendors/${id}`).then((r) => r.deleted);
export const GetVendorDocumentCount = (id: string) =>
  get<{ count: number }>(`/vendors/${id}/document-count`).then((r) => r.count);

// ---- Imports (F114 — consolidated China shipments purchase orders link to) ----

export const GetImports = (organizationId: string) =>
  get<Import[]>(`/organizations/${organizationId}/imports`);
export const GetNextImportNumber = (organizationId: string) =>
  get<{ number: string }>(`/organizations/${organizationId}/imports/next-number`).then(
    (r) => r.number,
  );
export const GetImport = (id: string) => get<Import>(`/imports/${id}`);
export const GetImportSummary = (id: string) => get<ImportSummary>(`/imports/${id}/summary`);
// Batch counterpart to GetImportSummary — every import's committed value/
// landed cost rate/linked PO count for an organization in one request,
// for the Imports list page (one row per import) rather than N per-row
// GetImportSummary calls. Absent key means "no linked purchase orders" is
// NOT the convention here (unlike GetMyOrganizationRoles) — every import
// the org has gets an entry, zeroed if nothing's linked; see db.GetImportSummaries.
export const GetImportSummaries = (organizationId: string) =>
  get<Record<string, ImportSummary>>(`/organizations/${organizationId}/imports/summaries`);
export const CreateImport = (req: Partial<Import>) => post<Import>("/imports", req);
export const UpdateImport = (id: string, req: Partial<Import>) =>
  put<Import>(`/imports/${id}`, req);
export const DeleteImport = (id: string) =>
  del<{ deleted: boolean }>(`/imports/${id}`).then((r) => r.deleted);

// ---- Purchase Orders ----

export const GetPurchaseOrders = (organizationId: string) =>
  get<PurchaseOrder[]>(`/organizations/${organizationId}/purchase-orders`);
export const GetNextPurchaseOrderNumber = (organizationId: string) =>
  get<{ number: string }>(`/organizations/${organizationId}/purchase-orders/next-number`).then(
    (r) => r.number,
  );
export const GetPurchaseOrder = (id: string) => get<PurchaseOrder>(`/purchase-orders/${id}`);
export const GetPurchaseOrderLineItems = (id: string) =>
  get<PurchaseOrderLineItem[]>(`/purchase-orders/${id}/line-items`);
export const CreatePurchaseOrder = (req: unknown) => post<PurchaseOrder>("/purchase-orders", req);
export const UpdatePurchaseOrder = (id: string, req: unknown) =>
  put<PurchaseOrder>(`/purchase-orders/${id}`, req);
export const UpdatePurchaseOrderStatus = (id: string, status: string) =>
  patch<PurchaseOrder>(`/purchase-orders/${id}/status`, { status });
export const DeletePurchaseOrder = (id: string) =>
  del<{ deleted: boolean }>(`/purchase-orders/${id}`).then((r) => r.deleted);
export const GetPurchaseOrderReceivedQuantities = (id: string) =>
  get<Record<string, number>>(`/purchase-orders/${id}/received-quantities`);

// ---- Inbound Deliveries (goods receipts) ----

export const GetInboundDeliveries = (organizationId: string) =>
  get<InboundDelivery[]>(`/organizations/${organizationId}/inbound-deliveries`);
export const GetNextInboundDeliveryNumber = (organizationId: string) =>
  get<{ number: string }>(`/organizations/${organizationId}/inbound-deliveries/next-number`).then(
    (r) => r.number,
  );
export const GetInboundDelivery = (id: string) => get<InboundDelivery>(`/inbound-deliveries/${id}`);
export const GetInboundDeliveryLineItems = (id: string) =>
  get<InboundDeliveryLineItem[]>(`/inbound-deliveries/${id}/line-items`);
export const CreateInboundDelivery = (req: unknown) =>
  post<InboundDelivery>("/inbound-deliveries", req);
export const UpdateInboundDelivery = (id: string, req: unknown) =>
  put<InboundDelivery>(`/inbound-deliveries/${id}`, req);
// serialNumbers, keyed by line-item id, is required for any line whose
// product is serialized when transitioning draft -> received.
export const UpdateInboundDeliveryStatus = (
  id: string,
  status: string,
  serialNumbers?: Record<string, string[]>,
) => patch<InboundDelivery>(`/inbound-deliveries/${id}/status`, { status, serialNumbers });
export const DeleteInboundDelivery = (id: string) =>
  del<{ deleted: boolean }>(`/inbound-deliveries/${id}`).then((r) => r.deleted);

// ---- Incoming Invoices (vendor bills) ----

export const GetIncomingInvoices = (organizationId: string) =>
  get<IncomingInvoice[]>(`/organizations/${organizationId}/incoming-invoices`);
export const GetIncomingInvoice = (id: string) => get<IncomingInvoice>(`/incoming-invoices/${id}`);
export const GetIncomingInvoiceLineItems = (id: string) =>
  get<IncomingInvoiceLineItem[]>(`/incoming-invoices/${id}/line-items`);
// The 3-way match is computed server-side on every read, so it always reflects
// the current state of the linked order and its goods receipts.
export const GetIncomingInvoiceMatch = (id: string) =>
  get<MatchLine[]>(`/incoming-invoices/${id}/match`);
export const CreateIncomingInvoice = (req: unknown) =>
  post<IncomingInvoice>("/incoming-invoices", req);
export const UpdateIncomingInvoice = (id: string, req: unknown) =>
  put<IncomingInvoice>(`/incoming-invoices/${id}`, req);
export const UpdateIncomingInvoiceState = (id: string, state: string) =>
  patch<IncomingInvoice>(`/incoming-invoices/${id}/state`, { state });
export const DeleteIncomingInvoice = (id: string) =>
  del<{ deleted: boolean }>(`/incoming-invoices/${id}`).then((r) => r.deleted);

// ---- Accounting: Chart of accounts ----

export const GetAccounts = (organizationId: string) =>
  get<Account[]>(`/organizations/${organizationId}/accounts`);
export const GetAccount = (id: string) => get<Account>(`/accounts/${id}`);
export const CreateAccount = (req: Partial<Account>) => post<Account>("/accounts", req);
export const UpdateAccount = (id: string, req: Partial<Account>) =>
  put<Account>(`/accounts/${id}`, req);
export const DeleteAccount = (id: string) =>
  del<{ deleted: boolean }>(`/accounts/${id}`).then((r) => r.deleted);

// ---- Accounting: Journals ----

export const GetJournals = (organizationId: string) =>
  get<Journal[]>(`/organizations/${organizationId}/journals`);
export const CreateJournal = (req: Partial<Journal>) => post<Journal>("/journals", req);
export const UpdateJournal = (id: string, req: Partial<Journal>) =>
  put<Journal>(`/journals/${id}`, req);
export const DeleteJournal = (id: string) =>
  del<{ deleted: boolean }>(`/journals/${id}`).then((r) => r.deleted);

// ---- Accounting: Fiscal years / periods ----

export const GetFiscalYears = (organizationId: string) =>
  get<FiscalYear[]>(`/organizations/${organizationId}/fiscal-years`);
export const CreateFiscalYear = (req: Partial<FiscalYear>) =>
  post<FiscalYear>("/fiscal-years", req);
export const GetFiscalPeriods = (fiscalYearId: string) =>
  get<FiscalPeriod[]>(`/fiscal-years/${fiscalYearId}/periods`);
export const CreateFiscalPeriod = (req: Partial<FiscalPeriod>) =>
  post<FiscalPeriod>("/fiscal-periods", req);
export const UpdateFiscalPeriodStatus = (id: string, status: string) =>
  patch<FiscalPeriod>(`/fiscal-periods/${id}/status`, { status });
// Irreversible — there is no reopen endpoint (db.CloseFiscalYear).
export const CloseFiscalYear = (id: string) => post<FiscalYear>(`/fiscal-years/${id}/close`, {});

// ---- Accounting: Journal entries ----

export const GetJournalEntries = (
  organizationId: string,
  filters?: { journalId?: string; status?: string },
) => {
  const params = new URLSearchParams();
  if (filters?.journalId) params.set("journalId", filters.journalId);
  if (filters?.status) params.set("status", filters.status);
  const qs = params.toString();
  return get<JournalEntry[]>(
    `/organizations/${organizationId}/journal-entries${qs ? `?${qs}` : ""}`,
  );
};
export const GetJournalEntry = (id: string) => get<JournalEntry>(`/journal-entries/${id}`);
export const GetJournalEntryLines = (id: string) =>
  get<JournalLine[]>(`/journal-entries/${id}/lines`);
export const CreateJournalEntry = (req: unknown) => post<JournalEntry>("/journal-entries", req);
export const PostJournalEntry = (id: string) =>
  patch<JournalEntry>(`/journal-entries/${id}/post`, {});
export const ReverseJournalEntry = (id: string, reason: string, date: number) =>
  post<JournalEntry>(`/journal-entries/${id}/reverse`, { reason, date });
export const DeleteJournalEntry = (id: string) =>
  del<{ deleted: boolean }>(`/journal-entries/${id}`).then((r) => r.deleted);

// ---- Accounting: Payments ----

export const GetPayments = (organizationId: string) =>
  get<Payment[]>(`/organizations/${organizationId}/payments`);
export const GetPayment = (id: string) => get<Payment>(`/payments/${id}`);
export const GetPaymentApplications = (id: string) =>
  get<PaymentApplication[]>(`/payments/${id}/applications`);
export const CreatePayment = (req: CreatePaymentRequest) => post<Payment>("/payments", req);
export const VoidPayment = (id: string) => post<Payment>(`/payments/${id}/void`, {});
export const GetInvoicePayments = (id: string) =>
  get<PaymentApplication[]>(`/invoices/${id}/payments`);
export const GetIncomingInvoicePayments = (id: string) =>
  get<PaymentApplication[]>(`/incoming-invoices/${id}/payments`);

// ---- Accounting: Reports ----

export const GetTrialBalance = (
  organizationId: string,
  fiscalYearId?: string,
  fiscalPeriodId?: string,
) => {
  const qs = new URLSearchParams();
  if (fiscalYearId) qs.set("fiscalYearId", fiscalYearId);
  if (fiscalPeriodId) qs.set("fiscalPeriodId", fiscalPeriodId);
  const suffix = qs.toString() ? `?${qs}` : "";
  return get<TrialBalanceRow[]>(`/organizations/${organizationId}/reports/trial-balance${suffix}`);
};

export const GetProfitAndLoss = (organizationId: string, startDate: number, endDate: number) =>
  get<ProfitAndLoss>(
    `/organizations/${organizationId}/reports/profit-and-loss?startDate=${startDate}&endDate=${endDate}`,
  );

export const GetBalanceSheet = (organizationId: string, asOfDate: number) =>
  get<BalanceSheet>(`/organizations/${organizationId}/reports/balance-sheet?asOfDate=${asOfDate}`);

export const GetReceivableAging = (organizationId: string) =>
  get<OutstandingSummary>(`/organizations/${organizationId}/reports/ar-aging`);

export interface OutstandingBillSummary {
  id: string;
  number: string;
  vendorName: string;
  dueDate: number | null;
  // See OutstandingInvoiceSummary above for what currency/foreignTotal carry.
  currency: string;
  foreignTotal: number;
  total: number;
  daysOverdue: number;
}

export interface PayableAgingSummary {
  total: number;
  current: number;
  days1To30: number;
  days31To60: number;
  days61To90: number;
  days90Plus: number;
  bills: OutstandingBillSummary[];
}

export const GetPayableAging = (organizationId: string) =>
  get<PayableAgingSummary>(`/organizations/${organizationId}/reports/ap-aging`);

export interface InventoryValuationLine {
  productId: string;
  name: string;
  quantity: number;
  value: number;
}

export interface InventoryValuation {
  glBalance: number;
  computedValue: number;
  difference: number;
  products: InventoryValuationLine[];
}

export const GetInventoryValuation = (organizationId: string) =>
  get<InventoryValuation>(`/organizations/${organizationId}/reports/inventory-valuation`);

// ---- Accounting: GL Export ----

// Shared by DownloadFEC/DownloadDATEV — fetches a gl-export sub-route as a
// blob and triggers a browser download using the filename the server chose
// (SirenFECAAAAMMJJ.txt / EXTF_Buchungsstapel_<year>.csv).
const downloadGLExport = async (url: string, fallbackFilename: string): Promise<void> => {
  const res = await fetch(url, { credentials: "same-origin" });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error ?? res.statusText);
  }
  const blob = await res.blob();
  const disposition = res.headers.get("Content-Disposition") ?? "";
  const filename = disposition.match(/filename="([^"]+)"/)?.[1] ?? fallbackFilename;
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
};

// Downloads a fiscal year's France FEC flat file (SirenFECAAAAMMJJ.txt —
// see db/export_fec.go).
export const DownloadFEC = (organizationId: string, fiscalYearId: string): Promise<void> =>
  downloadGLExport(
    `/api/organizations/${organizationId}/gl-export/fec?fiscalYearId=${encodeURIComponent(fiscalYearId)}`,
    "FEC.txt",
  );

// Downloads a fiscal year's DATEV Buchungsstapel EXTF file (Windows-1252,
// semicolon-separated — see db/export_datev.go).
export const DownloadDATEV = (organizationId: string, fiscalYearId: string): Promise<void> =>
  downloadGLExport(
    `/api/organizations/${organizationId}/gl-export/datev?fiscalYearId=${encodeURIComponent(fiscalYearId)}`,
    "Buchungsstapel.csv",
  );

// ---- Reporting ----
//
// Document-derived sales/purchasing analytics — a distinct tier from
// Accounting's GL-derived reports above (see db/sales_reports.go). Reuses
// MonthlyRevenue/ClientRevenue/ProductRevenue from the Dashboard section:
// these functions and the dashboard widget now share the same underlying
// db query, just with a real date range and no top-10 cap.

export const GetRevenueTrend = (organizationId: string, startDate: number, endDate: number) =>
  get<MonthlyRevenue[]>(
    `/organizations/${organizationId}/reporting/revenue-trend?startDate=${startDate}&endDate=${endDate}`,
  );

export const GetSalesByClient = (organizationId: string, startDate: number, endDate: number) =>
  get<ClientRevenue[]>(
    `/organizations/${organizationId}/reporting/sales-by-client?startDate=${startDate}&endDate=${endDate}`,
  );

export const GetSalesByProduct = (organizationId: string, startDate: number, endDate: number) =>
  get<ProductRevenue[]>(
    `/organizations/${organizationId}/reporting/sales-by-product?startDate=${startDate}&endDate=${endDate}`,
  );

export interface VendorSpend {
  vendorId: string;
  name: string;
  spend: number;
}

export const GetPurchasesByVendor = (organizationId: string, startDate: number, endDate: number) =>
  get<VendorSpend[]>(
    `/organizations/${organizationId}/reporting/purchases-by-vendor?startDate=${startDate}&endDate=${endDate}`,
  );

// taxRateId "" (and name/categoryCode "") is the "Unrated" bucket — line
// items with no tax rate set.
export interface TaxSummaryLine {
  taxRateId: string;
  name: string;
  categoryCode: string;
  percentage: number;
  base: number;
  tax: number;
}

export interface TaxSummary {
  output: TaxSummaryLine[]; // sales / output VAT
  input: TaxSummaryLine[]; // purchases / input VAT
}

export const GetTaxSummary = (organizationId: string, startDate: number, endDate: number) =>
  get<TaxSummary>(
    `/organizations/${organizationId}/reporting/tax-summary?startDate=${startDate}&endDate=${endDate}`,
  );
