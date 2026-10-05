package api

import (
	"net/http"
)

// outstandingDocument is one invoice or bill that still has a balance: what is
// left to pay in the organization's currency and how late it is. The Invoices
// and Incoming invoices lists read these instead of the document's state,
// which is a manual flag — a sent invoice that has been paid in full stays
// "sent" until someone changes it.
type outstandingDocument struct {
	ID          string `json:"id"`
	Outstanding int64  `json:"outstanding"`
	DaysOverdue int    `json:"daysOverdue"`
	Bucket      string `json:"bucket"`
}

// listOutstandingInvoices returns the sent invoices with a balance, from the
// same query as the receivable aging report and the Dashboard.
func (h *handler) listOutstandingInvoices(w http.ResponseWriter, r *http.Request) {
	aging, err := h.db.GetReceivableAging(r.PathValue("orgId"))
	if err != nil {
		writeInternalError(w, err)
		return
	}
	out := make([]outstandingDocument, 0, len(aging.Invoices))
	for _, inv := range aging.Invoices {
		out = append(out, outstandingDocument{ID: inv.ID, Outstanding: inv.Total, DaysOverdue: inv.DaysOverdue, Bucket: inv.Bucket})
	}
	writeJSON(w, http.StatusOK, out)
}

// listOutstandingIncomingInvoices is the payables mirror: approved bills with
// a balance, from the payable aging report's query.
func (h *handler) listOutstandingIncomingInvoices(w http.ResponseWriter, r *http.Request) {
	aging, err := h.db.GetPayableAging(r.PathValue("orgId"))
	if err != nil {
		writeInternalError(w, err)
		return
	}
	out := make([]outstandingDocument, 0, len(aging.Bills))
	for _, bill := range aging.Bills {
		out = append(out, outstandingDocument{ID: bill.ID, Outstanding: bill.Total, DaysOverdue: bill.DaysOverdue, Bucket: bill.Bucket})
	}
	writeJSON(w, http.StatusOK, out)
}
