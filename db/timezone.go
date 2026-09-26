package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
	// Embeds the IANA zone database so time.LoadLocation works the same in
	// the slim Docker runtime and in tests, whatever the host ships.
	_ "time/tzdata"
)

// Dates are stored as Unix-ms instants, and two conventions coexist: a form
// field left at its dayjs() default holds the moment of entry, while a date
// picked in an antd DatePicker holds *local midnight* of the picked day. A
// calendar day can only be read back correctly in the zone the date was
// entered in, which is organizations.timezone (migration 0092). Every
// server-side "which day/month/year is this timestamp" question goes
// through orgLocation; an unset zone keeps the old behaviour, UTC days.

// orgLocation resolves an organization's stored timezone. NULL, "" and an
// unloadable name (validateTimezone keeps new ones out) all resolve to UTC.
func orgLocation(tz *string) *time.Location {
	if tz == nil || *tz == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(*tz)
	if err != nil {
		return time.UTC
	}
	return loc
}

// hasOrgTimezone reports whether tz names a zone orgLocation will use,
// for the one caller (formatMillis in einvoice.go) that keeps a different
// fallback when no zone is set.
func hasOrgTimezone(tz *string) bool {
	if tz == nil || *tz == "" {
		return false
	}
	_, err := time.LoadLocation(*tz)
	return err == nil
}

// validateTimezone accepts an IANA zone name, or "" to clear back to UTC.
// A nil pointer (field omitted) passes through unchanged, the same
// convention as validateDocumentLanguage. "Local" is rejected: it would
// mean the server's own zone, which is exactly the ambiguity this setting
// removes.
func validateTimezone(p *string) error {
	if p == nil || *p == "" {
		return nil
	}
	if *p == "Local" {
		return newValidationError("time zone %q is not a valid IANA time zone", *p)
	}
	if _, err := time.LoadLocation(*p); err != nil {
		return newValidationError("time zone %q is not a valid IANA time zone", *p)
	}
	return nil
}

// organizationLocation loads organizationID's zone, for code paths that
// only have the id. A missing organization resolves to UTC rather than
// failing: every caller has already authorized and resolved the org, and a
// date's rendering is never a reason to fail a request.
func organizationLocation(q interface {
	Get(dest any, query string, args ...any) error
}, organizationID string) (*time.Location, error) {
	var tz *string
	if err := q.Get(&tz, `SELECT timezone FROM organizations WHERE id = ?`, organizationID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.UTC, nil
		}
		return nil, fmt.Errorf("organization_location: %w", err)
	}
	return orgLocation(tz), nil
}

// floorToDay truncates a Unix-ms timestamp to local midnight of its
// calendar day in loc.
func floorToDay(ms int64, loc *time.Location) time.Time {
	t := time.UnixMilli(ms).In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}
