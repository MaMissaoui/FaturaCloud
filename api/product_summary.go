package api

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) getProductSummaries(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	summaries, err := h.db.GetProductSummaries(orgID)
	if err != nil {
		if verr, ok := err.(*db.ValidationError); ok {
			writeError(w, http.StatusConflict, verr.Error())
			return
		}
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summaries)
}

// getProductSummary is membership-level like the rest of the product routes,
// but who a product was sold to is sales content and who it was bought from
// is purchasing content: the counterparties are kept only for a role that
// sees client balances (clients) or vendor balances (vendors and the last
// vendor), as listPayments does for what a payment paid for.
func (h *handler) getProductSummary(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")
	product, err := h.db.GetProduct(productID)
	if err != nil {
		writeDBError(w, err, "product not found")
		return
	}
	summary, err := h.db.GetProductSummary(product.OrganizationID, productID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "product not found")
			return
		}
		if verr, ok := err.(*db.ValidationError); ok {
			writeError(w, http.StatusConflict, verr.Error())
			return
		}
		writeInternalError(w, err)
		return
	}
	role, _, err := h.db.GetOrganizationRole(product.OrganizationID, getClaims(r).UserID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	stripProductSummaryCounterparties(summary,
		roleCanUseSection(role, sectionClientBalances),
		roleCanUseSection(role, sectionVendorBalances))
	writeJSON(w, http.StatusOK, summary)
}

func stripProductSummaryCounterparties(summary *db.ProductSummary, clients, vendors bool) {
	for i := range summary.RecentMovements {
		m := &summary.RecentMovements[i]
		if (m.CounterpartyKind == "client" && !clients) || (m.CounterpartyKind == "vendor" && !vendors) {
			m.CounterpartyKind, m.CounterpartyID, m.CounterpartyName = "", nil, nil
		}
	}
	if !vendors {
		summary.LastVendor = nil
	}
}
