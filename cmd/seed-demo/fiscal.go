package main

import (
	"fmt"
	"time"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// setupFiscalCoverage creates one open fiscal year per calendar year the
// [start, end] range touches (Jan 1 - Dec 31), each with 12 open monthly
// periods, so every date the simulation posts a document against has a
// fiscal year+period to resolve — see CLAUDE.md's fiscal_period.go note:
// resolveFiscalPeriodForDate requires an *open* year/period covering the
// posting date, and this tool deliberately never closes one (closing is
// irreversible — see README.md's "what this deliberately doesn't do").
func (s *Seeder) setupFiscalCoverage(start, end time.Time) error {
	for year := start.Year(); year <= end.Year(); year++ {
		yearStart := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
		yearEnd := time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC)

		fy := db.CreateFiscalYearRequest{
			OrganizationID: s.orgID,
			Name:           fmt.Sprintf("FY%d", year),
			StartDate:      midnightUTC(yearStart),
			EndDate:        midnightUTC(yearEnd),
		}
		var created db.FiscalYear
		if err := s.c.Post("/api/fiscal-years", fy, &created); err != nil {
			return fmt.Errorf("fiscal year %d: %w", year, err)
		}

		for month := 1; month <= 12; month++ {
			periodStart := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
			periodEnd := periodStart.AddDate(0, 1, -1)
			period := db.CreateFiscalPeriodRequest{
				OrganizationID: s.orgID,
				FiscalYearID:   created.ID,
				Name:           periodStart.Format("2006-01"),
				StartDate:      midnightUTC(periodStart),
				EndDate:        midnightUTC(periodEnd),
			}
			if err := s.c.Post("/api/fiscal-periods", period, nil); err != nil {
				return fmt.Errorf("fiscal period %s: %w", period.Name, err)
			}
		}
		s.log.Printf("seed-demo: fiscal year %d ready (12 periods)", year)
	}
	return nil
}
