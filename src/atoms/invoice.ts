import { atom } from "jotai";
import type { Dayjs } from "dayjs";
import type { Invoice, InvoiceDisplay, InvoiceLineItem } from "src/types/invoice";
import { message } from "src/utils/message";
import { nanoid } from "nanoid";
import { t } from "@lingui/core/macro";
import dayjs from "dayjs";
import isEqual from "lodash/isEqual";
import omit from "lodash/omit";
import orderBy from "lodash/orderBy";
import keyBy from "lodash/keyBy";
import map from "lodash/map";
import reject from "lodash/reject";
import {
  GetInvoices,
  GetInvoice,
  GetInvoiceLineItems,
  CreateInvoice,
  UpdateInvoice,
  UpdateInvoiceState,
  DeleteInvoice,
} from "src/api";

import { centsToUnits, unitsToCents, multiplyDecimal } from "src/utils/currency";
import { organizationIdAtom, nextInvoiceNumberAtom } from "./organization";

function invoiceToDisplay(invoice: Invoice): InvoiceDisplay {
  return {
    ...invoice,
    total: centsToUnits(invoice.total),
    taxTotal: centsToUnits(invoice.taxTotal),
    subTotal: centsToUnits(invoice.subTotal),
    fiscalStampAmount: centsToUnits(invoice.fiscalStampAmount || 0),
    withholdingTaxAmount:
      invoice.withholdingTaxAmount != null ? centsToUnits(invoice.withholdingTaxAmount) : null,
  };
}

// Invoices — display shape (money in currency units, not cents); see
// InvoiceDisplay in src/types/invoice.ts.
export const invoicesAtom = atom<InvoiceDisplay[]>([]);
export const setInvoicesAtom = atom(null, async (get, set) => {
  const organizationId = get(organizationIdAtom);
  try {
    const response = await GetInvoices(organizationId!);
    // Convert cents to units for display
    set(invoicesAtom, response.map(invoiceToDisplay));
  } catch (error) {
    console.error("Failed to fetch invoices:", error);
    message.error(t`Failed to fetch invoices`);
    set(invoicesAtom, []);
  }
});

// The invoice form edits dates as dayjs objects and money (including each
// line item's unitPrice/total) in display currency units — not the wire
// shape either field is stored as.
type InvoiceLineItemFormValues = Omit<Partial<InvoiceLineItem>, "unitPrice"> & {
  unitPrice?: number;
  // Display-only computed total (quantity * unitPrice); stripped before save.
  total?: number;
};
type InvoiceFormValues = Omit<
  Partial<Invoice>,
  | "date"
  | "dueDate"
  | "exchangeRateDate"
  | "total"
  | "taxTotal"
  | "subTotal"
  | "fiscalStampAmount"
  | "withholdingTaxAmount"
> & {
  date?: Dayjs | number;
  dueDate?: Dayjs | number | null;
  exchangeRateDate?: Dayjs | number | null;
  total?: number;
  taxTotal?: number;
  subTotal?: number;
  fiscalStampAmount?: number;
  withholdingTaxAmount?: number | null;
  lineItems?: InvoiceLineItemFormValues[];
};

const toTimestamp = (v: Dayjs | number | null | undefined) =>
  v && typeof v === "object" && "valueOf" in v ? v.valueOf() : v;

// Invoice
export const invoiceIdAtom = atom<string | null>(null);
export const invoiceAtom = atom(
  async (get) => {
    const invoiceId = get(invoiceIdAtom);
    if (!invoiceId) return null;

    try {
      const [invoice, lineItems] = await Promise.all([
        GetInvoice(invoiceId),
        GetInvoiceLineItems(invoiceId),
      ]);

      if (!invoice) return null;

      return {
        ...invoice,
        date: dayjs(invoice.date),
        dueDate: invoice.dueDate ? dayjs(invoice.dueDate) : null,
        exchangeRateDate: invoice.exchangeRateDate ? dayjs(invoice.exchangeRateDate) : null,
        // Convert cents to currency units for display
        total: centsToUnits(invoice.total),
        taxTotal: centsToUnits(invoice.taxTotal),
        subTotal: centsToUnits(invoice.subTotal),
        fiscalStampAmount: centsToUnits(invoice.fiscalStampAmount || 0),
        withholdingTaxAmount:
          invoice.withholdingTaxAmount != null ? centsToUnits(invoice.withholdingTaxAmount) : null,
        lineItems: (lineItems || []).map((item) => ({
          ...item,
          unitPrice: centsToUnits(item.unitPrice),
          total: centsToUnits(multiplyDecimal(item.quantity, item.unitPrice)),
        })),
      };
    } catch (error) {
      console.error("Failed to fetch invoice:", error);
      message.error(t`Failed to fetch invoice`);
      return null;
    }
  },
  async (get, set, newValues: InvoiceFormValues) => {
    const invoiceId = get(invoiceIdAtom);

    const invoice = omit(newValues, "lineItems");
    const lineItems = newValues.lineItems || [];

    try {
      if (!invoiceId) {
        // Insert
        const invoiceData = {
          ...invoice,
          id: nanoid(),
          organizationId: get(organizationIdAtom)!,
          state: invoice.state || "draft", // Default to draft if not specified
          // Convert dayjs objects to unix timestamps
          date: toTimestamp(invoice.date),
          dueDate: toTimestamp(invoice.dueDate),
          exchangeRateDate: toTimestamp(invoice.exchangeRateDate),
          // Convert currency units to cents for storage
          total: unitsToCents(invoice.total ?? 0),
          taxTotal: unitsToCents(invoice.taxTotal ?? 0),
          subTotal: unitsToCents(invoice.subTotal ?? 0),
          fiscalStampAmount: unitsToCents(invoice.fiscalStampAmount || 0),
          withholdingTaxRate: invoice.withholdingTaxRate ?? null,
          withholdingTaxAmount:
            invoice.withholdingTaxAmount != null
              ? unitsToCents(invoice.withholdingTaxAmount)
              : null,
          overdueCharge: invoice.overdueCharge,
          lineItems: lineItems.map((item) => ({
            ...omit(item, ["id", "total"]),
            unitPrice: unitsToCents(item.unitPrice ?? 0),
          })),
        };

        const createdInvoice = await CreateInvoice(invoiceData);

        set(invoiceIdAtom, createdInvoice.id);
        message.success(t`Invoice created`);

        // Update the invoices list
        const invoices = get(invoicesAtom);
        set(invoicesAtom, [invoiceToDisplay(createdInvoice), ...invoices]);

        // Force refresh organization data to get updated invoice counter
        const currentOrgId = get(organizationIdAtom);
        if (currentOrgId) {
          set(organizationIdAtom, null);
          set(organizationIdAtom, currentOrgId);
        }
      } else {
        // Update
        const updateData = {
          // State is not editable via PUT — it changes only through the PATCH
          // state endpoint (server rejects it on PUT), so drop it here.
          ...omit(invoice, "state"),
          // Convert dayjs objects to unix timestamps
          date: toTimestamp(invoice.date),
          dueDate: toTimestamp(invoice.dueDate),
          exchangeRateDate: toTimestamp(invoice.exchangeRateDate),
          // Convert currency units to cents for storage
          total: invoice.total != null ? unitsToCents(invoice.total) : undefined,
          taxTotal: invoice.taxTotal != null ? unitsToCents(invoice.taxTotal) : undefined,
          subTotal: invoice.subTotal != null ? unitsToCents(invoice.subTotal) : undefined,
          fiscalStampAmount:
            invoice.fiscalStampAmount != null ? unitsToCents(invoice.fiscalStampAmount) : undefined,
          withholdingTaxRate: invoice.withholdingTaxRate ?? null,
          withholdingTaxAmount:
            invoice.withholdingTaxAmount != null
              ? unitsToCents(invoice.withholdingTaxAmount)
              : null,
          overdueCharge: invoice.overdueCharge,
          lineItems: lineItems
            ? lineItems.map((item) => ({
                ...omit(item, ["id", "total"]),
                unitPrice: unitsToCents(item.unitPrice ?? 0),
              }))
            : undefined,
        };

        const updatedInvoice = await UpdateInvoice(invoiceId, updateData);

        message.success(t`Invoice updated successfully`);

        // Update the invoices list
        const invoices = get(invoicesAtom);
        const mergedInvoices = keyBy([...invoices, invoiceToDisplay(updatedInvoice)], "id");
        set(invoicesAtom, orderBy(map(mergedInvoices), "date", "desc"));
      }
    } catch (error) {
      console.error("Invoice operation failed:", error);
      if (!invoiceId) {
        message.error(t`Invoice creation failed`);
      } else {
        message.error(t`Invoice update failed`);
      }
      // Rethrow (F56/F67 convention — see CLAUDE.md's src/atoms/product.ts
      // entry, and every other document-type atom) so the details page's
      // handleSubmit can tell a failed save apart from a successful one and
      // skip clearing its isDirty flag — otherwise a failed save would
      // silently re-enable Excel/PDF export over stale server-persisted data.
      throw error;
    }
  },
);

// Delete invoice
export const deleteInvoiceAtom = atom(null, async (get, set, invoiceId: string) => {
  try {
    const success = await DeleteInvoice(invoiceId);

    if (success) {
      // Remove invoice from the list
      const invoices = reject(get(invoicesAtom), (obj) => isEqual(obj.id, invoiceId));
      set(invoicesAtom, invoices);
      message.success(t`Invoice deleted`);
    } else {
      message.error(t`Invoice deletion failed`);
    }
    return success;
  } catch (error) {
    console.error("Failed to delete invoice:", error);
    message.error(error instanceof Error ? error.message : t`Invoice deletion failed`);
    return false;
  }
});

// Update invoice state only
export const updateInvoiceStateAtom = atom(
  null,
  async (get, set, { invoiceId, state }: { invoiceId: string; state: string }) => {
    try {
      const updatedInvoice = await UpdateInvoiceState(invoiceId, state);

      message.success(t`Invoice state updated`);

      // Update the invoices list
      const invoices = get(invoicesAtom);
      const mergedInvoices = keyBy([...invoices, invoiceToDisplay(updatedInvoice)], "id");
      set(invoicesAtom, orderBy(map(mergedInvoices), "date", "desc"));
    } catch (error) {
      // Surface the server's message (e.g. a posted GL entry blocked by an
      // open payment, or a 3-way match variance) rather than a generic one.
      console.error("Failed to update invoice state:", error);
      message.error(error instanceof Error ? error.message : t`Failed to update invoice state`);
    }
  },
);

// Duplicate invoice
export const duplicateInvoiceAtom = atom(null, async (get, set, invoiceId: string) => {
  try {
    // Fetch the original invoice with line items
    const [originalInvoice, lineItems] = await Promise.all([
      GetInvoice(invoiceId),
      GetInvoiceLineItems(invoiceId),
    ]);

    if (!originalInvoice) {
      message.error(t`Invoice not found`);
      return null;
    }

    // Generate new invoice number
    const nextNumber = await get(nextInvoiceNumberAtom);
    if (!nextNumber) {
      message.error(t`Failed to generate invoice number`);
      return null;
    }

    // Create new invoice data with duplicated information
    const newInvoiceId = nanoid();
    const currentDate = dayjs();
    const duplicatedInvoice = {
      id: newInvoiceId,
      organizationId: get(organizationIdAtom)!,
      number: nextNumber,
      state: "draft", // Always start as draft
      clientId: originalInvoice.clientId,
      date: currentDate.valueOf(),
      dueDate: originalInvoice.dueDate
        ? currentDate
            .add(dayjs(originalInvoice.dueDate).diff(dayjs(originalInvoice.date), "day"), "day")
            .valueOf()
        : null,
      currency: originalInvoice.currency,
      // Carry the rate forward (the server requires one whenever currency
      // differs from the organization's), but date it today like a freshly
      // captured document rather than keeping the original's now-stale date.
      exchangeRate: originalInvoice.exchangeRate,
      exchangeRateDate: originalInvoice.exchangeRate ? currentDate.valueOf() : null,
      total: originalInvoice.total,
      taxTotal: originalInvoice.taxTotal,
      subTotal: originalInvoice.subTotal,
      customerNotes: originalInvoice.customerNotes,
      overdueCharge: originalInvoice.overdueCharge,
      buyerReference: originalInvoice.buyerReference,
      paymentTerms: originalInvoice.paymentTerms,
      lineItems: (lineItems || []).map((item) => ({
        ...omit(item, ["id", "invoiceId", "createdAt"]),
      })),
    };

    // Create the duplicated invoice
    const createdInvoice = await CreateInvoice(duplicatedInvoice);

    message.success(t`Invoice duplicated successfully`);

    // Update the invoices list
    const invoices = get(invoicesAtom);
    set(invoicesAtom, [invoiceToDisplay(createdInvoice), ...invoices]);

    // Force refresh organization data to get updated invoice counter
    const currentOrgId = get(organizationIdAtom);
    if (currentOrgId) {
      set(organizationIdAtom, null);
      set(organizationIdAtom, currentOrgId);
    }

    return createdInvoice.id;
  } catch (error) {
    console.error("Failed to duplicate invoice:", error);
    message.error(t`Invoice duplication failed`);
    return null;
  }
});
