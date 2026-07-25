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
  taxRateId: string | null;
  stockEnabled: number;
  stockQuantity: number;
  createdAt: string | null;
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
}

export interface Organization {
  id: string;
  code: string | null;
  name: string | null;
  country: string | null;
  address: string | null;
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
  // 3-way matching tolerance policy (percent) for incoming vendor invoices.
  match_price_tolerance_percent: number | null;
  match_quantity_tolerance_percent: number | null;
  // EN 16931 (XRechnung/ZUGFeRD) seller fields. address/country above stay
  // as free-text legacy display fields; these structured fields are what
  // e-invoice export reads and validates.
  bic: string | null;
  tax_number: string | null;
  street: string | null;
  house_number: string | null;
  postal_code: string | null;
  city: string | null;
  country_code: string | null;
}

export interface Order {
  id: string;
  organizationId: string;
  clientId: string | null;
  orderNumber: string;
  status: string;
  orderDate: number;
  deliveryDate: number | null;
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
  clientId: string | null;
  clientName: string | null;
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
  createdAt: string | null;
  productName?: string | null;
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
  deliveryAddress: string | null;
  notes: string | null;
  vendorName: string | null;
  createdAt: number;
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
