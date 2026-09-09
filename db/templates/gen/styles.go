package main

import (
	"log"

	"github.com/xuri/excelize/v2"
)

// templateStyles are the handful of cell styles every generated document
// template shares — kept in one place so every document type's header band,
// bold labels, and right-aligned totals look identical rather than drifting
// per generator.
type templateStyles struct {
	bold   int
	title  int
	header int
	right  int
}

func newTemplateStyles(f *excelize.File) templateStyles {
	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		log.Fatal(err)
	}
	title, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 18}})
	if err != nil {
		log.Fatal(err)
	}
	header, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"1E293B"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	if err != nil {
		log.Fatal(err)
	}
	right, err := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{Horizontal: "right"}})
	if err != nil {
		log.Fatal(err)
	}
	return templateStyles{bold: bold, title: title, header: header, right: right}
}

// applyFitToPageWidth scales a sheet to always fit one page wide when
// printed or converted to PDF, leaving height unconstrained. Required on
// every generated template, not just cosmetic: without it, LibreOffice's PDF
// conversion (db/pdf_convert.go) silently splits a wide line-item table
// across multiple pages — found only by actually converting the invoice
// template end-to-end (see CLAUDE.md's db/xlsx_export.go entry) after the
// alignment fix alone looked correct in every other check.
func applyFitToPageWidth(f *excelize.File, sheet string) {
	fitToPage := true
	if err := f.SetSheetProps(sheet, &excelize.SheetPropsOptions{FitToPage: &fitToPage}); err != nil {
		log.Fatal(err)
	}
	fitToWidth, fitToHeight := 1, 0
	if err := f.SetPageLayout(sheet, &excelize.PageLayoutOptions{
		FitToWidth:  &fitToWidth,
		FitToHeight: &fitToHeight,
	}); err != nil {
		log.Fatal(err)
	}
}
