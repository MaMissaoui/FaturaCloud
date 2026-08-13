package db

import (
	"fmt"
	"time"
)

// closingLine returns the line that zeros a normal-balance-signed amount x
// against accountID — used both to zero out a revenue/expense account and
// to record the net into retained earnings. debitWhenPositive controls
// which side a positive x lands on:
//
//   - Revenue zeroing: x is credit-debit (positive on a normal revenue
//     balance), so a positive x is erased with a Debit — debitWhenPositive
//     is true.
//   - Expense zeroing and the retained-earnings line: x is debit-credit
//     for expense (positive on a normal expense balance), and net income
//     for retained earnings (a profit should credit equity) — both erase/
//     record a positive x with a Credit, so debitWhenPositive is false.
//
// Whichever side actually receives the amount, the entry as a whole still
// balances: see CloseFiscalYear's comment for why.
func closingLine(accountID string, x int64, debitWhenPositive bool) CreateJournalLineRequest {
	amount := x
	if amount < 0 {
		amount = -amount
	}
	line := CreateJournalLineRequest{AccountID: accountID}
	if (x > 0) == debitWhenPositive {
		line.Debit = amount
	} else {
		line.Credit = amount
	}
	return line
}

// CloseFiscalYear closes fiscalYearID: posts one closing journal entry that
// zeroes every revenue/expense account with activity posted into this
// specific year (scoped by journal_entries.fiscalYearId, not a date range —
// resolveFiscalPeriodForDate already guarantees every such entry's date
// falls inside the year, so the two are equivalent, but fiscalYearId is the
// more direct statement of "everything that actually belongs to this
// year"), moving the net (revenue − expense) to the organization's
// retainedEarningsAccountId, then marks the year status='closed'.
//
// Whichever side a zeroing line lands its amount on, its contribution to
// (debit − credit) always equals the account's own normal-balance-signed
// net: a revenue account with balance R contributes exactly R (debit R
// when R>0; credit −R, i.e. a negative contribution of R, when R<0), and an
// expense account with balance E contributes exactly −E by the same
// argument. So the zeroing lines together contribute
// Σrevenue − Σexpense = netIncome to (debit − credit), and giving the
// retained-earnings line the opposite contribution (credit netIncome when
// positive, debit −netIncome when negative) brings the whole entry back to
// balance — this is why no rounding plug is needed here, unlike a document
// with independently-rounded tax lines.
//
// A year with zero net revenue/expense activity closes with no entry at
// all — the same "a zero-amount group emits no row" rule
// buildInvoiceGLLines already follows for a 0% tax rate.
//
// Must run the closing entry's post *before* flipping status to 'closed':
// postAutoEntryTx resolves the entry's fiscal year via
// resolveFiscalPeriodForDate, which only matches an *open* year — posting
// after the flip would 409 with "no open fiscal year covers this date"
// against the very year being closed.
func (d *Database) CloseFiscalYear(fiscalYearID string) (*FiscalYear, error) {
	year, err := d.GetFiscalYear(fiscalYearID)
	if err != nil {
		return nil, fmt.Errorf("close_fiscal_year get_year: %w", err)
	}
	if year.Status != "open" {
		return nil, newValidationError("only an open fiscal year can be closed")
	}

	org, err := d.GetOrganization(year.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("close_fiscal_year get_organization: %w", err)
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("close_fiscal_year begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// F49: the activity snapshot runs inside the transaction, against tx —
	// not d.DB — so a posting committed after the pre-tx `year` read above
	// but before this transaction acquires the connection is still included.
	// Read outside a transaction, this SELECT could miss activity that a
	// concurrent request commits in the gap, silently excluding it from the
	// closing entry and unbalancing the year with no error raised anywhere.
	rows := []glAccountActivityRow{}
	if err := tx.Select(&rows, `
		SELECT a.id AS accountId, a.code AS code, a.name AS name, a.type AS type,
		       COALESCE(SUM(jl.debit), 0) AS debit, COALESCE(SUM(jl.credit), 0) AS credit
		FROM journal_lines jl
		JOIN journal_entries je ON je.id = jl.journalEntryId
		JOIN accounts a ON a.id = jl.accountId
		WHERE je.fiscalYearId = ? AND je.status IN ('posted', 'reversed')
		      AND a.type IN ('revenue', 'expense')
		GROUP BY a.id
		ORDER BY a.code ASC`,
		fiscalYearID,
	); err != nil {
		return nil, fmt.Errorf("close_fiscal_year select: %w", err)
	}

	var lines []CreateJournalLineRequest
	var netIncome int64
	for _, r := range rows {
		if r.Type == "revenue" {
			balance := r.Credit - r.Debit
			if balance == 0 {
				continue
			}
			netIncome += balance
			lines = append(lines, closingLine(r.AccountID, balance, true))
		} else {
			balance := r.Debit - r.Credit
			if balance == 0 {
				continue
			}
			netIncome -= balance
			lines = append(lines, closingLine(r.AccountID, balance, false))
		}
	}
	if netIncome != 0 {
		if org.RetainedEarningsAccountID == nil {
			return nil, newValidationError("cannot close fiscal year: no retained earnings account configured for this organization")
		}
		lines = append(lines, closingLine(*org.RetainedEarningsAccountID, netIncome, false))
	}

	if len(lines) > 0 {
		journal, err := getJournalByTypeTx(tx, year.OrganizationID, "miscellaneous")
		if err != nil {
			return nil, err
		}
		description := fmt.Sprintf("Closing entry for fiscal year %s", year.Name)
		if _, err := postAutoEntryTx(
			tx, year.OrganizationID, journal.ID, "closing", year.ID, year.EndDate, "", description, lines,
		); err != nil {
			return nil, err
		}
	}

	// F49: the WHERE status = 'open' re-check (not just the pre-tx read
	// above) is what makes two concurrent closes idempotent instead of both
	// posting their own closing entry — SetMaxOpenConns(1) serializes the
	// transactions themselves, so the second call's Beginx() blocks until
	// the first commits; without this guard the second call's stale `year`
	// (still read as status='open' before either transaction began) would
	// sail through and post a second closing entry against an already-closed
	// year.
	now := time.Now().UnixMilli()
	res, err := tx.Exec(
		`UPDATE fiscal_years SET status = 'closed', closedAt = ? WHERE id = ? AND status = 'open'`,
		now, year.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("close_fiscal_year update: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, newValidationError("only an open fiscal year can be closed")
	}
	// Every still-open period closes with the year — resolveFiscalPeriodForDate
	// already blocks new postings once the year itself is closed, so this is
	// a display-consistency fix, not a correctness one: without it the
	// fiscal periods page would keep showing "Open" periods, with working
	// close/reopen buttons, under a year that no longer accepts entries.
	if _, err := tx.Exec(
		`UPDATE fiscal_periods SET status = 'closed', closedAt = ? WHERE fiscalYearId = ? AND status = 'open'`,
		now, year.ID,
	); err != nil {
		return nil, fmt.Errorf("close_fiscal_year update_periods: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("close_fiscal_year commit: %w", err)
	}
	return d.GetFiscalYear(year.ID)
}
