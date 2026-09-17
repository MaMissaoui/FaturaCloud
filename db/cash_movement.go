package db

import (
	"fmt"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// CashMovement mirrors the cash_movements table — see migration
// 0080_add_cash_movements for why this exists instead of routing a bank
// deposit or an undocumented expense through the payments table.
type CashMovement struct {
	ID                 string  `db:"id"                 json:"id"`
	OrganizationID     string  `db:"organizationId"     json:"organizationId"`
	AccountID          string  `db:"accountId"          json:"accountId"`
	Date               int64   `db:"date"               json:"date"`
	CounterAccountType string  `db:"counterAccountType" json:"counterAccountType"`
	CounterAccountID   string  `db:"counterAccountId"   json:"counterAccountId"`
	Amount             int64   `db:"amount"              json:"amount"`
	Note               *string `db:"note"                json:"note"`
	JournalEntryID     *string `db:"journalEntryId"      json:"journalEntryId"`
	CreatedAt          int64   `db:"createdAt"           json:"createdAt"`
}

// CreateCashMovementRequest is the payload for taking money out of a cash
// register — either deposited to the bank or spent on something with no
// vendor bill to attach a payment to (see CreateCashMovement's doc comment).
type CreateCashMovementRequest struct {
	OrganizationID string `json:"organizationId"`
	// AccountID is the register account being credited — defaults to
	// organizations.defaultCashRegisterAccountId when empty, 409 if neither
	// is set, the same "resolve-or-409" shape as CreateCashSale's
	// BankAccountID.
	AccountID string `json:"accountId"`
	Date      int64  `json:"date"`
	// CounterAccountType picks which side of the movement CounterAccountID
	// names: "bank" (a deposit — Dr a bank/asset account) or "expense" (an
	// undocumented spend — Dr an expense account). Cross-checked against
	// CounterAccountID's own accounts.type below, so picking the wrong kind
	// of account for the chosen flavor 409s instead of silently posting a
	// structurally-odd entry.
	CounterAccountType string  `json:"counterAccountType"`
	CounterAccountID   string  `json:"counterAccountId"`
	Amount             int64   `json:"amount"`
	Note               *string `json:"note"`
}

// CashMovementResult is what CreateCashMovement returns: the created
// movement row and the register account's balance immediately afterward,
// so the caller doesn't need a second round trip just to refresh it.
type CashMovementResult struct {
	CashMovement *CashMovement `json:"cashMovement"`
	Balance      int64         `json:"balance"`
}

var cashMovementCounterAccountTypes = map[string]bool{"bank": true, "expense": true}

// CreateCashMovement records money leaving a cash register: a deposit to
// the bank, or an ad-hoc expense with no vendor bill. Paying an EXISTING
// vendor bill out of the till already works today via the ordinary payments
// flow (CreatePayment with direction="outbound") — this function only
// covers the document-less case payments cannot represent (see migration
// 0080's doc comment for why).
//
// Follows CreateCashSale's exact shape and for the same reason: every read
// happens before Beginx() (db.SetMaxOpenConns(1) means a d.DB read while a
// *sqlx.Tx is open deadlocks rather than errors), then one transaction
// inserts the cash_movements row and posts its two-line entry
// (postAutoEntryTx) atomically — never a draft-then-post pair of frontend
// calls, which would reopen the exact "orphaned unbalanced entry on partial
// failure" risk CreateCashSale exists to avoid.
func (d *Database) CreateCashMovement(req CreateCashMovementRequest) (*CashMovementResult, error) {
	if req.OrganizationID == "" {
		return nil, newValidationError("organizationId is required")
	}
	if req.Amount <= 0 {
		return nil, newValidationError("amount must be positive")
	}
	if !cashMovementCounterAccountTypes[req.CounterAccountType] {
		return nil, newValidationError("invalid counter account type %q", req.CounterAccountType)
	}
	if req.CounterAccountID == "" {
		return nil, newValidationError("a counter account is required")
	}

	org, err := d.GetOrganization(req.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("create_cash_movement organization: %w", err)
	}

	accountID := req.AccountID
	if accountID == "" {
		if org.DefaultCashRegisterAccountID == nil {
			return nil, newValidationError("cannot record a cash movement: organization has no default cash register account configured")
		}
		accountID = *org.DefaultCashRegisterAccountID
	}
	registerAccount, err := d.resolveCashReportAccount(req.OrganizationID, accountID)
	if err != nil {
		return nil, err
	}

	counterAccount, err := d.GetAccount(req.CounterAccountID)
	if err != nil {
		return nil, newValidationError("counter account not found")
	}
	if err := requireSameOrg(req.OrganizationID, counterAccount.OrganizationID, "counter account"); err != nil {
		return nil, err
	}
	if counterAccount.IsGroup != 0 {
		return nil, newValidationError("counter account %q is a group header and cannot be posted to", counterAccount.Name)
	}
	if counterAccount.ID == registerAccount.ID {
		return nil, newValidationError("the counter account must be different from the register account")
	}
	switch req.CounterAccountType {
	case "bank":
		if counterAccount.Type != "asset" {
			return nil, newValidationError("counter account %q is not an asset account — pick a bank/cash account for a deposit", counterAccount.Name)
		}
	case "expense":
		if counterAccount.Type != "expense" {
			return nil, newValidationError("counter account %q is not an expense account", counterAccount.Name)
		}
	}

	// Every cash movement posts through the "cash" journal (KA) regardless
	// of where the money is going — it's the register side that defines
	// this as a cash-book transaction, the same way CreateCashSale picks
	// its settlement journal by payment method rather than destination.
	journal, err := getJournalByTypeTx(d.DB, req.OrganizationID, "cash")
	if err != nil {
		return nil, err
	}
	// Resolve-only, for an early 409 before any work happens —
	// postAutoEntryTx re-resolves inside the transaction regardless (see its
	// own doc comment).
	if _, _, err := resolveFiscalPeriodForDate(d.DB, req.OrganizationID, req.Date); err != nil {
		return nil, err
	}

	movementID, err := gonanoid.New()
	if err != nil {
		return nil, fmt.Errorf("create_cash_movement id: %w", err)
	}

	description := "Cash withdrawal"
	if req.Note != nil && *req.Note != "" {
		description = *req.Note
	}
	// Always functional-currency: a cash register and its deposit/expense
	// counter account are both this organization's own accounts, never a
	// foreign-currency document — no FX shadow columns needed.
	lines := []CreateJournalLineRequest{
		glLine(counterAccount.ID, req.Amount, 0, "", 0, nil, false, nil, nil, nil),
		glLine(registerAccount.ID, 0, req.Amount, "", 0, nil, false, nil, nil, nil),
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("create_cash_movement begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(`
		INSERT INTO cash_movements (
			id, organizationId, accountId, date, counterAccountType, counterAccountId, amount, note
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		movementID, req.OrganizationID, registerAccount.ID, req.Date,
		req.CounterAccountType, counterAccount.ID, req.Amount, req.Note,
	); err != nil {
		return nil, fmt.Errorf("create_cash_movement insert: %w", err)
	}

	entryID, err := postAutoEntryTx(
		tx, req.OrganizationID, journal.ID, "cash_movement", movementID, req.Date, "", description, lines,
	)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE cash_movements SET journalEntryId = ? WHERE id = ?`, entryID, movementID); err != nil {
		return nil, fmt.Errorf("create_cash_movement link_entry: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create_cash_movement commit: %w", err)
	}

	var movement CashMovement
	if err := d.DB.Get(&movement, `SELECT * FROM cash_movements WHERE id = ?`, movementID); err != nil {
		return nil, fmt.Errorf("create_cash_movement reload: %w", err)
	}
	// No hard balance floor: an over-withdrawal (or a backdated movement)
	// can legitimately leave the register negative for a moment — this is
	// displayed, not refused, the same "visibility, not guarantee" stance
	// GetInventoryValuation already takes for inventory drift.
	balance, err := d.GetAccountBalance(req.OrganizationID, registerAccount.ID, 0)
	if err != nil {
		return nil, err
	}

	return &CashMovementResult{CashMovement: &movement, Balance: balance}, nil
}
