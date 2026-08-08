package db

import (
	"errors"
	"fmt"
	"strings"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// ErrAccountInUse is returned by DeleteAccount when the account is still
// referenced by journal lines, an organization GL default, a tax rate's
// output/input tax account, or a product's revenue/expense account.
var ErrAccountInUse = errors.New("account is still referenced by journal entries or other records")

// ErrAccountHasChildren is returned by DeleteAccount when other accounts
// still point at it as their parent — deleting it would orphan them.
var ErrAccountHasChildren = errors.New("account has child accounts")

var accountTypes = map[string]bool{
	"asset":     true,
	"liability": true,
	"equity":    true,
	"revenue":   true,
	"expense":   true,
}

// Account mirrors the accounts table — a node in the chart of accounts.
// isGroup=1 rows are headers only, never postable; that's enforced by
// allocateAndFinalizeEntryTx (db/journal_entry.go) on every posting path,
// not by a DB constraint. normalBalance is deliberately not a column — it's
// a pure function of Type, computed by AccountNormalBalance below.
type Account struct {
	ID                 string  `db:"id"                 json:"id"`
	OrganizationID     string  `db:"organizationId"     json:"organizationId"`
	ParentID           *string `db:"parentId"            json:"parentId"`
	Code               string  `db:"code"                json:"code"`
	Name               string  `db:"name"                json:"name"`
	Type               string  `db:"type"                json:"type"`
	IsGroup            int     `db:"isGroup"             json:"isGroup"`
	IsActive           int     `db:"isActive"            json:"isActive"`
	DATEVAccountNumber *string `db:"datevAccountNumber"  json:"datevAccountNumber"`
	Description        *string `db:"description"         json:"description"`
	CreatedAt          int64   `db:"createdAt"           json:"createdAt"`
}

// AccountNormalBalance returns "debit" or "credit" for an account type —
// asset/expense accounts increase on the debit side, liability/equity/
// revenue accounts increase on the credit side. Computed, never stored, so
// it can never drift from Type.
func AccountNormalBalance(accountType string) string {
	if accountType == "asset" || accountType == "expense" {
		return "debit"
	}
	return "credit"
}

// CreateAccountRequest is the payload for creating a chart-of-accounts entry.
type CreateAccountRequest struct {
	ID                 string  `json:"id"`
	OrganizationID     string  `json:"organizationId"`
	ParentID           *string `json:"parentId"`
	Code               string  `json:"code"`
	Name               string  `json:"name"`
	Type               string  `json:"type"`
	IsGroup            int     `json:"isGroup"`
	IsActive           *int    `json:"isActive"`
	DATEVAccountNumber *string `json:"datevAccountNumber"`
	Description        *string `json:"description"`
}

// UpdateAccountRequest is the payload for updating a chart-of-accounts entry.
type UpdateAccountRequest struct {
	ParentID           *string `json:"parentId"`
	Code               string  `json:"code"`
	Name               string  `json:"name"`
	Type               string  `json:"type"`
	IsGroup            int     `json:"isGroup"`
	IsActive           int     `json:"isActive"`
	DATEVAccountNumber *string `json:"datevAccountNumber"`
	Description        *string `json:"description"`
}

func (d *Database) GetAccounts(organizationID string) ([]Account, error) {
	accounts := []Account{}
	err := d.DB.Select(&accounts,
		`SELECT * FROM accounts WHERE organizationId = ? ORDER BY code ASC`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_accounts: %w", err)
	}
	return accounts, nil
}

func (d *Database) GetAccount(accountID string) (*Account, error) {
	var account Account
	err := d.DB.Get(&account, `SELECT * FROM accounts WHERE id = ? LIMIT 1`, accountID)
	if err != nil {
		return nil, fmt.Errorf("get_account: %w", err)
	}
	return &account, nil
}

func (d *Database) CreateAccount(req CreateAccountRequest) (*Account, error) {
	if !accountTypes[req.Type] {
		return nil, newValidationError("invalid account type %q", req.Type)
	}
	if req.ID == "" {
		req.ID, _ = gonanoid.New()
	}
	isActive := 1
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	_, err := d.DB.Exec(
		`INSERT INTO accounts (id, organizationId, parentId, code, name, type, isGroup, isActive, datevAccountNumber, description)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.ParentID, req.Code, req.Name, req.Type, req.IsGroup, isActive,
		req.DATEVAccountNumber, req.Description,
	)
	if err != nil {
		if isDuplicateAccountCode(err) {
			return nil, newValidationError("an account with code %q already exists", req.Code)
		}
		return nil, fmt.Errorf("create_account: %w", err)
	}
	return d.GetAccount(req.ID)
}

func (d *Database) UpdateAccount(accountID string, updates UpdateAccountRequest) (*Account, error) {
	if !accountTypes[updates.Type] {
		return nil, newValidationError("invalid account type %q", updates.Type)
	}

	_, err := d.DB.Exec(
		`UPDATE accounts
		 SET parentId = ?, code = ?, name = ?, type = ?, isGroup = ?, isActive = ?,
		     datevAccountNumber = ?, description = ?
		 WHERE id = ?`,
		updates.ParentID, updates.Code, updates.Name, updates.Type, updates.IsGroup, updates.IsActive,
		updates.DATEVAccountNumber, updates.Description, accountID,
	)
	if err != nil {
		if isDuplicateAccountCode(err) {
			return nil, newValidationError("an account with code %q already exists", updates.Code)
		}
		return nil, fmt.Errorf("update_account: %w", err)
	}
	return d.GetAccount(accountID)
}

// isDuplicateAccountCode recognizes the raw SQLite unique-index violation on
// (organizationId, code). Mirrors isDuplicateSKU (db/product.go).
func isDuplicateAccountCode(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed") && strings.Contains(err.Error(), "accounts.code")
}

// accountReferencingColumns lists every {table, column} pair outside
// journal_lines that can point at an account: org-level GL defaults, a tax
// rate's output/input tax account, a product's revenue/expense account.
// journal_lines is counted separately below since it's keyed by accountId
// like every other domain's usage count, while these are one-off named FKs
// on their owning tables.
var accountReferencingOrganizationColumns = []string{
	"defaultArAccountId", "defaultApAccountId", "defaultRevenueAccountId",
	"defaultExpenseAccountId", "defaultCashAccountId", "fxGainAccountId",
	"fxLossAccountId", "retainedEarningsAccountId", "datevClearingAccountId",
}

// GetAccountUsageCount returns how many rows reference this account, so
// callers can decide whether it's safe to delete. Unlike GetTaxRateUsageCount
// (a single referencing column name across several tables), an account is
// referenced by many differently-named columns across several tables, so
// this is assembled by hand rather than from one generic list.
func (d *Database) GetAccountUsageCount(accountID string) (int64, error) {
	subqueries := []string{
		"(SELECT COUNT(*) FROM journal_lines WHERE accountId = ?)",
		"(SELECT COUNT(*) FROM taxRates WHERE outputTaxAccountId = ? OR inputTaxAccountId = ?)",
		"(SELECT COUNT(*) FROM products WHERE revenueAccountId = ? OR expenseAccountId = ?)",
		"(SELECT COUNT(*) FROM payments WHERE bankAccountId = ?)",
	}
	args := []any{accountID, accountID, accountID, accountID, accountID, accountID}
	for _, col := range accountReferencingOrganizationColumns {
		subqueries = append(subqueries, fmt.Sprintf("(SELECT COUNT(*) FROM organizations WHERE %s = ?)", col))
		args = append(args, accountID)
	}

	var count int64
	if err := d.DB.Get(&count, "SELECT "+strings.Join(subqueries, " + "), args...); err != nil {
		return 0, fmt.Errorf("get_account_usage_count: %w", err)
	}
	return count, nil
}

func (d *Database) getAccountChildCount(accountID string) (int64, error) {
	var count int64
	if err := d.DB.Get(&count, `SELECT COUNT(*) FROM accounts WHERE parentId = ?`, accountID); err != nil {
		return 0, fmt.Errorf("get_account_child_count: %w", err)
	}
	return count, nil
}

// DeleteAccount refuses to delete an account that has child accounts (would
// orphan them) or that's still referenced anywhere (would either fail on a
// foreign key or silently break the reference it once served).
func (d *Database) DeleteAccount(accountID string) (bool, error) {
	children, err := d.getAccountChildCount(accountID)
	if err != nil {
		return false, err
	}
	if children > 0 {
		return false, ErrAccountHasChildren
	}

	usage, err := d.GetAccountUsageCount(accountID)
	if err != nil {
		return false, err
	}
	if usage > 0 {
		return false, ErrAccountInUse
	}

	res, err := d.DB.Exec(`DELETE FROM accounts WHERE id = ?`, accountID)
	if err != nil {
		return false, fmt.Errorf("delete_account: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// defaultChartAccount is one row of the minimal generic starter chart of
// accounts seedDefaultChartOfAccounts installs for a new organization — not
// a country-specific SKR03/SKR04/PCG import, same honesty stance as the
// e-invoice generator's Peppol decision (db/einvoice.go).
type defaultChartAccount struct {
	code, name, accountType string
	isGroup                 bool
	parentCode              string // "" for a top-level group
}

var defaultChartOfAccounts = []defaultChartAccount{
	{code: "1000", name: "Assets", accountType: "asset", isGroup: true},
	{code: "1010", name: "Cash", accountType: "asset", parentCode: "1000"},
	{code: "1020", name: "Bank", accountType: "asset", parentCode: "1000"},
	{code: "1100", name: "Accounts Receivable", accountType: "asset", parentCode: "1000"},
	{code: "1200", name: "Input Tax Receivable", accountType: "asset", parentCode: "1000"},

	{code: "2000", name: "Liabilities", accountType: "liability", isGroup: true},
	{code: "2100", name: "Accounts Payable", accountType: "liability", parentCode: "2000"},
	{code: "2200", name: "Output Tax Payable", accountType: "liability", parentCode: "2000"},

	{code: "3000", name: "Equity", accountType: "equity", isGroup: true},
	{code: "3100", name: "Retained Earnings", accountType: "equity", parentCode: "3000"},

	{code: "4000", name: "Revenue", accountType: "revenue", isGroup: true},
	{code: "4100", name: "Sales Revenue", accountType: "revenue", parentCode: "4000"},
	{code: "4200", name: "Foreign Exchange Gain", accountType: "revenue", parentCode: "4000"},

	{code: "5000", name: "Expenses", accountType: "expense", isGroup: true},
	{code: "5100", name: "Purchases / Cost of Goods Sold", accountType: "expense", parentCode: "5000"},
	{code: "5200", name: "Foreign Exchange Loss", accountType: "expense", parentCode: "5000"},
}

// seedDefaultChartOfAccounts inserts the minimal generic starter chart for
// organizationID and wires organizations.default*AccountId to the created
// rows, so auto-posting (Phase 2) has somewhere to post to immediately. Runs
// on the given exec so callers can fold it into their own transaction
// (CreateOrganization) or run it standalone (the startup backfill in main.go).
func seedDefaultChartOfAccounts(exec sqlGetExecer, organizationID string) error {
	codeToID := make(map[string]string, len(defaultChartOfAccounts))
	for _, a := range defaultChartOfAccounts {
		id, err := gonanoid.New()
		if err != nil {
			return fmt.Errorf("seed_default_chart_of_accounts new_id: %w", err)
		}
		codeToID[a.code] = id

		var parentID any
		if a.parentCode != "" {
			parentID = codeToID[a.parentCode]
		}
		isGroup := 0
		if a.isGroup {
			isGroup = 1
		}
		if _, err := exec.Exec(
			`INSERT INTO accounts (id, organizationId, parentId, code, name, type, isGroup)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			id, organizationID, parentID, a.code, a.name, a.accountType, isGroup,
		); err != nil {
			return fmt.Errorf("seed_default_chart_of_accounts insert %s: %w", a.code, err)
		}
	}

	if _, err := exec.Exec(
		`UPDATE organizations
		 SET defaultArAccountId = ?, defaultApAccountId = ?, defaultRevenueAccountId = ?,
		     defaultExpenseAccountId = ?, defaultCashAccountId = ?,
		     fxGainAccountId = ?, fxLossAccountId = ?, retainedEarningsAccountId = ?
		 WHERE id = ?`,
		codeToID["1100"], codeToID["2100"], codeToID["4100"],
		codeToID["5100"], codeToID["1020"],
		codeToID["4200"], codeToID["5200"], codeToID["3100"],
		organizationID,
	); err != nil {
		return fmt.Errorf("seed_default_chart_of_accounts wire_defaults: %w", err)
	}
	return nil
}

// SeedAccountingDefaultsForAllOrganizations installs the default chart of
// accounts and journals for every organization that doesn't have any yet —
// called once at startup (main.go), mirroring EnsureFirstAdmin's
// count-then-seed shape (api/users.go), so upgrading an existing database
// backfills the new GL tables the same way a fresh install gets them.
func (d *Database) SeedAccountingDefaultsForAllOrganizations() error {
	var orgIDs []string
	if err := d.DB.Select(&orgIDs, `SELECT id FROM organizations`); err != nil {
		return fmt.Errorf("seed_accounting_defaults list_organizations: %w", err)
	}

	for _, orgID := range orgIDs {
		var accountCount int
		if err := d.DB.Get(&accountCount, `SELECT COUNT(*) FROM accounts WHERE organizationId = ?`, orgID); err != nil {
			return fmt.Errorf("seed_accounting_defaults count_accounts: %w", err)
		}
		if accountCount == 0 {
			if err := seedDefaultChartOfAccounts(d.DB, orgID); err != nil {
				return fmt.Errorf("seed_accounting_defaults chart %s: %w", orgID, err)
			}
		}

		var journalCount int
		if err := d.DB.Get(&journalCount, `SELECT COUNT(*) FROM journals WHERE organizationId = ?`, orgID); err != nil {
			return fmt.Errorf("seed_accounting_defaults count_journals: %w", err)
		}
		if journalCount == 0 {
			if err := seedDefaultJournals(d.DB, orgID); err != nil {
				return fmt.Errorf("seed_accounting_defaults journals %s: %w", orgID, err)
			}
		}
	}
	return nil
}
