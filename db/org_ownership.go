package db

// requireSameOrg is the one-line ownership check every cross-org
// foreign-key validation in this package should use: a create/update
// request names its own organizationId directly, but a referenced id inside
// the body (a fiscalYearId, accountId, clientId, productId, ...) points at a
// row that carries its OWN organizationId, fetched separately — nothing
// stops a member of org B from creating a resource under org B while
// pointing that field at a row that actually belongs to org A. See issue
// #189 and the audit it's based on (db/fiscal_period.go's CreateFiscalPeriod
// was the first, verified example: it validated a fiscalYearId's date range
// but never its organizationId).
//
// field is named in the returned message, not compared — callers pass
// whatever's already fetched (typically via each domain's existing GetX),
// so this never issues a query itself and carries no transaction-locking
// concerns of its own. Callers must still mind where THEIR OWN fetch runs
// relative to an open tx under SetMaxOpenConns(1) — reuse an existing
// pre-tx fetch where one already happens (e.g. CreateFiscalPeriod already
// calls GetFiscalYear), or fetch before Beginx() rather than through d.DB
// while a tx is open.
func requireSameOrg(orgID, otherOrgID, field string) error {
	if otherOrgID != orgID {
		return newValidationError("%s belongs to a different organization", field)
	}
	return nil
}
