package main

import (
	"log"
	"strconv"

	"github.com/xuri/excelize/v2"
)

// fieldRef documents one placeholder a fill engine function
// (db/xlsx_export.go's FillInvoiceTemplate, db/xlsx_export_purchase_order.go's
// FillPurchaseOrderTemplate, …) resolves. Kept in sync with that allowlist by
// hand — there's no reflection-based way to derive this from the Go source,
// and an entry going stale here is a documentation gap, not a broken export.
type fieldRef struct {
	group       string
	placeholder string
	description string
}

// addAvailableFieldsSheet adds a second, documentation-only sheet listing
// every placeholder a document type's fill engine understands, grouped the
// way a template author encounters them (Header / Item lines / Footer).
// Shared across every document type's generator — only the fields slice
// differs per type. The fill engine (db/xlsx_export.go's fillTemplate)
// strips this sheet before an actual export goes out, so it never reaches a
// document sent to anyone.
func addAvailableFieldsSheet(f *excelize.File, fields []fieldRef) {
	const refSheet = "Available fields"
	if _, err := f.NewSheet(refSheet); err != nil {
		log.Fatal(err)
	}

	title, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 14}})
	header, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"1E293B"}, Pattern: 1},
	})
	group, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"E2E8F0"}, Pattern: 1},
	})
	note, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Italic: true, Color: "666666"}})

	set := func(cell, value string) { _ = f.SetCellStr(refSheet, cell, value) }

	set("A1", "Available template placeholders")
	f.SetCellStyle(refSheet, "A1", "A1", title)
	set("A2", "Use {{placeholder}} in any cell. An unresolved one is left blank in the export, never an error.")
	f.SetCellStyle(refSheet, "A2", "A2", note)
	if err := f.MergeCell(refSheet, "A2", "C2"); err != nil {
		log.Fatal(err)
	}

	headerRow := 4
	set("A"+strconv.Itoa(headerRow), "Group")
	set("B"+strconv.Itoa(headerRow), "Placeholder")
	set("C"+strconv.Itoa(headerRow), "Description")
	f.SetCellStyle(refSheet, "A"+strconv.Itoa(headerRow), "C"+strconv.Itoa(headerRow), header)

	row := headerRow + 1
	lastGroup := ""
	for _, fr := range fields {
		if fr.group != lastGroup {
			cell := "A" + strconv.Itoa(row)
			set(cell, fr.group)
			if err := f.MergeCell(refSheet, cell, "C"+strconv.Itoa(row)); err != nil {
				log.Fatal(err)
			}
			f.SetCellStyle(refSheet, cell, cell, group)
			lastGroup = fr.group
			row++
		}
		set("B"+strconv.Itoa(row), fr.placeholder)
		set("C"+strconv.Itoa(row), fr.description)
		row++
	}

	f.SetColWidth(refSheet, "A", "A", 14)
	f.SetColWidth(refSheet, "B", "B", 32)
	f.SetColWidth(refSheet, "C", "C", 60)
}

// finalizeWorkbook keeps contentSheet active by default when a template
// author opens the file (the "Available fields" sheet is documentation, not
// what should greet them) and saves the workbook to outputPath.
func finalizeWorkbook(f *excelize.File, contentSheet, outputPath string) {
	idx, err := f.GetSheetIndex(contentSheet)
	if err != nil {
		log.Fatal(err)
	}
	f.SetActiveSheet(idx)
	if err := f.SaveAs(outputPath); err != nil {
		log.Fatal(err)
	}
}
