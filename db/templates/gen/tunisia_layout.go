package main

import (
	"strconv"

	"github.com/xuri/excelize/v2"
)

// tunisiaLayoutSpec parameterizes the shared Tunisian document layout
// (buildTunisiaLayout) for every document type. The layout is the same
// "Facture" frame the invoice generator produces — seller block with a logo
// cell, boxed buyer block, boxed title, line-item table, optional per-rate
// VAT recap beside the totals block, amount-in-words, signature area,
// banking footer — with per-type labels, placeholders and which blocks are
// present. Keeping it parameterized (rather than six hand-copied layouts) is
// what keeps a future layout tweak to one place.
type tunisiaLayoutSpec struct {
	sheet string
	title string // e.g. "Facture N°: {{invoice.number}}"
	date  string // e.g. "Date : {{invoice.date}}"
	// partyName is the second party block's name placeholder (e.g.
	// "{{client.name}}"); partyFields are the lines below it. Both omitted
	// together when there is no second party to show.
	partyName   string
	partyFields []string
	columns     []tableColumn
	// totals are the label/amount rows of the totals block ("Total Brut
	// HTVA", "Remise", ...). A nil slice omits the block (delivery notes).
	totals []totalRow
	// hasVatRecap adds the {{#taxLines}} table (invoices, bills, orders with
	// tax). Delivery notes and goods receipts have no prices, so no recap.
	hasVatRecap bool
	// amountInWords is the placeholder for the sentence line, or "" to omit.
	amountInWords string
	// secondPartyLabel is the heading above the buyer block's name ("Client",
	// "Fournisseur", ...) — omitted when empty.
	secondPartyLabel string
	// footerLeft/footerRight are the banking footer placeholders.
	footerLeft  string
	footerRight string
}

// tableColumn is one visible column of the line-item table: the label, the
// placeholder, and whether the column should be right-aligned (numbers).
type tableColumn struct {
	label       string
	placeholder string
	right       bool
}

// totalRow is one label/amount pair in the totals block.
type totalRow struct {
	label  string
	amount string
	bold   bool
}

// buildTunisiaLayout writes the shared layout onto a fresh workbook and
// returns it with the content sheet active. The caller adds its own field
// reference sheet and saves.
func buildTunisiaLayout(spec tunisiaLayoutSpec) *excelize.File {
	f := excelize.NewFile()
	sheet := spec.sheet
	f.SetSheetName(f.GetSheetName(0), sheet)
	styles := newTemplateStyles(f)

	set := func(cell, value string) { _ = f.SetCellStr(sheet, cell, value) }
	style := func(cell string, s int) { f.SetCellStyle(sheet, cell, cell, s) }

	// Seller block (top-left). Logo anchored to A1, address below it.
	set("A1", "{{organization.logo}}")
	set("A3", "{{organization.name}}")
	style("A3", styles.bold)
	set("A4", "{{organization.street}} {{organization.houseNumber}}")
	set("A5", "{{organization.postalCode}} {{organization.city}}")
	set("A6", "Tél : {{organization.phone}}")
	set("A7", "MF : {{organization.vatin}}")

	// Boxed second-party (buyer/vendor) block (top-right).
	if spec.partyName != "" {
		set("C1", spec.partyName)
		style("C1", styles.bold)
		mergeCell(f, sheet, "C1", "F1")
		for i, line := range spec.partyFields {
			cell := "C" + strconv.Itoa(i+2)
			set(cell, line)
			mergeCell(f, sheet, cell, "F"+strconv.Itoa(i+2))
		}
		for i := 0; i <= len(spec.partyFields)+1; i++ {
			for _, c := range []string{"C", "D", "E", "F"} {
				style(c+strconv.Itoa(i+1), styles.boxed)
			}
		}
	}

	// Boxed title.
	set("C8", spec.title)
	style("C8", styles.titleSmall)
	mergeCell(f, sheet, "C8", "F8")
	set("D9", spec.date)
	style("D9", styles.center)
	mergeCell(f, sheet, "D9", "F9")
	for _, c := range []string{"C8", "D8", "E8", "F8", "C9", "D9", "E9", "F9"} {
		style(c, styles.boxed)
	}

	headerRow := 11
	// Columns are laid out A..; the last visible column is tracked so the
	// recap/totals blocks can be positioned after it.
	colLetters := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
	// Wide rows (amount-in-words, signature, footer) span to at least G so
	// they look like a full-width band even on a short table (a delivery note
	// has only four visible columns).
	wideCol := "G"
	if len(spec.columns) > 7 {
		wideCol = colLetters[len(spec.columns)-1]
	}

	for i, col := range spec.columns {
		letter := colLetters[i]
		set(letter+strconv.Itoa(headerRow), col.label)
		style(letter+strconv.Itoa(headerRow), styles.header)
	}

	repeatRow := headerRow + 1
	for i, col := range spec.columns {
		letter := colLetters[i]
		set(letter+strconv.Itoa(repeatRow), col.placeholder)
		style(letter+strconv.Itoa(repeatRow), styles.boxed)
		if col.right {
			style(letter+strconv.Itoa(repeatRow), styles.right)
		}
	}
	// Markers live in a fixed column well to the right of every visible
	// column (the widest layout uses A..G, so J is always clear). A shared
	// marker column keeps the two repeat blocks' markers from colliding with
	// the visible totals/recap columns.
	const markerCol = "J"
	set(markerCol+strconv.Itoa(repeatRow), "{{#lineItems}}")

	// VAT recap on the left, totals block on the right, both after a gap.
	vatHeader := repeatRow + 3
	wordsRow := vatHeader
	if spec.hasVatRecap {
		set("A"+strconv.Itoa(vatHeader), "Taux TVA %")
		set("B"+strconv.Itoa(vatHeader), "Base TVA")
		set("C"+strconv.Itoa(vatHeader), "Montant TVA")
		for _, c := range []string{"A", "B", "C"} {
			style(c+strconv.Itoa(vatHeader), styles.header)
		}
		vatRepeat := vatHeader + 1
		set("A"+strconv.Itoa(vatRepeat), "{{taxLines.rate}}")
		set("B"+strconv.Itoa(vatRepeat), "{{taxLines.base}}")
		set("C"+strconv.Itoa(vatRepeat), "{{taxLines.amount}}")
		style("A"+strconv.Itoa(vatRepeat), styles.center)
		style("B"+strconv.Itoa(vatRepeat), styles.right)
		style("C"+strconv.Itoa(vatRepeat), styles.right)
		style("A"+strconv.Itoa(vatRepeat), styles.boxed)
		style("B"+strconv.Itoa(vatRepeat), styles.boxed)
		style("C"+strconv.Itoa(vatRepeat), styles.boxed)
		// Marker in the same far-right column as the line-item marker.
		set(markerCol+strconv.Itoa(vatRepeat), "{{#taxLines}}")
		wordsRow = vatRepeat + len(spec.totals) + 2
		if len(spec.totals) == 0 {
			wordsRow = vatRepeat + 2
		}
	}

	if len(spec.totals) > 0 {
		// Totals block, right of the recap: label in D:D and amount in F:F —
		// fixed columns, independent of how many columns the line-item table
		// happens to use, so the recap (A:C) and the totals (D..) never
		// overlap even on a narrow table.
		for i, t := range spec.totals {
			r := vatHeader + i
			rc := strconv.Itoa(r)
			set("D"+rc, t.label)
			set("F"+rc, t.amount)
			if t.bold {
				style("D"+rc, styles.bold)
				style("F"+rc, styles.boldRight)
			} else {
				style("F"+rc, styles.right)
			}
		}
	}

	if spec.amountInWords != "" {
		set("A"+strconv.Itoa(wordsRow), spec.amountInWords)
		mergeCell(f, sheet, "A"+strconv.Itoa(wordsRow), wideCol+strconv.Itoa(wordsRow))
		style("A"+strconv.Itoa(wordsRow), styles.boxed)
	}

	sigRow := wordsRow + 3
	set("D"+strconv.Itoa(sigRow), "Cachet et Signature")
	mergeCell(f, sheet, "D"+strconv.Itoa(sigRow), wideCol+strconv.Itoa(sigRow))
	style("D"+strconv.Itoa(sigRow), styles.center)

	footerRow := sigRow + 4
	if spec.footerLeft != "" {
		set("A"+strconv.Itoa(footerRow), spec.footerLeft)
		mergeCell(f, sheet, "A"+strconv.Itoa(footerRow), "C"+strconv.Itoa(footerRow))
	}
	if spec.footerRight != "" {
		set("D"+strconv.Itoa(footerRow), spec.footerRight)
		mergeCell(f, sheet, "D"+strconv.Itoa(footerRow), wideCol+strconv.Itoa(footerRow))
	}

	// Column widths: first column (Code) narrow, the description column wide,
	// the rest numeric-width.
	f.SetColWidth(sheet, "A", "A", 14)
	f.SetColWidth(sheet, "B", "B", 34)
	lastIdx := len(spec.columns)
	for i := 3; i <= lastIdx; i++ {
		name, _ := excelize.ColumnNumberToName(i)
		f.SetColWidth(sheet, name, name, 16)
	}

	applyFitToPageWidth(f, sheet)
	applyRepeatingHeaderRows(f, sheet, headerRow)
	applyPageFooter(f, sheet)

	f.SetActiveSheet(0)
	return f
}
