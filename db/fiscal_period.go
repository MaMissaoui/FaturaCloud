package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// ErrFiscalYearInUse is returned by DeleteFiscalYear when journal entries
// still reference it.
var ErrFiscalYearInUse = errors.New("fiscal year is still used by one or more journal entries")

// FiscalYear mirrors the fiscal_years table.
type FiscalYear struct {
	ID             string `db:"id"             json:"id"`
	OrganizationID string `db:"organizationId" json:"organizationId"`
	Name           string `db:"name"           json:"name"`
	StartDate      int64  `db:"startDate"      json:"startDate"`
	EndDate        int64  `db:"endDate"        json:"endDate"`
	Status         string `db:"status"         json:"status"`
	LockDate       *int64 `db:"lockDate"       json:"lockDate"`
	ClosedAt       *int64 `db:"closedAt"       json:"closedAt"`
	CreatedAt      int64  `db:"createdAt"      json:"createdAt"`
}

// FiscalPeriod mirrors the fiscal_periods table.
type FiscalPeriod struct {
	ID             string `db:"id"             json:"id"`
	OrganizationID string `db:"organizationId" json:"organizationId"`
	FiscalYearID   string `db:"fiscalYearId"   json:"fiscalYearId"`
	Name           string `db:"name"           json:"name"`
	StartDate      int64  `db:"startDate"      json:"startDate"`
	EndDate        int64  `db:"endDate"        json:"endDate"`
	Status         string `db:"status"         json:"status"`
	ClosedAt       *int64 `db:"closedAt"       json:"closedAt"`
	CreatedAt      int64  `db:"createdAt"      json:"createdAt"`
}

// CreateFiscalYearRequest is the payload for creating a fiscal year. Periods
// are created separately via CreateFiscalPeriod — a year with no periods yet
// is still valid (resolveFiscalPeriodForDate only requires a covering year).
type CreateFiscalYearRequest struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	Name           string `json:"name"`
	StartDate      int64  `json:"startDate"`
	EndDate        int64  `json:"endDate"`
}

// CreateFiscalPeriodRequest is the payload for creating a fiscal period
// within a fiscal year.
type CreateFiscalPeriodRequest struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	FiscalYearID   string `json:"fiscalYearId"`
	Name           string `json:"name"`
	StartDate      int64  `json:"startDate"`
	EndDate        int64  `json:"endDate"`
}

func (d *Database) GetFiscalYears(organizationID string) ([]FiscalYear, error) {
	years := []FiscalYear{}
	err := d.DB.Select(&years,
		`SELECT * FROM fiscal_years WHERE organizationId = ? ORDER BY startDate DESC`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_fiscal_years: %w", err)
	}
	return years, nil
}

func (d *Database) GetFiscalYear(fiscalYearID string) (*FiscalYear, error) {
	var year FiscalYear
	err := d.DB.Get(&year, `SELECT * FROM fiscal_years WHERE id = ? LIMIT 1`, fiscalYearID)
	if err != nil {
		return nil, fmt.Errorf("get_fiscal_year: %w", err)
	}
	return &year, nil
}

// F52 (2026-08-13 audit): a second fiscal year spanning the same dates as an
// already-open one would make resolveFiscalPeriodForDate's choice of which
// year a posting lands in ambiguous. A closed prior year legitimately
// overlaps the new year's opening days in some workflows (e.g. closing
// happens after the new year has already started posting), so overlap is
// only rejected against other *open* years — a closed one doesn't compete
// for postings anymore.
func (d *Database) CreateFiscalYear(req CreateFiscalYearRequest) (*FiscalYear, error) {
	if req.EndDate <= req.StartDate {
		return nil, newValidationError("fiscal year end date must be after its start date")
	}
	if req.ID == "" {
		req.ID, _ = gonanoid.New()
	}

	var overlapping bool
	if err := d.DB.Get(&overlapping, `
		SELECT EXISTS(
			SELECT 1 FROM fiscal_years
			WHERE organizationId = ? AND status = 'open' AND startDate <= ? AND endDate >= ?
		)`,
		req.OrganizationID, req.EndDate, req.StartDate,
	); err != nil {
		return nil, fmt.Errorf("create_fiscal_year overlap_check: %w", err)
	}
	if overlapping {
		return nil, newValidationError("this date range overlaps an existing open fiscal year")
	}

	_, err := d.DB.Exec(
		`INSERT INTO fiscal_years (id, organizationId, name, startDate, endDate) VALUES (?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.Name, req.StartDate, req.EndDate,
	)
	if err != nil {
		return nil, fmt.Errorf("create_fiscal_year: %w", err)
	}
	return d.GetFiscalYear(req.ID)
}

func (d *Database) GetFiscalPeriods(fiscalYearID string) ([]FiscalPeriod, error) {
	periods := []FiscalPeriod{}
	err := d.DB.Select(&periods,
		`SELECT * FROM fiscal_periods WHERE fiscalYearId = ? ORDER BY startDate ASC`,
		fiscalYearID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_fiscal_periods: %w", err)
	}
	return periods, nil
}

func (d *Database) GetFiscalPeriod(fiscalPeriodID string) (*FiscalPeriod, error) {
	var period FiscalPeriod
	err := d.DB.Get(&period, `SELECT * FROM fiscal_periods WHERE id = ? LIMIT 1`, fiscalPeriodID)
	if err != nil {
		return nil, fmt.Errorf("get_fiscal_period: %w", err)
	}
	return &period, nil
}

// F52 (2026-08-13 audit): a period is only meaningful as a subdivision of
// its year, so it must fall entirely inside the year's own range, and two
// periods in the same year must not overlap — resolveFiscalPeriodForDate's
// period lookup has the identical ambiguity problem CreateFiscalYear's does
// otherwise.
func (d *Database) CreateFiscalPeriod(req CreateFiscalPeriodRequest) (*FiscalPeriod, error) {
	if req.EndDate <= req.StartDate {
		return nil, newValidationError("fiscal period end date must be after its start date")
	}
	if req.ID == "" {
		req.ID, _ = gonanoid.New()
	}

	year, err := d.GetFiscalYear(req.FiscalYearID)
	if err != nil {
		return nil, err
	}
	if req.StartDate < year.StartDate || req.EndDate > year.EndDate {
		return nil, newValidationError("fiscal period must fall entirely within its fiscal year's date range")
	}

	var overlapping bool
	if err := d.DB.Get(&overlapping, `
		SELECT EXISTS(
			SELECT 1 FROM fiscal_periods
			WHERE fiscalYearId = ? AND startDate <= ? AND endDate >= ?
		)`,
		req.FiscalYearID, req.EndDate, req.StartDate,
	); err != nil {
		return nil, fmt.Errorf("create_fiscal_period overlap_check: %w", err)
	}
	if overlapping {
		return nil, newValidationError("this date range overlaps an existing fiscal period in the same year")
	}

	_, err = d.DB.Exec(
		`INSERT INTO fiscal_periods (id, organizationId, fiscalYearId, name, startDate, endDate)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.FiscalYearID, req.Name, req.StartDate, req.EndDate,
	)
	if err != nil {
		return nil, fmt.Errorf("create_fiscal_period: %w", err)
	}
	return d.GetFiscalPeriod(req.ID)
}

// UpdateFiscalPeriodStatus opens or closes a single period. Closing the
// fiscal year itself (Phase 6's CloseFiscalYear) is a bigger operation —
// closing entries, retained-earnings roll-forward — and lives separately.
func (d *Database) UpdateFiscalPeriodStatus(fiscalPeriodID, status string) (*FiscalPeriod, error) {
	if status != "open" && status != "closed" {
		return nil, newValidationError("invalid fiscal period status %q", status)
	}

	var closedAt any
	if status == "closed" {
		closedAt = time.Now().UnixMilli()
	}
	_, err := d.DB.Exec(
		`UPDATE fiscal_periods SET status = ?, closedAt = ? WHERE id = ?`,
		status, closedAt, fiscalPeriodID,
	)
	if err != nil {
		return nil, fmt.Errorf("update_fiscal_period_status: %w", err)
	}
	return d.GetFiscalPeriod(fiscalPeriodID)
}

// resolveFiscalPeriodForDate finds the open fiscal year (and, if one
// exists, the open period within it) covering date. Every posting path
// (manual post, invoice/bill auto-post, payments, closing, revaluation)
// calls this before writing a journal entry — a *ValidationError here is
// what turns "no fiscal year set up yet" into a clean 409 instead of a
// journal entry with a dangling fiscalYearId.
func resolveFiscalPeriodForDate(exec sqlGetExecer, organizationID string, date int64) (fiscalYearID string, fiscalPeriodID *string, err error) {
	// F52: ORDER BY startDate DESC makes the choice deterministic (the most
	// recently started open year wins) even in a data anomaly where
	// overlapping open years exist despite CreateFiscalYear's guard — e.g.
	// rows seeded directly, or created before this check existed. Under the
	// normal case (no overlap), there's at most one match and the ordering
	// is a no-op.
	var year FiscalYear
	err = exec.Get(&year,
		`SELECT * FROM fiscal_years
		 WHERE organizationId = ? AND status = 'open' AND startDate <= ? AND endDate >= ?
		 ORDER BY startDate DESC
		 LIMIT 1`,
		organizationID, date, date,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Distinguish "nothing was ever set up here" (point the user at
			// creating a year) from "a year exists but CloseFiscalYear
			// already closed it" (a materially different, now-common
			// situation once Phase 6 is in use — "create one first" would
			// send the user the wrong way when a year already covers this
			// date, just not an open one).
			var closedYearExists bool
			if err := exec.Get(&closedYearExists,
				`SELECT EXISTS(SELECT 1 FROM fiscal_years WHERE organizationId = ? AND status = 'closed' AND startDate <= ? AND endDate >= ?)`,
				organizationID, date, date,
			); err != nil {
				return "", nil, fmt.Errorf("resolve_fiscal_period_for_date closed_year_check: %w", err)
			}
			if closedYearExists {
				return "", nil, newValidationError("cannot post: the fiscal year covering this date is closed")
			}
			return "", nil, newValidationError("no open fiscal year covers this date — create one first")
		}
		return "", nil, fmt.Errorf("resolve_fiscal_period_for_date year: %w", err)
	}

	var period FiscalPeriod
	err = exec.Get(&period,
		`SELECT * FROM fiscal_periods
		 WHERE fiscalYearId = ? AND status = 'open' AND startDate <= ? AND endDate >= ?
		 ORDER BY startDate DESC
		 LIMIT 1`,
		year.ID, date, date,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Periods are optional — a year with none yet still accepts entries.
			return year.ID, nil, nil
		}
		return "", nil, fmt.Errorf("resolve_fiscal_period_for_date period: %w", err)
	}
	return year.ID, &period.ID, nil
}
