package db

import (
	"errors"
	"testing"
)

// F95 (audit 2026-09-14): an organization's GL-default account columns must
// follow the same three-way convention every other nullable field on this
// request already follows — omitted keeps, "" clears, a value sets.
//
// Before this, neither spelling could unset one: nil meant "don't touch"
// through COALESCE, and "" failed the foreign key with a raw constraint
// violation. An organization could never remove a GL default once set.
func TestUpdateOrganizationAccountDefaultsThreeWay(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-f95-threeway"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if org.DefaultArAccountID == nil {
		t.Fatal("fixture: seeding left no default AR account to clear")
	}
	seeded := *org.DefaultArAccountID

	// (1) omitted -> keep. Another field changing must not disturb it.
	after, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{Name: ptr("Renamed")})
	if err != nil {
		t.Fatalf("omitted update: %v", err)
	}
	if after.DefaultArAccountID == nil || *after.DefaultArAccountID != seeded {
		t.Fatalf("omitted: defaultArAccountId = %v, want it kept at %q", after.DefaultArAccountID, seeded)
	}

	// (2) "" -> clear.
	after, err = d.UpdateOrganization(org.ID, UpdateOrganizationRequest{DefaultArAccountID: ptr("")})
	if err != nil {
		t.Fatalf("clearing update: %v", err)
	}
	if after.DefaultArAccountID != nil {
		t.Fatalf("empty string: defaultArAccountId = %q, want nil", *after.DefaultArAccountID)
	}

	// (3) a value -> set it back.
	after, err = d.UpdateOrganization(org.ID, UpdateOrganizationRequest{DefaultArAccountID: &seeded})
	if err != nil {
		t.Fatalf("set update: %v", err)
	}
	if after.DefaultArAccountID == nil || *after.DefaultArAccountID != seeded {
		t.Fatalf("set: defaultArAccountId = %v, want %q", after.DefaultArAccountID, seeded)
	}
}

// Clearing one account default must not disturb the other fourteen — the
// dynamic SET clause only ever names the columns the request carries.
func TestUpdateOrganizationClearingOneAccountLeavesTheRest(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-f95-isolation"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	before := map[string]*string{
		"ap":        org.DefaultApAccountID,
		"revenue":   org.DefaultRevenueAccountID,
		"cash":      org.DefaultCashAccountID,
		"inventory": org.DefaultInventoryAccountID,
		"grni":      org.DefaultGRNIAccountID,
		"cogs":      org.DefaultCOGSAccountID,
	}

	after, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{DefaultArAccountID: ptr("")})
	if err != nil {
		t.Fatalf("clearing update: %v", err)
	}
	if after.DefaultArAccountID != nil {
		t.Fatalf("defaultArAccountId = %q, want cleared", *after.DefaultArAccountID)
	}
	got := map[string]*string{
		"ap":        after.DefaultApAccountID,
		"revenue":   after.DefaultRevenueAccountID,
		"cash":      after.DefaultCashAccountID,
		"inventory": after.DefaultInventoryAccountID,
		"grni":      after.DefaultGRNIAccountID,
		"cogs":      after.DefaultCOGSAccountID,
	}
	for name, want := range before {
		if want == nil {
			continue
		}
		if got[name] == nil || *got[name] != *want {
			t.Errorf("%s account = %v, want it untouched at %q", name, got[name], *want)
		}
	}
}

// Every one of the fifteen is wired, not just the one the tests above
// exercise — a column missing from the accountDefaults literal would
// silently fall back to "can never be cleared", which is the whole bug.
func TestUpdateOrganizationEveryAccountDefaultCanBeCleared(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-f95-all"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	account, err := d.CreateAccount(CreateAccountRequest{
		OrganizationID: org.ID, Code: "9999", Name: "Probe", Type: "asset",
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	set := func(v *string) UpdateOrganizationRequest {
		return UpdateOrganizationRequest{
			DefaultArAccountID: v, DefaultApAccountID: v, DefaultRevenueAccountID: v,
			DefaultExpenseAccountID: v, DefaultCashAccountID: v, FxGainAccountID: v,
			FxLossAccountID: v, RetainedEarningsAccountID: v, DatevClearingAccountID: v,
			DefaultInventoryAccountID: v, DefaultGRNIAccountID: v, DefaultCOGSAccountID: v,
			DefaultInventoryAdjustmentAccountID: v, DefaultImportCostsPayableAccountID: v,
			DefaultStampDutyAccountID: v,
		}
	}

	if _, err := d.UpdateOrganization(org.ID, set(&account.ID)); err != nil {
		t.Fatalf("setting all fifteen: %v", err)
	}
	cleared, err := d.UpdateOrganization(org.ID, set(ptr("")))
	if err != nil {
		t.Fatalf("clearing all fifteen: %v", err)
	}

	for name, v := range map[string]*string{
		"defaultArAccountId":                  cleared.DefaultArAccountID,
		"defaultApAccountId":                  cleared.DefaultApAccountID,
		"defaultRevenueAccountId":             cleared.DefaultRevenueAccountID,
		"defaultExpenseAccountId":             cleared.DefaultExpenseAccountID,
		"defaultCashAccountId":                cleared.DefaultCashAccountID,
		"fxGainAccountId":                     cleared.FxGainAccountID,
		"fxLossAccountId":                     cleared.FxLossAccountID,
		"retainedEarningsAccountId":           cleared.RetainedEarningsAccountID,
		"datevClearingAccountId":              cleared.DatevClearingAccountID,
		"defaultInventoryAccountId":           cleared.DefaultInventoryAccountID,
		"defaultGRNIAccountId":                cleared.DefaultGRNIAccountID,
		"defaultCOGSAccountId":                cleared.DefaultCOGSAccountID,
		"defaultInventoryAdjustmentAccountId": cleared.DefaultInventoryAdjustmentAccountID,
		"defaultImportCostsPayableAccountId":  cleared.DefaultImportCostsPayableAccountID,
		"defaultStampDutyAccountId":           cleared.DefaultStampDutyAccountID,
	} {
		if v != nil {
			t.Errorf("%s = %q, want cleared — is it missing from the accountDefaults literal?", name, *v)
		}
	}
}

// A genuinely bad id must still be rejected rather than quietly ignored:
// only the empty string is a clear signal.
func TestUpdateOrganizationRejectsAnUnknownAccountID(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-f95-badid"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{
		DefaultArAccountID: ptr("no-such-account"),
	}); err == nil {
		t.Fatal("an unknown account id should not be accepted")
	}
}

// The convention this restores is shared, not special-cased: brandColor and
// the plain-text fields behave identically, which is the point of F95.
func TestUpdateOrganizationEmptyStringClearsPlainTextFieldsToo(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{
		ID: "org-f95-text", Email: ptr("a@b.c"),
	})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{
		BrandColor: ptr(brandColorPalette[0]),
	}); err != nil {
		t.Fatalf("set brand color: %v", err)
	}
	after, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{
		Email: ptr(""), BrandColor: ptr(""),
	})
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if after.Email == nil || *after.Email != "" {
		t.Errorf("email = %v, want the empty string", after.Email)
	}
	if after.BrandColor == nil || *after.BrandColor != "" {
		t.Errorf("brandColor = %v, want the empty string", after.BrandColor)
	}
	var verr *ValidationError
	if _, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{
		BrandColor: ptr("#123456"),
	}); !errors.As(err, &verr) {
		t.Errorf("an off-palette brand color should still be a *ValidationError, got %T", err)
	}
}

// The exact regression this guards: cmd/seed-demo stored
// "INV-{YYYY}-{NNNN}" (uppercase tokens the generator doesn't recognize)
// directly via the API, bypassing the frontend's own validateInvoiceFormat
// check — every invoice it created rendered that literal, unsubstituted
// string as its "number". Both CreateOrganization and UpdateOrganization
// must reject an unrecognized {...} token the same way the frontend does.
func TestInvoiceNumberFormatRejectsUnrecognizedToken(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)

	var verr *ValidationError
	if _, err := d.CreateOrganization(CreateOrganizationRequest{
		ID: "org-badformat-create", InvoiceNumberFormat: ptr("INV-{YYYY}-{NNNN}"),
	}); !errors.As(err, &verr) {
		t.Fatalf("CreateOrganization with an unrecognized token should be a *ValidationError, got %T (%v)", err, err)
	}

	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-badformat-update"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{
		InvoiceNumberFormat: ptr("INV-{YYYY}-{NNNN}"),
	}); !errors.As(err, &verr) {
		t.Fatalf("UpdateOrganization with an unrecognized token should be a *ValidationError, got %T (%v)", err, err)
	}

	// A recognized-token format must still be accepted, and an explicit ""
	// (clear, same three-way convention as every other plain-text column)
	// must not be treated as containing an unrecognized token.
	if _, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{
		InvoiceNumberFormat: ptr("INV-{year}-{number}"),
	}); err != nil {
		t.Fatalf("a valid format should be accepted: %v", err)
	}
	if _, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{
		InvoiceNumberFormat: ptr(""),
	}); err != nil {
		t.Fatalf("clearing to an empty string should be accepted: %v", err)
	}
}

// TestUpdateOrganizationAmountInWords confirms the amount-in-words toggle
// round-trips through a real update (it is a plain COALESCE'd 0/1 column,
// same as fiscalStampEnabled/withholdingTaxEnabled).
func TestUpdateOrganizationAmountInWords(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-words"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if org.AmountInWordsEnabled == nil || *org.AmountInWordsEnabled != 0 {
		t.Fatalf("new organization amountInWordsEnabled = %v, want 0", org.AmountInWordsEnabled)
	}

	on := int64(1)
	updated, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{AmountInWordsEnabled: &on})
	if err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}
	if updated.AmountInWordsEnabled == nil || *updated.AmountInWordsEnabled != 1 {
		t.Fatalf("amountInWordsEnabled = %v, want 1", updated.AmountInWordsEnabled)
	}
}
