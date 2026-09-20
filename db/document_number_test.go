package db

import (
	"testing"
	"time"
)

func TestValidateDocumentNumberFormat(t *testing.T) {
	t.Parallel()
	cases := []struct {
		format  string
		wantErr bool
	}{
		{"ORD-{number}", false},
		{"ORD-{number:3}", false},
		{"PO-{number:10}", false},
		{"{year}-{month}-{day}-{number:4}-{clientCode}", false},
		{"", true},
		{"  ", true},
		{"ORD-{NUMBER}", true},
		{"ORD-{number:abc}", true},
		{"ORD-{number:123}", true}, // 3-digit width not allowed (matches app's max)
	}
	for _, c := range cases {
		err := validateDocumentNumberFormat(c.format)
		if c.wantErr && err == nil {
			t.Errorf("validateDocumentNumberFormat(%q): want error, got nil", c.format)
		}
		if !c.wantErr && err != nil {
			t.Errorf("validateDocumentNumberFormat(%q): want no error, got %v", c.format, err)
		}
	}
}

func TestGenerateFormattedDocumentNumber(t *testing.T) {
	t.Parallel()
	date := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		format string
		want   string
	}{
		{"ORD-{number:3}", "ORD-007"},
		{"PO-{number:4}", "PO-0007"},
		{"{number}", "7"},
		{"{year}-{number:4}", "2026-0007"},
		{"{y}/{month}/{day}-{number}", "26/03/05-7"},
	}
	for _, c := range cases {
		if got := generateFormattedDocumentNumber(c.format, 7, date, ""); got != c.want {
			t.Errorf("generateFormattedDocumentNumber(%q, 7): got %q, want %q", c.format, got, c.want)
		}
	}
}

func TestGetDocumentNumberSettingDefaultsWhenNoRow(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, _ := d.CreateOrganization(CreateOrganizationRequest{ID: "org-dns-1"})

	setting, err := d.GetDocumentNumberSetting(org.ID, "order")
	if err != nil {
		t.Fatalf("GetDocumentNumberSetting: %v", err)
	}
	if setting.HasOverride {
		t.Fatal("HasOverride: got true, want false for a never-configured type")
	}
	if setting.Format != "ORD-{number:3}" || setting.Counter != 0 {
		t.Fatalf("got %+v, want default format with a zero counter", setting)
	}

	if _, err := d.GetDocumentNumberSetting(org.ID, "invoice"); err == nil {
		t.Fatal("expected an error for a type not covered by this table (invoice keeps its own mechanism)")
	}
}

func TestUpdateDocumentNumberSetting(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, _ := d.CreateOrganization(CreateOrganizationRequest{ID: "org-dns-2"})

	if _, err := d.UpdateDocumentNumberSetting(org.ID, "order", UpdateDocumentNumberSettingRequest{Format: ""}); err == nil {
		t.Fatal("expected a validation error for a blank format")
	}
	if _, err := d.UpdateDocumentNumberSetting(org.ID, "order", UpdateDocumentNumberSettingRequest{Format: "ORD-{nope}"}); err == nil {
		t.Fatal("expected a validation error for an unrecognized token")
	}
	neg := int64(-1)
	if _, err := d.UpdateDocumentNumberSetting(org.ID, "order", UpdateDocumentNumberSettingRequest{Format: "ORD-{number:4}", Counter: &neg}); err == nil {
		t.Fatal("expected a validation error for a negative counter")
	}

	setting, err := d.UpdateDocumentNumberSetting(org.ID, "order", UpdateDocumentNumberSettingRequest{Format: "ORD-{number:5}"})
	if err != nil {
		t.Fatalf("UpdateDocumentNumberSetting: %v", err)
	}
	if !setting.HasOverride || setting.Format != "ORD-{number:5}" || setting.Counter != 0 {
		t.Fatalf("got %+v, want an override with counter preserved at 0", setting)
	}

	// Format-only update (Counter omitted) must not reset an already-advanced counter.
	ten := int64(10)
	if _, err := d.UpdateDocumentNumberSetting(org.ID, "order", UpdateDocumentNumberSettingRequest{Format: "ORD-{number:5}", Counter: &ten}); err != nil {
		t.Fatalf("UpdateDocumentNumberSetting (set counter): %v", err)
	}
	setting, err = d.UpdateDocumentNumberSetting(org.ID, "order", UpdateDocumentNumberSettingRequest{Format: "ORD2-{number:5}"})
	if err != nil {
		t.Fatalf("UpdateDocumentNumberSetting (format only): %v", err)
	}
	if setting.Counter != 10 {
		t.Fatalf("counter: got %d, want 10 to be preserved by the format-only update", setting.Counter)
	}
}

// F99 regression: the old implementation read the counter, then wrote it
// back in a separate statement, so a GenerateNextDocumentNumberTx that
// committed between the two was silently overwritten — reissuing a document
// number, with no unique index to catch it. A format-only update (Counter
// nil) must now leave the counter column alone entirely.
func TestUpdateDocumentNumberSettingFormatOnlyDoesNotRewindCounter(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, _ := d.CreateOrganization(CreateOrganizationRequest{ID: "org-dns-rewind"})

	// Seed an override, then advance the counter twice as a concurrent
	// document creation would.
	if _, err := d.UpdateDocumentNumberSetting(org.ID, "order", UpdateDocumentNumberSettingRequest{Format: "ORD-{number:3}"}); err != nil {
		t.Fatalf("seed override: %v", err)
	}
	for i := 0; i < 2; i++ {
		tx, err := d.DB.Beginx()
		if err != nil {
			t.Fatalf("Beginx: %v", err)
		}
		if _, err := GenerateNextDocumentNumberTx(tx, org.ID, "order", time.UnixMilli(1700000000000), ""); err != nil {
			t.Fatalf("GenerateNextDocumentNumberTx #%d: %v", i, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}
	}

	// A format-only save must preserve the advanced counter.
	setting, err := d.UpdateDocumentNumberSetting(org.ID, "order", UpdateDocumentNumberSettingRequest{Format: "ORD-{number:4}"})
	if err != nil {
		t.Fatalf("format-only update: %v", err)
	}
	if setting.Counter != 2 {
		t.Fatalf("counter after format-only update: got %d, want 2 (a format edit must not rewind it)", setting.Counter)
	}

	// A deliberate counter set must still work.
	seven := int64(7)
	setting, err = d.UpdateDocumentNumberSetting(org.ID, "order", UpdateDocumentNumberSettingRequest{Format: "ORD-{number:4}", Counter: &seven})
	if err != nil {
		t.Fatalf("counter set: %v", err)
	}
	if setting.Counter != 7 {
		t.Fatalf("counter after explicit set: got %d, want 7", setting.Counter)
	}
}

// F105 regression: a real query error must propagate, not masquerade as the
// "no row yet" default. The connection is closed out from under the query to
// force a non-ErrNoRows failure.
func TestGetDocumentNumberSettingPropagatesQueryError(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, _ := d.CreateOrganization(CreateOrganizationRequest{ID: "org-dns-err"})

	d.DB.Close()
	if _, err := d.GetDocumentNumberSetting(org.ID, "order"); err == nil {
		t.Fatal("expected a query error to propagate, got nil (defaulted instead)")
	}
}

// F105 regression: GenerateNextDocumentNumberTx's counter default of 1 is
// only legitimate for a never-configured type (sql.ErrNoRows). A real query
// failure must abort the caller's transaction rather than silently starting
// over at 1.
func TestGenerateNextDocumentNumberTxPropagatesQueryError(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, _ := d.CreateOrganization(CreateOrganizationRequest{ID: "org-dns-gen-err"})

	tx, err := d.DB.Beginx()
	if err != nil {
		t.Fatalf("Beginx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Drop the table the query reads from to force a non-ErrNoRows failure
	// ("no such table") on the same connection the transaction holds.
	if _, err := tx.Exec("DROP TABLE document_number_settings"); err != nil {
		t.Fatalf("drop table: %v", err)
	}
	if _, err := GenerateNextDocumentNumberTx(tx, org.ID, "order", time.UnixMilli(1700000000000), ""); err == nil {
		t.Fatal("expected a query error to propagate, got nil (defaulted to 1 instead)")
	}
}

func TestGenerateNextDocumentNumberTxAdvancesAtomically(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, _ := d.CreateOrganization(CreateOrganizationRequest{ID: "org-dns-3"})
	date := time.UnixMilli(1700000000000)

	for i, want := range []string{"ORD-001", "ORD-002", "ORD-003"} {
		tx, err := d.DB.Beginx()
		if err != nil {
			t.Fatalf("Beginx: %v", err)
		}
		got, err := GenerateNextDocumentNumberTx(tx, org.ID, "order", date, "")
		if err != nil {
			t.Fatalf("GenerateNextDocumentNumberTx #%d: %v", i, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}
		if got != want {
			t.Fatalf("call #%d: got %q, want %q", i, got, want)
		}
	}

	setting, err := d.GetDocumentNumberSetting(org.ID, "order")
	if err != nil {
		t.Fatalf("GetDocumentNumberSetting: %v", err)
	}
	if setting.Counter != 3 {
		t.Fatalf("counter: got %d, want 3", setting.Counter)
	}
}
