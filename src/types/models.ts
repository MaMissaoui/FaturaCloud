// Domain models in the API (storage) shape — monetary values in integer cents,
// dates as Unix-millisecond numbers, nullable columns as `| null` (Go pointers
// marshaled to JSON). These mirror the structs in the Go `db` package. The
// atoms convert some fields for display (cents→units, ms→Dayjs), but the types
// stay numeric so a converted list value still satisfies the same interface.

export type { Client } from "./client";
export type { Vendor } from "./vendor";
export type { PurchaseOrderStatus } from "./purchase-order";
export type { InboundDeliveryStatus } from "./inbound-delivery";
export type { IncomingInvoiceState, MatchLine, MatchStatus } from "./incoming-invoice";
export type { Invoice, InvoiceLineItem, InvoiceState } from "./invoice";

export interface Product {
  id: string;
  organizationId: string;
  name: string;
  description: string | null;
  sku: string | null;
  price: number;
  unitCost: number | null;
  unit: string | null;
  type: "product" | "service";
  // Distinguishes a purchasable component/intermediate from a sellable
  // finished good — orthogonal to type (the server clears it whenever type
  // isn't "product"). null means "unclassified" — product pickers treat
  // that as eligible on both the purchasing and sales side.
  category: "finished" | "component" | null;
  taxRateId: string | null;
  stockEnabled: number;
  stockQuantity: number;
  // Individually-tracked units (see SerialNumber) rather than a fungible
  // quantity. Only meaningful when stockEnabled is set; the server blocks
  // toggling this while stockQuantity is non-zero, in either direction.
  serialized: number;
  createdAt: string | null;
}

export interface BillOfMaterialsLine {
  id: string;
  organizationId: string;
  finishedProductId: string;
  componentProductId: string;
  quantityPerUnit: number;
  createdAt: string | null;
  // Joined for display — resolved from componentProductId server-side.
  componentName: string;
  componentSku: string | null;
  componentUnit: string | null;
}

export interface SerialNumber {
  id: string;
  organizationId: string;
  productId: string;
  serialNumber: string;
  createdAt: string | null;
  // 0/1, computed server-side from the sign of the unit's most recent
  // linked stock movement — not a value you set directly.
  inStock: number;
}

export interface TaxRate {
  id: string;
  organizationId: string;
  name: string;
  description: string | null;
  percentage: number;
  isDefault: number | null;
  createdAt?: string | null;
  // BT-118 VAT category code (UNTDID 5305: S/Z/E/AE/...), defaults to "S".
  // BT-120 exemption reason, required by EN 16931 when the category needs one.
  category_code: string;
  exemption_reason: string | null;
  // Accounting (general ledger): same rate row serves both sales (a
  // liability) and purchase (a reclaimable asset) line items, so two
  // separate FKs rather than one.
  outputTaxAccountId: string | null;
  inputTaxAccountId: string | null;
  // DATEV's own tax key (BU-Schlüssel), independent of `percentage`.
  datev_bu_key: string | null;
}

// A maintained, per-organization list of selectable labels (e.g. "Net 30")
// for the invoice form's Payment terms field. No FK from invoices —
// invoices.paymentTerms stays a plain string, so deleting a term here never
// touches an invoice that already stored its name.
export interface PaymentTerm {
  id: string;
  organizationId: string;
  name: string;
  isDefault: number | null;
  createdAt: string;
}

export interface Organization {
  id: string;
  code: string | null;
  name: string | null;
  country: string | null;
  email: string | null;
  phone: string | null;
  website: string | null;
  registration_number: string | null;
  vatin: string | null;
  bank_name: string | null;
  iban: string | null;
  currency: string | null;
  minimum_fraction_digits: number | null;
  due_days: number | null;
  overdueCharge: number | null;
  customerNotes: string | null;
  createdAt: string | null;
  // Not part of the server JSON — the logo BLOB is excluded from Organization
  // responses (json:"-" in Go; fetch it via GET /organizations/{id}/logo
  // instead). organizationAtom populates this with a data URI fetched from
  // that endpoint so PDF templates and the settings page can keep reading
  // organization.logo as a ready-to-use image source.
  logo: string | null;
  invoiceNumberFormat: string | null;
  invoiceNumberCounter: number | null;
  date_format: string | null;
  // Theme support Phase 1: an accent color chosen from the curated swatch
  // set in src/components/organizations/brand-color-picker.tsx (which must
  // stay in sync with brandColorPalette in db/organization.go, the
  // server-side source of truth). Null or "" both mean "use antd's default
  // blue" — see app.tsx's ConfigProvider wiring.
  brandColor: string | null;
  // 3-way matching tolerance policy (percent) for incoming vendor invoices.
  match_price_tolerance_percent: number | null;
  match_quantity_tolerance_percent: number | null;
  // EN 16931 (XRechnung/ZUGFeRD) seller fields. Also the single source of
  // truth for address display everywhere (forms, PDFs) — there is no
  // separate free-text address field. `country` above stays a free-text
  // display name, distinct from the ISO `country_code` below.
  bic: string | null;
  tax_number: string | null;
  street: string | null;
  house_number: string | null;
  postal_code: string | null;
  city: string | null;
  country_code: string | null;
  // Accounting (general ledger) defaults — nullable so an org can use
  // manual journal entries before wiring auto-posting; seeded automatically
  // at org creation against the default chart of accounts.
  defaultArAccountId: string | null;
  defaultApAccountId: string | null;
  defaultRevenueAccountId: string | null;
  defaultExpenseAccountId: string | null;
  defaultCashAccountId: string | null;
  fxGainAccountId: string | null;
  fxLossAccountId: string | null;
  retainedEarningsAccountId: string | null;
  // Inventory/COGS GL integration (Phase 7) — same nullable-until-wired
  // convention as the accounts above.
  defaultInventoryAccountId: string | null;
  defaultGRNIAccountId: string | null;
  defaultCOGSAccountId: string | null;
  defaultInventoryAdjustmentAccountId: string | null;
  // F114 (China imports landed cost) — credited for the freight/customs
  // allocated to a receipt whose PO belongs to an import; separate from
  // defaultGRNIAccountId, which stays valued at vendor goods price only.
  defaultImportCostsPayableAccountId: string | null;
  // DATEV export. datevClearingAccountId is the synthetic Gegenkonto for a
  // manual entry with more than one line on both sides (no natural anchor).
  datevClearingAccountId: string | null;
  datev_consultant_number: string | null;
  datev_client_number: string | null;
  // Invoice feature toggles (fiscal stamp / withholding tax, first added for
  // Tunisia invoice support) and the invoice PDF layout — three independent
  // settings. fiscalStampEnabled/withholdingTaxEnabled gate whether the
  // invoice form shows those fields at all (any organization, any layout);
  // defaultFiscalStampAmount prefills a new invoice's fiscalStampAmount
  // (cents); defaultStampDutyAccountId is the liability account the stamp
  // posts to (see db/gl_posting.go's resolveStampDutyAccount).
  // invoiceLayout selects which PDF template this organization's invoices
  // render with — null/"" is the original single-layout template; see
  // src/components/invoices/layouts.ts for the registry of other values.
  fiscalStampEnabled: number | null;
  withholdingTaxEnabled: number | null;
  defaultFiscalStampAmount: number | null;
  defaultStampDutyAccountId: string | null;
  invoiceLayout: string | null;
}

export interface Order {
  id: string;
  organizationId: string;
  clientId: string | null;
  orderNumber: string;
  status: string;
  orderDate: number;
  deliveryDate: number | null;
  currency: string | null;
  // 1 unit of `currency` = exchangeRate units of the organization's own
  // currency, frozen at save time. Nil when currency equals the org's.
  exchangeRate: number | null;
  exchangeRateDate: number | null;
  shippingAddress: string | null;
  trackingNumber: string | null;
  notes: string | null;
  clientName: string | null;
  createdAt: string;
}

export interface OrderLineItem {
  id: string;
  orderId: string;
  productId: string | null;
  description: string;
  quantity: number;
  unitPrice: number;
  position: number;
}

export interface Delivery {
  id: string;
  organizationId: string;
  orderId: string | null;
  deliveryNumber: string;
  deliveryDate: number;
  shippingAddress: string | null;
  trackingNumber: string | null;
  notes: string | null;
  status: string;
  createdAt: number;
  orderNumber: string | null;
  // clientId is the *effective* client — the linked order's client when
  // orderId is set, else ownClientId below. ownClientId is only ever
  // settable (and only ever meaningful) when there's no order — see the
  // client picker in deliveries/details.tsx.
  clientId: string | null;
  clientName: string | null;
  ownClientId: string | null;
}

export interface DeliveryLineItem {
  id: string;
  deliveryId: string;
  orderLineItemId: string | null;
  productId: string | null;
  description: string;
  quantity: number;
  unit: string | null;
  position: number;
  stockEnabled: number | null;
  availableStock: number | null;
  serialized: number | null;
}

export interface StockMovement {
  id: string;
  organizationId: string;
  productId: string;
  type: string;
  quantity: number;
  unitCost: number | null;
  note: string | null;
  reference: string | null;
  // Set only for a serialized product, where this row represents exactly
  // one physical unit (quantity is always ±1).
  serialNumberId: string | null;
  sourceDocumentId: string | null;
  createdAt: string | null;
  productName?: string | null;
  // Joined display value of serialNumberId, for the Inventory ledger.
  serialNumberValue?: string | null;
}

export interface PurchaseOrder {
  id: string;
  organizationId: string;
  vendorId: string | null;
  orderNumber: string;
  status: string;
  orderDate: number;
  expectedDate: number | null;
  currency: string | null;
  // 1 unit of `currency` = exchangeRate units of the organization's own
  // currency, frozen at save time. Nil when currency equals the org's.
  exchangeRate: number | null;
  exchangeRateDate: number | null;
  deliveryAddress: string | null;
  notes: string | null;
  // The shipment this order's goods travel in, if any — drives landed cost
  // (freight/customs) allocation at receiving time.
  importId: string | null;
  vendorName: string | null;
  importNumber: string | null;
  createdAt: number;
}

export interface Import {
  id: string;
  organizationId: string;
  importNumber: string;
  date: number;
  // Prefill default for purchase orders linked to this import — never
  // converted or posted itself. Each linked PO still stores/freezes its own
  // currency/exchangeRate (see PurchaseOrder above).
  currency: string | null;
  exchangeRate: number | null;
  exchangeRateDate: number | null;
  // Both cents, in the organization's own functional currency.
  freightCost: number;
  customsCost: number;
  notes: string | null;
  createdAt: number;
  // The serial-number range this import reserves for whatever gets
  // produced from its components — all null unless this import is used
  // for production. See CLAUDE.md's db/import.go note.
  serialNumberPrefix: string | null;
  serialNumberRangeStart: number | null;
  serialNumberRangeEnd: number | null;
}

export interface ImportSummary {
  totalCommittedValue: number; // Σ non-cancelled linked POs, org-currency cents
  freightCost: number;
  customsCost: number;
  landedCostRate: number; // (freight+customs) / totalCommittedValue, 0 if undistributable
  purchaseOrderCount: number;
}

export interface PurchaseOrderLineItem {
  id: string;
  purchaseOrderId: string;
  productId: string | null;
  description: string;
  quantity: number;
  unitPrice: number;
  unit: string | null;
  taxRate: string | null;
  position: number;
}

export interface InboundDelivery {
  id: string;
  organizationId: string;
  purchaseOrderId: string | null;
  vendorId: string | null;
  deliveryNumber: string;
  deliveryDate: number;
  vendorDeliveryNote: string | null;
  trackingNumber: string | null;
  notes: string | null;
  status: string;
  createdAt: number;
  // Nullable — null means "the organization's own currency". Drives how
  // unitCost on each line converts into stockMovements at receipt time.
  currency: string | null;
  exchangeRate: number | null;
  exchangeRateDate: number | null;
  orderNumber: string | null;
  vendorName: string | null;
}

export interface InboundDeliveryLineItem {
  id: string;
  deliveryId: string;
  purchaseOrderLineItemId: string | null;
  productId: string | null;
  description: string;
  quantity: number;
  unitCost: number | null;
  unit: string | null;
  position: number;
  stockEnabled: number | null;
  currentStock: number | null;
  productName: string | null;
  serialized: number | null;
}

export interface IncomingInvoice {
  id: string;
  organizationId: string;
  vendorId: string;
  purchaseOrderId: string | null;
  vendorInvoiceNumber: string;
  reference: string | null;
  state: string;
  date: number;
  dueDate: number | null;
  currency: string;
  exchangeRate: number | null;
  exchangeRateDate: number | null;
  notes: string | null;
  total: number;
  taxTotal: number;
  subTotal: number;
  matchOverride: number;
  matchOverrideReason: string | null;
  createdAt: number;
  vendorName: string | null;
  orderNumber: string | null;
}

export interface IncomingInvoiceLineItem {
  id: string;
  incomingInvoiceId: string;
  purchaseOrderLineItemId: string | null;
  productId: string | null;
  description: string;
  quantity: number;
  unitPrice: number;
  taxRate: string | null;
  position: number;
}

// ---- Accounting (general ledger) ----

export interface Account {
  id: string;
  organizationId: string;
  parentId: string | null;
  code: string;
  name: string;
  type: "asset" | "liability" | "equity" | "revenue" | "expense";
  isGroup: number;
  isActive: number;
  datevAccountNumber: string | null;
  description: string | null;
  createdAt: number;
}

export interface Journal {
  id: string;
  organizationId: string;
  code: string;
  name: string;
  type: "sales" | "purchases" | "cash" | "bank" | "miscellaneous";
  isSystem: number;
  createdAt: number;
}

export interface FiscalYear {
  id: string;
  organizationId: string;
  name: string;
  startDate: number;
  endDate: number;
  status: "open" | "closed";
  lockDate: number | null;
  closedAt: number | null;
  createdAt: number;
}

export interface FiscalPeriod {
  id: string;
  organizationId: string;
  fiscalYearId: string;
  name: string;
  startDate: number;
  endDate: number;
  status: "open" | "closed";
  closedAt: number | null;
  createdAt: number;
}

export interface JournalLine {
  id: string;
  journalEntryId: string;
  accountId: string;
  description: string | null;
  debit: number;
  credit: number;
  currency: string | null;
  foreignAmount: number | null;
  exchangeRate: string | null;
  clientId: string | null;
  vendorId: string | null;
  taxRateId: string | null;
  reconciliationGroupId: string | null;
  position: number;
  createdAt: number;
}

export interface JournalEntry {
  id: string;
  organizationId: string;
  journalId: string;
  fiscalYearId: string;
  fiscalPeriodId: string | null;
  entryNumber: number | null;
  date: number;
  reference: string | null;
  description: string;
  sourceDocumentType: string | null;
  sourceDocumentId: string | null;
  status: "draft" | "posted" | "reversed";
  reversalOfEntryId: string | null;
  reversalReason: string | null;
  postedAt: number | null;
  createdBy: string | null;
  createdAt: number;
}

export interface TrialBalanceRow {
  accountId: string;
  code: string;
  name: string;
  type: string;
  debit: number;
  credit: number;
}

export interface ProfitAndLossLine {
  accountId: string;
  code: string;
  name: string;
  amount: number;
}

export interface ProfitAndLoss {
  revenue: ProfitAndLossLine[];
  totalRevenue: number;
  expenses: ProfitAndLossLine[];
  totalExpenses: number;
  netIncome: number;
}

export interface BalanceSheetLine {
  accountId: string;
  code: string;
  name: string;
  amount: number;
}

export interface BalanceSheet {
  assets: BalanceSheetLine[];
  totalAssets: number;
  liabilities: BalanceSheetLine[];
  totalLiabilities: number;
  equity: BalanceSheetLine[];
  currentEarnings: number;
  totalEquity: number;
}

export interface Payment {
  id: string;
  organizationId: string;
  direction: "inbound" | "outbound";
  clientId: string | null;
  vendorId: string | null;
  bankAccountId: string;
  amount: number;
  currency: string;
  exchangeRate: string | null;
  exchangeRateDate: number | null;
  date: number;
  method: "bank_transfer" | "cash" | "card" | "direct_debit" | "check" | "other";
  reference: string | null;
  notes: string | null;
  status: "posted" | "voided";
  journalEntryId: string | null;
  voidingEntryId: string | null;
  createdAt: number;
}

export interface PaymentApplication {
  id: string;
  paymentId: string;
  documentType: "invoice" | "incoming_invoice";
  documentId: string;
  amount: number;
  createdAt: number;
}

export interface CreatePaymentApplicationRequest {
  documentType: "invoice" | "incoming_invoice";
  documentId: string;
  amount: number;
}

export interface CreatePaymentRequest {
  organizationId: string;
  direction: "inbound" | "outbound";
  clientId?: string | null;
  vendorId?: string | null;
  bankAccountId: string;
  amount: number;
  currency: string;
  exchangeRate?: number | null;
  exchangeRateDate?: number | null;
  date: number;
  method: "bank_transfer" | "cash" | "card" | "direct_debit" | "check" | "other";
  reference?: string | null;
  notes?: string | null;
  applications: CreatePaymentApplicationRequest[];
}
