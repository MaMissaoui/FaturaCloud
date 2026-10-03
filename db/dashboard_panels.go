package db

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// The Dashboard panels added with its 2026-10 redesign: the till, the
// customers to chase, and the products running low. Each helper takes now and
// the organization's zone so the day boundaries are testable.

// LoanFollowUpWindowDays is how many days before LoanStaleAfterDays a
// customer starts showing on the Dashboard as "close to" stalled.
const LoanFollowUpWindowDays = 10

// loanFollowUpLimit caps the customers the Dashboard names; the counts cover
// all of them.
const loanFollowUpLimit = 8

// LowStockThreshold is the quantity at or below which a stock-tracked product
// is listed as running low. Products have no reorder level yet, so this one
// number stands in for every product.
const LowStockThreshold = 2.0

// DashboardCashRegister is the till panel: the organization's default cash
// register account, today's opening/in/out/closing and yesterday's.
type DashboardCashRegister struct {
	AccountID   string               `json:"accountId"`
	AccountName string               `json:"accountName"`
	Today       DailyCashMovementRow `json:"today"`
	Yesterday   DailyCashMovementRow `json:"yesterday"`
}

// LoanFollowUpCustomer is a customer who still owes and has paid nothing on
// their loan invoices for a while (see loanRegisterCustomers). Stale means
// past LoanStaleAfterDays, the Cash Book register's stalled rule.
type LoanFollowUpCustomer struct {
	ClientID    string `json:"clientId"`
	ClientName  string `json:"clientName"`
	Outstanding int64  `json:"outstanding"`
	IdleDays    int    `json:"idleDays"`
	Stale       bool   `json:"stale"`
}

// LoanFollowUp lists the stalled customers first, then those within
// LoanFollowUpWindowDays of it, each group longest-idle first, capped at
// loanFollowUpLimit; the two counts are before the cap.
type LoanFollowUp struct {
	StaleAfterDays   int                    `json:"staleAfterDays"`
	StaleCount       int                    `json:"staleCount"`
	ApproachingCount int                    `json:"approachingCount"`
	Customers        []LoanFollowUpCustomer `json:"customers"`
}

// LowStockItem is one stock-tracked product at or below LowStockThreshold.
type LowStockItem struct {
	ProductID string  `db:"id"            json:"productId"`
	Name      string  `db:"name"          json:"name"`
	Quantity  float64 `db:"stockQuantity" json:"quantity"`
}

// LowStock is how many stock-tracked products are at or below the threshold,
// with the lowest topN of them.
type LowStock struct {
	Threshold float64        `json:"threshold"`
	Count     int            `json:"count"`
	Items     []LowStockItem `json:"items"`
}

// getDashboardCashRegister returns the till panel, or nil when there is no
// till to show: no default cash register account, one that no longer resolves
// (deleted, another organization's, a group header), or one that has never
// had a posted entry, so an organization that doesn't use the Cash Book shows
// no empty 0,00 till. Only a real database error fails the Dashboard.
func (d *Database) getDashboardCashRegister(organizationID string, now time.Time, loc *time.Location) (*DashboardCashRegister, error) {
	var accountID *string
	if err := d.DB.Get(&accountID, `SELECT defaultCashRegisterAccountId FROM organizations WHERE id = ?`, organizationID); err != nil {
		return nil, fmt.Errorf("get_dashboard_cash_register: %w", err)
	}
	if accountID == nil || *accountID == "" {
		return nil, nil
	}
	account, err := d.resolveCashReportAccount(organizationID, *accountID)
	if err != nil {
		var ve *ValidationError
		if errors.As(err, &ve) {
			return nil, nil
		}
		return nil, err
	}
	var used bool
	if err := d.DB.Get(&used, `
		SELECT EXISTS (
			SELECT 1 FROM journal_lines jl
			JOIN journal_entries je ON je.id = jl.journalEntryId
			WHERE je.organizationId = ? AND je.status IN ('posted', 'reversed') AND jl.accountId = ?
		)`, organizationID, account.ID); err != nil {
		return nil, fmt.Errorf("get_dashboard_cash_register: %w", err)
	}
	if !used {
		return nil, nil
	}

	// dailyCashMovements takes calendarDayMs-style days: UTC noon of the
	// date, read back as that date in the organization's zone.
	y, m, day := now.In(loc).Date()
	today := time.Date(y, m, day, 12, 0, 0, 0, time.UTC)
	rows, err := d.dailyCashMovements(organizationID, account.ID, today.AddDate(0, 0, -1).UnixMilli(), today.UnixMilli(), loc)
	if err != nil {
		return nil, err
	}
	if len(rows) != 2 {
		return nil, fmt.Errorf("get_dashboard_cash_register: expected 2 days, got %d", len(rows))
	}
	return &DashboardCashRegister{
		AccountID:   account.ID,
		AccountName: account.Name,
		Yesterday:   rows[0],
		Today:       rows[1],
	}, nil
}

// getLoanFollowUp builds the Dashboard's customers-to-chase list from the
// loan register (every unpaid sent invoice, Cash Book sale or not).
func (d *Database) getLoanFollowUp(organizationID string, now time.Time, loc *time.Location) (LoanFollowUp, error) {
	rows, err := d.GetLoanStatus(organizationID, "")
	if err != nil {
		return LoanFollowUp{}, err
	}
	payments, err := d.loanRegisterPayments(organizationID)
	if err != nil {
		return LoanFollowUp{}, fmt.Errorf("get_loan_follow_up: %w", err)
	}
	return loanFollowUp(loanRegisterCustomers(rows, payments, now.UnixMilli(), loc)), nil
}

// loanFollowUp picks and orders the customers to chase. Split out of
// getLoanFollowUp so the selection is testable without building invoices.
func loanFollowUp(customers map[string]loanRegisterCustomer) LoanFollowUp {
	out := LoanFollowUp{StaleAfterDays: LoanStaleAfterDays, Customers: []LoanFollowUpCustomer{}}
	from := LoanStaleAfterDays - LoanFollowUpWindowDays
	for id, c := range customers {
		if c.Outstanding <= 0 || c.IdleDays <= from {
			continue
		}
		stale := c.IdleDays > LoanStaleAfterDays
		if stale {
			out.StaleCount++
		} else {
			out.ApproachingCount++
		}
		out.Customers = append(out.Customers, LoanFollowUpCustomer{
			ClientID: id, ClientName: c.ClientName, Outstanding: c.Outstanding, IdleDays: c.IdleDays, Stale: stale,
		})
	}
	sort.Slice(out.Customers, func(i, j int) bool {
		a, b := out.Customers[i], out.Customers[j]
		if a.IdleDays != b.IdleDays {
			return a.IdleDays > b.IdleDays
		}
		if a.Outstanding != b.Outstanding {
			return a.Outstanding > b.Outstanding
		}
		return a.ClientID < b.ClientID
	})
	if len(out.Customers) > loanFollowUpLimit {
		out.Customers = out.Customers[:loanFollowUpLimit]
	}
	return out
}

// getLowStock lists the stock-tracked products at or below LowStockThreshold,
// lowest first.
func (d *Database) getLowStock(organizationID string) (LowStock, error) {
	out := LowStock{Threshold: LowStockThreshold, Items: []LowStockItem{}}
	if err := d.DB.Get(&out.Count, `
		SELECT COUNT(*) FROM products
		WHERE organizationId = ? AND stockEnabled = 1 AND stockQuantity <= ?`,
		organizationID, LowStockThreshold); err != nil {
		return LowStock{}, fmt.Errorf("get_low_stock: %w", err)
	}
	if err := d.DB.Select(&out.Items, `
		SELECT id, name, stockQuantity FROM products
		WHERE organizationId = ? AND stockEnabled = 1 AND stockQuantity <= ?
		ORDER BY stockQuantity ASC, name ASC
		LIMIT ?`,
		organizationID, LowStockThreshold, topN); err != nil {
		return LowStock{}, fmt.Errorf("get_low_stock: %w", err)
	}
	return out, nil
}
