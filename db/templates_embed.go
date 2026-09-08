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

// embeddedDefaultTemplates maps a documentType to its built-in template.
// Phase 2 (purchase orders, deliveries, incoming invoices) adds one more
// //go:embed + map entry each, no other change to this file.
var embeddedDefaultTemplates = map[string][]byte{
	"invoice": invoiceDefaultTemplate,
}
