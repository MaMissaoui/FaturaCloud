package db

import (
	"fmt"
	"math/big"

	"github.com/jmoiron/sqlx"
	gonanoid "github.com/matoous/go-nanoid/v2"
)

// postAutoEntryTx creates and immediately posts a system-generated journal
// entry within tx. Unlike a manual entry (draft, then a separate PostJournalEntry
// call), a system-generated entry is already balanced by construction
// (buildInvoiceGLLines/buildIncomingInvoiceGLLines guarantee it) so there's
// no draft step — it goes straight to posted via the same
// allocateAndFinalizeEntryTx choke point every other posting path uses.
func postAutoEntryTx(
	tx *sqlx.Tx,
	organizationID, journalID, sourceDocumentType, sourceDocumentID string,
	date int64, reference, description string,
	lines []CreateJournalLineRequest,
) (string, error) {
	entryID, err := gonanoid.New()
	if err != nil {
		return "", fmt.Errorf("post_auto_entry new_id: %w", err)
	}

	fiscalYearID, fiscalPeriodID, err := resolveFiscalPeriodForDate(tx, organizationID, date)
	if err != nil {
		return "", err
	}

	if _, err := tx.Exec(
		`INSERT INTO journal_entries
		   (id, organizationId, journalId, fiscalYearId, fiscalPeriodId, date, reference, description, sourceDocumentType, sourceDocumentId)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entryID, organizationID, journalID, fiscalYearID, fiscalPeriodID, date, reference, description,
		sourceDocumentType, sourceDocumentID,
	); err != nil {
		return "", fmt.Errorf("post_auto_entry insert: %w", err)
	}

	if err := insertJournalLinesTx(tx, entryID, lines); err != nil {
		return "", err
	}

	if err := allocateAndFinalizeEntryTx(tx, entryID, organizationID, fiscalYearID, journalID); err != nil {
		return "", err
	}

	return entryID, nil
}

// reverseEntryTx is the transactional body of ReverseJournalEntry, extracted
// so the invoice/incoming-invoice GL sync (which already holds its own tx
// for the state-column update) can reverse a stale posted entry without
// nesting a second Beginx() — db.SetMaxOpenConns(1) means a second
// connection attempt while a tx is open would block forever, not error.
func reverseEntryTx(tx *sqlx.Tx, original *JournalEntry, reason string, reversalDate int64) (string, error) {
	var lines []JournalLine
	if err := tx.Select(&lines, `SELECT * FROM journal_lines WHERE journalEntryId = ? ORDER BY position ASC`, original.ID); err != nil {
		return "", fmt.Errorf("reverse_entry lines: %w", err)
	}

	fiscalYearID, fiscalPeriodID, err := resolveFiscalPeriodForDate(tx, original.OrganizationID, reversalDate)
	if err != nil {
		return "", err
	}

	reversalID, err := gonanoid.New()
	if err != nil {
		return "", fmt.Errorf("reverse_entry new_id: %w", err)
	}
	description := "Reversal of " + original.Description
	if _, err := tx.Exec(
		`INSERT INTO journal_entries
		   (id, organizationId, journalId, fiscalYearId, fiscalPeriodId, date, description,
		    sourceDocumentType, sourceDocumentId, reversalOfEntryId, reversalReason)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		reversalID, original.OrganizationID, original.JournalID, fiscalYearID, fiscalPeriodID, reversalDate, description,
		original.SourceDocumentType, original.SourceDocumentID, original.ID, reason,
	); err != nil {
		return "", fmt.Errorf("reverse_entry insert: %w", err)
	}

	reversedLines := make([]CreateJournalLineRequest, len(lines))
	for i, l := range lines {
		reversedLines[i] = CreateJournalLineRequest{
			AccountID:     l.AccountID,
			Description:   l.Description,
			Debit:         l.Credit, // flipped: what was credited is now debited, and vice versa
			Credit:        l.Debit,
			Currency:      l.Currency,
			ForeignAmount: l.ForeignAmount,
			ExchangeRate:  l.ExchangeRate,
			ClientID:      l.ClientID,
			VendorID:      l.VendorID,
			TaxRateID:     l.TaxRateID,
		}
	}
	if err := insertJournalLinesTx(tx, reversalID, reversedLines); err != nil {
		return "", err
	}

	if err := allocateAndFinalizeEntryTx(tx, reversalID, original.OrganizationID, fiscalYearID, original.JournalID); err != nil {
		return "", err
	}

	if _, err := tx.Exec(`UPDATE journal_entries SET status = 'reversed' WHERE id = ?`, original.ID); err != nil {
		return "", fmt.Errorf("reverse_entry mark_original: %w", err)
	}

	return reversalID, nil
}

// getJournalByTypeTx resolves the journal a system-generated entry posts
// into by type (sales/purchases), preferring the seeded isSystem journal
// deterministically when more than one journal shares a type.
func getJournalByTypeTx(exec sqlGetExecer, organizationID, journalType string) (*Journal, error) {
	var journal Journal
	err := exec.Get(&journal,
		`SELECT * FROM journals WHERE organizationId = ? AND type = ?
		 ORDER BY isSystem DESC, code ASC LIMIT 1`,
		organizationID, journalType,
	)
	if err != nil {
		return nil, newValidationError("cannot post: no %q journal configured for this organization", journalType)
	}
	return &journal, nil
}

// resolveRevenueAccount picks a line item's revenue account: the product's
// override if it has one, else the organization's default — mirrors
// products.taxRateId overriding taxRates.isDefault, the existing pattern for
// per-product overrides of an org-level default.
func resolveRevenueAccount(product *Product, defaultAccountID *string) (string, error) {
	if product != nil && product.RevenueAccountID != nil {
		return *product.RevenueAccountID, nil
	}
	if defaultAccountID != nil {
		return *defaultAccountID, nil
	}
	return "", newValidationError("cannot post invoice: no revenue account configured (set a default on the organization or the product)")
}

// resolveExpenseAccount is buildIncomingInvoiceGLLines' counterpart to
// resolveRevenueAccount.
func resolveExpenseAccount(product *Product, defaultAccountID *string) (string, error) {
	if product != nil && product.ExpenseAccountID != nil {
		return *product.ExpenseAccountID, nil
	}
	if defaultAccountID != nil {
		return *defaultAccountID, nil
	}
	return "", newValidationError("cannot post bill: no expense account configured (set a default on the organization or the product)")
}

// groupAndRound sums exact-rational per-line cents into groups keyed by key(),
// then rounds each group once to the nearest cent — the same "round once per
// group, not per line" rule validateInvoiceTotals uses for tax, applied here
// to both the tax and the revenue/expense partitions.
func groupAndRound(rats map[string]*big.Rat, order []string) map[string]int64 {
	out := make(map[string]int64, len(order))
	for _, key := range order {
		out[key] = roundHalfUp(rats[key], 0).Num().Int64()
	}
	return out
}

// convertGroup converts a map of document-currency cents to the
// organization's functional currency, returning the per-key converted
// amounts and their sum.
func convertGroup(docCents map[string]int64, order []string, convert func(int64) int64) (map[string]int64, int64) {
	functional := make(map[string]int64, len(order))
	var sum int64
	for _, key := range order {
		amount := convert(docCents[key])
		functional[key] = amount
		sum += amount
	}
	return functional, sum
}

// absorbResidual nudges the largest entry in functional so the group sums to
// target exactly — used to absorb rounding/conversion residuals onto the
// largest revenue (or expense) line so AR/AP always balances against the
// rest of the entry by construction.
func absorbResidual(functional map[string]int64, order []string, target int64) {
	if len(order) == 0 {
		return
	}
	var sum int64
	for _, key := range order {
		sum += functional[key]
	}
	residual := target - sum
	if residual == 0 {
		return
	}
	largest := order[0]
	for _, key := range order {
		if functional[key] > functional[largest] {
			largest = key
		}
	}
	functional[largest] += residual
}

// buildInvoiceGLLines computes the balanced GL lines for a sales invoice's
// auto-posted entry: one AR line taken directly from invoice.Total (so it
// matches exactly what Phase 3 payments will settle against, independent of
// any per-line rounding below), one revenue line per resolved account, and
// one output-tax line per distinct tax rate actually used. Revenue and tax
// are computed in the invoice's own currency using the same exact-rational
// approach as validateInvoiceTotals, then converted to the organization's
// functional currency via the invoice's frozen exchangeRate; any residual
// from independent per-group rounding/conversion is absorbed onto the
// largest revenue line so the entry balances by construction.
func (d *Database) buildInvoiceGLLines(invoice *Invoice, lineItems []InvoiceLineItem) ([]CreateJournalLineRequest, *Journal, error) {
	org, err := d.GetOrganization(invoice.OrganizationID)
	if err != nil {
		return nil, nil, fmt.Errorf("build_invoice_gl_lines organization: %w", err)
	}
	if org.DefaultArAccountID == nil {
		return nil, nil, newValidationError("cannot post invoice: organization has no default AR account configured")
	}
	journal, err := getJournalByTypeTx(d.DB, invoice.OrganizationID, "sales")
	if err != nil {
		return nil, nil, err
	}

	rate, err := parseExchangeRate(invoice.ExchangeRate)
	if err != nil {
		return nil, nil, err
	}
	// exchangeRate is nil exactly when the invoice's currency equals the
	// organization's — see resolveExchangeRateForSave.
	foreign := invoice.ExchangeRate != nil
	convert := func(cents int64) int64 {
		if !foreign {
			return cents
		}
		return convertCents(cents, rate)
	}

	revenueRats := map[string]*big.Rat{}
	var revenueOrder []string
	taxRats := map[string]*big.Rat{}
	var taxOrder []string
	taxAccounts := map[string]string{}

	productCache := map[string]*Product{}
	taxRateCache := map[string]*TaxRate{}

	for i, item := range lineItems {
		qty, err := floatToRat(item.Quantity)
		if err != nil {
			return nil, nil, newValidationError("line %d: invalid quantity", i+1)
		}
		lineCents := new(big.Rat).Mul(qty, new(big.Rat).SetInt64(item.UnitPrice))

		var product *Product
		if item.ProductID != nil {
			if cached, ok := productCache[*item.ProductID]; ok {
				product = cached
			} else {
				product, err = d.GetProduct(*item.ProductID)
				if err != nil {
					return nil, nil, fmt.Errorf("build_invoice_gl_lines product: %w", err)
				}
				productCache[*item.ProductID] = product
			}
		}
		revenueAccountID, err := resolveRevenueAccount(product, org.DefaultRevenueAccountID)
		if err != nil {
			return nil, nil, err
		}
		if _, ok := revenueRats[revenueAccountID]; !ok {
			revenueRats[revenueAccountID] = new(big.Rat)
			revenueOrder = append(revenueOrder, revenueAccountID)
		}
		revenueRats[revenueAccountID].Add(revenueRats[revenueAccountID], lineCents)

		if item.TaxRate != nil && *item.TaxRate != "" {
			taxRate, ok := taxRateCache[*item.TaxRate]
			if !ok {
				taxRate, err = d.GetTaxRate(*item.TaxRate)
				if err != nil {
					return nil, nil, newValidationError("unknown tax rate on invoice line item")
				}
				taxRateCache[*item.TaxRate] = taxRate
			}
			if _, ok := taxRats[*item.TaxRate]; !ok {
				if taxRate.OutputTaxAccountID == nil {
					return nil, nil, newValidationError(
						"cannot post invoice: tax rate %q has no output tax account configured", taxRate.Name,
					)
				}
				taxRats[*item.TaxRate] = new(big.Rat)
				taxOrder = append(taxOrder, *item.TaxRate)
				taxAccounts[*item.TaxRate] = *taxRate.OutputTaxAccountID
			}
			taxRats[*item.TaxRate].Add(taxRats[*item.TaxRate], lineCents)
		}
	}

	// Tax is rounded per group before the percentage is applied's result is
	// rounded — mirrors validateInvoiceTotals exactly (in cents rather than
	// units; 1 cent is the smallest unit of either representation, so
	// rounding to the nearest cent is equivalent to rounding 2 decimal
	// places on unit amounts).
	taxAmountsDoc := map[string]int64{}
	for _, taxRateID := range taxOrder {
		taxRate := taxRateCache[taxRateID]
		pct, err := floatToRat(taxRate.Percentage)
		if err != nil {
			return nil, nil, newValidationError("invalid tax rate percentage")
		}
		tax := new(big.Rat).Mul(taxRats[taxRateID], pct)
		tax.Quo(tax, hundred)
		taxAmountsDoc[taxRateID] = roundHalfUp(tax, 0).Num().Int64()
	}
	revenueAmountsDoc := groupAndRound(revenueRats, revenueOrder)

	arFunctional := convert(invoice.Total)
	revenueFunctional, _ := convertGroup(revenueAmountsDoc, revenueOrder, convert)
	taxFunctional, taxSum := convertGroup(taxAmountsDoc, taxOrder, convert)
	absorbResidual(revenueFunctional, revenueOrder, arFunctional-taxSum)

	// journal_lines has CHECK((debit=0) <> (credit=0)) — a group that nets to
	// exactly zero (e.g. a 0% tax rate, or a free line item) must not emit a
	// row at all rather than a debit=0/credit=0 line, which the DB rejects.
	var lines []CreateJournalLineRequest
	for _, accountID := range revenueOrder {
		if revenueFunctional[accountID] == 0 {
			continue
		}
		lines = append(lines, glLine(accountID, 0, revenueFunctional[accountID], invoice.Currency, revenueAmountsDoc[accountID], invoice.ExchangeRate, foreign, nil, nil, nil))
	}
	for _, taxRateID := range taxOrder {
		if taxFunctional[taxRateID] == 0 {
			continue
		}
		lines = append(lines, glLine(taxAccounts[taxRateID], 0, taxFunctional[taxRateID], invoice.Currency, taxAmountsDoc[taxRateID], invoice.ExchangeRate, foreign, nil, nil, &taxRateID))
	}
	lines = append(lines, glLine(*org.DefaultArAccountID, arFunctional, 0, invoice.Currency, invoice.Total, invoice.ExchangeRate, foreign, &invoice.ClientID, nil, nil))

	return lines, journal, nil
}

// buildIncomingInvoiceGLLines is buildInvoiceGLLines' purchases counterpart:
// one AP line taken directly from the bill's Total, one expense line per
// resolved account, and one input-tax line per distinct tax rate used.
func (d *Database) buildIncomingInvoiceGLLines(bill *IncomingInvoice, lineItems []IncomingInvoiceLineItem) ([]CreateJournalLineRequest, *Journal, error) {
	org, err := d.GetOrganization(bill.OrganizationID)
	if err != nil {
		return nil, nil, fmt.Errorf("build_incoming_invoice_gl_lines organization: %w", err)
	}
	if org.DefaultApAccountID == nil {
		return nil, nil, newValidationError("cannot post bill: organization has no default AP account configured")
	}
	journal, err := getJournalByTypeTx(d.DB, bill.OrganizationID, "purchases")
	if err != nil {
		return nil, nil, err
	}

	rate, err := parseExchangeRate(bill.ExchangeRate)
	if err != nil {
		return nil, nil, err
	}
	foreign := bill.ExchangeRate != nil
	convert := func(cents int64) int64 {
		if !foreign {
			return cents
		}
		return convertCents(cents, rate)
	}

	expenseRats := map[string]*big.Rat{}
	var expenseOrder []string
	taxRats := map[string]*big.Rat{}
	var taxOrder []string
	taxAccounts := map[string]string{}

	productCache := map[string]*Product{}
	taxRateCache := map[string]*TaxRate{}

	for i, item := range lineItems {
		qty, err := floatToRat(item.Quantity)
		if err != nil {
			return nil, nil, newValidationError("line %d: invalid quantity", i+1)
		}
		lineCents := new(big.Rat).Mul(qty, new(big.Rat).SetInt64(item.UnitPrice))

		var product *Product
		if item.ProductID != nil {
			if cached, ok := productCache[*item.ProductID]; ok {
				product = cached
			} else {
				product, err = d.GetProduct(*item.ProductID)
				if err != nil {
					return nil, nil, fmt.Errorf("build_incoming_invoice_gl_lines product: %w", err)
				}
				productCache[*item.ProductID] = product
			}
		}
		expenseAccountID, err := resolveExpenseAccount(product, org.DefaultExpenseAccountID)
		if err != nil {
			return nil, nil, err
		}
		if _, ok := expenseRats[expenseAccountID]; !ok {
			expenseRats[expenseAccountID] = new(big.Rat)
			expenseOrder = append(expenseOrder, expenseAccountID)
		}
		expenseRats[expenseAccountID].Add(expenseRats[expenseAccountID], lineCents)

		if item.TaxRate != nil && *item.TaxRate != "" {
			taxRate, ok := taxRateCache[*item.TaxRate]
			if !ok {
				taxRate, err = d.GetTaxRate(*item.TaxRate)
				if err != nil {
					return nil, nil, newValidationError("unknown tax rate on bill line item")
				}
				taxRateCache[*item.TaxRate] = taxRate
			}
			if _, ok := taxRats[*item.TaxRate]; !ok {
				if taxRate.InputTaxAccountID == nil {
					return nil, nil, newValidationError(
						"cannot post bill: tax rate %q has no input tax account configured", taxRate.Name,
					)
				}
				taxRats[*item.TaxRate] = new(big.Rat)
				taxOrder = append(taxOrder, *item.TaxRate)
				taxAccounts[*item.TaxRate] = *taxRate.InputTaxAccountID
			}
			taxRats[*item.TaxRate].Add(taxRats[*item.TaxRate], lineCents)
		}
	}

	taxAmountsDoc := map[string]int64{}
	for _, taxRateID := range taxOrder {
		taxRate := taxRateCache[taxRateID]
		pct, err := floatToRat(taxRate.Percentage)
		if err != nil {
			return nil, nil, newValidationError("invalid tax rate percentage")
		}
		tax := new(big.Rat).Mul(taxRats[taxRateID], pct)
		tax.Quo(tax, hundred)
		taxAmountsDoc[taxRateID] = roundHalfUp(tax, 0).Num().Int64()
	}
	expenseAmountsDoc := groupAndRound(expenseRats, expenseOrder)

	apFunctional := convert(bill.Total)
	expenseFunctional, _ := convertGroup(expenseAmountsDoc, expenseOrder, convert)
	taxFunctional, taxSum := convertGroup(taxAmountsDoc, taxOrder, convert)
	absorbResidual(expenseFunctional, expenseOrder, apFunctional-taxSum)

	// See buildInvoiceGLLines: a group netting to exactly zero must not emit
	// a row — journal_lines' CHECK((debit=0) <> (credit=0)) rejects it.
	var lines []CreateJournalLineRequest
	for _, accountID := range expenseOrder {
		if expenseFunctional[accountID] == 0 {
			continue
		}
		lines = append(lines, glLine(accountID, expenseFunctional[accountID], 0, bill.Currency, expenseAmountsDoc[accountID], bill.ExchangeRate, foreign, nil, nil, nil))
	}
	for _, taxRateID := range taxOrder {
		if taxFunctional[taxRateID] == 0 {
			continue
		}
		lines = append(lines, glLine(taxAccounts[taxRateID], taxFunctional[taxRateID], 0, bill.Currency, taxAmountsDoc[taxRateID], bill.ExchangeRate, foreign, nil, nil, &taxRateID))
	}
	lines = append(lines, glLine(*org.DefaultApAccountID, 0, apFunctional, bill.Currency, bill.Total, bill.ExchangeRate, foreign, nil, &bill.VendorID, nil))

	return lines, journal, nil
}

// glLine builds one journal line, populating the foreign-currency shadow
// columns (currency/foreignAmount/exchangeRate) only when foreign is true —
// debit/credit are always in functional-currency cents.
func glLine(
	accountID string, debit, credit int64,
	docCurrency string, docCents int64, exchangeRate *string, foreign bool,
	clientID, vendorID, taxRateID *string,
) CreateJournalLineRequest {
	line := CreateJournalLineRequest{
		AccountID: accountID,
		Debit:     debit,
		Credit:    credit,
		ClientID:  clientID,
		VendorID:  vendorID,
		TaxRateID: taxRateID,
	}
	if foreign {
		currency := docCurrency
		amount := docCents
		line.Currency = &currency
		line.ForeignAmount = &amount
		line.ExchangeRate = exchangeRate
	}
	return line
}

// --- Phase 7: inventory / COGS GL integration ---
//
// See docs/inventory-cogs-integration.md for the full design and
// /Users/mam/.claude/plans/model-nifty-sunrise.md (Phase 7) for the
// implementation plan this section follows.

// buildReceiptGRNILines posts Dr Inventory / Cr GRNI for a goods receipt's
// stock-enabled lines, at each line's own converted unitCost (already
// resolved by UpdateInboundDeliveryStatus's currency conversion before this
// is called — convertedUnitCosts is keyed by inboundStockLine.LineItemID).
// A line with no unitCost is skipped, not rejected: an uncosted receipt
// accrues nothing yet, the same "uncosted inflows stay out of the
// valuation pool" rule recomputeAverageCostTx applies — deliberately
// asymmetric with COGS on shipment, which 409s when no cost basis exists
// (see Design decision #5 in the plan). Since there's no per-product
// override for Inventory/GRNI (Design decision #6), every costed line
// collapses into a single Dr/Cr pair rather than a per-account grouping.
// Returns (nil, nil, nil) if every line was uncosted — the caller must
// then simply not post an entry, and treat "no entry exists" as a no-op
// on cancellation rather than an error.
func buildReceiptGRNILines(d *Database, receipt *InboundDelivery, lines []inboundStockLine, convertedUnitCosts map[string]int64) ([]CreateJournalLineRequest, *Journal, error) {
	org, err := d.GetOrganization(receipt.OrganizationID)
	if err != nil {
		return nil, nil, fmt.Errorf("build_receipt_grni_lines organization: %w", err)
	}

	var total int64
	for _, line := range lines {
		unitCost, ok := convertedUnitCosts[line.LineItemID]
		if !ok {
			continue
		}
		qty, err := floatToRat(line.Quantity)
		if err != nil {
			return nil, nil, newValidationError("line for %q: invalid quantity", line.ProductName)
		}
		cents := new(big.Rat).Mul(qty, new(big.Rat).SetInt64(unitCost))
		total += roundHalfUp(cents, 0).Num().Int64()
	}
	if total == 0 {
		return nil, nil, nil
	}

	if org.DefaultInventoryAccountID == nil {
		return nil, nil, newValidationError("cannot post goods receipt: organization has no default Inventory account configured")
	}
	if org.DefaultGRNIAccountID == nil {
		return nil, nil, newValidationError("cannot post goods receipt: organization has no default GRNI account configured")
	}

	journal, err := getJournalByTypeTx(d.DB, receipt.OrganizationID, "purchases")
	if err != nil {
		return nil, nil, err
	}

	lines2 := []CreateJournalLineRequest{
		{AccountID: *org.DefaultInventoryAccountID, Debit: total},
		{AccountID: *org.DefaultGRNIAccountID, Credit: total},
	}
	return lines2, journal, nil
}

// grniClearedQtyForPOLine sums quantity already cleared by bills OTHER than
// excludeBillID against purchase order line poLineID — filtered to
// state IN ('approved','paid'), not != 'cancelled' like
// MatchLine.PreviouslyInvoiced, because only a bill that actually reached
// GL posting has actually cleared anything against GRNI; a draft bill
// sitting in variance hasn't touched the ledger yet. excludeBillID may be
// "" (no exclusion — used by the cancel-guard call site in
// UpdateInboundDeliveryStatus, which isn't itself a bill).
func grniClearedQtyForPOLine(exec sqlSelectExecer, poLineID, excludeBillID string) (float64, error) {
	var rows []struct {
		Quantity float64 `db:"quantity"`
	}
	if err := exec.Select(&rows, `
		SELECT ili.quantity AS quantity
		FROM incoming_invoice_line_items ili
		JOIN incoming_invoices ii ON ii.id = ili.incomingInvoiceId
		WHERE ili.purchaseOrderLineItemId = ? AND ii.state IN ('approved', 'paid') AND ii.id != ?`,
		poLineID, excludeBillID,
	); err != nil {
		return 0, fmt.Errorf("grni_cleared_qty_for_po_line: %w", err)
	}
	var total float64
	for _, r := range rows {
		total += r.Quantity
	}
	return total, nil
}
