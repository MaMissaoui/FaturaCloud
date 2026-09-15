package db

// Empty-string normalization for optional foreign-key ids (F94, audit
// 2026-09-14).
//
// Every optional-FK guard in this codebase spells its presence test
// `if x != nil && *x != ""` — checkPurchaseOrderFKOwnership,
// checkOrderFKOwnership, checkProductFKOwnership and the rest. That is
// correct as *validation*: an empty string means "unset", so there is
// nothing to look up and nothing to ownership-check.
//
// The INSERT that follows did not apply the same rule. It passed the
// *string straight through, so a literal "" reached a column with a
// foreign key, SQLite rejected it, and the caller got a wrapped raw driver
// error:
//
//	create_production_order: constraint failed: FOREIGN KEY constraint failed (787)
//
// A 500 "internal error", in other words, for what is really an unset
// optional field spelled slightly differently — the same 409-vs-500
// asymmetry F72 fixed for products and tax rates. Not reachable from this
// app's own UI (antd's `Select allowClear` emits `undefined`, which
// JSON.stringify drops entirely), which is why it went unnoticed; reachable
// by any other API client.
//
// nilIfEmptyID is called at the top of each Create/Update, before the
// ownership guard rather than inside it: the guards take their arguments by
// value, so normalizing in there would not reach the request struct the
// INSERT later reads.
func nilIfEmptyID(s *string) *string {
	if s != nil && *s == "" {
		return nil
	}
	return s
}

// nullableFKClassification records what every nullable foreign-key column in
// the live schema does about an empty-string id. TestNullableForeignKeysAreClassified
// reads the schema and fails when a column is missing here, so adding a new
// nullable FK forces a decision instead of silently inheriting the bug —
// the same tripwire idiom as vendorReferencingTables and
// taxRateReferencingTables.
type fkDisposition int

const (
	// fkNormalized: reachable from a client-supplied request field, and the
	// owning Create/Update runs it through nilIfEmptyID.
	fkNormalized fkDisposition = iota
	// fkServerSet: never client-supplied. Written by posting//fan-out code
	// with a real id or nil, so "" was never reachable.
	fkServerSet
	// fkExplicitClear: client-supplied, and deliberately NOT normalized —
	// "" is this column's *clear* signal, not a malformed id. Normalizing
	// it to nil would mean "don't touch" and silently swallow the clear.
	//
	// These columns are not written through COALESCE at all; their SET
	// assignments are emitted in Go so the three-way convention every other
	// nullable organization field already has (omitted = keep, "" = clear,
	// value = set) can hold for a column with a foreign key too. See F95
	// and db/organization.go's UpdateOrganization.
	fkExplicitClear
)

var nullableFKClassification = map[string]fkDisposition{
	// --- client-supplied, normalized ---
	"accounts.parentId":                                   fkNormalized,
	"inbound_deliveries.purchaseOrderId":                  fkNormalized,
	"inbound_deliveries.vendorId":                         fkNormalized,
	"inbound_delivery_line_items.productId":               fkNormalized,
	"inbound_delivery_line_items.purchaseOrderLineItemId": fkNormalized,
	"incoming_invoice_line_items.productId":               fkNormalized,
	"incoming_invoice_line_items.purchaseOrderLineItemId": fkNormalized,
	"incoming_invoice_line_items.taxRate":                 fkNormalized,
	"incoming_invoices.purchaseOrderId":                   fkNormalized,
	"invoiceLineItems.productId":                          fkNormalized,
	"invoiceLineItems.taxRate":                            fkNormalized,
	"journal_lines.taxRateId":                             fkNormalized,
	"orderLineItems.productId":                            fkNormalized,
	"orders.clientId":                                     fkNormalized,
	"outbound_deliveries.clientId":                        fkNormalized,
	"outbound_deliveries.orderId":                         fkNormalized,
	"outbound_delivery_line_items.orderLineItemId":        fkNormalized,
	"outbound_delivery_line_items.productId":              fkNormalized,
	"payments.clientId":                                   fkNormalized,
	"payments.vendorId":                                   fkNormalized,
	"production_orders.finishedProductId":                 fkNormalized,
	"production_orders.importId":                          fkNormalized,
	"products.expenseAccountId":                           fkNormalized,
	"products.revenueAccountId":                           fkNormalized,
	"products.taxRateId":                                  fkNormalized,
	"products.unitOfMeasureId":                            fkNormalized,
	"purchase_order_line_items.productId":                 fkNormalized,
	"purchase_order_line_items.taxRate":                   fkNormalized,
	"purchase_orders.importId":                            fkNormalized,
	"purchase_orders.vendorId":                            fkNormalized,
	"taxRates.inputTaxAccountId":                          fkNormalized,
	"taxRates.outputTaxAccountId":                         fkNormalized,

	// --- server-set: "" was never reachable ---
	"bill_of_materials_version_lines.componentProductId":  fkServerSet,
	"journal_entries.createdBy":                           fkServerSet,
	"journal_entries.fiscalPeriodId":                      fkServerSet,
	"journal_entries.reversalOfEntryId":                   fkServerSet,
	"journal_lines.clientId":                              fkServerSet,
	"journal_lines.vendorId":                              fkServerSet,
	"payments.journalEntryId":                             fkServerSet,
	"payments.voidingEntryId":                             fkServerSet,
	"production_order_component_lines.componentProductId": fkServerSet,
	"stockMovements.serialNumberId":                       fkServerSet,
	// Legacy tables with no live Create/Update path in this codebase.
	"projects.clientId":    fkServerSet,
	"timeEntries.clientId": fkServerSet,

	// --- deliberately not normalized: "" is the clear signal (F95) ---
	"organizations.datevClearingAccountId":              fkExplicitClear,
	"organizations.defaultApAccountId":                  fkExplicitClear,
	"organizations.defaultArAccountId":                  fkExplicitClear,
	"organizations.defaultCOGSAccountId":                fkExplicitClear,
	"organizations.defaultCashAccountId":                fkExplicitClear,
	"organizations.defaultExpenseAccountId":             fkExplicitClear,
	"organizations.defaultGRNIAccountId":                fkExplicitClear,
	"organizations.defaultImportCostsPayableAccountId":  fkExplicitClear,
	"organizations.defaultInventoryAccountId":           fkExplicitClear,
	"organizations.defaultInventoryAdjustmentAccountId": fkExplicitClear,
	"organizations.defaultRevenueAccountId":             fkExplicitClear,
	"organizations.defaultStampDutyAccountId":           fkExplicitClear,
	"organizations.fxGainAccountId":                     fkExplicitClear,
	"organizations.fxLossAccountId":                     fkExplicitClear,
	"organizations.retainedEarningsAccountId":           fkExplicitClear,
}

// Per-line-item normalization. Each document type has its own line-item
// request struct, so these are four small loops rather than one generic
// walk — the same "a plain map, not reflection" preference db/xlsx_export.go
// states, for the same reason: an unhandled field should be a visible
// omission here, not a silent miss inside a reflective sweep.

func normalizePurchaseOrderLineItemIDs(items []CreatePurchaseOrderLineItemRequest) {
	for i := range items {
		items[i].ID = nilIfEmptyID(items[i].ID)
		items[i].ProductID = nilIfEmptyID(items[i].ProductID)
		items[i].TaxRate = nilIfEmptyID(items[i].TaxRate)
	}
}

func normalizeOrderLineItemIDs(items []CreateOrderLineItemRequest) {
	for i := range items {
		items[i].ID = nilIfEmptyID(items[i].ID)
		items[i].ProductID = nilIfEmptyID(items[i].ProductID)
	}
}

func normalizeDeliveryLineItemIDs(items []CreateDeliveryLineItemRequest) {
	for i := range items {
		items[i].OrderLineItemID = nilIfEmptyID(items[i].OrderLineItemID)
		items[i].ProductID = nilIfEmptyID(items[i].ProductID)
	}
}

func normalizeInboundDeliveryLineItemIDs(items []CreateInboundDeliveryLineItemRequest) {
	for i := range items {
		items[i].PurchaseOrderLineItemID = nilIfEmptyID(items[i].PurchaseOrderLineItemID)
		items[i].ProductID = nilIfEmptyID(items[i].ProductID)
	}
}

// Sales invoices and incoming invoices share CreateInvoiceLineItemRequest.
func normalizeInvoiceLineItemIDs(items []CreateInvoiceLineItemRequest) {
	for i := range items {
		items[i].ProductID = nilIfEmptyID(items[i].ProductID)
		items[i].TaxRate = nilIfEmptyID(items[i].TaxRate)
		items[i].PurchaseOrderLineItemID = nilIfEmptyID(items[i].PurchaseOrderLineItemID)
	}
}
