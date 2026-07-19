// Domain models in the API (storage) shape — monetary values in integer cents,
// dates as Unix-millisecond numbers, nullable columns as `| null` (Go pointers
// marshaled to JSON). These mirror the structs in the Go `db` package. The
// atoms convert some fields for display (cents→units, ms→Dayjs), but the types
// stay numeric so a converted list value still satisfies the same interface.

export type { Client } from "./client";
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
  logo: string | null;
  invoiceNumberFormat: string | null;
  invoiceNumberCounter: number | null;
  date_format: string | null;
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
