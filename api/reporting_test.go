package api

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestReportingEndDateDefaultsToNow(t *testing.T) {
	before := time.Now().UnixMilli()
	r := httptest.NewRequest("GET", "/api/organizations/org1/reporting/tax-summary", nil)
	got := reportingEndDate(r)
	after := time.Now().UnixMilli()
	if got < before || got > after {
		t.Fatalf("reportingEndDate() = %d, want between %d and %d", got, before, after)
	}
}

func TestReportingEndDateHonorsQueryParam(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/organizations/org1/reporting/tax-summary?endDate=12345", nil)
	if got := reportingEndDate(r); got != 12345 {
		t.Fatalf("reportingEndDate() = %d, want 12345", got)
	}
}

// F42: an omitted startDate must not stay unbounded at the API boundary —
// GetTaxSummary/GetRevenueByMonth have no LIMIT, so "unbounded" means
// "aggregate the organization's entire history on every call". It must fall
// back to a bounded lookback from endDate, not 0.
func TestReportingStartDateDefaultsToBoundedLookback(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/organizations/org1/reporting/tax-summary", nil)
	endDate := time.Now().UnixMilli()
	got := reportingStartDate(r, endDate)
	if got <= 0 {
		t.Fatalf("reportingStartDate() = %d, want a positive bounded timestamp, not unbounded (0)", got)
	}
	wantApprox := time.UnixMilli(endDate).AddDate(-reportingDefaultLookback, 0, 0).UnixMilli()
	if diff := got - wantApprox; diff < -1000 || diff > 1000 {
		t.Fatalf("reportingStartDate() = %d, want approximately %d (endDate - %d years)", got, wantApprox, reportingDefaultLookback)
	}
}

func TestReportingStartDateHonorsQueryParam(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/organizations/org1/reporting/tax-summary?startDate=999", nil)
	if got := reportingStartDate(r, time.Now().UnixMilli()); got != 999 {
		t.Fatalf("reportingStartDate() = %d, want 999", got)
	}
}
