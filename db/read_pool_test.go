package db

import (
	"strings"
	"testing"
	"time"
)

// The read pool (audit F177) refuses writes, sees a committed write at once,
// and doesn't wait on a transaction open on the writer.
func TestReadPool(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	r := d.ReadOnly()
	if r.DB == d.DB {
		t.Fatal("ReadOnly returned the writer connection")
	}

	_, err := r.CreateOrganization(CreateOrganizationRequest{ID: "org-ro-write"})
	if err == nil || !strings.Contains(err.Error(), "readonly") {
		t.Fatalf("a write through the read view: want a readonly error, got %v", err)
	}

	if _, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-ro"}); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := r.GetOrganization("org-ro"); err != nil {
		t.Fatalf("the read view doesn't see a committed write: %v", err)
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		t.Fatalf("Beginx: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`INSERT INTO organizations (id, name) VALUES ('org-ro-tx', 'pending')`); err != nil {
		t.Fatalf("insert in tx: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := r.GetOrganization("org-ro-tx")
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("the read view saw an uncommitted row")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a read through the read view waited on the writer's open transaction")
	}
}
