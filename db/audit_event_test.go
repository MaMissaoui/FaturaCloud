package db

import (
	"testing"
	"time"
)

// Pruning drops what is older than two years and keeps the rest, platform
// rows (no organization) included.
func TestPruneAuditEventsKeepsTwoYears(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	for id, at := range map[string]time.Time{
		"old":      now.Add(-AuditRetention - time.Hour),
		"recent":   now.Add(-AuditRetention + time.Hour),
		"platform": now.Add(-time.Hour),
	} {
		e := AuditEvent{ID: id, CreatedAt: at.UnixMilli(), Method: "POST", Route: "/api/clients"}
		if id != "platform" {
			org := "org-prune"
			e.OrganizationID = &org
		}
		if err := d.InsertAuditEvent(e); err != nil {
			t.Fatalf("InsertAuditEvent(%s): %v", id, err)
		}
	}
	if n, err := d.PruneAuditEvents(now); err != nil || n != 1 {
		t.Fatalf("PruneAuditEvents = %d, %v; want 1", n, err)
	}
	org, err := d.ListAuditEvents(AuditEventFilter{OrganizationID: "org-prune"})
	if err != nil || len(org) != 1 || org[0].ID != "recent" {
		t.Errorf("organization history = %+v, %v; want only recent", org, err)
	}
	platform, err := d.ListAuditEvents(AuditEventFilter{Platform: true})
	if err != nil || len(platform) != 1 || platform[0].ID != "platform" {
		t.Errorf("platform history = %+v, %v", platform, err)
	}
	if _, err := d.ListAuditEvents(AuditEventFilter{}); err == nil {
		t.Error("a listing with no scope was allowed")
	}
}

// TestMigration0104RepairsCreatedEntities: before setAuditEntity (audit
// F164), a member add's row named the membership row and a cash sale's the
// client. 0104 renames them to the user and the sale's invoice where that
// can be traced unambiguously, and leaves everything else alone.
func TestMigration0104RepairsCreatedEntities(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-0104"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	for _, id := range []string{"kept", "removed"} {
		d.DB.MustExec(`INSERT INTO users (id, email, passwordHash, displayName, role, isActive) VALUES (?, ?, 'x', ?, 'user', 1)`,
			id, id+"@test.local", id)
	}
	kept, err := d.AddOrganizationUser(org.ID, "kept", "general")
	if err != nil {
		t.Fatalf("AddOrganizationUser(kept): %v", err)
	}
	removed, err := d.AddOrganizationUser(org.ID, "removed", "general")
	if err != nil {
		t.Fatalf("AddOrganizationUser(removed): %v", err)
	}
	if err := d.RemoveOrganizationUser(org.ID, "removed"); err != nil {
		t.Fatalf("RemoveOrganizationUser: %v", err)
	}

	client := func(name string) string {
		c, err := d.CreateClient(CreateClientRequest{OrganizationID: org.ID, Name: &name})
		if err != nil {
			t.Fatalf("CreateClient(%s): %v", name, err)
		}
		return c.ID
	}
	single, twice := client("Single"), client("Twice")
	at := time.Date(2026, 10, 8, 9, 30, 0, 0, time.UTC)
	invoice := func(id, number, clientID string, movesStock int, created time.Time) {
		d.DB.MustExec(`INSERT INTO invoices (id, organizationId, number, state, clientId, date, dueDate,
			subTotal, taxTotal, total, movesStock, createdAt)
			VALUES (?, ?, ?, 'paid', ?, 0, 0, 100, 0, 100, ?, ?)`,
			id, org.ID, number, clientID, movesStock, created.Format("2006-01-02 15:04:05"))
	}
	invoice("inv-single", "#1", single, 1, at)
	invoice("inv-ordinary", "F-9", single, 0, at) // not a Cash Book sale
	invoice("inv-twice-a", "#2", twice, 1, at)
	invoice("inv-twice-b", "#3", twice, 1, at.Add(2*time.Second))

	event := func(id, route, resource, entityID string, createdAt time.Time) {
		if err := d.InsertAuditEvent(AuditEvent{
			ID: id, CreatedAt: createdAt.UnixMilli(), OrganizationID: &org.ID, Method: "POST",
			Route: route, Resource: resource, EntityID: entityID,
		}); err != nil {
			t.Fatalf("InsertAuditEvent(%s): %v", id, err)
		}
	}
	event("ev-member", "/api/organizations/{orgId}/members", "members", kept.ID, at)
	event("ev-member-gone", "/api/organizations/{orgId}/members", "members", removed.ID, at)
	event("ev-sale", "/api/cash-sales", "cash-sales", single, at.Add(400*time.Millisecond))
	event("ev-sale-twice", "/api/cash-sales", "cash-sales", twice, at.Add(2400*time.Millisecond))
	event("ev-line-payment", "/api/cash-sales/{id}/payments", "cash-sales", "inv-single", at.Add(time.Minute))
	event("ev-sale-fixed", "/api/cash-sales", "cash-sales", "inv-single", at.Add(time.Hour))

	sql, err := migrationsFS.ReadFile("migrations/0104_repair_audit_created_entities.up.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	d.DB.MustExec(string(sql))

	for _, c := range []struct{ id, entity, label string }{
		{"ev-member", "kept", "kept@test.local"},
		{"ev-member-gone", removed.ID, ""},
		{"ev-sale", "inv-single", "#1"},
		{"ev-sale-twice", twice, ""},
		{"ev-line-payment", "inv-single", ""},
		{"ev-sale-fixed", "inv-single", ""},
	} {
		var got struct {
			EntityID    string `db:"entityId"`
			EntityLabel string `db:"entityLabel"`
		}
		if err := d.DB.Get(&got, `SELECT entityId, entityLabel FROM audit_events WHERE id = ?`, c.id); err != nil {
			t.Fatalf("read %s: %v", c.id, err)
		}
		if got.EntityID != c.entity || got.EntityLabel != c.label {
			t.Errorf("%s = %q %q, want %q %q", c.id, got.EntityID, got.EntityLabel, c.entity, c.label)
		}
	}
}
