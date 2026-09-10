package api

import (
	"database/sql"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

const documentTemplateContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// listDocumentTemplates reports which document types currently have a
// custom override for this org — content is deliberately excluded (this is
// what the Settings page uses to show "Custom" vs "Default" per document
// type and to decide whether "Reset to default" makes sense, not to read
// the template itself).
func (h *handler) listDocumentTemplates(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	templates, err := h.db.GetDocumentTemplates(orgID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, templates)
}

// getDocumentTemplate streams an org's uploaded template override, or the
// embedded default if none was uploaded — either way a real, downloadable
// .xlsx, so "download the current template" and "download the default"
// (the org has no override yet) share this one handler.
func (h *handler) getDocumentTemplate(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	documentType := r.PathValue("documentType")
	if !db.IsKnownDocumentType(documentType) {
		writeError(w, http.StatusBadRequest, "unknown document type")
		return
	}

	content, filename, err := h.db.ExportDocumentTemplateBytes(orgID, documentType)
	if err != nil {
		writeMutationError(w, err)
		return
	}

	w.Header().Set("Content-Type", documentTemplateContentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeContentDispositionFilename(filename)+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

// uploadDocumentTemplate replaces an org's override for a document type.
// Validation is stronger than the organization logo upload's content-type
// sniff: this file will later be opened and filled by the export engine, not
// just displayed, so db.UploadDocumentTemplate actually parses it via
// excelize before accepting it.
func (h *handler) uploadDocumentTemplate(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	documentType := r.PathValue("documentType")
	if !db.IsKnownDocumentType(documentType) {
		writeError(w, http.StatusBadRequest, "unknown document type")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "file too large (max 2 MB)")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		writeInternalError(w, err)
		return
	}

	filename := header.Filename
	if filename == "" {
		filename = documentType + ".xlsx"
	}
	if err := h.db.UploadDocumentTemplate(orgID, documentType, filename, data); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "organization not found")
			return
		}
		writeMutationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) deleteDocumentTemplate(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	documentType := r.PathValue("documentType")
	if !db.IsKnownDocumentType(documentType) {
		writeError(w, http.StatusBadRequest, "unknown document type")
		return
	}

	ok, err := h.db.DeleteDocumentTemplate(orgID, documentType)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "no custom template uploaded for this document type")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// documentTemplateOrientationResponse is the shape both getDocumentTemplateOrientation
// and updateDocumentTemplateOrientation return. Orientation is "" when the
// org has never set one — the fill engine's own "no override" signal (see
// db/xlsx_export.go's fillTemplate) — which the frontend renders as
// whatever the template itself is authored with, defaulting its own Select
// display to "portrait" since that's what every embedded default in fact
// renders as with no override at all.
type documentTemplateOrientationResponse struct {
	Orientation string `json:"orientation"`
}

// getDocumentTemplateOrientation returns the org's orientation override for
// a document type, or {"orientation": ""} if none was ever set.
func (h *handler) getDocumentTemplateOrientation(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	documentType := r.PathValue("documentType")
	if !db.IsKnownDocumentType(documentType) {
		writeError(w, http.StatusBadRequest, "unknown document type")
		return
	}

	orientation, err := h.db.GetDocumentTemplateOrientation(orgID, documentType)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, documentTemplateOrientationResponse{Orientation: orientation})
}

// updateDocumentTemplateOrientation sets the org's page orientation
// override for a document type — see db.SetDocumentTemplateOrientation for
// why this wins over the template's own authored page setup at export time.
func (h *handler) updateDocumentTemplateOrientation(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	documentType := r.PathValue("documentType")
	if !db.IsKnownDocumentType(documentType) {
		writeError(w, http.StatusBadRequest, "unknown document type")
		return
	}

	var req documentTemplateOrientationResponse
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}

	if err := h.db.SetDocumentTemplateOrientation(orgID, documentType, req.Orientation); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "organization not found")
			return
		}
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, documentTemplateOrientationResponse{Orientation: req.Orientation})
}

// deleteDocumentTemplateOrientation removes an org's orientation override,
// reverting to whatever the template (embedded default or upload) is
// authored with.
func (h *handler) deleteDocumentTemplateOrientation(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	documentType := r.PathValue("documentType")
	if !db.IsKnownDocumentType(documentType) {
		writeError(w, http.StatusBadRequest, "unknown document type")
		return
	}

	ok, err := h.db.DeleteDocumentTemplateOrientation(orgID, documentType)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "no orientation override set for this document type")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// exportInvoiceDocument fills the org's invoice template (uploaded override,
// or the embedded default) with a real invoice's data and streams it back as
// .xlsx or, if converted, .pdf.
//
// Registered directly on mux, NOT through protected()'s auto-withDB — see
// api/router.go's comment on that route. A multi-second LibreOffice
// subprocess held inside withDB's request-long RLock would block a pending
// /api/restore write-lock acquisition and every request behind it, the same
// class of problem F59 fixed for provisionOrSyncUser's bcrypt hash. The DB
// reads below take their own short-lived RLock; the fill/convert step that
// follows runs with no lock held at all.
func (h *handler) exportInvoiceDocument(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	format := r.URL.Query().Get("format")
	if format != "xlsx" && format != "pdf" {
		writeError(w, http.StatusBadRequest, "format must be xlsx or pdf")
		return
	}

	h.dbMu.RLock()
	invoice, lineItems, org, client, templateBytes, taxRates, orientation, err := h.db.FetchInvoiceExportData(id)
	h.dbMu.RUnlock()
	if err != nil {
		writeDBError(w, err, "invoice not found")
		return
	}

	filled, unresolved, err := db.FillInvoiceTemplate(templateBytes, *invoice, lineItems, *org, *client, taxRates, orientation)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	if len(unresolved) > 0 {
		log.Printf("export invoice %s: template has unresolved placeholders: %v", id, unresolved)
	}

	filenameBase := "invoice-" + invoice.Number
	if format == "xlsx" {
		w.Header().Set("Content-Type", documentTemplateContentType)
		w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeContentDispositionFilename(filenameBase)+`.xlsx"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(filled)
		return
	}

	pdfBytes, err := db.ConvertXLSXToPDF(r.Context(), filled)
	if err != nil {
		if errors.Is(err, db.ErrPDFConversionUnavailable) {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		writeInternalError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeContentDispositionFilename(filenameBase)+`.pdf"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdfBytes)
}

// exportPurchaseOrderDocument mirrors exportInvoiceDocument above — same
// document_templates/xlsx_export machinery, same not-through-protected()
// registration for the same reason (a multi-second LibreOffice conversion
// must never hold dbMu's request-long read lock).
func (h *handler) exportPurchaseOrderDocument(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	format := r.URL.Query().Get("format")
	if format != "xlsx" && format != "pdf" {
		writeError(w, http.StatusBadRequest, "format must be xlsx or pdf")
		return
	}

	h.dbMu.RLock()
	order, lineItems, org, vendor, templateBytes, taxRates, orientation, err := h.db.FetchPurchaseOrderExportData(id)
	h.dbMu.RUnlock()
	if err != nil {
		writeDBError(w, err, "purchase order not found")
		return
	}

	filled, unresolved, err := db.FillPurchaseOrderTemplate(templateBytes, *order, lineItems, *org, *vendor, taxRates, orientation)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	if len(unresolved) > 0 {
		log.Printf("export purchase order %s: template has unresolved placeholders: %v", id, unresolved)
	}

	filenameBase := "purchase-order-" + order.OrderNumber
	if format == "xlsx" {
		w.Header().Set("Content-Type", documentTemplateContentType)
		w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeContentDispositionFilename(filenameBase)+`.xlsx"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(filled)
		return
	}

	pdfBytes, err := db.ConvertXLSXToPDF(r.Context(), filled)
	if err != nil {
		if errors.Is(err, db.ErrPDFConversionUnavailable) {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		writeInternalError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeContentDispositionFilename(filenameBase)+`.pdf"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdfBytes)
}

// exportIncomingInvoiceDocument mirrors exportPurchaseOrderDocument above —
// same document_templates/xlsx_export machinery, same not-through-protected()
// registration for the same reason. Unlike purchase orders/orders, an
// incoming invoice has server-validated stored totals, so
// FillIncomingInvoiceTemplate reads them directly rather than computing them.
func (h *handler) exportIncomingInvoiceDocument(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	format := r.URL.Query().Get("format")
	if format != "xlsx" && format != "pdf" {
		writeError(w, http.StatusBadRequest, "format must be xlsx or pdf")
		return
	}

	h.dbMu.RLock()
	invoice, lineItems, org, vendor, templateBytes, taxRates, orientation, err := h.db.FetchIncomingInvoiceExportData(id)
	h.dbMu.RUnlock()
	if err != nil {
		writeDBError(w, err, "incoming invoice not found")
		return
	}

	filled, unresolved, err := db.FillIncomingInvoiceTemplate(templateBytes, *invoice, lineItems, *org, *vendor, taxRates, orientation)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	if len(unresolved) > 0 {
		log.Printf("export incoming invoice %s: template has unresolved placeholders: %v", id, unresolved)
	}

	filenameBase := "incoming-invoice-" + invoice.VendorInvoiceNumber
	if format == "xlsx" {
		w.Header().Set("Content-Type", documentTemplateContentType)
		w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeContentDispositionFilename(filenameBase)+`.xlsx"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(filled)
		return
	}

	pdfBytes, err := db.ConvertXLSXToPDF(r.Context(), filled)
	if err != nil {
		if errors.Is(err, db.ErrPDFConversionUnavailable) {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		writeInternalError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeContentDispositionFilename(filenameBase)+`.pdf"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdfBytes)
}

// exportDeliveryDocument mirrors exportOrderDocument above — same
// document_templates/xlsx_export machinery, same not-through-protected()
// registration for the same reason. outbound_delivery_line_items has no
// price columns, so FillDeliveryTemplate takes no tax rate map.
func (h *handler) exportDeliveryDocument(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	format := r.URL.Query().Get("format")
	if format != "xlsx" && format != "pdf" {
		writeError(w, http.StatusBadRequest, "format must be xlsx or pdf")
		return
	}

	h.dbMu.RLock()
	delivery, lineItems, org, client, templateBytes, orientation, err := h.db.FetchDeliveryExportData(id)
	h.dbMu.RUnlock()
	if err != nil {
		writeDBError(w, err, "delivery not found")
		return
	}

	filled, unresolved, err := db.FillDeliveryTemplate(templateBytes, *delivery, lineItems, *org, *client, orientation)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	if len(unresolved) > 0 {
		log.Printf("export delivery %s: template has unresolved placeholders: %v", id, unresolved)
	}

	filenameBase := "delivery-" + delivery.DeliveryNumber
	if format == "xlsx" {
		w.Header().Set("Content-Type", documentTemplateContentType)
		w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeContentDispositionFilename(filenameBase)+`.xlsx"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(filled)
		return
	}

	pdfBytes, err := db.ConvertXLSXToPDF(r.Context(), filled)
	if err != nil {
		if errors.Is(err, db.ErrPDFConversionUnavailable) {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		writeInternalError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeContentDispositionFilename(filenameBase)+`.pdf"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdfBytes)
}

// exportInboundDeliveryDocument mirrors exportPurchaseOrderDocument above —
// same document_templates/xlsx_export machinery, same not-through-protected()
// registration for the same reason. inbound_delivery_line_items has no tax
// rate, so FillInboundDeliveryTemplate takes no tax rate map either.
func (h *handler) exportInboundDeliveryDocument(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	format := r.URL.Query().Get("format")
	if format != "xlsx" && format != "pdf" {
		writeError(w, http.StatusBadRequest, "format must be xlsx or pdf")
		return
	}

	h.dbMu.RLock()
	delivery, lineItems, org, vendor, templateBytes, orientation, err := h.db.FetchInboundDeliveryExportData(id)
	h.dbMu.RUnlock()
	if err != nil {
		writeDBError(w, err, "goods receipt not found")
		return
	}

	filled, unresolved, err := db.FillInboundDeliveryTemplate(templateBytes, *delivery, lineItems, *org, *vendor, orientation)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	if len(unresolved) > 0 {
		log.Printf("export inbound delivery %s: template has unresolved placeholders: %v", id, unresolved)
	}

	filenameBase := "goods-receipt-" + delivery.DeliveryNumber
	if format == "xlsx" {
		w.Header().Set("Content-Type", documentTemplateContentType)
		w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeContentDispositionFilename(filenameBase)+`.xlsx"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(filled)
		return
	}

	pdfBytes, err := db.ConvertXLSXToPDF(r.Context(), filled)
	if err != nil {
		if errors.Is(err, db.ErrPDFConversionUnavailable) {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		writeInternalError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeContentDispositionFilename(filenameBase)+`.pdf"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdfBytes)
}

// exportOrderDocument mirrors exportInvoiceDocument/exportPurchaseOrderDocument
// above — same document_templates/xlsx_export machinery, same
// not-through-protected() registration for the same reason.
func (h *handler) exportOrderDocument(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	format := r.URL.Query().Get("format")
	if format != "xlsx" && format != "pdf" {
		writeError(w, http.StatusBadRequest, "format must be xlsx or pdf")
		return
	}

	h.dbMu.RLock()
	order, lineItems, org, client, templateBytes, orientation, err := h.db.FetchOrderExportData(id)
	h.dbMu.RUnlock()
	if err != nil {
		writeDBError(w, err, "order not found")
		return
	}

	filled, unresolved, err := db.FillOrderTemplate(templateBytes, *order, lineItems, *org, *client, orientation)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	if len(unresolved) > 0 {
		log.Printf("export order %s: template has unresolved placeholders: %v", id, unresolved)
	}

	filenameBase := "order-" + order.OrderNumber
	if format == "xlsx" {
		w.Header().Set("Content-Type", documentTemplateContentType)
		w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeContentDispositionFilename(filenameBase)+`.xlsx"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(filled)
		return
	}

	pdfBytes, err := db.ConvertXLSXToPDF(r.Context(), filled)
	if err != nil {
		if errors.Is(err, db.ErrPDFConversionUnavailable) {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		writeInternalError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeContentDispositionFilename(filenameBase)+`.pdf"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdfBytes)
}
