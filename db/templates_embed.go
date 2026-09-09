package db

import _ "embed"

// invoiceDefaultTemplate is the built-in Invoice export template, used
// whenever an organization hasn't uploaded its own override (see
// resolveTemplateBytes in document_template.go). It isn't hand-authored in a
// spreadsheet app — this environment has none available — but generated
// programmatically via excelize by db/templates/gen/main.go; the layout is
// still a real, valid .xlsx a user can open and edit like any other.
// TestEmbeddedDefaultInvoiceTemplatePlaceholdersAllResolve is what actually
// guards it against a placeholder typo, since a committed binary isn't
// diff-reviewable.
//
//go:embed templates/invoice_default.xlsx
var invoiceDefaultTemplate []byte

// purchaseOrderDefaultTemplate is the built-in Purchase Order export
// template, generated the same way (db/templates/gen/main.go) and guarded
// the same way (TestEmbeddedDefaultPurchaseOrderTemplatePlaceholdersAllResolve).
//
//go:embed templates/purchase_order_default.xlsx
var purchaseOrderDefaultTemplate []byte

// embeddedDefaultTemplates maps a documentType to its built-in template.
// Remaining Phase 2 types (deliveries, orders, incoming invoices) each add
// one more //go:embed + map entry, no other change to this file.
var embeddedDefaultTemplates = map[string][]byte{
	"invoice":        invoiceDefaultTemplate,
	"purchase_order": purchaseOrderDefaultTemplate,
}
