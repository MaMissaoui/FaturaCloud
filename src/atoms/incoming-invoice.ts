import { atom } from "jotai";
import { message } from "antd";
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

export const incomingInvoicesAtom = atom<any[]>([]);
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
        total: centsToUnits(invoice.total),
        taxTotal: centsToUnits(invoice.taxTotal),
        subTotal: centsToUnits(invoice.subTotal),
        lineItems: (lineItems || []).map((item: any) => ({
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
  async (get, set, newValues: any) => {
    const invoiceId = get(incomingInvoiceIdAtom);
    // State is not editable through PUT — it changes only via the PATCH state
    // endpoint, which is where the matching gate lives.
    const invoice = omit(newValues, ["lineItems", "state"]);
    const lineItems = newValues.lineItems || [];

    const toTimestamp = (v: any) => (v?.valueOf ? v.valueOf() : v);
    const toPayload = (values: any) => ({
      ...values,
      date: toTimestamp(values.date),
      dueDate: values.dueDate ? toTimestamp(values.dueDate) : null,
      total: unitsToCents(values.total ?? 0),
      taxTotal: unitsToCents(values.taxTotal ?? 0),
      subTotal: unitsToCents(values.subTotal ?? 0),
      lineItems: lineItems.map((item: any) => ({
        ...omit(item, ["id", "incomingInvoiceId", "position"]),
        unitPrice: unitsToCents(item.unitPrice ?? 0),
      })),
    });

    try {
      if (!invoiceId) {
        const created = await CreateIncomingInvoice({
          ...toPayload(invoice),
          id: nanoid(),
          organizationId: get(organizationIdAtom),
        });
        set(incomingInvoiceIdAtom, created.id);
        message.success(t`Incoming invoice created`);
        const list: any = get(incomingInvoicesAtom);
        set(incomingInvoicesAtom, [created, ...list]);
      } else {
        const updated = await UpdateIncomingInvoice(invoiceId, toPayload(invoice));
        message.success(t`Incoming invoice saved`);
        const list: any = get(incomingInvoicesAtom);
        const merged: any = keyBy([...list, updated], "id");
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
      const list: any = get(incomingInvoicesAtom);
      const merged: any = keyBy([...list, updated], "id");
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
      const list: any = reject(get(incomingInvoicesAtom), (i: any) => isEqual(i.id, invoiceId));
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
