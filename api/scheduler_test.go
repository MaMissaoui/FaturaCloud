package api

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestApplyRetentionOnlyDeletesFaturaPrefixedFiles is F60's regression test:
// applyRetention used to delete any file in backupDir past the cutoff by
// modtime alone, so a stray log or editor artifact left in the directory
// would be silently destroyed too. It must only ever remove files sharing
// the fatura- prefix every backup (scheduled or manual) this app creates.
func TestApplyRetentionOnlyDeletesFaturaPrefixedFiles(t *testing.T) {
	dir := t.TempDir()
	h := &handler{backupDir: dir}

	old := time.Now().AddDate(0, 0, -30)
	files := []string{
		"fatura-auto-2026-01-01.db",
		"fatura-backup-2026-01-01T00-00-00.db",
		"config.json",
		"notes.txt",
		".DS_Store",
	}
	for _, name := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatalf("chtimes %s: %v", name, err)
		}
	}

	h.applyRetention(7)

	for _, name := range []string{"fatura-auto-2026-01-01.db", "fatura-backup-2026-01-01T00-00-00.db"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed by retention, err=%v", name, err)
		}
	}
	for _, name := range []string{"config.json", "notes.txt", ".DS_Store"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected %s to survive retention (not fatura-prefixed), err=%v", name, err)
		}
	}
}

// TestApplyRetentionKeepsRecentFiles confirms the cutoff itself still works
// — a fatura-prefixed file newer than the retention window survives.
func TestApplyRetentionKeepsRecentFiles(t *testing.T) {
	dir := t.TempDir()
	h := &handler{backupDir: dir}

	recentPath := filepath.Join(dir, "fatura-auto-2026-08-13.db")
	if err := os.WriteFile(recentPath, []byte("x"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	h.applyRetention(7)

	if _, err := os.Stat(recentPath); err != nil {
		t.Errorf("expected a recent fatura-prefixed file to survive retention, err=%v", err)
	}
}
