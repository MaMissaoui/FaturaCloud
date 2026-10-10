package db

import (
	"fmt"
	"strings"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// The activity history (migration 0102): one row per successful change made
// through the API, written by api/audit.go after the response. A row records
// who, when, the route, and the document it touched — its id, a label read
// from the document (an invoice number, a client name), and for a state or
// status change the state before and after. It does not record a change's
// side effects (a payment marking an invoice paid shows as the payment).

// AuditRetention is how long the history is kept (owner decision 2026-10-05).
const AuditRetention = 2 * 365 * 24 * time.Hour

type AuditEvent struct {
	ID             string  `db:"id" json:"id"`
	CreatedAt      int64   `db:"createdAt" json:"createdAt"`
	OrganizationID *string `db:"organizationId" json:"organizationId"`
	UserID         *string `db:"userId" json:"userId"`
	UserEmail      string  `db:"userEmail" json:"userEmail"`
	Method         string  `db:"method" json:"method"`
	Route          string  `db:"route" json:"route"`
	Resource       string  `db:"resource" json:"resource"`
	EntityID       string  `db:"entityId" json:"entityId"`
	EntityLabel    string  `db:"entityLabel" json:"entityLabel"`
	FromState      string  `db:"fromState" json:"fromState"`
	ToState        string  `db:"toState" json:"toState"`
	RequestID      string  `db:"requestId" json:"requestId"`
	// Changes is what the change changed (migration 0103, audit_changes.go).
	Changes AuditChanges `db:"changes" json:"changes"`
}

// AuditChanges is what a change changed, as stored: a JSON array of
// {"field","from","to"[,"masked"]}, or "" when nothing was recorded. It is
// served as that array, or null.
type AuditChanges string

func (c AuditChanges) MarshalJSON() ([]byte, error) {
	if c == "" {
		return []byte("null"), nil
	}
	return []byte(c), nil
}

func (d *Database) InsertAuditEvent(e AuditEvent) error {
	if e.ID == "" {
		e.ID, _ = gonanoid.New()
	}
	if e.CreatedAt == 0 {
		e.CreatedAt = time.Now().UnixMilli()
	}
	_, err := d.DB.NamedExec(`
		INSERT INTO audit_events (id, createdAt, organizationId, userId, userEmail, method, route,
			resource, entityId, entityLabel, fromState, toState, requestId, changes)
		VALUES (:id, :createdAt, :organizationId, :userId, :userEmail, :method, :route,
			:resource, :entityId, :entityLabel, :fromState, :toState, :requestId, :changes)`, e)
	if err != nil {
		return fmt.Errorf("insert_audit_event: %w", err)
	}
	return nil
}

// AuditEventFilter selects a page of the history, newest first. Exactly one
// scope applies: OrganizationID, or Platform (the rows with no organization).
// Before/BeforeID are the cursor: the createdAt and id of the last row of the
// previous page.
type AuditEventFilter struct {
	OrganizationID string
	Platform       bool
	UserID         string
	EntityID       string
	From, To       int64
	Before         int64
	BeforeID       string
	Limit          int
}

const (
	defaultAuditPage = 100
	maxAuditPage     = 500
)

func (d *Database) ListAuditEvents(f AuditEventFilter) ([]AuditEvent, error) {
	where := []string{}
	args := []any{}
	switch {
	case f.Platform:
		where = append(where, "organizationId IS NULL")
	case f.OrganizationID != "":
		where = append(where, "organizationId = ?")
		args = append(args, f.OrganizationID)
	default:
		return nil, newValidationError("an organization or the platform scope is required")
	}
	if f.UserID != "" {
		where = append(where, "userId = ?")
		args = append(args, f.UserID)
	}
	if f.EntityID != "" {
		where = append(where, "entityId = ?")
		args = append(args, f.EntityID)
	}
	if f.From > 0 {
		where = append(where, "createdAt >= ?")
		args = append(args, f.From)
	}
	if f.To > 0 {
		where = append(where, "createdAt < ?")
		args = append(args, f.To)
	}
	if f.Before > 0 {
		where = append(where, "(createdAt < ? OR (createdAt = ? AND id < ?))")
		args = append(args, f.Before, f.Before, f.BeforeID)
	}
	limit := f.Limit
	if limit <= 0 {
		limit = defaultAuditPage
	}
	if limit > maxAuditPage {
		limit = maxAuditPage
	}
	args = append(args, limit)
	events := []AuditEvent{}
	err := d.DB.Select(&events, `SELECT * FROM audit_events WHERE `+strings.Join(where, " AND ")+
		` ORDER BY createdAt DESC, id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("list_audit_events: %w", err)
	}
	return events, nil
}

// IsClosed reports whether err comes from using a closed database.
// database/sql keeps that error unexported, so its message is the test.
func IsClosed(err error) bool {
	return err != nil && strings.Contains(err.Error(), "sql: database is closed")
}

// PruneAuditEvents deletes the history older than AuditRetention.
func (d *Database) PruneAuditEvents(now time.Time) (int64, error) {
	res, err := d.DB.Exec(`DELETE FROM audit_events WHERE createdAt < ?`, now.Add(-AuditRetention).UnixMilli())
	if err != nil {
		return 0, fmt.Errorf("prune_audit_events: %w", err)
	}
	return res.RowsAffected()
}

// auditDocument says, for a route's resource, which table holds the document,
// the column that names it, and the column holding its state or status, if
// any. The resource is the route's path segment ("incoming-invoices").
type auditDocument struct{ table, label, state string }

var auditDocuments = map[string]auditDocument{
	"invoices":           {"invoices", "number", "state"},
	"cash-sales":         {"invoices", "number", "state"},
	"incoming-invoices":  {"incoming_invoices", "vendorInvoiceNumber", "state"},
	"orders":             {"orders", "orderNumber", "status"},
	"purchase-orders":    {"purchase_orders", "orderNumber", "status"},
	"deliveries":         {"outbound_deliveries", "deliveryNumber", "status"},
	"inbound-deliveries": {"inbound_deliveries", "deliveryNumber", "status"},
	"production-orders":  {"production_orders", "orderNumber", "status"},
	"journal-entries":    {"journal_entries", "CAST(entryNumber AS TEXT)", "status"},
	"fiscal-periods":     {"fiscal_periods", "name", "status"},
	"fiscal-years":       {"fiscal_years", "name", "status"},
	"payments":           {"payments", "reference", "status"},
	"clients":            {"clients", "name", ""},
	"vendors":            {"vendors", "name", ""},
	"products":           {"products", "name", ""},
	"imports":            {"imports", "importNumber", ""},
	"organizations":      {"organizations", "name", ""},
	"users":              {"users", "email", ""},
	"members":            {"users", "email", ""},
	"accounts":           {"accounts", "code || ' ' || name", ""},
	"tax-rates":          {"taxRates", "name", ""},
	"payment-terms":      {"payment_terms", "name", ""},
	"units-of-measure":   {"units_of_measure", "name", ""},
	"product-families":   {"product_families", "name", ""},
	"journals":           {"journals", "code", ""},
}

// AuditDocumentInfo reads the label and the state of the document a route's
// resource and id point at, for the history row. Missing or unknown documents
// give empty strings — a history row is never refused for want of a label.
func (d *Database) AuditDocumentInfo(resource, id string) (label, state string) {
	doc, ok := auditDocuments[resource]
	if !ok || id == "" {
		return "", ""
	}
	stateExpr := "''"
	if doc.state != "" {
		stateExpr = doc.state
	}
	var row struct {
		Label *string `db:"label"`
		State *string `db:"state"`
	}
	// Table and column names come from the map above, never the request.
	err := d.DB.Get(&row, fmt.Sprintf(`SELECT %s AS label, %s AS state FROM %s WHERE id = ?`, doc.label, stateExpr, doc.table), id)
	if err != nil {
		return "", ""
	}
	if row.Label != nil {
		label = *row.Label
	}
	if row.State != nil {
		state = *row.State
	}
	return label, state
}
