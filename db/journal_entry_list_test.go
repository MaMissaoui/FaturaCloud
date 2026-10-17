package db

import (
	"fmt"
	"testing"
)

// TestGetJournalEntriesPaging pins the journal entry list's opt-in paging
// (audit F176): no limit lists everything, a limit returns one stable page
// and the total, and the sort applies across pages.
func TestGetJournalEntriesPaging(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newJournalEntryTestFixture(t, d)
	var fiscalYearID string
	if err := d.DB.Get(&fiscalYearID, `SELECT id FROM fiscal_years WHERE organizationId = ? LIMIT 1`, fx.orgID); err != nil {
		t.Fatalf("fiscal year: %v", err)
	}

	for i := range 7 {
		if _, err := d.DB.Exec(`
			INSERT INTO journal_entries (id, organizationId, journalId, fiscalYearId, date, description, status)
			VALUES (?, ?, ?, ?, ?, ?, 'draft')`,
			fmt.Sprintf("je-%d", i), fx.orgID, fx.journalID, fiscalYearID, int64(1735689600000+i*86400000), fmt.Sprintf("Entry %d", i),
		); err != nil {
			t.Fatalf("insert entry: %v", err)
		}
	}

	all, total, err := d.GetJournalEntries(fx.orgID, JournalEntryListOptions{})
	if err != nil {
		t.Fatalf("GetJournalEntries: %v", err)
	}
	if len(all) != 7 || total != 7 {
		t.Fatalf("unpaged = %d rows, total %d; want 7 and 7", len(all), total)
	}
	if all[0].ID != "je-6" {
		t.Fatalf("default order starts with %s, want the newest (je-6)", all[0].ID)
	}

	seen := map[string]bool{}
	for offset := 0; offset < 7; offset += 3 {
		page, total, err := d.GetJournalEntries(fx.orgID, JournalEntryListOptions{Limit: 3, Offset: offset, SortField: "description"})
		if err != nil {
			t.Fatalf("page at %d: %v", offset, err)
		}
		if total != 7 {
			t.Fatalf("total = %d, want 7", total)
		}
		for i, e := range page {
			want := fmt.Sprintf("je-%d", offset+i)
			if e.ID != want {
				t.Fatalf("offset %d row %d = %s, want %s (ascending by description)", offset, i, e.ID, want)
			}
			seen[e.ID] = true
		}
	}
	if len(seen) != 7 {
		t.Fatalf("pages covered %d entries, want 7", len(seen))
	}

	if _, total, err := d.GetJournalEntries(fx.orgID, JournalEntryListOptions{Status: "posted", Limit: 3}); err != nil || total != 0 {
		t.Fatalf("posted total = %d (%v), want 0", total, err)
	}
}
