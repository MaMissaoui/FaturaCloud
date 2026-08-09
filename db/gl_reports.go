package db

import (
	"fmt"
	"strings"
	"time"
)

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

// glAccountActivityRow is the shared per-account debit/credit shape both
// GetProfitAndLoss and GetBalanceSheet aggregate journal_lines into, before
// each applies its own normal-balance sign and account-type grouping.
type glAccountActivityRow struct {
	AccountID string `db:"accountId"`
	Code      string `db:"code"`
	Name      string `db:"name"`
	Type      string `db:"type"`
	Debit     int64  `db:"debit"`
	Credit    int64  `db:"credit"`
}

func (d *Database) getAccountActivity(organizationID string, accountTypes []string, dateFilter string, args []any) ([]glAccountActivityRow, error) {
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(accountTypes)), ", ")
	for _, t := range accountTypes {
		args = append(args, t)
	}
	rows := []glAccountActivityRow{}
	err := d.DB.Select(&rows, `
		SELECT a.id AS accountId, a.code AS code, a.name AS name, a.type AS type,
		       COALESCE(SUM(jl.debit), 0) AS debit, COALESCE(SUM(jl.credit), 0) AS credit
		FROM journal_lines jl
		JOIN journal_entries je ON je.id = jl.journalEntryId
		JOIN accounts a ON a.id = jl.accountId
		WHERE je.organizationId = ? AND je.status IN ('posted', 'reversed')`+dateFilter+`
		      AND a.type IN (`+placeholders+`)
		GROUP BY a.id
		ORDER BY a.code ASC`,
		append([]any{organizationID}, args...)...,
	)
	if err != nil {
		return nil, fmt.Errorf("get_account_activity: %w", err)
	}
	return rows, nil
}

// ProfitAndLossLine is one revenue or expense account's net activity over
// the report period, signed by that account's own normal balance (credit
// for revenue, debit for expense) so every line is a positive number that
// adds toward its section's total.
type ProfitAndLossLine struct {
	AccountID   string `json:"accountId"`
	AccountCode string `json:"code"`
	AccountName string `json:"name"`
	Amount      int64  `json:"amount"`
}

// ProfitAndLoss is the Phase 4 P&L report: revenue and expense accounts'
// net activity between startDate and endDate (inclusive), computed on read
// from journal_lines like every other report here — there is no separate
// stored P&L, and nothing is closed to retained earnings until Phase 6.
type ProfitAndLoss struct {
	Revenue       []ProfitAndLossLine `json:"revenue"`
	TotalRevenue  int64               `json:"totalRevenue"`
	Expenses      []ProfitAndLossLine `json:"expenses"`
	TotalExpenses int64               `json:"totalExpenses"`
	NetIncome     int64               `json:"netIncome"`
}

func (d *Database) GetProfitAndLoss(organizationID string, startDate, endDate int64) (*ProfitAndLoss, error) {
	if endDate < startDate {
		return nil, newValidationError("end date must not be before start date")
	}
	rows, err := d.getAccountActivity(
		organizationID, []string{"revenue", "expense"},
		" AND je.date >= ? AND je.date <= ?", []any{startDate, endDate},
	)
	if err != nil {
		return nil, err
	}

	pl := &ProfitAndLoss{Revenue: []ProfitAndLossLine{}, Expenses: []ProfitAndLossLine{}}
	for _, r := range rows {
		line := ProfitAndLossLine{AccountID: r.AccountID, AccountCode: r.Code, AccountName: r.Name}
		if r.Type == "revenue" {
			line.Amount = r.Credit - r.Debit
			pl.Revenue = append(pl.Revenue, line)
			pl.TotalRevenue += line.Amount
		} else {
			line.Amount = r.Debit - r.Credit
			pl.Expenses = append(pl.Expenses, line)
			pl.TotalExpenses += line.Amount
		}
	}
	pl.NetIncome = pl.TotalRevenue - pl.TotalExpenses
	return pl, nil
}

// BalanceSheetLine is one asset/liability/equity account's cumulative
// balance as of the report date, signed by its own normal balance.
type BalanceSheetLine struct {
	AccountID string `json:"accountId"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	Amount    int64  `json:"amount"`
}

// BalanceSheet is the Phase 4 balance sheet: every asset/liability/equity
// account's cumulative balance from inception through asOfDate, plus
// CurrentEarnings — revenue minus expense over that same cumulative window,
// folded into equity. Nothing closes revenue/expense into retained earnings
// until Phase 6 (CloseFiscalYear), so without CurrentEarnings the sheet
// would not balance: every journal entry balances debit=credit across ALL
// accounts including revenue/expense, which is exactly the accounting
// identity Assets - Liabilities - Equity = Revenue - Expense — folding the
// right-hand side into Equity here is what makes TotalAssets equal
// TotalLiabilities + TotalEquity below.
type BalanceSheet struct {
	Assets           []BalanceSheetLine `json:"assets"`
	TotalAssets      int64              `json:"totalAssets"`
	Liabilities      []BalanceSheetLine `json:"liabilities"`
	TotalLiabilities int64              `json:"totalLiabilities"`
	Equity           []BalanceSheetLine `json:"equity"`
	CurrentEarnings  int64              `json:"currentEarnings"`
	TotalEquity      int64              `json:"totalEquity"`
}

func (d *Database) GetBalanceSheet(organizationID string, asOfDate int64) (*BalanceSheet, error) {
	// asOfDate == 0 (epoch) almost certainly means a caller omitted the query
	// param rather than deliberately asking for a balance sheet from before
	// any data could exist — treat it as "now" rather than silently returning
	// an all-zero report that reads as "no activity" instead of "bad request".
	if asOfDate == 0 {
		asOfDate = time.Now().UnixMilli()
	}
	rows, err := d.getAccountActivity(
		organizationID, []string{"asset", "liability", "equity", "revenue", "expense"},
		" AND je.date <= ?", []any{asOfDate},
	)
	if err != nil {
		return nil, err
	}

	bs := &BalanceSheet{
		Assets:      []BalanceSheetLine{},
		Liabilities: []BalanceSheetLine{},
		Equity:      []BalanceSheetLine{},
	}
	var revenue, expense int64
	for _, r := range rows {
		switch r.Type {
		case "asset":
			line := BalanceSheetLine{AccountID: r.AccountID, Code: r.Code, Name: r.Name, Amount: r.Debit - r.Credit}
			bs.Assets = append(bs.Assets, line)
			bs.TotalAssets += line.Amount
		case "liability":
			line := BalanceSheetLine{AccountID: r.AccountID, Code: r.Code, Name: r.Name, Amount: r.Credit - r.Debit}
			bs.Liabilities = append(bs.Liabilities, line)
			bs.TotalLiabilities += line.Amount
		case "equity":
			line := BalanceSheetLine{AccountID: r.AccountID, Code: r.Code, Name: r.Name, Amount: r.Credit - r.Debit}
			bs.Equity = append(bs.Equity, line)
			bs.TotalEquity += line.Amount
		case "revenue":
			revenue += r.Credit - r.Debit
		case "expense":
			expense += r.Debit - r.Credit
		}
	}
	bs.CurrentEarnings = revenue - expense
	bs.TotalEquity += bs.CurrentEarnings
	return bs, nil
}

// InventoryValuationLine is one stock-enabled product's computed value —
// quantity times its current weighted-average (or specific-serial-averaged)
// cost, the same expression products.unitCost/stockQuantity always drive.
type InventoryValuationLine struct {
	ProductID string  `db:"id"            json:"productId"`
	Name      string  `db:"name"          json:"name"`
	Quantity  float64 `db:"stockQuantity" json:"quantity"`
	Value     int64   `db:"value"         json:"value"`
}

// InventoryValuation compares the GL's Inventory account balance against
// the computed Σ(stockQuantity × unitCost) across every stock-enabled
// product — both are independently correct by construction (the GL side
// from postAutoEntryTx's balanced double-entry postings, the computed side
// from recomputeAverageCostTx's running average), but nothing enforces they
// stay equal over time: a receipt/shipment/adjustment predating Phase 7,
// or an uncosted inflow that never accrued GL value in the first place,
// would drift the two apart silently. This report makes that drift
// visible; it does not guarantee there isn't any, and a nonzero Difference
// is not necessarily a bug — see the callers above for cases where GL value
// and computed value can legitimately diverge (e.g. a product costed before
// this organization had a defaultInventoryAccountId configured).
type InventoryValuation struct {
	GLBalance     int64                    `json:"glBalance"`
	ComputedValue int64                    `json:"computedValue"`
	Difference    int64                    `json:"difference"`
	Products      []InventoryValuationLine `json:"products"`
}

func (d *Database) GetInventoryValuation(organizationID string) (*InventoryValuation, error) {
	org, err := d.GetOrganization(organizationID)
	if err != nil {
		return nil, err
	}

	var glBalance int64
	if org.DefaultInventoryAccountID != nil {
		err := d.DB.Get(&glBalance, `
			SELECT COALESCE(SUM(jl.debit), 0) - COALESCE(SUM(jl.credit), 0)
			FROM journal_lines jl
			JOIN journal_entries je ON je.id = jl.journalEntryId
			WHERE je.organizationId = ? AND je.status IN ('posted', 'reversed')
			      AND jl.accountId = ?`,
			organizationID, *org.DefaultInventoryAccountID,
		)
		if err != nil {
			return nil, fmt.Errorf("get_inventory_valuation gl balance: %w", err)
		}
	}

	products := []InventoryValuationLine{}
	err = d.DB.Select(&products, `
		SELECT id, name, stockQuantity,
		       CAST(ROUND(stockQuantity * COALESCE(unitCost, 0)) AS INTEGER) AS value
		FROM products
		WHERE organizationId = ? AND stockEnabled = 1
		ORDER BY name ASC`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_inventory_valuation products: %w", err)
	}

	var computedValue int64
	for _, p := range products {
		computedValue += p.Value
	}

	return &InventoryValuation{
		GLBalance:     glBalance,
		ComputedValue: computedValue,
		Difference:    glBalance - computedValue,
		Products:      products,
	}, nil
}
