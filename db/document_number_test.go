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
