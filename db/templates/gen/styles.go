package main

import (
	"fmt"
	"log"

	"github.com/xuri/excelize/v2"
)

// templateStyles are the handful of cell styles every generated document
// template shares — kept in one place so every document type's header band,
// bold labels, and right-aligned totals look identical rather than drifting
// per generator.
type templateStyles struct {
	bold       int
	title      int
	titleSmall int
	header     int
	right      int
	boldRight  int
	boxed      int
	center     int
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
	// The boxed document title ("Facture N°: …") — smaller than the old
	// standalone title so it fits inside the bordered title box.
	titleSmall, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 12},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	if err != nil {
		log.Fatal(err)
	}
	header, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"E2E8F0"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    thinBorder(),
	})
	if err != nil {
		log.Fatal(err)
	}
	right, err := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{Horizontal: "right"}})
	if err != nil {
		log.Fatal(err)
	}
	boldRight, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Alignment: &excelize.Alignment{Horizontal: "right"},
	})
	if err != nil {
		log.Fatal(err)
	}
	// The plain gridline outline the Tunisian format draws around the buyer
	// block, title box, line-item table and totals.
	boxed, err := f.NewStyle(&excelize.Style{
		Border:    thinBorder(),
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
	})
	if err != nil {
		log.Fatal(err)
	}
	center, err := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		log.Fatal(err)
	}
	return templateStyles{
		bold:       bold,
		title:      title,
		titleSmall: titleSmall,
		header:     header,
		right:      right,
		boldRight:  boldRight,
		boxed:      boxed,
		center:     center,
	}
}

// thinBorder is the single thin-line outline used by the boxed/header styles
// — the printed format's plain gridlines.
func thinBorder() []excelize.Border {
	side := func(s string) excelize.Border {
		return excelize.Border{Type: s, Color: "808080", Style: 1}
	}
	return []excelize.Border{side("left"), side("right"), side("top"), side("bottom")}
}

// mergeCell merges a range, failing loudly on error (generation is
// offline, an error here is a programming mistake in a generator).
func mergeCell(f *excelize.File, sheet, from, to string) {
	if err := f.MergeCell(sheet, from, to); err != nil {
		log.Fatal(err)
	}
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

// applyRepeatingHeaderRows sets Excel/LibreOffice's native "print titles" —
// rows 1 through headerRow (the seller/buyer identity block plus the
// line-item table's own column-label row) repeat at the top of every printed
// page, not just the first. Without this, a document with enough line items
// to spill onto a second page loses all context there: no company name, no
// document number, no client/vendor block, not even the "Description /
// Quantity / …" column labels above the continuing rows — found by actually
// exporting a 60-line invoice to PDF and looking at page 2 (unresolved
// FitToHeight above only controls *whether* a document paginates at all, not
// what repeats once it does). The sheet name is always single-quoted in the
// formula since several document types use a name with a space in it
// ("Purchase Order", "Incoming Invoice", …), which Excel's reference syntax
// requires quoting for.
func applyRepeatingHeaderRows(f *excelize.File, sheet string, headerRow int) {
	if err := f.SetDefinedName(&excelize.DefinedName{
		Name:     "_xlnm.Print_Titles",
		RefersTo: fmt.Sprintf("'%s'!$1:$%d", sheet, headerRow),
		Scope:    sheet,
	}); err != nil {
		log.Fatal(err)
	}
}

// applyPageFooter adds a simple centered "Page X of Y" to every printed
// page — the page-numbering half of pagination; applyRepeatingHeaderRows is
// the context half. Deliberately just page numbers, not a repeat of the
// totals/payment-terms block: that block is the actual document footer and
// must appear exactly once, at the true end of the document (wherever
// fillTemplate's placeholder scan locates it after line-item expansion), not
// on every page the way this print footer does.
func applyPageFooter(f *excelize.File, sheet string) {
	if err := f.SetHeaderFooter(sheet, &excelize.HeaderFooterOptions{
		OddFooter:  "&CPage &P of &N",
		EvenFooter: "&CPage &P of &N",
	}); err != nil {
		log.Fatal(err)
	}
}
