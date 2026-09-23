package db

import _ "embed"

// invoiceDefaultTemplate is the built-in generic ("default" layout) Invoice
// export template, used whenever an organization on the default layout hasn't
// uploaded its own override (see resolveTemplateBytes in
// document_template.go). It isn't hand-authored in a
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

// The Tunisian "Facture"-style counterparts of the templates above, used when
// an organization's documentLayout is "tunisia" (see embeddedTemplatesFor).
// Generated from the shared frame in db/templates/gen/tunisia_layout.go and
// guarded by the TestEmbeddedTunisia*TemplatePlaceholdersAllResolve tests.

//go:embed templates/invoice_tunisia.xlsx
var invoiceTunisiaTemplate []byte

//go:embed templates/purchase_order_tunisia.xlsx
var purchaseOrderTunisiaTemplate []byte

//go:embed templates/order_tunisia.xlsx
var orderTunisiaTemplate []byte

//go:embed templates/incoming_invoice_tunisia.xlsx
var incomingInvoiceTunisiaTemplate []byte

//go:embed templates/delivery_tunisia.xlsx
var deliveryTunisiaTemplate []byte

//go:embed templates/inbound_delivery_tunisia.xlsx
var inboundDeliveryTunisiaTemplate []byte

// embeddedDefaultTemplates maps a documentType to its built-in generic
// ("default" layout) template — every document type from the original Phase 2
// list has one. Its key set is also the document-type allowlist
// (IsKnownDocumentType), independent of layout.
var embeddedDefaultTemplates = map[string][]byte{
	"invoice":          invoiceDefaultTemplate,
	"purchase_order":   purchaseOrderDefaultTemplate,
	"order":            orderDefaultTemplate,
	"incoming_invoice": incomingInvoiceDefaultTemplate,
	"delivery":         deliveryDefaultTemplate,
	"inbound_delivery": inboundDeliveryDefaultTemplate,
}

// embeddedTunisiaTemplates maps a documentType to its built-in Tunisian
// ("tunisia" layout) template — the same key set as embeddedDefaultTemplates.
var embeddedTunisiaTemplates = map[string][]byte{
	"invoice":          invoiceTunisiaTemplate,
	"purchase_order":   purchaseOrderTunisiaTemplate,
	"order":            orderTunisiaTemplate,
	"incoming_invoice": incomingInvoiceTunisiaTemplate,
	"delivery":         deliveryTunisiaTemplate,
	"inbound_delivery": inboundDeliveryTunisiaTemplate,
}

// Organization document layouts (organizations.documentLayout).
const (
	DocumentLayoutDefault = "default"
	DocumentLayoutTunisia = "tunisia"
)

// normalizeDocumentLayout maps a stored documentLayout to the layout actually
// used: exactly "tunisia" selects the Tunisian templates, and anything else —
// NULL, "", "default", or an unrecognized value — falls back to "default". A
// layout choice must never hard-fail an export.
func normalizeDocumentLayout(layout *string) string {
	if layout != nil && *layout == DocumentLayoutTunisia {
		return DocumentLayoutTunisia
	}
	return DocumentLayoutDefault
}

// embeddedTemplatesFor returns the embedded template set for a (normalized)
// layout.
func embeddedTemplatesFor(layout string) map[string][]byte {
	if layout == DocumentLayoutTunisia {
		return embeddedTunisiaTemplates
	}
	return embeddedDefaultTemplates
}
