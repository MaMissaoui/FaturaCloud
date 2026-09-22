package db

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// assertEmbeddedTemplatePlaceholdersResolve is the shared guard every
// embedded default template's "placeholders all resolve" test now uses: it
// opens the template, walks every cell outside a repeat marker, and fails on
// any {{placeholder}} the supplied scalar map plus the given per-line-item /
// per-tax-recap key sets don't cover.
//
// It understands the three non-scalar markers: the line-item and tax-recap
// repeat markers (their own keys are supplied by the caller) and the logo
// marker (an image anchor, never a text value — always considered resolved).
// This is what makes the invoice/purchase-order/order/delivery/goods-receipt
// and vendor-bill guards one implementation rather than six near-copies.
func assertEmbeddedTemplatePlaceholdersResolve(
	t *testing.T,
	templateBytes []byte,
	label string,
	scalars map[string]string,
	lineItemKeys map[string]bool,
) {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(templateBytes))
	if err != nil {
		t.Fatalf("open embedded %s template: %v", label, err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)

	// The logo marker is an image anchor; treat it as resolved regardless of
	// whether the caller's scalar map carries an entry for it.
	scalars[logoMarkerKey()] = ""

	taxLineKeys := map[string]bool{
		"taxLines.rate": true, "taxLines.base": true, "taxLines.amount": true,
	}

	rows, err := f.GetRows(sheet)
	if err != nil {
		t.Fatalf("read rows: %v", err)
	}

	sawMarker := false
	for _, row := range rows {
		for _, cell := range row {
			switch strings.TrimSpace(cell) {
			case lineItemMarker:
				sawMarker = true
				continue
			case taxLineMarker:
				continue
			}
			for _, match := range placeholderPattern.FindAllStringSubmatch(cell, -1) {
				key := match[1]
				if _, ok := scalars[key]; ok {
					continue
				}
				if lineItemKeys[key] || taxLineKeys[key] {
					continue
				}
				t.Errorf("embedded default %s template has unresolvable placeholder {{%s}}", label, key)
			}
		}
	}
	if !sawMarker {
		t.Errorf("embedded default %s template has no {{#lineItems}} marker row", label)
	}
}

// logoMarkerKey strips the braces from logoMarker so callers/templates and
// the placeholder scanner agree on the key.
func logoMarkerKey() string {
	return strings.TrimSuffix(strings.TrimPrefix(logoMarker, "{{"), "}}")
}
