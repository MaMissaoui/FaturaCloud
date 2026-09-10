import { atom } from "jotai";
import type { Dayjs } from "dayjs";
import type { IncomingInvoice, IncomingInvoiceLineItem } from "src/types/models";
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
  GetIncomingInvoices,
  GetIncomingInvoice,
  GetIncomingInvoiceLineItems,
  CreateIncomingInvoice,
  UpdateIncomingInvoice,
  UpdateIncomingInvoiceState,
  DeleteIncomingInvoice,
} from "src/api";
import { centsToUnits, unitsToCents } from "src/utils/currency";
import { organizationIdAtom } from "./organization";

export const incomingInvoicesAtom = atom<IncomingInvoice[]>([]);
incomingInvoicesAtom.debugLabel = "incomingInvoicesAtom";

export const setIncomingInvoicesAtom = atom(null, async (get, set) => {
  const organizationId = get(organizationIdAtom);
  try {
    const response = await GetIncomingInvoices(organizationId!);
    set(incomingInvoicesAtom, response);
  } catch (error) {
    console.error("Failed to fetch incoming invoices:", error);
    message.error(t`Failed to fetch incoming invoices`);
    set(incomingInvoicesAtom, []);
  }
});

// The incoming invoice form edits dates as dayjs objects and money in
// display currency units — not the wire shape either field is stored as.
type IncomingInvoiceLineItemFormValues = Omit<Partial<IncomingInvoiceLineItem>, "unitPrice"> & {
  unitPrice?: number;
};
type IncomingInvoiceFormValues = Omit<
  Partial<IncomingInvoice>,
  "date" | "dueDate" | "exchangeRateDate" | "total" | "taxTotal" | "subTotal"
> & {
  date?: Dayjs | number;
  dueDate?: Dayjs | number | null;
  exchangeRateDate?: Dayjs | number | null;
  total?: number;
  taxTotal?: number;
  subTotal?: number;
  lineItems?: IncomingInvoiceLineItemFormValues[];
};

export const incomingInvoiceIdAtom = atom<string | null>(null);

export const incomingInvoiceAtom = atom(
  async (get) => {
    const invoiceId = get(incomingInvoiceIdAtom);
    if (!invoiceId) return null;
    try {
      const [invoice, lineItems] = await Promise.all([
        GetIncomingInvoice(invoiceId),
        GetIncomingInvoiceLineItems(invoiceId),
      ]);
      if (!invoice) return null;
      return {
        ...invoice,
        date: dayjs(invoice.date),
        dueDate: invoice.dueDate ? dayjs(invoice.dueDate) : null,
        exchangeRateDate: invoice.exchangeRateDate ? dayjs(invoice.exchangeRateDate) : null,
        total: centsToUnits(invoice.total),
        taxTotal: centsToUnits(invoice.taxTotal),
        subTotal: centsToUnits(invoice.subTotal),
        lineItems: (lineItems || []).map((item) => ({
          ...item,
          unitPrice: centsToUnits(item.unitPrice),
        })),
      };
    } catch (error) {
      console.error("Failed to fetch incoming invoice:", error);
      message.error(t`Failed to fetch incoming invoice`);
      return null;
    }
  },
  async (get, set, newValues: IncomingInvoiceFormValues) => {
    const invoiceId = get(incomingInvoiceIdAtom);
    // State is not editable through PUT — it changes only via the PATCH state
    // endpoint, which is where the matching gate lives.
    const invoice = omit(newValues, ["lineItems", "state"]);
    const lineItems = newValues.lineItems || [];

    const toTimestamp = (v: Dayjs | number | null | undefined) =>
      v && typeof v === "object" && "valueOf" in v ? v.valueOf() : v;
    const toPayload = (values: typeof invoice) => ({
      ...values,
      date: toTimestamp(values.date),
      dueDate: values.dueDate ? toTimestamp(values.dueDate) : null,
      exchangeRateDate: values.exchangeRateDate ? toTimestamp(values.exchangeRateDate) : null,
      total: unitsToCents(values.total ?? 0),
      taxTotal: unitsToCents(values.taxTotal ?? 0),
      subTotal: unitsToCents(values.subTotal ?? 0),
      lineItems: lineItems.map((item) => ({
        ...omit(item, ["id", "incomingInvoiceId", "position"]),
        unitPrice: unitsToCents(item.unitPrice ?? 0),
      })),
    });

    try {
      if (!invoiceId) {
        const created = await CreateIncomingInvoice({
          ...toPayload(invoice),
          id: nanoid(),
          organizationId: get(organizationIdAtom)!,
        });
        set(incomingInvoiceIdAtom, created.id);
        message.success(t`Incoming invoice created`);
        const list = get(incomingInvoicesAtom);
        set(incomingInvoicesAtom, [created, ...list]);
      } else {
        const updated = await UpdateIncomingInvoice(invoiceId, toPayload(invoice));
        message.success(t`Incoming invoice saved`);
        const list = get(incomingInvoicesAtom);
        const merged = keyBy([...list, updated], "id");
        set(incomingInvoicesAtom, orderBy(map(merged), "date", "desc"));
      }
      return true;
    } catch (error) {
      // Totals mismatches, duplicate vendor numbers and override-without-reason
      // all arrive as a 409 whose message is written for the user.
      console.error("Incoming invoice operation failed:", error);
      const fallback = invoiceId
        ? t`Incoming invoice update failed`
        : t`Incoming invoice creation failed`;
      message.error(error instanceof Error ? error.message : fallback);
      return false;
    }
  },
);

export const updateIncomingInvoiceStateAtom = atom(
  null,
  async (get, set, { invoiceId, state }: { invoiceId: string; state: string }) => {
    try {
      const updated = await UpdateIncomingInvoiceState(invoiceId, state);
      message.success(t`Incoming invoice state updated`);
      const list = get(incomingInvoicesAtom);
      const merged = keyBy([...list, updated], "id");
      set(incomingInvoicesAtom, orderBy(map(merged), "date", "desc"));
      return true;
    } catch (error) {
      // Approving with an unresolved matching variance is rejected here, with a
      // message naming the offending lines.
      console.error("Failed to update incoming invoice state:", error);
      message.error(
        error instanceof Error ? error.message : t`Failed to update incoming invoice state`,
      );
      return false;
    }
  },
);

export const deleteIncomingInvoiceAtom = atom(null, async (get, set, invoiceId: string) => {
  try {
    const success = await DeleteIncomingInvoice(invoiceId);
    if (success) {
      const list = reject(get(incomingInvoicesAtom), (i) => isEqual(i.id, invoiceId));
      set(incomingInvoicesAtom, list);
      message.success(t`Incoming invoice deleted`);
    } else {
      message.error(t`Incoming invoice deletion failed`);
    }
    return success;
  } catch (error) {
    console.error("Failed to delete incoming invoice:", error);
    message.error(error instanceof Error ? error.message : t`Incoming invoice deletion failed`);
    return false;
  }
});
