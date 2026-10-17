package db

import "testing"

func TestNormalizeEmail(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"Ali@Shop.TN":       "ali@shop.tn",
		"  ali@shop.tn \t":  "ali@shop.tn",
		"ÉLodie@Example.fr": "Élodie@example.fr", // only ASCII folds, like SQLite's lower()
		"":                  "",
	} {
		if got := NormalizeEmail(in); got != want {
			t.Errorf("NormalizeEmail(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestMigration0107NormalizesEmails: existing emails are normalized, and an
// account that would collide with another once normalized is left alone.
func TestMigration0107NormalizesEmails(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	for id, email := range map[string]string{
		"u-mixed":   " Mixed.Case@Example.com ",
		"u-twin-a":  "Twin@Example.com",
		"u-twin-b":  "twin@example.com",
		"u-already": "plain@example.com",
	} {
		d.DB.MustExec(`INSERT INTO users (id, email, passwordHash, displayName, role) VALUES (?, ?, 'x', ?, 'user')`, id, email, id)
	}
	sql, err := migrationsFS.ReadFile("migrations/0107_normalize_user_emails.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	d.DB.MustExec(string(sql))

	for id, want := range map[string]string{
		"u-mixed":   "mixed.case@example.com",
		"u-twin-a":  "Twin@Example.com", // would collide with u-twin-b: merged by hand
		"u-twin-b":  "twin@example.com",
		"u-already": "plain@example.com",
	} {
		var got string
		if err := d.DB.Get(&got, `SELECT email FROM users WHERE id = ?`, id); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s: email %q, want %q", id, got, want)
		}
	}
}
