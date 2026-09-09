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

// orderDefaultTemplate is the built-in sales Order export template.
//
//go:embed templates/order_default.xlsx
var orderDefaultTemplate []byte

// incomingInvoiceDefaultTemplate is the built-in Incoming Invoice (vendor
// bill) export template.
//
//go:embed templates/incoming_invoice_default.xlsx
var incomingInvoiceDefaultTemplate []byte

// deliveryDefaultTemplate is the built-in Outbound Delivery (delivery note)
// export template.
//
//go:embed templates/delivery_default.xlsx
var deliveryDefaultTemplate []byte

// inboundDeliveryDefaultTemplate is the built-in Inbound Delivery (goods
// receipt) export template.
//
//go:embed templates/inbound_delivery_default.xlsx
var inboundDeliveryDefaultTemplate []byte

// embeddedDefaultTemplates maps a documentType to its built-in template —
// every document type from the original Phase 2 list now has one.
var embeddedDefaultTemplates = map[string][]byte{
	"invoice":          invoiceDefaultTemplate,
	"purchase_order":   purchaseOrderDefaultTemplate,
	"order":            orderDefaultTemplate,
	"incoming_invoice": incomingInvoiceDefaultTemplate,
	"delivery":         deliveryDefaultTemplate,
	"inbound_delivery": inboundDeliveryDefaultTemplate,
}
