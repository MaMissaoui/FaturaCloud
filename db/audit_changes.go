package db

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

// What a change changed: api/audit.go reads the document's row before and
// after the handler (AuditSnapshot) and records the columns that differ
// (AuditChangesJSON). Nothing per handler, like the rest of the history.
// Creates and deletions record no changes — there is no before or no after
// to compare. Line items (invoice, order and delivery lines) live in their
// own tables and aren't compared; a document's totals are.
//
// The same contract as STBvirement's activity history (internal/api/
// audit_changes.go there): keep the two equal.

// auditExtras add fields kept outside the document's own row: a query
// returning one row, with :id (the document id) and :org (the request's
// organization) as parameters.
var auditExtras = map[string]string{
	// The logo is a blob, never recorded; whether there is one is.
	"organizations": `SELECT logo IS NOT NULL AS hasLogo FROM organizations WHERE id = :id`,
	// A member's role in this organization lives in organization_users.
	"members": `SELECT COALESCE((SELECT role FROM organization_users WHERE userId = :id AND organizationId = :org), '') AS role`,
}

// auditSkippedColumns are bookkeeping, not content.
var auditSkippedColumns = map[string]bool{"id": true, "organizationId": true, "createdAt": true, "updatedAt": true}

// auditSecretColumn matches columns that hold a secret (a password hash, a
// token, an API or signing key). They are never recorded, whatever table
// they turn up in; TestAuditColumnsAreReviewed checks every audited table.
var auditSecretColumn = regexp.MustCompile(`(?i)(pass|secret|token|hash|api_?key|private_?key|signing_?key|enc$)`)

// auditBankColumn matches columns holding a bank account (an IBAN or RIB),
// recorded masked.
func auditBankColumn(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "iban") || strings.HasSuffix(n, "rib")
}

// auditReferences name what a column points at: a query reading the
// referenced document's label by its id. A changed reference is recorded
// as those labels, as they read at the time, rather than as raw ids. Any
// other column ending in AccountId points at an account.
var auditReferences = map[string]string{
	"clientId":          `SELECT name FROM clients WHERE id = ?`,
	"vendorId":          `SELECT name FROM vendors WHERE id = ?`,
	"parentId":          `SELECT code || ' ' || name FROM accounts WHERE id = ?`,
	"taxRateId":         `SELECT name FROM taxRates WHERE id = ?`,
	"familyId":          `SELECT name FROM product_families WHERE id = ?`,
	"unitOfMeasureId":   `SELECT name FROM units_of_measure WHERE id = ?`,
	"journalId":         `SELECT code FROM journals WHERE id = ?`,
	"fiscalYearId":      `SELECT name FROM fiscal_years WHERE id = ?`,
	"fiscalPeriodId":    `SELECT name FROM fiscal_periods WHERE id = ?`,
	"orderId":           `SELECT orderNumber FROM orders WHERE id = ?`,
	"purchaseOrderId":   `SELECT orderNumber FROM purchase_orders WHERE id = ?`,
	"importId":          `SELECT importNumber FROM imports WHERE id = ?`,
	"finishedProductId": `SELECT name FROM products WHERE id = ?`,
	"journalEntryId":    `SELECT CAST(entryNumber AS TEXT) FROM journal_entries WHERE id = ?`,
	"voidingEntryId":    `SELECT CAST(entryNumber AS TEXT) FROM journal_entries WHERE id = ?`,
	"reversalOfEntryId": `SELECT CAST(entryNumber AS TEXT) FROM journal_entries WHERE id = ?`,
	"createdBy":         `SELECT email FROM users WHERE id = ?`,
}

const accountLabelQuery = `SELECT code || ' ' || name FROM accounts WHERE id = ?`

func auditReferenceQuery(field string) (string, bool) {
	if q, ok := auditReferences[field]; ok {
		return q, true
	}
	if strings.HasSuffix(field, "AccountId") {
		return accountLabelQuery, true
	}
	return "", false
}

// auditMaxText caps a recorded text value; a longer one is cut.
const auditMaxText = 300

type auditChange struct {
	Field  string `json:"field"`
	From   any    `json:"from"`
	To     any    `json:"to"`
	Masked bool   `json:"masked,omitempty"`
}

// AuditField is one recorded column's value, in the table's column order.
type AuditField struct {
	Name  string
	Value any
}

// AuditSnapshot reads the recordable fields of the document a route's
// resource and id point at, or nil when the resource isn't mapped or the
// document can't be read. orgID is the request's organization.
func (d *Database) AuditSnapshot(resource, id, orgID string) []AuditField {
	doc, ok := auditDocuments[resource]
	if !ok || id == "" {
		return nil
	}
	// Table names come from auditDocuments, never the request.
	fields := d.readAuditRow(fmt.Sprintf(`SELECT * FROM %s WHERE id = :id`, doc.table), id, orgID)
	if fields == nil {
		return nil
	}
	if q, ok := auditExtras[resource]; ok {
		fields = append(fields, d.readAuditRow(q, id, orgID)...)
	}
	return fields
}

func (d *Database) readAuditRow(query, id, orgID string) []AuditField {
	rows, err := d.DB.NamedQuery(query, map[string]any{"id": id, "org": orgID})
	if err != nil {
		return nil
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil || !rows.Next() {
		return nil
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil
	}
	fields := make([]AuditField, 0, len(cols))
	for i, c := range cols {
		if auditSkippedColumns[c] || auditSecretColumn.MatchString(c) {
			continue
		}
		if _, isBlob := vals[i].([]byte); isBlob {
			continue
		}
		fields = append(fields, AuditField{c, vals[i]})
	}
	return fields
}

// auditDiff lists the fields whose value differs between two snapshots, nil
// when nothing differs or either snapshot is missing.
func auditDiff(before, after []AuditField) []auditChange {
	if before == nil || after == nil {
		return nil
	}
	old := make(map[string]any, len(before))
	for _, f := range before {
		old[f.Name] = f.Value
	}
	var changes []auditChange
	for _, f := range after {
		prev, seen := old[f.Name]
		if !seen || reflect.DeepEqual(prev, f.Value) {
			continue
		}
		c := auditChange{Field: f.Name, From: auditValue(prev), To: auditValue(f.Value)}
		if auditBankColumn(f.Name) {
			c.From, c.To, c.Masked = maskAccount(prev), maskAccount(f.Value), true
		}
		changes = append(changes, c)
	}
	return changes
}

// AuditChangesJSON is what audit_events.changes stores for two snapshots:
// the changed fields with references named, as JSON, or "" for none.
func (d *Database) AuditChangesJSON(before, after []AuditField) string {
	changes := auditDiff(before, after)
	if len(changes) == 0 {
		return ""
	}
	for i, c := range changes {
		if q, ok := auditReferenceQuery(c.Field); ok {
			changes[i].From, changes[i].To = d.auditReferenceLabel(q, c.From), d.auditReferenceLabel(q, c.To)
		}
	}
	b, err := json.Marshal(changes)
	if err != nil {
		return ""
	}
	return string(b)
}

// auditReferenceLabel reads a referenced document's label; an id that no
// longer resolves is kept as it is.
func (d *Database) auditReferenceLabel(query string, id any) any {
	s, ok := id.(string)
	if !ok || s == "" {
		return id
	}
	var label string
	if err := d.DB.Get(&label, query, s); err != nil || label == "" {
		return id
	}
	return label
}

func auditValue(v any) any {
	if s, ok := v.(string); ok && len([]rune(s)) > auditMaxText {
		return string([]rune(s)[:auditMaxText]) + "…"
	}
	return v
}

// maskAccount keeps a bank account's first two characters (the country of
// an IBAN, the bank code of a Tunisian RIB) and its last four:
// "DE89370400440532013000" → "DE••••3000".
func maskAccount(v any) any {
	s, ok := v.(string)
	if !ok || s == "" {
		return v
	}
	s = strings.Join(strings.Fields(s), "")
	r := []rune(s)
	if len(r) <= 6 {
		return "••••"
	}
	return string(r[:2]) + "••••" + string(r[len(r)-4:])
}
