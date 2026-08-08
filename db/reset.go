package db

import "fmt"

// ResetOrganizationDataRequest selects which record collections to wipe for
// an organization, without deleting the organization itself (unlike
// DeleteOrganization). "Master data" is clients, vendors, products, and tax
// rates; "transactional data" is every document and stock movement.
type ResetOrganizationDataRequest struct {
	ResetMasterData        bool `json:"resetMasterData"`
	ResetTransactionalData bool `json:"resetTransactionalData"`
}

// transactionalDataTables lists every organizationId-scoped table that holds
// documents/movements rather than reference data. Order doesn't matter for FK
// purposes — every reference among these tables is ON DELETE CASCADE or SET
// NULL, never RESTRICT (that's reserved for vendors, in masterDataTables) —
// but stock movements first keeps the intent readable: clear the ledger
// before the documents that drove it.
//
// TestResetOrganizationDataCoversEveryOrganizationScopedTable discovers every
// table with an organizationId column from the live schema and requires it to
// be covered by this list, masterDataTables, or its own explicit exclusion —
// the same tripwire shape as vendorReferencingTables, guarding the opposite
// mistake: a table a future migration adds silently surviving a reset instead
// of silently breaking a delete.
var transactionalDataTables = []string{
	"stockMovements",
	// Serial identity is tied to receiving/shipping history, the same as the
	// movements themselves — a unit's serial record without any movement
	// history to explain how it got here would be orphaned, meaningless data.
	"product_serial_numbers",
	"incoming_invoices",
	"inbound_deliveries",
	"purchase_orders",
	"outbound_deliveries",
	"orders",
	"invoices",
	// payments before journal_entries: payments.journalEntryId/voidingEntryId
	// reference journal_entries with no ON DELETE clause, so the referencing
	// row must go first. journal_lines and payment_applications are NOT
	// listed here — neither has an organizationId column (both reach it
	// transitively through journal_entries/payments, both ON DELETE CASCADE),
	// the same reason invoiceLineItems is never listed alongside invoices.
	// fiscal_periods cascades from fiscal_years (in masterDataTables) the
	// same way and is likewise not listed separately.
	"payments",
	"journal_entries",
	"reconciliation_groups",
}

// masterDataTables lists the organizationId-scoped reference-data tables.
// Products and tax rates are referenced by transactional line items with ON
// DELETE SET NULL, so — unlike clients/vendors — deleting them independently
// of transactional data would work at the DB level; they're included in the
// same all-or-nothing rule anyway for one consistent, predictable behavior
// rather than a partial one.
// journals/fiscal_periods/fiscal_years/accounts are appended with accounts
// LAST: taxRates.outputTaxAccountId/inputTaxAccountId and
// products.revenueAccountId/expenseAccountId reference accounts with no
// ON DELETE clause, so accounts can only be deleted once taxRates and
// products (both earlier in this list) are already gone. fiscal_periods is
// listed explicitly even though it would also cascade from fiscal_years
// (ON DELETE CASCADE) — unlike line-item tables, it carries its own
// organizationId column, so the schema-introspection tripwire test
// (TestResetOrganizationDataCoversEveryOrganizationScopedTable) requires it
// to be named here directly.
var masterDataTables = []string{
	"clients", "vendors", "products", "taxRates",
	"journals", "fiscal_periods", "fiscal_years", "accounts",
}

// ResetOrganizationData deletes the selected record collections for an
// organization and returns how many rows of each kind were removed, so the
// caller can show what actually happened.
//
// Master data cannot be reset on its own: invoices.clientId cascades on
// delete, so wiping clients would silently take every invoice with it
// regardless of what the caller asked for, and vendors have the opposite
// problem — purchase_orders/inbound_deliveries/incoming_invoices reference
// vendors with no ON DELETE clause at all (deliberately, see DeleteVendor),
// so deleting a vendor while any purchasing document still references it
// fails outright. Requesting master data therefore always wipes
// transactional data first, inside the same transaction, whether or not the
// caller also asked for it — the alternative is a request that either
// silently deletes more than asked (clients) or errors on vendors depending
// on what data happens to exist. The frontend enforces this pairing in the
// UI (checking "master data" forces "transactional data" on) so this is
// never a surprise by the time it reaches here.
//
// Products/tax rates referenced by transactional line items use ON DELETE
// SET NULL, so — unlike clients/vendors — master-only deletion of those two
// would have worked at the DB level; they're included in the same rule
// anyway for one consistent, predictable behavior rather than a partial one.
func (d *Database) ResetOrganizationData(organizationID string, req ResetOrganizationDataRequest) (*OrganizationUsageCount, error) {
	if !req.ResetMasterData && !req.ResetTransactionalData {
		return nil, newValidationError("select at least one of master data or transactional data to reset")
	}
	wipeTransactional := req.ResetTransactionalData || req.ResetMasterData

	deleted, err := d.GetOrganizationUsageCount(organizationID)
	if err != nil {
		return nil, fmt.Errorf("reset_organization_data usage_count: %w", err)
	}
	if !req.ResetMasterData {
		deleted.Clients, deleted.Vendors, deleted.Products, deleted.TaxRates = 0, 0, 0, 0
		deleted.Accounts, deleted.Journals = 0, 0
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("reset_organization_data begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if wipeTransactional {
		for _, table := range transactionalDataTables {
			if _, err := tx.Exec(`DELETE FROM `+table+` WHERE organizationId = ?`, organizationID); err != nil {
				return nil, fmt.Errorf("reset_organization_data delete %s: %w", table, err)
			}
		}

		// Numbering and stock levels are derived from the documents/movements
		// just deleted; nothing recomputes them automatically for a bulk wipe
		// the way the per-document code paths do for a single change.
		if _, err := tx.Exec(
			`UPDATE organizations SET invoice_number_counter = 0 WHERE id = ?`, organizationID,
		); err != nil {
			return nil, fmt.Errorf("reset_organization_data invoice_counter: %w", err)
		}
		if _, err := tx.Exec(
			`UPDATE products SET stockQuantity = 0 WHERE organizationId = ?`, organizationID,
		); err != nil {
			return nil, fmt.Errorf("reset_organization_data stock_quantity: %w", err)
		}
	}

	if req.ResetMasterData {
		// organizations itself is never deleted by a reset, but carries
		// thirteen FK columns pointing into accounts (set by
		// seedDefaultChartOfAccounts/seedInventoryAccountsTx or manual
		// configuration). Null them before accounts rows are deleted below,
		// or those columns would dangle — a foreign key violation with
		// enforcement on, silent corruption without it. Same placement/style
		// as the invoice_number_counter reset above.
		if _, err := tx.Exec(`
			UPDATE organizations
			SET defaultArAccountId = NULL, defaultApAccountId = NULL, defaultRevenueAccountId = NULL,
			    defaultExpenseAccountId = NULL, defaultCashAccountId = NULL,
			    fxGainAccountId = NULL, fxLossAccountId = NULL, retainedEarningsAccountId = NULL,
			    datevClearingAccountId = NULL,
			    defaultInventoryAccountId = NULL, defaultGRNIAccountId = NULL,
			    defaultCOGSAccountId = NULL, defaultInventoryAdjustmentAccountId = NULL
			WHERE id = ?`, organizationID,
		); err != nil {
			return nil, fmt.Errorf("reset_organization_data clear_gl_defaults: %w", err)
		}

		for _, table := range masterDataTables {
			if _, err := tx.Exec(`DELETE FROM `+table+` WHERE organizationId = ?`, organizationID); err != nil {
				return nil, fmt.Errorf("reset_organization_data delete %s: %w", table, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("reset_organization_data commit: %w", err)
	}

	if !wipeTransactional {
		deleted.Invoices, deleted.Orders, deleted.Deliveries = 0, 0, 0
		deleted.PurchaseOrders, deleted.InboundDeliveries, deleted.IncomingInvoices = 0, 0, 0
		deleted.StockMovements = 0
		deleted.JournalEntries, deleted.Payments = 0, 0
	}
	return deleted, nil
}
