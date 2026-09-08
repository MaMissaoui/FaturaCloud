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
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
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
	invoice, lineItems, org, client, templateBytes, taxRates, err := h.db.FetchInvoiceExportData(id)
	h.dbMu.RUnlock()
	if err != nil {
		writeDBError(w, err, "invoice not found")
		return
	}

	filled, unresolved, err := db.FillInvoiceTemplate(templateBytes, *invoice, lineItems, *org, *client, taxRates)
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
		w.Header().Set("Content-Disposition", `attachment; filename="`+filenameBase+`.xlsx"`)
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
	w.Header().Set("Content-Disposition", `attachment; filename="`+filenameBase+`.pdf"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdfBytes)
}
