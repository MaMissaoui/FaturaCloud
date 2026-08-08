package db

import "fmt"

// TrialBalanceRow is one account's posted debit/credit totals — computed on
// read from journal_lines, never stored, the same "compute, don't cache"
// rule products.stockQuantity/unitCost already follow. Only accounts with
// at least one posted line appear; an account with no activity contributes
// nothing to a trial balance.
type TrialBalanceRow struct {
	AccountID   string `db:"accountId" json:"accountId"`
	AccountCode string `db:"code"      json:"code"`
	AccountName string `db:"name"      json:"name"`
	AccountType string `db:"type"      json:"type"`
	Debit       int64  `db:"debit"     json:"debit"`
	Credit      int64  `db:"credit"    json:"credit"`
}

// GetTrialBalance sums debit and credit per account for an organization,
// optionally scoped to a single fiscal period. Includes both 'posted' and
// 'reversed' entries — a reversed entry's lines are real history that
// actually happened and were posted to the ledger; the reversal is a
// separate entry with flipped lines that offsets it. Excluding 'reversed'
// entries here (counting only their reversal) would report the exact
// negation of what was originally posted instead of a net zero. Draft
// entries are always excluded — they were never posted at all.
func (d *Database) GetTrialBalance(organizationID, fiscalPeriodID string) ([]TrialBalanceRow, error) {
	where := "WHERE je.organizationId = ? AND je.status IN ('posted', 'reversed')"
	args := []any{organizationID}
	if fiscalPeriodID != "" {
		where += " AND je.fiscalPeriodId = ?"
		args = append(args, fiscalPeriodID)
	}

	rows := []TrialBalanceRow{}
	err := d.DB.Select(&rows, `
		SELECT a.id AS accountId, a.code AS code, a.name AS name, a.type AS type,
		       COALESCE(SUM(jl.debit), 0) AS debit, COALESCE(SUM(jl.credit), 0) AS credit
		FROM journal_lines jl
		JOIN journal_entries je ON je.id = jl.journalEntryId
		JOIN accounts a ON a.id = jl.accountId
		`+where+`
		GROUP BY a.id
		ORDER BY a.code ASC`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("get_trial_balance: %w", err)
	}
	return rows, nil
}
