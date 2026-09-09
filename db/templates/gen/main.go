// Command gen generates the embedded default Excel export templates under
// db/templates/ (invoice_default.xlsx, purchase_order_default.xlsx,
// order_default.xlsx, incoming_invoice_default.xlsx, delivery_default.xlsx,
// inbound_delivery_default.xlsx, …) — see each buildXTemplate function's own
// doc comment (invoice.go, purchase_order.go, order.go,
// incoming_invoice.go, delivery.go, inbound_delivery.go) for that document
// type's layout. A real Excel/LibreOffice session wasn't available in the
// environment this feature was built in, so each output is built
// programmatically via excelize rather than hand-authored — every output is
// still a real, valid .xlsx a user can open and edit like any other.
//
// Run from the repo root and copy the outputs over the committed defaults
// whenever a layout needs a deliberate change:
//
//	go run ./db/templates/gen \
//	  && mv db/templates/gen/invoice_default.xlsx db/templates/invoice_default.xlsx \
//	  && mv db/templates/gen/purchase_order_default.xlsx db/templates/purchase_order_default.xlsx \
//	  && mv db/templates/gen/order_default.xlsx db/templates/order_default.xlsx \
//	  && mv db/templates/gen/incoming_invoice_default.xlsx db/templates/incoming_invoice_default.xlsx \
//	  && mv db/templates/gen/delivery_default.xlsx db/templates/delivery_default.xlsx \
//	  && mv db/templates/gen/inbound_delivery_default.xlsx db/templates/inbound_delivery_default.xlsx
package main

func main() {
	buildInvoiceTemplate()
	buildPurchaseOrderTemplate()
	buildOrderTemplate()
	buildIncomingInvoiceTemplate()
	buildDeliveryTemplate()
	buildInboundDeliveryTemplate()
}
