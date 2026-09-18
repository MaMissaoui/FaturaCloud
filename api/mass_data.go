package api

import (
	"io"
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// This file wires up Excel download/upload for mass maintenance of the
// app's flat reference-data lists — clients, vendors, products, tax rates,
// payment terms, units of measure, and the chart of accounts. The actual
// mechanics (workbook layout, row-by-row Create*/Update* dispatch, foreign
// key resolution by name/code) all live in db/mass_data*.go; every handler
// here is a thin export/import pair, the same "shared engine, one small
// wrapper per type" shape db/xlsx_export.go's document exporters already
// established. There is no bulk-delete endpoint — see db/mass_data.go's own
// comment for why.

// massDataMaxUploadBytes caps an uploaded mass-data workbook — generous for
// even a several-thousand-row chart of accounts or client list, well below
// the request body limits multipart file uploads elsewhere in this app
// already use (organization logo, document templates).
const massDataMaxUploadBytes = 8 << 20 // 8MB

// writeMassDataXLSX sends an exported workbook with the same
// Content-Type/Content-Disposition shape document export uses.
func writeMassDataXLSX(w http.ResponseWriter, filenameBase string, content []byte) {
	w.Header().Set("Content-Type", documentTemplateContentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeContentDispositionFilename(filenameBase)+`.xlsx"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

// readMassDataUpload extracts the uploaded file from a "file" multipart
// field, the same shape uploadDocumentTemplate already uses. Returns false
// (having already written a response) on any failure.
func readMassDataUpload(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, massDataMaxUploadBytes)
	if err := r.ParseMultipartForm(massDataMaxUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "file too large (max 8MB)")
		return nil, false
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field is required")
		return nil, false
	}
	defer file.Close() //nolint:errcheck
	data, err := io.ReadAll(file)
	if err != nil {
		writeInternalError(w, err)
		return nil, false
	}
	return data, true
}

// writeMassDataImportResult handles the top-level error a mass-data import
// call can return (a malformed upload, or a real internal failure building
// the lookup context) — a *db.ValidationError maps to 409, same convention
// as writeMutationError. Per-row failures never reach here: they're
// captured inside result.Rows instead, so a bad row 40 of 500 doesn't cost
// the other 499 an HTTP-level error.
func writeMassDataImportResult(w http.ResponseWriter, result *db.MassDataImportResult, err error) {
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *handler) exportClients(w http.ResponseWriter, r *http.Request) {
	content, err := h.db.ExportClientsXLSX(r.PathValue("orgId"))
	if err != nil {
		writeDBError(w, err, "organization not found")
		return
	}
	writeMassDataXLSX(w, "clients", content)
}

func (h *handler) importClients(w http.ResponseWriter, r *http.Request) {
	data, ok := readMassDataUpload(w, r)
	if !ok {
		return
	}
	result, err := h.db.ImportClientsXLSX(r.PathValue("orgId"), data)
	writeMassDataImportResult(w, result, err)
}

func (h *handler) exportVendors(w http.ResponseWriter, r *http.Request) {
	content, err := h.db.ExportVendorsXLSX(r.PathValue("orgId"))
	if err != nil {
		writeDBError(w, err, "organization not found")
		return
	}
	writeMassDataXLSX(w, "vendors", content)
}

func (h *handler) importVendors(w http.ResponseWriter, r *http.Request) {
	data, ok := readMassDataUpload(w, r)
	if !ok {
		return
	}
	result, err := h.db.ImportVendorsXLSX(r.PathValue("orgId"), data)
	writeMassDataImportResult(w, result, err)
}

func (h *handler) exportProducts(w http.ResponseWriter, r *http.Request) {
	content, err := h.db.ExportProductsXLSX(r.PathValue("orgId"))
	if err != nil {
		writeDBError(w, err, "organization not found")
		return
	}
	writeMassDataXLSX(w, "products", content)
}

func (h *handler) importProducts(w http.ResponseWriter, r *http.Request) {
	data, ok := readMassDataUpload(w, r)
	if !ok {
		return
	}
	result, err := h.db.ImportProductsXLSX(r.PathValue("orgId"), data)
	writeMassDataImportResult(w, result, err)
}

func (h *handler) exportTaxRates(w http.ResponseWriter, r *http.Request) {
	content, err := h.db.ExportTaxRatesXLSX(r.PathValue("orgId"))
	if err != nil {
		writeDBError(w, err, "organization not found")
		return
	}
	writeMassDataXLSX(w, "tax-rates", content)
}

func (h *handler) importTaxRates(w http.ResponseWriter, r *http.Request) {
	data, ok := readMassDataUpload(w, r)
	if !ok {
		return
	}
	result, err := h.db.ImportTaxRatesXLSX(r.PathValue("orgId"), data)
	writeMassDataImportResult(w, result, err)
}

func (h *handler) exportPaymentTerms(w http.ResponseWriter, r *http.Request) {
	content, err := h.db.ExportPaymentTermsXLSX(r.PathValue("orgId"))
	if err != nil {
		writeDBError(w, err, "organization not found")
		return
	}
	writeMassDataXLSX(w, "payment-terms", content)
}

func (h *handler) importPaymentTerms(w http.ResponseWriter, r *http.Request) {
	data, ok := readMassDataUpload(w, r)
	if !ok {
		return
	}
	result, err := h.db.ImportPaymentTermsXLSX(r.PathValue("orgId"), data)
	writeMassDataImportResult(w, result, err)
}

func (h *handler) exportUnitsOfMeasure(w http.ResponseWriter, r *http.Request) {
	content, err := h.db.ExportUnitsOfMeasureXLSX(r.PathValue("orgId"))
	if err != nil {
		writeDBError(w, err, "organization not found")
		return
	}
	writeMassDataXLSX(w, "units-of-measure", content)
}

func (h *handler) importUnitsOfMeasure(w http.ResponseWriter, r *http.Request) {
	data, ok := readMassDataUpload(w, r)
	if !ok {
		return
	}
	result, err := h.db.ImportUnitsOfMeasureXLSX(r.PathValue("orgId"), data)
	writeMassDataImportResult(w, result, err)
}

func (h *handler) exportAccounts(w http.ResponseWriter, r *http.Request) {
	content, err := h.db.ExportAccountsXLSX(r.PathValue("orgId"))
	if err != nil {
		writeDBError(w, err, "organization not found")
		return
	}
	writeMassDataXLSX(w, "chart-of-accounts", content)
}

func (h *handler) importAccounts(w http.ResponseWriter, r *http.Request) {
	data, ok := readMassDataUpload(w, r)
	if !ok {
		return
	}
	result, err := h.db.ImportAccountsXLSX(r.PathValue("orgId"), data)
	writeMassDataImportResult(w, result, err)
}
