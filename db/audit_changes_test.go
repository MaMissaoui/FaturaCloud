package db

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// auditTreatment is what readAuditRow and auditDiff do with a column of a
// text or number type (blobs are skipped by their value).
func auditTreatment(col string) string {
	switch {
	case auditSkippedColumns[col] || auditSecretColumn.MatchString(col):
		return "skip"
	case auditBankColumn(col):
		return "mask"
	}
	return "record"
}

func TestAuditColumnsAreReviewed(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	tables := map[string]bool{}
	for _, doc := range auditDocuments {
		tables[doc.table] = true
	}
	for table := range tables {
		var cols []struct {
			Name string `db:"name"`
			Type string `db:"type"`
		}
		if err := d.DB.Select(&cols, `SELECT name, type FROM pragma_table_info(?)`, table); err != nil || len(cols) == 0 {
			t.Fatalf("table %s has no columns (%v)", table, err)
		}
		review, ok := auditColumnReview[table]
		if !ok {
			t.Errorf("table %s is not in auditColumnReview", table)
			continue
		}
		for _, c := range cols {
			want, listed := review[c.Name]
			if !listed {
				t.Errorf("%s.%s is not reviewed: add it to auditColumnReview as record, mask or skip", table, c.Name)
				continue
			}
			got := auditTreatment(c.Name)
			if strings.EqualFold(c.Type, "BLOB") {
				got = "skip"
			}
			if got != want {
				t.Errorf("%s.%s: the history would %s it, the review says %s", table, c.Name, got, want)
			}
		}
	}
}

func TestMaskAccount(t *testing.T) {
	t.Parallel()
	for in, want := range map[any]any{
		"DE89370400440532013000":      "DE••••3000",
		"DE89 3704 0044 0532 0130 00": "DE••••3000",
		"10006035183598478831":        "10••••8831",
		"123456":                      "••••",
		"":                            "",
		nil:                           nil,
	} {
		if got := maskAccount(in); got != want {
			t.Errorf("maskAccount(%v) = %v, want %v", in, got, want)
		}
	}
}

func TestAuditDiff(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("é", auditMaxText+10)
	before := []AuditField{{"name", "A"}, {"total", int64(100)}, {"iban", "DE89370400440532013000"}, {"notes", nil}, {"same", "x"}}
	after := []AuditField{{"name", "B"}, {"total", int64(250)}, {"iban", "DE89370400440532093000"}, {"notes", long}, {"same", "x"}}
	b, _ := json.Marshal(auditDiff(before, after))
	got := string(b)
	for _, want := range []string{
		`{"field":"name","from":"A","to":"B"}`,
		`{"field":"total","from":100,"to":250}`,
		// The masks match, but the account changed: still recorded.
		`{"field":"iban","from":"DE••••3000","to":"DE••••3000","masked":true}`,
		`"from":null`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("diff %s lacks %s", got, want)
		}
	}
	if strings.Contains(got, `"same"`) || !strings.Contains(got, strings.Repeat("é", auditMaxText)+"…") || strings.Contains(got, strings.Repeat("é", auditMaxText+1)) {
		t.Errorf("diff %s: unchanged field kept or long text not cut", got)
	}
	if auditDiff(before, before) != nil || auditDiff(nil, after) != nil || auditDiff(before, nil) != nil {
		t.Error("no changes, or a missing side, must record nothing")
	}
}

func TestAuditSecretPattern(t *testing.T) {
	t.Parallel()
	for _, c := range []string{"passwordHash", "smtpPasswordEnc", "apiToken", "clientSecret", "tokenVersion", "signingKey", "api_key", "privateKey"} {
		if !auditSecretColumn.MatchString(c) {
			t.Errorf("%s is not treated as a secret", c)
		}
	}
	// datev_bu_key is a DATEV tax key, not a secret.
	if slices.ContainsFunc([]string{"name", "iban", "currency", "reference", "amount", "datev_bu_key", "keyword"}, auditSecretColumn.MatchString) {
		t.Error("an ordinary column is treated as a secret")
	}
}

func TestAuditReferencesAreNamed(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	if _, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-ref"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.DB.Exec(`INSERT INTO accounts (id, organizationId, code, name, type) VALUES ('acc-1', 'org-ref', '411', 'Clients', 'asset')`); err != nil {
		t.Fatal(err)
	}
	got := d.AuditChangesJSON(
		[]AuditField{{"defaultArAccountId", nil}, {"clientId", "gone"}},
		[]AuditField{{"defaultArAccountId", "acc-1"}, {"clientId", "gone-too"}},
	)
	for _, want := range []string{
		`{"field":"defaultArAccountId","from":null,"to":"411 Clients"}`,
		// An id that no longer resolves is kept as it is.
		`{"field":"clientId","from":"gone","to":"gone-too"}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("changes %s lack %s", got, want)
		}
	}
}
