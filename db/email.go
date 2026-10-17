package db

import "strings"

// NormalizeEmail is the one form a user's email is stored and looked up in
// (audit F174): surrounding spaces trimmed and ASCII letters lowered. Only
// ASCII is folded, exactly like SQLite's lower() in migration 0107, so a
// stored address and a lookup can never disagree; addresses are ASCII in
// practice.
func NormalizeEmail(email string) string {
	email = strings.TrimSpace(email)
	return strings.Map(func(r rune) rune {
		if 'A' <= r && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, email)
}
