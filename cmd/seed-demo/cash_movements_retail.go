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

// maybeWithdrawFromRegister takes the day's takings to the bank when the
// counter closes, Monday to Saturday, leaving a float in the till. It runs
// after the day's sales, so the Dashboard's till shows money in and out every
// open day, and the till's balance stays a plausible float instead of growing
// with every week's cash sales (a weekly deposit of part of the excess left
// ~90,000 TND in the till).
func (s *Seeder) maybeWithdrawFromRegister(day time.Time) error {
	if day.Weekday() == time.Sunday {
		return nil
	}
	balance, err := s.registerBalanceAsOf(day)
	if err != nil {
		return fmt.Errorf("read register balance: %w", err)
	}
	float := int64(s.rng.IntRange(800, 1500)) * 100
	amount := (balance - float) / 1000 * 1000 // whole tens of dinars
	if amount < 20000 {
		return nil // not worth the trip to the bank
	}
	req := db.CreateCashMovementRequest{
		OrganizationID:     s.orgID,
		AccountID:          s.registerAccountID,
		Date:               midnightUTC(day),
		CounterAccountType: "bank",
		CounterAccountID:   s.cashAccountID,
		Amount:             amount,
		Note:               strPtr("Versement de la recette en banque"),
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
