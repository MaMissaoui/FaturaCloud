package main

import (
	"fmt"
	"time"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// registerBalanceAsOf reads the register's real current balance via the
// same endpoint the Cash Book/Daily Cash Movements screens call
// (GetAccountBalance) — this tool doesn't keep its own shadow balance for
// the register (unlike stock.go's onHand estimate), since the balance is a
// GL derivation, not something this tool's own writes could cheaply
// replicate without duplicating db/gl_reports.go's query.
func (s *Seeder) registerBalanceAsOf(day time.Time) (int64, error) {
	var resp struct {
		Balance int64 `json:"balance"`
	}
	path := fmt.Sprintf("/api/organizations/%s/reports/account-balance?accountId=%s&asOfDate=%d",
		s.orgID, s.registerAccountID, midnightUTC(day))
	if err := s.c.Get(path, &resp); err != nil {
		return 0, err
	}
	return resp.Balance, nil
}

// maybeWithdrawFromRegister runs weekly (Monday, the same weekly cadence
// maybeStartPurchaseOrder/maybeRestockAssemblyComponents use elsewhere in
// this tool) — if the till holds more than withdrawFloorCents, deposit
// most of the excess to the organization's real Bank account
// (s.cashAccountID). Without this, 18 months of Cash Book sales with no
// offsetting movement would grow the register balance to an implausible
// figure and the Daily Cash Movements report would show only "in" activity
// — exactly the gap this generator exists to cover.
const withdrawFloorCents = 150000 // 1,500 TND — a plausible till float to leave behind

func (s *Seeder) maybeWithdrawFromRegister(day time.Time) error {
	if day.Weekday() != time.Monday {
		return nil
	}
	balance, err := s.registerBalanceAsOf(day)
	if err != nil {
		return fmt.Errorf("read register balance: %w", err)
	}
	excess := balance - withdrawFloorCents
	if excess <= 0 {
		return nil
	}
	amount := excess * int64(s.rng.IntRange(60, 85)) / 100
	if amount <= 0 {
		return nil
	}
	req := db.CreateCashMovementRequest{
		OrganizationID:     s.orgID,
		AccountID:          s.registerAccountID,
		Date:               midnightUTC(day),
		CounterAccountType: "bank",
		CounterAccountID:   s.cashAccountID,
		Amount:             amount,
		Note:               strPtr("Dépôt hebdomadaire en banque"),
	}
	if err := s.c.Post("/api/cash-movements", req, nil); err != nil {
		return fmt.Errorf("deposit to bank: %w", err)
	}
	s.stats.CashWithdrawals++
	return nil
}

// pettyCashExpenseLabels are the small undocumented spends a counter
// plausibly pays for straight out of the till, with no vendor bill to
// attach a payment to — exactly the document-less case CreateCashMovement
// exists for (see db/cash_movement.go's own doc comment).
var pettyCashExpenseLabels = []string{
	"Fournitures de bureau", "Produits d'entretien", "Petite réparation", "Carburant du livreur", "Matériel d'emballage",
}

// maybeRecordPettyCashExpense fires roughly once a month (a flat ~1/20
// daily chance, checked only on a day the register generator already
// runs) — small, deliberately infrequent, so it reads as an occasional
// real expense rather than a daily pattern.
func (s *Seeder) maybeRecordPettyCashExpense(day time.Time) error {
	if !s.rng.Chance(1.0 / 20) {
		return nil
	}
	req := db.CreateCashMovementRequest{
		OrganizationID:     s.orgID,
		AccountID:          s.registerAccountID,
		Date:               midnightUTC(day),
		CounterAccountType: "expense",
		CounterAccountID:   s.expenseAccountID,
		Amount:             int64(s.rng.IntRange(2000, 12000)), // 20-120 TND
		Note:               strPtr(Pick(s.rng, pettyCashExpenseLabels)),
	}
	if err := s.c.Post("/api/cash-movements", req, nil); err != nil {
		return fmt.Errorf("petty cash expense: %w", err)
	}
	s.stats.CashWithdrawals++
	return nil
}
