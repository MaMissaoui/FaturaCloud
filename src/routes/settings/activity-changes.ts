import { t } from "@lingui/core/macro";

import type { AuditFieldChange } from "src/api";
import { roleOptions } from "src/components/organizations/organization-members-panel";

// How the Activity page shows a recorded field change (db/audit_changes.go):
// the field's label and its before/after values, formatted for a reader
// rather than as stored. References (clientId, the *AccountId columns, ...)
// arrive already named by the server, as they read when the change was
// made. Same rules as STBvirement's Activity page.

// The label of a recorded column; a column added later shows its name.
export const fieldLabel = (field: string): string => {
  switch (field) {
    case "parentId":
      return t`Parent account`;
    case "code":
      return t`Code`;
    case "name":
      return t`Name`;
    case "type":
      return t`Type`;
    case "isGroup":
      return t`Group account`;
    case "isActive":
      return t`Active`;
    case "datevAccountNumber":
      return t`DATEV account number`;
    case "description":
      return t`Description`;
    case "emails":
      return t`E-mails`;
    case "phone":
      return t`Phone`;
    case "website":
      return t`Website`;
    case "registration_number":
      return t`Registration number`;
    case "vatin":
      return t`VAT ID`;
    case "street":
      return t`Street`;
    case "house_number":
      return t`House number`;
    case "postal_code":
      return t`Postal code`;
    case "city":
      return t`City`;
    case "country_code":
      return t`Country`;
    case "tax_number":
      return t`Tax number`;
    case "default_buyer_reference":
      return t`Default buyer reference`;
    case "defaultCurrency":
      return t`Default currency`;
    case "identity_number":
      return t`Identity number`;
    case "iban":
      return t`IBAN`;
    case "phone2":
      return t`Phone 2`;
    case "phone3":
      return t`Phone 3`;
    case "guarantor":
      return t`Guarantor`;
    case "address":
      return t`Address`;
    case "importBatchId":
      return t`Import batch`;
    case "fiscalYearId":
      return t`Fiscal year`;
    case "startDate":
      return t`Start date`;
    case "endDate":
      return t`End date`;
    case "status":
      return t`Status`;
    case "closedAt":
      return t`Closed on`;
    case "lockDate":
      return t`Lock date`;
    case "importNumber":
      return t`Import number`;
    case "date":
      return t`Date`;
    case "currency":
      return t`Currency`;
    case "exchangeRate":
      return t`Exchange rate`;
    case "exchangeRateDate":
      return t`Exchange rate date`;
    case "freightCost":
      return t`Freight cost`;
    case "customsCost":
      return t`Customs cost`;
    case "notes":
      return t`Notes`;
    case "serialNumberPrefix":
      return t`Serial number prefix`;
    case "serialNumberRangeStart":
      return t`Serial number range start`;
    case "serialNumberRangeEnd":
      return t`Serial number range end`;
    case "purchaseOrderId":
      return t`Purchase order`;
    case "vendorId":
      return t`Vendor`;
    case "deliveryNumber":
      return t`Delivery number`;
    case "deliveryDate":
      return t`Delivery date`;
    case "vendorDeliveryNote":
      return t`Vendor delivery note`;
    case "trackingNumber":
      return t`Tracking number`;
    case "vendorInvoiceNumber":
      return t`Vendor invoice number`;
    case "reference":
      return t`Reference`;
    case "state":
      return t`State`;
    case "dueDate":
      return t`Due date`;
    case "total":
      return t`Total`;
    case "taxTotal":
      return t`Tax total`;
    case "subTotal":
      return t`Subtotal`;
    case "matchOverride":
      return t`Match override`;
    case "matchOverrideReason":
      return t`Match override reason`;
    case "number":
      return t`Number`;
    case "clientId":
      return t`Client`;
    case "customerNotes":
      return t`Customer notes`;
    case "overdueCharge":
      return t`Overdue charge`;
    case "buyerReference":
      return t`Buyer reference`;
    case "paymentTerms":
      return t`Payment terms`;
    case "fiscalStampAmount":
      return t`Fiscal stamp`;
    case "withholdingTaxRate":
      return t`Withholding tax rate`;
    case "withholdingTaxAmount":
      return t`Withholding tax`;
    case "discountAmount":
      return t`Discount`;
    case "movesStock":
      return t`Moves stock`;
    case "origin":
      return t`Origin`;
    case "journalId":
      return t`Journal`;
    case "fiscalPeriodId":
      return t`Fiscal period`;
    case "entryNumber":
      return t`Entry number`;
    case "sourceDocumentType":
      return t`Source document type`;
    case "sourceDocumentId":
      return t`Source document`;
    case "reversalOfEntryId":
      return t`Reversal of entry`;
    case "reversalReason":
      return t`Reversal reason`;
    case "postedAt":
      return t`Posted on`;
    case "createdBy":
      return t`Created by`;
    case "isSystem":
      return t`System journal`;
    case "orderNumber":
      return t`Order number`;
    case "orderDate":
      return t`Order date`;
    case "shippingAddress":
      return t`Shipping address`;
    case "country":
      return t`Country`;
    case "email":
      return t`E-mail`;
    case "bank_name":
      return t`Bank`;
    case "minimum_fraction_digits":
      return t`Decimal places`;
    case "due_days":
      return t`Payment terms (days)`;
    case "invoice_number_format":
      return t`Invoice number format`;
    case "invoice_number_counter":
      return t`Invoice number counter`;
    case "date_format":
      return t`Date format`;
    case "match_price_tolerance_percent":
      return t`Price match tolerance (%)`;
    case "match_quantity_tolerance_percent":
      return t`Quantity match tolerance (%)`;
    case "bic":
      return t`BIC`;
    case "defaultArAccountId":
      return t`Receivables account`;
    case "defaultApAccountId":
      return t`Payables account`;
    case "defaultRevenueAccountId":
      return t`Revenue account`;
    case "defaultExpenseAccountId":
      return t`Expense account`;
    case "defaultCashAccountId":
      return t`Bank account`;
    case "fxGainAccountId":
      return t`Exchange gain account`;
    case "fxLossAccountId":
      return t`Exchange loss account`;
    case "retainedEarningsAccountId":
      return t`Retained earnings account`;
    case "datevClearingAccountId":
      return t`DATEV clearing account`;
    case "datev_consultant_number":
      return t`DATEV consultant number`;
    case "datev_client_number":
      return t`DATEV client number`;
    case "defaultInventoryAccountId":
      return t`Inventory account`;
    case "defaultGRNIAccountId":
      return t`Goods received not invoiced account`;
    case "defaultCOGSAccountId":
      return t`Cost of goods sold account`;
    case "defaultInventoryAdjustmentAccountId":
      return t`Inventory adjustment account`;
    case "brandColor":
      return t`Brand color`;
    case "defaultFiscalStampAmount":
      return t`Default fiscal stamp`;
    case "defaultStampDutyAccountId":
      return t`Stamp duty account`;
    case "documentLayout":
      return t`Document layout`;
    case "fiscalStampEnabled":
      return t`Fiscal stamp enabled`;
    case "withholdingTaxEnabled":
      return t`Withholding tax enabled`;
    case "defaultImportCostsPayableAccountId":
      return t`Import costs payable account`;
    case "defaultCashRegisterAccountId":
      return t`Cash register account`;
    case "amountInWordsEnabled":
      return t`Amount in words`;
    case "documentLanguage":
      return t`Document language`;
    case "timezone":
      return t`Time zone`;
    case "inventoryValuation":
      return t`Inventory valuation`;
    case "isTest":
      return t`Test organization`;
    case "masterDataSummaries":
      return t`Master data summaries`;
    case "orderId":
      return t`Order`;
    case "isDefault":
      return t`Default`;
    case "direction":
      return t`Direction`;
    case "bankAccountId":
      return t`Bank account`;
    case "amount":
      return t`Amount`;
    case "method":
      return t`Method`;
    case "journalEntryId":
      return t`Journal entry`;
    case "voidingEntryId":
      return t`Voiding entry`;
    case "finishedProductId":
      return t`Finished product`;
    case "finishedProductName":
      return t`Finished product name`;
    case "quantity":
      return t`Quantity`;
    case "importId":
      return t`Import`;
    case "sku":
      return t`SKU`;
    case "price":
      return t`Price`;
    case "unitCost":
      return t`Unit cost`;
    case "unit":
      return t`Unit`;
    case "taxRateId":
      return t`Tax rate`;
    case "stockEnabled":
      return t`Stock tracked`;
    case "stockQuantity":
      return t`Stock quantity`;
    case "serialized":
      return t`Serial numbers`;
    case "revenueAccountId":
      return t`Revenue account`;
    case "expenseAccountId":
      return t`Expense account`;
    case "category":
      return t`Category`;
    case "unitOfMeasureId":
      return t`Unit of measure`;
    case "familyId":
      return t`Product family`;
    case "expectedDate":
      return t`Expected date`;
    case "deliveryAddress":
      return t`Delivery address`;
    case "percentage":
      return t`Percentage`;
    case "category_code":
      return t`Tax category code`;
    case "exemption_reason":
      return t`Exemption reason`;
    case "outputTaxAccountId":
      return t`Output tax account`;
    case "inputTaxAccountId":
      return t`Input tax account`;
    case "datev_bu_key":
      return t`DATEV tax key`;
    case "displayName":
      return t`Display name`;
    case "role":
      return t`Role`;
    case "organizationRole":
      return t`Role in the organization`;
    case "lastLoginAt":
      return t`Last login`;
    case "isPlatformAdmin":
      return t`Platform admin`;
    case "paymentTermsDays":
      return t`Payment terms (days)`;
    case "hasLogo":
      return t`Logo`;
    default:
      return field;
  }
};

// Document states as the lists name them.
export const stateLabel = (state: string): string => {
  switch (state) {
    case "draft":
      return t`Draft`;
    case "sent":
      return t`Sent`;
    case "paid":
      return t`Paid`;
    case "cancelled":
      return t`Cancelled`;
    case "approved":
      return t`Approved`;
    case "confirmed":
      return t`Confirmed`;
    case "shipped":
      return t`Shipped`;
    case "delivered":
      return t`Delivered`;
    case "received":
      return t`Received`;
    case "completed":
      return t`Completed`;
    case "posted":
      return t`Posted`;
    case "reversed":
      return t`Reversed`;
    case "voided":
      return t`Voided`;
    case "open":
      return t`Open`;
    case "closed":
      return t`Closed`;
    default:
      return state;
  }
};

// Amounts are stored in cents.
const AMOUNT_FIELDS = new Set([
  "total",
  "taxTotal",
  "subTotal",
  "amount",
  "price",
  "unitCost",
  "freightCost",
  "customsCost",
  "fiscalStampAmount",
  "withholdingTaxAmount",
  "discountAmount",
  "defaultFiscalStampAmount",
]);
const DATE_FIELDS = new Set([
  "date",
  "dueDate",
  "startDate",
  "endDate",
  "lockDate",
  "exchangeRateDate",
  "deliveryDate",
  "orderDate",
  "expectedDate",
]);
const DATETIME_FIELDS = new Set(["closedAt", "postedAt", "lastLoginAt"]);
const YES_NO_FIELDS = new Set([
  "isGroup",
  "isActive",
  "matchOverride",
  "movesStock",
  "isSystem",
  "fiscalStampEnabled",
  "withholdingTaxEnabled",
  "amountInWordsEnabled",
  "isTest",
  "masterDataSummaries",
  "isDefault",
  "stockEnabled",
  "serialized",
  "isPlatformAdmin",
  "hasLogo",
]);

export interface ValueFormatters {
  date: (ms: number) => string;
  dateTime: (ms: number) => string;
  locale: string;
}

// formatChangeValue renders one side of a change. Empty values show as "—".
export const formatChangeValue = (
  change: AuditFieldChange,
  value: AuditFieldChange["from"],
  fmt: ValueFormatters,
): string => {
  if (value === null || value === undefined || value === "") return "—";
  const { field } = change;
  if (change.masked) return String(value);
  if (typeof value === "number") {
    if (AMOUNT_FIELDS.has(field)) {
      return new Intl.NumberFormat(fmt.locale, {
        minimumFractionDigits: 2,
        maximumFractionDigits: 2,
      }).format(value / 100);
    }
    if (DATE_FIELDS.has(field)) return fmt.date(value);
    if (DATETIME_FIELDS.has(field)) return fmt.dateTime(value);
    if (YES_NO_FIELDS.has(field)) return value ? t`Yes` : t`No`;
    return String(value);
  }
  switch (field) {
    case "emails":
      return parseEmails(value) || "—";
    case "state":
    case "status":
      return stateLabel(value);
    case "organizationRole":
      return roleOptions().find((r) => r.value === value)?.label ?? value;
  }
  return value;
};

// emails is stored as a JSON array or a comma-separated list.
const parseEmails = (v: string): string => {
  try {
    const list: unknown = JSON.parse(v);
    if (Array.isArray(list)) return list.filter((m) => typeof m === "string").join(", ");
  } catch {
    // not JSON: shown as stored
  }
  return v;
};

// summarizeChanges is the one-line Change cell for field changes: the
// changed fields' labels, at most three, then a count of the rest.
export const summarizeChanges = (changes: AuditFieldChange[]): string => {
  const labels = changes.map((c) => fieldLabel(c.field));
  if (labels.length <= 3) return labels.join(", ");
  const fields = labels.slice(0, 3).join(", ");
  const count = labels.length - 3;
  return t`${fields} and ${count} more`;
};
