// Command gen generates the embedded Excel export templates under
// db/templates/ — two per document type, one per organization document layout
// (organizations.documentLayout):
//
//   - <type>_default.xlsx — the generic layout (seller block, document title
//     top-right, a labeled party block, a Product/Description/Quantity/Unit
//     Price/Tax Rate/Line Total table, Subtotal/Tax/Total footer), built by
//     each build<Type>DefaultTemplate in <type>_default.go.
//   - <type>_tunisia.xlsx — the Tunisian "Facture"-style layout, built by each
//     build<Type>TunisiaTemplate in <type>.go on top of the shared frame in
//     tunisia_layout.go.
//
// The document types are invoice, purchase_order, order, incoming_invoice,
// delivery and inbound_delivery. A real Excel/LibreOffice session wasn't
// available in the environment this feature was built in, so each output is
// built programmatically via excelize rather than hand-authored — every output
// is still a real, valid .xlsx a user can open and edit like any other.
//
// Run from the repo root and move the outputs over the committed templates
// whenever a layout needs a deliberate change:
//
//	go run ./db/templates/gen && mv db/templates/gen/*.xlsx db/templates/
package main

func main() {
	buildInvoiceDefaultTemplate()
	buildPurchaseOrderDefaultTemplate()
	buildOrderDefaultTemplate()
	buildIncomingInvoiceDefaultTemplate()
	buildDeliveryDefaultTemplate()
	buildInboundDeliveryDefaultTemplate()

	buildInvoiceTunisiaTemplate()
	buildPurchaseOrderTunisiaTemplate()
	buildOrderTunisiaTemplate()
	buildIncomingInvoiceTunisiaTemplate()
	buildDeliveryTunisiaTemplate()
	buildInboundDeliveryTunisiaTemplate()
}
