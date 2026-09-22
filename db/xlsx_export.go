package db

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
	// image.DecodeConfig (used by excelize's AddPictureFromBytes to size an
	// embedded image) only knows the formats whose decoders are linked in —
	// the blank imports register PNG/JPEG/GIF so an organization logo in any
	// of those common formats actually embeds rather than failing with
	// "image: unknown format".
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// lineItemMarker and taxLineMarker are the sentinels a template author places
// alone in a cell of the row that should repeat once per line item / per
// distinct tax rate. They're cleared from the output (never left as literal
// text) once located. logoMarker marks the cell an organization logo image is
// anchored to.
const (
	lineItemMarker = "{{#lineItems}}"
	taxLineMarker  = "{{#taxLines}}"
	logoMarker     = "{{organization.logo}}"
)

// logoScale shrinks an organization logo to a header-sized image. Excel
// anchors pictures at 96 DPI, so an unscaled logo would overflow the header
// band; a fixed scale keeps the common case (a few-hundred-pixel logo) sane
// without needing to know the image's dimensions.
const logoScale = 0.3

var placeholderPattern = regexp.MustCompile(`\{\{([a-zA-Z0-9_.]+)\}\}`)

// repeatBlock is one repeatable region of a template: the marker cell that
// says "repeat this row once per entry", the already-resolved placeholder map
// for each repetition, and whether the marker must be present. An optional
// block (the tax-rate breakdown on a document type that has none) is simply
// skipped when its marker is absent.
type repeatBlock struct {
	marker   string
	rows     []map[string]string
	required bool
}

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
	logo []byte,
	orientation string,
) ([]byte, []string, error) {
	currency := ""
	if org.Currency != nil {
		currency = *org.Currency
	}
	scalars := buildScalarPlaceholders(invoice, org, client)
	mergeExportMetaPlaceholders(scalars, org.DateFormat)
	lineRows := make([]map[string]string, len(lineItems))
	for i, li := range lineItems {
		lineRows[i] = buildLineItemPlaceholders(li, currency, org.MinimumFractionDigits, org.CountryCode, resolveTaxRatePercent(taxRates, li.TaxRate))
	}
	taxRows := buildTaxBreakdownRows(lineItems, taxRates, currency, org.MinimumFractionDigits, org.CountryCode, invoice.DiscountAmount)
	blocks := []repeatBlock{
		{marker: lineItemMarker, rows: lineRows, required: true},
		{marker: taxLineMarker, rows: taxRows},
	}
	return fillTemplate(templateBytes, scalars, blocks, logo, orientation)
}

// mergeExportMetaPlaceholders adds placeholders describing the export
// operation itself — when this specific file was generated, not any
// business-document date field — into an already-built scalars map. Called
// from every FillXTemplate wrapper (not from the shared fillTemplate engine
// below, which deliberately knows nothing about organizations or date
// formats) so every document type gets these for free with no per-type
// duplication of the actual date/time formatting. Useful for a "Printed on
// ..." footer note, or simply to tell two exports of the same document
// apart when it's re-exported later than it was created — export.generatedDate
// follows the organization's own date_format for the same look-consistency
// reason every other customer-facing date on these templates does;
// export.generatedTime is always 24-hour HH:MM server time (organizations
// have no separate time_format setting to follow).
//
// "Current page" / "total pages" are deliberately NOT offered here as a
// {{}} placeholder: fillTemplate runs before LibreOffice ever paginates the
// output, so no page count is knowable yet at substitution time — a
// {{page.number}} placeholder could only ever resolve to a fabricated
// constant. The true equivalent is Excel/LibreOffice's own native "&P" (page
// number) / "&N" (total pages) codes, which only work inside the sheet's own
// Page Layout ▸ Header/Footer — applyPageFooter (db/templates/gen/styles.go)
// already wires "&CPage &P of &N" into every embedded default template's
// footer for exactly this; a template author can move or restyle that from
// within Excel/LibreOffice's own header/footer editor.
func mergeExportMetaPlaceholders(scalars map[string]string, dateFormat *string) {
	now := time.Now()
	scalars["export.generatedDate"] = formatOrgDate(now.UnixMilli(), dateFormat)
	scalars["export.generatedTime"] = now.Format("15:04")
}

// fillTemplate is the document-type-agnostic engine every FillXTemplate
// function (FillInvoiceTemplate, FillPurchaseOrderTemplate, …) is a thin
// wrapper around: it knows nothing about invoices, purchase orders, or any
// other domain type — only "scalars" (resolved once) and "lineRows" (one
// placeholder map per repeated row, already resolved by the caller). This
// split exists so adding a new exportable document type only ever means
// writing that type's own scalar/line-item placeholder builders, never
// touching the marker-expansion/substitution/sheet-stripping mechanics below.
//
// orientation is "portrait", "landscape", or "" — "" means no org override
// exists (db.GetDocumentTemplateOrientation's no-row case) and the
// template's own authored page setup is left completely untouched, so
// every organization that has never visited the orientation setting gets
// byte-identical output to before this parameter existed. A non-empty
// value always wins over whatever the source template (embedded default or
// an org's own upload) itself specifies — see the doc comment on
// db.SetDocumentTemplateOrientation for why that's the deliberate
// direction, not just a default.
func fillTemplate(templateBytes []byte, scalars map[string]string, blocks []repeatBlock, logo []byte, orientation string) ([]byte, []string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(templateBytes))
	if err != nil {
		return nil, nil, fmt.Errorf("fill_template: open: %w", err)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	if sheet == "" {
		return nil, nil, fmt.Errorf("fill_template: template has no sheets")
	}

	// Expand every repeat block, re-locating each marker every time: an
	// earlier block's expansion shifts every row below it, so a position
	// captured before that can't be trusted for a later block. Each block
	// records the placeholder map for every row it produced, so the single
	// substitution pass below can look one up by row number.
	rowPlaceholders := map[int]map[string]string{}
	for _, block := range blocks {
		markerRow, markerCol, found, err := findMarkerRow(f, sheet, block.marker)
		if err != nil {
			return nil, nil, err
		}
		if !found {
			if block.required {
				return nil, nil, newValidationError("template has no %s marker cell", block.marker)
			}
			continue
		}

		n := len(block.rows)
		for i := 1; i < n; i++ {
			if err := f.DuplicateRowTo(sheet, markerRow, markerRow+i); err != nil {
				return nil, nil, fmt.Errorf("fill_template: duplicate row: %w", err)
			}
		}
		count := n
		if count == 0 {
			count = 1 // keep the lone template row; its per-item placeholders stay unresolved below
		}
		for i := 0; i < count; i++ {
			cell, err := excelize.CoordinatesToCellName(markerCol, markerRow+i)
			if err != nil {
				return nil, nil, fmt.Errorf("fill_template: %w", err)
			}
			if err := f.SetCellStr(sheet, cell, ""); err != nil {
				return nil, nil, fmt.Errorf("fill_template: clear marker: %w", err)
			}
			if i < n {
				rowPlaceholders[markerRow+i] = block.rows[i]
			}
		}
	}

	unresolvedSet := map[string]bool{}
	logoCell := ""

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
		liPlaceholders := rowPlaceholders[rowNum]

		for cIdx, cellVal := range rowVals {
			if !strings.Contains(cellVal, "{{") {
				continue
			}
			colName, err := excelize.ColumnNumberToName(cIdx + 1)
			if err != nil {
				return nil, nil, fmt.Errorf("fill_template: %w", err)
			}
			cellRef := colName + strconv.Itoa(rowNum)

			// A cell holding exactly the logo marker is the image anchor, not
			// text: remember it and clear the cell; the picture is inserted
			// after the substitution pass (an AddPicture during the row scan
			// would be clobbered by nothing here, but keeping it out of the
			// scan keeps this loop purely about strings).
			if strings.TrimSpace(cellVal) == logoMarker {
				logoCell = cellRef
				if err := f.SetCellStr(sheet, cellRef, ""); err != nil {
					return nil, nil, fmt.Errorf("fill_template: clear logo cell: %w", err)
				}
				continue
			}

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

	if logoCell != "" && len(logo) > 0 {
		if ext, ok := imageExtension(logo); ok {
			printObject := true
			pic := &excelize.Picture{
				Extension: ext,
				File:      logo,
				Format: &excelize.GraphicOptions{
					ScaleX:          logoScale,
					ScaleY:          logoScale,
					LockAspectRatio: true,
					PrintObject:     &printObject,
					Positioning:     "oneCell",
				},
			}
			if err := f.AddPictureFromBytes(sheet, logoCell, pic); err != nil {
				return nil, nil, fmt.Errorf("fill_template: add logo: %w", err)
			}
		}
	}

	if orientation != "" {
		if err := f.SetPageLayout(sheet, &excelize.PageLayoutOptions{Orientation: &orientation}); err != nil {
			return nil, nil, fmt.Errorf("fill_template: set orientation: %w", err)
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

// imageExtension maps an organization logo's bytes to the extension excelize
// expects for AddPictureFromBytes, or false for a format it can't embed
// (webp, or a non-image upload). Detection is by content, not a stored
// filename — the logo endpoint stores raw bytes with no name.
func imageExtension(data []byte) (string, bool) {
	switch http.DetectContentType(data) {
	case "image/png":
		return ".png", true
	case "image/jpeg":
		return ".jpeg", true
	case "image/gif":
		return ".gif", true
	case "image/bmp":
		return ".bmp", true
	case "image/svg+xml":
		return ".svg", true
	default:
		return "", false
	}
}

// FetchInvoiceExportData gathers everything FillInvoiceTemplate needs for one
// invoice: the invoice itself, its line items, the owning organization and
// client, the org's resolved template bytes (override or embedded default),
// a map of every distinct tax rate referenced by a line item (one query per
// distinct rate, not per line), and the org's orientation override for this
// document type ("" if none — see db.GetDocumentTemplateOrientation).
// Callers hold dbMu only around this call — see
// api/document_templates.go's exportInvoiceDocument for why the
// fill/convert step that follows must run lock-free.
func (d *Database) FetchInvoiceExportData(invoiceID string) (*Invoice, []InvoiceLineItem, *Organization, *Client, []byte, map[string]TaxRate, []byte, string, error) {
	fail := func(err error) (*Invoice, []InvoiceLineItem, *Organization, *Client, []byte, map[string]TaxRate, []byte, string, error) {
		return nil, nil, nil, nil, nil, nil, nil, "", err
	}
	invoice, err := d.GetInvoice(invoiceID)
	if err != nil {
		return fail(fmt.Errorf("fetch_invoice_export_data: get invoice: %w", err))
	}
	lineItems, err := d.GetInvoiceLineItems(invoiceID)
	if err != nil {
		return fail(fmt.Errorf("fetch_invoice_export_data: get line items: %w", err))
	}
	org, err := d.GetOrganization(invoice.OrganizationID)
	if err != nil {
		return fail(fmt.Errorf("fetch_invoice_export_data: get organization: %w", err))
	}
	client, err := d.GetClient(invoice.ClientID)
	if err != nil {
		return fail(fmt.Errorf("fetch_invoice_export_data: get client: %w", err))
	}
	templateBytes, _, err := resolveTemplateBytes(d, invoice.OrganizationID, "invoice")
	if err != nil {
		return fail(fmt.Errorf("fetch_invoice_export_data: resolve template: %w", err))
	}
	orientation, err := d.GetDocumentTemplateOrientation(invoice.OrganizationID, "invoice")
	if err != nil {
		return fail(fmt.Errorf("fetch_invoice_export_data: resolve orientation: %w", err))
	}
	// A missing logo is not an error — the template's logo cell just stays
	// empty (GetOrganizationLogo returns nil, nil for an org that never
	// uploaded one).
	logo, err := d.GetOrganizationLogo(invoice.OrganizationID)
	if err != nil {
		return fail(fmt.Errorf("fetch_invoice_export_data: get logo: %w", err))
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

	return invoice, lineItems, org, client, templateBytes, taxRates, logo, orientation, nil
}

// findMarkerRow scans every cell for the given repeat marker, returning its
// 1-indexed row and column. The marker isn't pinned to column A: a template
// author can place it in whichever column is otherwise unused (e.g. off to
// the right of the visible columns), freeing the leftmost columns of the
// repeat row for real content like a merged multi-column description cell.
// found is false when the template simply doesn't carry this marker (an
// optional block) — only a read failure returns an error.
func findMarkerRow(f *excelize.File, sheet, marker string) (row int, col int, found bool, err error) {
	rows, err := f.GetRows(sheet)
	if err != nil {
		return 0, 0, false, fmt.Errorf("find_marker_row: %w", err)
	}
	for i, r := range rows {
		for j, cell := range r {
			if strings.TrimSpace(cell) == marker {
				return i + 1, j + 1, true, nil
			}
		}
	}
	return 0, 0, false, nil
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

	// One Go-computed line rather than two placeholders embedded in static
	// template text ("Withholding tax ({{rate}}): {{amount}}") — rate/amount
	// are nullable and blank for every organization that doesn't use this
	// Tunisia-specific feature (virtually all of them), which left a
	// broken-looking "Withholding tax (): " on every invoice PDF. Blank
	// entirely, not just the values, when unset.
	withholdingTaxLine := ""
	if invoice.WithholdingTaxRate != nil && invoice.WithholdingTaxAmount != nil {
		rate := strconv.FormatFloat(*invoice.WithholdingTaxRate, 'f', -1, 64)
		amount := formatMoneyCents(*invoice.WithholdingTaxAmount, currency, org.MinimumFractionDigits, org.CountryCode)
		withholdingTaxLine = fmt.Sprintf("Withholding tax (%s%%): %s", rate, amount)
	}

	// Net taxable base = gross subtotal − discount, in cents. Always
	// non-negative: validateInvoiceTotals rejects a discount exceeding the
	// subtotal.
	netTaxable := invoice.SubTotal - invoice.DiscountAmount
	if netTaxable < 0 {
		netTaxable = 0
	}

	return map[string]string{
		"invoice.number":   invoice.Number,
		"invoice.date":     formatOrgDate(invoice.Date, org.DateFormat),
		"invoice.dueDate":  formatOptionalOrgDate(invoice.DueDate, org.DateFormat),
		"invoice.currency": currency,
		"invoice.subTotal": formatMoneyCents(invoice.SubTotal, currency, org.MinimumFractionDigits, org.CountryCode),
		"invoice.taxTotal": formatMoneyCents(invoice.TaxTotal, currency, org.MinimumFractionDigits, org.CountryCode),
		"invoice.total":    formatMoneyCents(invoice.Total, currency, org.MinimumFractionDigits, org.CountryCode),
		// Remise: the discount amount and the resulting pre-tax base
		// ("Total Brut HTVA" / "Total Net HTVA" on the Tunisian layout).
		"invoice.discountAmount": formatMoneyCents(invoice.DiscountAmount, currency, org.MinimumFractionDigits, org.CountryCode),
		"invoice.netTaxable":     formatMoneyCents(netTaxable, currency, org.MinimumFractionDigits, org.CountryCode),
		"invoice.buyerReference": derefString(invoice.BuyerReference),
		"invoice.paymentTerms":   derefString(invoice.PaymentTerms),
		// Tunisia invoice support (see db/invoice.go's own comment on these
		// three fields) — fiscalStampAmount is NOT NULL DEFAULT 0, so this is
		// always a real amount, "0.00 EUR" for an organization that doesn't
		// use it, same as every other always-present label on this template.
		"invoice.fiscalStampAmount":  formatMoneyCents(invoice.FiscalStampAmount, currency, org.MinimumFractionDigits, org.CountryCode),
		"invoice.withholdingTaxLine": withholdingTaxLine,
		// The whole "Arrêtée la présente facture à la somme de …" sentence,
		// blank when the organization hasn't enabled the Formatting toggle
		// (db/amount_in_words.go) — same blank-when-unset shape as
		// withholdingTaxLine, so the template line disappears cleanly.
		"invoice.amountInWords": amountInWordsLine(invoice.Total, currency, org.AmountInWordsEnabled != nil && *org.AmountInWordsEnabled != 0),

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

		"client.name":           derefString(client.Name),
		"client.vatin":          derefString(client.Vatin),
		"client.identityNumber": derefString(client.IdentityNumber),
		"client.address":        derefString(client.Address),
		"client.email":          firstEmail(client.Emails),
		"client.phone":          derefString(client.Phone),
		"client.phone2":         derefString(client.Phone2),
		"client.phone3":         derefString(client.Phone3),
		"client.street":         derefString(client.Street),
		"client.houseNumber":    derefString(client.HouseNumber),
		"client.postalCode":     derefString(client.PostalCode),
		"client.city":           derefString(client.City),
	}
}

// buildLineItemPlaceholders is the per-row namespace for the repeated line
// item block.
func buildLineItemPlaceholders(li InvoiceLineItem, currency string, minimumFractionDigits *int64, countryCode *string, taxRatePercent string) map[string]string {
	lineTotal := lineTotalCents(li.Quantity, li.UnitPrice)
	return map[string]string{
		"lineItems.sku":         derefString(li.SKU),
		"lineItems.description": derefString(li.Description),
		"lineItems.quantity":    formatQuantity(li.Quantity),
		"lineItems.unitPrice":   formatMoneyCents(li.UnitPrice, currency, minimumFractionDigits, countryCode),
		"lineItems.taxRate":     taxRatePercent,
		"lineItems.lineTotal":   formatMoneyCents(lineTotal, currency, minimumFractionDigits, countryCode),
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

// buildTaxBreakdownRows resolves the per-tax-rate recap table the Tunisian
// layout prints ("Taux TVA % | Base TVA | Montant TVA ..."). One row per
// distinct rate that appears on a line item, in first-seen order, so the
// template's {{#taxLines}} block expands to exactly the rates in use. The
// base is net of the proportional discount share — the same allocation
// validateInvoiceTotals (db/invoice_totals.go) and buildInvoiceGLLines
// (db/gl_posting.go) use — and the tax is rounded once per group, matching
// those two.
func buildTaxBreakdownRows(
	lineItems []InvoiceLineItem,
	taxRates map[string]TaxRate,
	currency string,
	minimumFractionDigits *int64,
	countryCode *string,
	discountAmount int64,
) []map[string]string {
	groupSubtotals := map[string]*big.Rat{}
	var order []string
	totalRat := new(big.Rat)
	for _, li := range lineItems {
		lineCents := new(big.Rat).SetInt64(lineTotalCents(li.Quantity, li.UnitPrice))
		totalRat.Add(totalRat, lineCents)
		if li.TaxRate == nil || *li.TaxRate == "" {
			continue
		}
		if _, ok := groupSubtotals[*li.TaxRate]; !ok {
			groupSubtotals[*li.TaxRate] = new(big.Rat)
			order = append(order, *li.TaxRate)
		}
		groupSubtotals[*li.TaxRate].Add(groupSubtotals[*li.TaxRate], lineCents)
	}

	discountRat := new(big.Rat).SetInt64(discountAmount)
	rows := make([]map[string]string, 0, len(order))
	for _, id := range order {
		netBase := new(big.Rat).Set(groupSubtotals[id])
		if totalRat.Sign() > 0 {
			share := new(big.Rat).Mul(discountRat, groupSubtotals[id])
			share.Quo(share, totalRat)
			netBase.Sub(netBase, share)
		}
		baseCents := roundHalfUp(netBase, 0).Num().Int64()

		percent := ""
		taxCents := int64(0)
		if rate, ok := taxRates[id]; ok {
			percent = strconv.FormatFloat(rate.Percentage, 'f', -1, 64) + "%"
			pct, err := floatToRat(rate.Percentage)
			if err == nil {
				tax := new(big.Rat).Mul(netBase, pct)
				tax.Quo(tax, hundred)
				taxCents = roundHalfUp(tax, 0).Num().Int64()
			}
		}
		rows = append(rows, map[string]string{
			"taxLines.rate":   percent,
			"taxLines.base":   formatMoneyCents(baseCents, currency, minimumFractionDigits, countryCode),
			"taxLines.amount": formatMoneyCents(taxCents, currency, minimumFractionDigits, countryCode),
		})
	}
	return rows
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
