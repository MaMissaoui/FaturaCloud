package db

import "testing"

// TestFormatOrgDate covers every value organizations.date_format can
// actually hold (src/utils/date.ts's DATE_FORMATS — raw dayjs tokens) plus
// nil and an unrecognized string, both of which must fall back to the ISO
// default rather than error. A wrong dayjs-token -> Go-layout mapping here is
// a silent-wrong-output bug on every exported document, not a crash — this
// is what actually guards it.
func TestFormatOrgDate(t *testing.T) {
	t.Parallel()
	// 2026-03-05 14:30:00 UTC.
	const ts = int64(1772721000000)

	tests := []struct {
		name   string
		format *string
		want   string
	}{
		{"nil (AUTO)", nil, "2026-03-05"},
		{"MM/DD/YYYY", ptr("MM/DD/YYYY"), "03/05/2026"},
		{"DD/MM/YYYY", ptr("DD/MM/YYYY"), "05/03/2026"},
		{"DD.MM.YYYY", ptr("DD.MM.YYYY"), "05.03.2026"},
		{"YYYY-MM-DD", ptr("YYYY-MM-DD"), "2026-03-05"},
		{"YYYY/MM/DD", ptr("YYYY/MM/DD"), "2026/03/05"},
		{"DD-MM-YYYY", ptr("DD-MM-YYYY"), "05-03-2026"},
		{"unrecognized falls back to ISO", ptr("not-a-real-format"), "2026-03-05"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatOrgDate(ts, tc.format)
			if got != tc.want {
				t.Fatalf("formatOrgDate(%d, %v) = %q, want %q", ts, derefOrNil(tc.format), got, tc.want)
			}
		})
	}
}

func TestFormatOptionalOrgDate(t *testing.T) {
	t.Parallel()
	if got := formatOptionalOrgDate(nil, ptr("YYYY-MM-DD")); got != "" {
		t.Fatalf("expected empty string for nil timestamp, got %q", got)
	}
	ts := int64(1772721000000)
	if got := formatOptionalOrgDate(&ts, ptr("YYYY-MM-DD")); got != "2026-03-05" {
		t.Fatalf("got %q, want 2026-03-05", got)
	}
}

func derefOrNil(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}
