package main

import (
	"strings"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func resetTestOrg(id, name string, isTest bool) db.Organization {
	return db.Organization{ID: id, Name: &name, IsTest: isTest}
}

// TestResetTarget pins audit F178: --reset deletes only a single, test-flagged
// organization whose id was confirmed, never one picked by name alone.
func TestResetTarget(t *testing.T) {
	demo := resetTestOrg("org-demo", "Demo", true)
	real := resetTestOrg("org-real", "Real Shop", false)
	orgs := []db.Organization{demo, real}

	cases := []struct {
		name      string
		orgs      []db.Organization
		orgName   string
		confirmID string
		wantID    string
		wantErr   string
	}{
		{name: "no match creates fresh", orgs: orgs, orgName: "Other", wantID: ""},
		{name: "unconfirmed prints the id", orgs: orgs, orgName: "Demo", wantErr: "--confirm-reset org-demo"},
		{name: "wrong id is refused", orgs: orgs, orgName: "Demo", confirmID: "org-real", wantErr: "--confirm-reset org-demo"},
		{name: "confirmed test org", orgs: orgs, orgName: "Demo", confirmID: "org-demo", wantID: "org-demo"},
		{name: "real org is refused even confirmed", orgs: orgs, orgName: "Real Shop", confirmID: "org-real", wantErr: "not flagged as a test organization"},
		{
			name:      "an ambiguous name is refused",
			orgs:      []db.Organization{demo, resetTestOrg("org-demo-2", "Demo", true)},
			orgName:   "Demo",
			confirmID: "org-demo",
			wantErr:   "2 organizations are named",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resetTarget(tc.orgs, tc.orgName, tc.confirmID)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v; want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			gotID := ""
			if got != nil {
				gotID = got.ID
			}
			if gotID != tc.wantID {
				t.Fatalf("target = %q; want %q", gotID, tc.wantID)
			}
		})
	}
}
