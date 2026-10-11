package api

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MaMissaoui/fatura-cloud/db"
	"github.com/MaMissaoui/fatura-cloud/db/dbtest"
)

// The background loops (runAuditPruning, sweepLoginBuckets, runScheduler)
// only sleep and call one pass each; these test the passes. Until audit
// F165 their only coverage was incidental — the api suite used to run long
// enough for their timers to fire — and #489's stop-once-closed fix had no
// test at all.

func TestPruneAuditEventsPassStopsOnceClosed(t *testing.T) {
	t.Parallel()
	database := dbtest.NewDatabase(t, filepath.Join(t.TempDir(), "test.db"))
	h := &handler{db: database}
	now := time.Now()
	for id, at := range map[string]time.Time{
		"old":    now.Add(-db.AuditRetention - 24*time.Hour),
		"recent": now.Add(-time.Hour),
	} {
		if err := database.InsertAuditEvent(db.AuditEvent{ID: id, CreatedAt: at.UnixMilli(), Method: "POST", Route: "/api/clients"}); err != nil {
			t.Fatalf("InsertAuditEvent(%s): %v", id, err)
		}
	}

	if closed := h.pruneAuditEvents(now); closed {
		t.Fatal("pruning an open database reported it closed")
	}
	var ids []string
	if err := database.DB.Select(&ids, `SELECT id FROM audit_events ORDER BY id`); err != nil {
		t.Fatalf("read events: %v", err)
	}
	if len(ids) != 1 || ids[0] != "recent" {
		t.Fatalf("after pruning: %v, want only the recent event", ids)
	}

	if err := database.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if closed := h.pruneAuditEvents(now); !closed {
		t.Fatal("pruning a closed database didn't stop the loop")
	}
}

func TestEvictExpiredLoginBuckets(t *testing.T) {
	t.Parallel()
	now := time.Now()
	ips := map[string]*loginBucket{
		"expired": {count: 3, windowEnd: now.Add(-time.Second)},
		"live":    {count: 1, windowEnd: now.Add(time.Minute)},
	}
	emails := map[string]*loginBucket{"expired@x": {count: 9, windowEnd: now.Add(-time.Hour)}}

	evictExpiredLoginBuckets(now, ips, emails)

	if _, ok := ips["expired"]; ok {
		t.Error("an expired IP bucket survived the sweep")
	}
	if _, ok := ips["live"]; !ok {
		t.Error("a live IP bucket was swept")
	}
	if len(emails) != 0 {
		t.Errorf("expired email buckets survived: %v", emails)
	}
}

func TestScheduledBackupRunsOncePerDayInItsHour(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	database := dbtest.NewDatabase(t, filepath.Join(dir, "test.db"))
	t.Cleanup(func() { _ = database.Close() })
	h := &handler{db: database, backupDir: backupDir}
	backupOf := func(day string) string { return filepath.Join(backupDir, "fatura-auto-"+day+".db") }
	exists := func(path string) bool { _, err := os.Stat(path); return err == nil }
	writeConfig := func(enabled bool) {
		t.Helper()
		data := []byte(`{"enabled":false,"scheduleHour":2,"retentionDays":30}`)
		if enabled {
			data = []byte(`{"enabled":true,"scheduleHour":2,"retentionDays":30}`)
		}
		if err := os.WriteFile(filepath.Join(backupDir, "config.json"), data, 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
	}
	at := func(day, hour int) time.Time { return time.Date(2026, 10, day, hour, 10, 0, 0, time.UTC) }

	writeConfig(false)
	if got := h.scheduledBackup(at(11, 2), ""); got != "" || exists(backupOf("2026-10-11")) {
		t.Fatalf("disabled schedule backed up (returned %q)", got)
	}

	writeConfig(true)
	if got := h.scheduledBackup(at(11, 3), ""); got != "" || exists(backupOf("2026-10-11")) {
		t.Fatalf("backed up outside the scheduled hour (returned %q)", got)
	}

	// A stale backup the retention rule should remove, and a file that
	// isn't a backup, which it must leave alone.
	stale := filepath.Join(backupDir, "fatura-auto-2026-08-01.db")
	other := filepath.Join(backupDir, "notes.txt")
	for _, f := range []string{stale, other} {
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
		old := time.Now().AddDate(0, 0, -40)
		if err := os.Chtimes(f, old, old); err != nil {
			t.Fatalf("chtimes %s: %v", f, err)
		}
	}

	last := h.scheduledBackup(at(11, 2), "")
	if last != "2026-10-11" || !exists(backupOf("2026-10-11")) {
		t.Fatalf("scheduled hour: returned %q, backup exists %v", last, exists(backupOf("2026-10-11")))
	}
	if exists(stale) || !exists(other) {
		t.Errorf("retention: stale backup kept %v, other file removed %v", exists(stale), !exists(other))
	}

	// Later in the same hour: already done today.
	if err := os.Remove(backupOf("2026-10-11")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := h.scheduledBackup(at(11, 2).Add(30*time.Minute), last); got != last || exists(backupOf("2026-10-11")) {
		t.Fatalf("second tick the same day backed up again (returned %q)", got)
	}

	if got := h.scheduledBackup(at(12, 2), last); got != "2026-10-12" || !exists(backupOf("2026-10-12")) {
		t.Fatalf("next day: returned %q", got)
	}
}
