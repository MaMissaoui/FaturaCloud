package db

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// lineItemMarker is the sentinel a template author places alone in column A
// of the row that should repeat once per invoice line item. It's cleared
// from the output (never left as literal text) once located.
const lineItemMarker = "{{#lineItems}}"

var placeholderPattern = regexp.MustCompile(`\{\{([a-zA-Z0-9_.]+)\}\}`)

// FillInvoiceTemplate fills an uploaded or embedded-default invoice template
// with an invoice's already-validated data, expanding the {{#lineItems}}
// marker row into one row per line item. Returns the filled workbook bytes
// and the list of placeholders that had no known value (never an error on
// their own — a typo in a user-edited template must not block export
// entirely). err is reserved for structural failures: an unreadable
// workbook, or no marker row found at all.
func FillInvoiceTemplate(
	templateBytes []byte,
	invoice Invoice,
	lineItems []InvoiceLineItem,
	org Organization,
	client Client,
	taxRates map[string]TaxRate,
) ([]byte, []string, error) {
	currency := ""
	if org.Currency != nil {
		currency = *org.Currency
	}
	scalars := buildScalarPlaceholders(invoice, org, client)
	lineRows := make([]map[string]string, len(lineItems))
	for i, li := range lineItems {
		lineRows[i] = buildLineItemPlaceholders(li, currency, org.MinimumFractionDigits, resolveTaxRatePercent(taxRates, li.TaxRate))
	}
	return fillTemplate(templateBytes, scalars, lineRows)
}

// fillTemplate is the document-type-agnostic engine every FillXTemplate
// function (FillInvoiceTemplate, FillPurchaseOrderTemplate, …) is a thin
// wrapper around: it knows nothing about invoices, purchase orders, or any
// other domain type — only "scalars" (resolved once) and "lineRows" (one
// placeholder map per repeated row, already resolved by the caller). This
// split exists so adding a new exportable document type only ever means
// writing that type's own scalar/line-item placeholder builders, never
// touching the marker-expansion/substitution/sheet-stripping mechanics below.
func fillTemplate(templateBytes []byte, scalars map[string]string, lineRows []map[string]string) ([]byte, []string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(templateBytes))
	if err != nil {
		return nil, nil, fmt.Errorf("fill_template: open: %w", err)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	if sheet == "" {
		return nil, nil, fmt.Errorf("fill_template: template has no sheets")
	}

	markerRow, markerCol, err := findMarkerRow(f, sheet)
	if err != nil {
		return nil, nil, err
	}

	n := len(lineRows)
	if n > 1 {
		for i := 1; i < n; i++ {
			if err := f.DuplicateRowTo(sheet, markerRow, markerRow+i); err != nil {
				return nil, nil, fmt.Errorf("fill_template: duplicate line item row: %w", err)
			}
		}
	}
	repeatRowCount := n
	if repeatRowCount == 0 {
		repeatRowCount = 1 // keep the lone template row; its per-item placeholders stay unresolved below
	}

	// Clear the marker cell in every row of the (now expanded) repeat block
	// so it never appears literally in the output.
	for i := 0; i < repeatRowCount; i++ {
		cell, err := excelize.CoordinatesToCellName(markerCol, markerRow+i)
		if err != nil {
			return nil, nil, fmt.Errorf("fill_template: %w", err)
		}
		if err := f.SetCellStr(sheet, cell, ""); err != nil {
			return nil, nil, fmt.Errorf("fill_template: clear marker: %w", err)
		}
	}

	unresolvedSet := map[string]bool{}

	// Read rows AFTER the duplication above, not before — DuplicateRowTo
	// shifts every row below the repeat block, so any totals/footer row is
	// only ever located here, by its own placeholder content, never by a
	// row index captured earlier.
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, nil, fmt.Errorf("fill_template: read rows: %w", err)
	}

	for rIdx, rowVals := range rows {
		rowNum := rIdx + 1 // excelize sheet rows are 1-indexed; GetRows is a 0-indexed slice
		var liPlaceholders map[string]string
		if n > 0 && rowNum >= markerRow && rowNum < markerRow+repeatRowCount {
			liPlaceholders = lineRows[rowNum-markerRow]
		}

		for cIdx, cellVal := range rowVals {
			if !strings.Contains(cellVal, "{{") {
				continue
			}
			colName, err := excelize.ColumnNumberToName(cIdx + 1)
			if err != nil {
				return nil, nil, fmt.Errorf("fill_template: %w", err)
			}
			cellRef := colName + strconv.Itoa(rowNum)

			newVal := placeholderPattern.ReplaceAllStringFunc(cellVal, func(match string) string {
				key := match[2 : len(match)-2] // strip the surrounding {{ }}
				if liPlaceholders != nil {
					if v, ok := liPlaceholders[key]; ok {
						return v
					}
				}
				if v, ok := scalars[key]; ok {
					return v
				}
				unresolvedSet[key] = true
				return ""
			})
			if newVal != cellVal {
				if err := f.SetCellStr(sheet, cellRef, newVal); err != nil {
					return nil, nil, fmt.Errorf("fill_template: set cell %s: %w", cellRef, err)
				}
			}
		}
	}

	// Strip every sheet but the content sheet before returning. A template
	// author may keep a reference/notes sheet alongside the content sheet
	// (the embedded default's "Available fields" tab documents every
	// placeholder this way) — useful while editing the template, but never
	// meant to ride along into a document actually sent to a customer.
	for _, name := range f.GetSheetList() {
		if name == sheet {
			continue
		}
		if err := f.DeleteSheet(name); err != nil {
			return nil, nil, fmt.Errorf("fill_template: delete extra sheet %q: %w", name, err)
		}
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, nil, fmt.Errorf("fill_template: write: %w", err)
	}

	unresolved := make([]string, 0, len(unresolvedSet))
	for k := range unresolvedSet {
		unresolved = append(unresolved, k)
	}
	sort.Strings(unresolved)

	return buf.Bytes(), unresolved, nil
}

// FetchInvoiceExportData gathers everything FillInvoiceTemplate needs for one
// invoice: the invoice itself, its line items, the owning organization and
// client, the org's resolved template bytes (override or embedded default),
// and a map of every distinct tax rate referenced by a line item (one query
// per distinct rate, not per line). Callers hold dbMu only around this call —
// see api/document_templates.go's exportInvoiceDocument for why the
// fill/convert step that follows must run lock-free.
func (d *Database) FetchInvoiceExportData(invoiceID string) (*Invoice, []InvoiceLineItem, *Organization, *Client, []byte, map[string]TaxRate, error) {
	invoice, err := d.GetInvoice(invoiceID)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("fetch_invoice_export_data: get invoice: %w", err)
	}
	lineItems, err := d.GetInvoiceLineItems(invoiceID)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("fetch_invoice_export_data: get line items: %w", err)
	}
	org, err := d.GetOrganization(invoice.OrganizationID)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("fetch_invoice_export_data: get organization: %w", err)
	}
	client, err := d.GetClient(invoice.ClientID)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("fetch_invoice_export_data: get client: %w", err)
	}
	templateBytes, _, err := resolveTemplateBytes(d, invoice.OrganizationID, "invoice")
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("fetch_invoice_export_data: resolve template: %w", err)
	}

	taxRates := map[string]TaxRate{}
	for _, li := range lineItems {
		if li.TaxRate == nil || *li.TaxRate == "" {
			continue
		}
		if _, ok := taxRates[*li.TaxRate]; ok {
			continue
		}
		rate, err := d.GetTaxRate(*li.TaxRate)
		if err != nil {
			continue // an unresolvable tax rate just leaves lineItems.taxRate blank, not a failed export
		}
		taxRates[*li.TaxRate] = *rate
	}

	return invoice, lineItems, org, client, templateBytes, taxRates, nil
}

// findMarkerRow scans every cell for the lineItemMarker, returning its
// 1-indexed row and column. This is the one structural requirement of a
// valid template — without it there's nothing to expand line items into.
// The marker isn't pinned to column A: a template author can place it in
// whichever column is otherwise unused (e.g. off to the right of the visible
// columns), freeing the leftmost columns of the repeat row for real content
// like a merged multi-column description cell.
func findMarkerRow(f *excelize.File, sheet string) (row int, col int, err error) {
	rows, err := f.GetRows(sheet)
	if err != nil {
		return 0, 0, fmt.Errorf("find_marker_row: %w", err)
	}
	for i, r := range rows {
		for j, cell := range r {
			if strings.TrimSpace(cell) == lineItemMarker {
				return i + 1, j + 1, nil
			}
		}
	}
	return 0, 0, newValidationError("template has no %s marker cell", lineItemMarker)
}

// buildScalarPlaceholders is the fixed namespace->field allowlist for
// invoice/organization/client values — a plain map keyed by
// "namespace.field", not reflection, so an unknown placeholder is a lookup
// miss rather than a panic.
func buildScalarPlaceholders(invoice Invoice, org Organization, client Client) map[string]string {
	currency := ""
	if org.Currency != nil {
		currency = *org.Currency
	}

	withholdingTaxRate := ""
	if invoice.WithholdingTaxRate != nil {
		withholdingTaxRate = strconv.FormatFloat(*invoice.WithholdingTaxRate, 'f', -1, 64) + "%"
	}
	withholdingTaxAmount := ""
	if invoice.WithholdingTaxAmount != nil {
		withholdingTaxAmount = formatMoneyCents(*invoice.WithholdingTaxAmount, currency, org.MinimumFractionDigits)
	}

	return map[string]string{
		"invoice.number":         invoice.Number,
		"invoice.date":           formatOrgDate(invoice.Date, org.DateFormat),
		"invoice.dueDate":        formatOptionalOrgDate(invoice.DueDate, org.DateFormat),
		"invoice.currency":       currency,
		"invoice.subTotal":       formatMoneyCents(invoice.SubTotal, currency, org.MinimumFractionDigits),
		"invoice.taxTotal":       formatMoneyCents(invoice.TaxTotal, currency, org.MinimumFractionDigits),
		"invoice.total":          formatMoneyCents(invoice.Total, currency, org.MinimumFractionDigits),
		"invoice.buyerReference": derefString(invoice.BuyerReference),
		"invoice.paymentTerms":   derefString(invoice.PaymentTerms),
		// Tunisia invoice support (see db/invoice.go's own comment on these
		// three fields) — fiscalStampAmount is NOT NULL DEFAULT 0, so this is
		// always a real amount, "0.00 EUR" for an organization that doesn't
		// use it, same as every other always-present label on this template.
		"invoice.fiscalStampAmount":    formatMoneyCents(invoice.FiscalStampAmount, currency, org.MinimumFractionDigits),
		"invoice.withholdingTaxRate":   withholdingTaxRate,
		"invoice.withholdingTaxAmount": withholdingTaxAmount,

		"organization.name":        derefString(org.Name),
		"organization.vatin":       derefString(org.Vatin),
		"organization.email":       derefString(org.Email),
		"organization.phone":       derefString(org.Phone),
		"organization.website":     derefString(org.Website),
		"organization.iban":        derefString(org.IBAN),
		"organization.bankName":    derefString(org.BankName),
		"organization.street":      derefString(org.Street),
		"organization.houseNumber": derefString(org.HouseNumber),
		"organization.postalCode":  derefString(org.PostalCode),
		"organization.city":        derefString(org.City),

		"client.name":        derefString(client.Name),
		"client.vatin":       derefString(client.Vatin),
		"client.email":       firstEmail(client.Emails),
		"client.phone":       derefString(client.Phone),
		"client.street":      derefString(client.Street),
		"client.houseNumber": derefString(client.HouseNumber),
		"client.postalCode":  derefString(client.PostalCode),
		"client.city":        derefString(client.City),
	}
}

// buildLineItemPlaceholders is the per-row namespace for the repeated line
// item block.
func buildLineItemPlaceholders(li InvoiceLineItem, currency string, minimumFractionDigits *int64, taxRatePercent string) map[string]string {
	lineTotal := lineTotalCents(li.Quantity, li.UnitPrice)
	return map[string]string{
		"lineItems.description": derefString(li.Description),
		"lineItems.quantity":    formatQuantity(li.Quantity),
		"lineItems.unitPrice":   formatMoneyCents(li.UnitPrice, currency, minimumFractionDigits),
		"lineItems.taxRate":     taxRatePercent,
		"lineItems.lineTotal":   formatMoneyCents(lineTotal, currency, minimumFractionDigits),
	}
}

// resolveTaxRatePercent looks up a line item's tax rate percentage (already
// fetched into taxRates by the caller — one query per distinct rate id on
// the invoice, not per line) and formats it as "19.5%"; a nil/unknown rate
// resolves to "".
func resolveTaxRatePercent(taxRates map[string]TaxRate, taxRateID *string) string {
	if taxRateID == nil {
		return ""
	}
	rate, ok := taxRates[*taxRateID]
	if !ok {
		return ""
	}
	return strconv.FormatFloat(rate.Percentage, 'f', -1, 64) + "%"
}

// lineTotalCents computes a line item's total in cents using the same
// exact-rational idiom validateInvoiceTotals uses for its own line totals
// (db/invoice_totals.go), never a float64 multiplication — unitPrice is
// already in cents, so this is quantity * unitPrice rounded half up, not the
// two-step units-then-cents conversion validateInvoiceTotals needs for its
// summed subtotal.
func lineTotalCents(quantity float64, unitPriceCents int64) int64 {
	qty, err := floatToRat(quantity)
	if err != nil {
		return 0
	}
	total := new(big.Rat).Mul(qty, new(big.Rat).SetInt64(unitPriceCents))
	rounded := roundHalfUp(total, 0)
	return rounded.Num().Int64()
}

// firstEmail parses clients.emails' stored JSON-array-of-strings form (see
// CLAUDE.md's note on src/atoms/client.ts's emails conversion) and returns
// the first address, or "" if unset/unparseable/empty.
func firstEmail(raw *string) string {
	if raw == nil || *raw == "" {
		return ""
	}
	var emails []string
	if err := json.Unmarshal([]byte(*raw), &emails); err != nil || len(emails) == 0 {
		return ""
	}
	return emails[0]
}
