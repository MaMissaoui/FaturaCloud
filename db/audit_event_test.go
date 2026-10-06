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
