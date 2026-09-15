package db

import (
	"errors"
	"strings"
	"testing"
)

// Remainder of F80's test-coverage cluster (audit 2026-09-14). Phase 4
// (PR #247) closed the two items the plan flagged as regressing standing
// requirements — the missing F48 concurrency test and the crossOrgProof
// entries — plus the dead production-order cancel branches. This file
// covers the enumerated branches that pass left untouched.
//
// Everything here is a branch that exists in production code and was
// reachable but never executed in CI. Nothing is a new invariant.

func assertValidationError(t *testing.T, err error, what string) *ValidationError {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected a rejection, got nil", what)
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("%s: expected *ValidationError (409), got %T: %v", what, err, err)
	}
	return verr
}

// ---------------------------------------------------------------------------
// validateSerialAgainstImportRange — only the out-of-range branch was tested.
// ---------------------------------------------------------------------------

func TestValidateSerialAgainstImportRange(t *testing.T) {
	t.Parallel()

	withRange := func(prefix *string, start, end int64) *Import {
		return &Import{
			ImportNumber:           "IMP-0001",
			SerialNumberPrefix:     prefix,
			SerialNumberRangeStart: &start,
			SerialNumberRangeEnd:   &end,
		}
	}

	cases := []struct {
		name     string
		imp      *Import
		serial   string
		wantErr  bool
		contains string
	}{
		{
			// The "absence is legal" convention: no range configured means
			// nothing to validate against, not a rejection.
			name:   "no range configured accepts anything",
			imp:    &Import{ImportNumber: "IMP-0001", SerialNumberPrefix: ptr("SN-")},
			serial: "literally-anything",
		},
		{
			// Only one half of the range set is still "not configured" —
			// validateSerialAgainstImportRange requires both.
			name: "half-configured range accepts anything",
			imp: &Import{
				ImportNumber:           "IMP-0001",
				SerialNumberRangeStart: ptr(int64(1000)),
			},
			serial: "whatever",
		},
		{
			name:   "in range",
			imp:    withRange(ptr("SN-"), 1000, 1050),
			serial: "SN-1025",
		},
		{
			name:   "at the lower bound",
			imp:    withRange(ptr("SN-"), 1000, 1050),
			serial: "SN-1000",
		},
		{
			name:   "at the upper bound",
			imp:    withRange(ptr("SN-"), 1000, 1050),
			serial: "SN-1050",
		},
		{
			name:     "prefix mismatch",
			imp:      withRange(ptr("SN-"), 1000, 1050),
			serial:   "XX-1025",
			wantErr:  true,
			contains: "does not start with",
		},
		{
			name:     "non-numeric suffix",
			imp:      withRange(ptr("SN-"), 1000, 1050),
			serial:   "SN-ABC",
			wantErr:  true,
			contains: "non-numeric suffix",
		},
		{
			// The suffix is everything after the prefix, so an empty suffix
			// fails to parse — it must not be read as zero.
			name:     "empty suffix",
			imp:      withRange(ptr("SN-"), 1000, 1050),
			serial:   "SN-",
			wantErr:  true,
			contains: "non-numeric suffix",
		},
		{
			name:     "below the range",
			imp:      withRange(ptr("SN-"), 1000, 1050),
			serial:   "SN-999",
			wantErr:  true,
			contains: "outside import",
		},
		{
			name:     "above the range",
			imp:      withRange(ptr("SN-"), 1000, 1050),
			serial:   "SN-1051",
			wantErr:  true,
			contains: "outside import",
		},
		{
			// A nil prefix means an empty prefix, which matches everything —
			// the serial is then required to be numeric on its own.
			name:   "nil prefix matches a bare number",
			imp:    withRange(nil, 1000, 1050),
			serial: "1025",
		},
		{
			name:     "nil prefix still rejects a non-numeric serial",
			imp:      withRange(nil, 1000, 1050),
			serial:   "SN-1025",
			wantErr:  true,
			contains: "non-numeric suffix",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateSerialAgainstImportRange(tc.imp, tc.serial)
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("expected %q to be accepted, got: %v", tc.serial, err)
				}
				return
			}
			verr := assertValidationError(t, err, tc.name)
			if !strings.Contains(verr.Error(), tc.contains) {
				t.Errorf("error = %q, want it to mention %q", verr.Error(), tc.contains)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CreateProductionOrder validation edges.
// ---------------------------------------------------------------------------

func TestCreateProductionOrderRejectsNonPositiveQuantity(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := seedProductionOrderFixture(t, d, false)

	for _, qty := range []float64{0, -1, -0.5} {
		_, err := d.CreateProductionOrder(CreateProductionOrderRequest{
			OrganizationID: fx.orgID, OrderNumber: "PRO-0001",
			FinishedProductID: fx.finished.ID, Quantity: qty, Date: fx.date,
		})
		verr := assertValidationError(t, err, "quantity")
		if !strings.Contains(verr.Error(), "greater than zero") {
			t.Errorf("quantity %v: error = %q, want it to mention 'greater than zero'", qty, verr.Error())
		}
	}
}

// A serialized finished product produces one registry row per physical unit,
// so a fractional quantity has no meaning — there is no such thing as half a
// serial number.
func TestCreateProductionOrderRejectsFractionalQuantityForSerializedFinishedProduct(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := seedProductionOrderFixture(t, d, true)

	_, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: fx.orgID, OrderNumber: "PRO-0001",
		FinishedProductID: fx.finished.ID, Quantity: 2.5, Date: fx.date,
	})
	verr := assertValidationError(t, err, "fractional serialized quantity")
	if !strings.Contains(verr.Error(), "whole number") {
		t.Errorf("error = %q, want it to mention 'whole number'", verr.Error())
	}

	// The same quantity is fine on a non-serialized product, which is what
	// makes this a serialization rule rather than a general one.
	d2 := newTestDB(t)
	fx2 := seedProductionOrderFixture(t, d2, false)
	if _, err := d2.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: fx2.orgID, OrderNumber: "PRO-0001",
		FinishedProductID: fx2.finished.ID, Quantity: 2.5, Date: fx2.date,
	}); err != nil {
		t.Fatalf("a fractional quantity must be legal for a non-serialized product: %v", err)
	}
}

func TestCreateProductionOrderRejectsUnknownImport(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := seedProductionOrderFixture(t, d, false)

	_, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: fx.orgID, OrderNumber: "PRO-0001",
		FinishedProductID: fx.finished.ID, Quantity: 1, Date: fx.date,
		ImportID: ptr("no-such-import"),
	})
	verr := assertValidationError(t, err, "unknown import")
	if !strings.Contains(verr.Error(), "import not found") {
		t.Errorf("error = %q, want it to mention 'import not found'", verr.Error())
	}
}

// ---------------------------------------------------------------------------
// UpdateProductionOrderStatus completion-time guards.
// ---------------------------------------------------------------------------

// CreateProductionOrder rejects a serialized component at creation, so this
// branch is only reachable if a component is toggled serialized afterwards.
// UpdateProduct only allows that toggle while stockQuantity is zero, which
// is why this fixture's component has no stock — and the serialized check
// deliberately runs before the insufficient-stock check, so it is the one
// that fires.
func TestCompleteProductionOrderRejectsAComponentTurnedSerializedAfterCreation(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := seedProductionOrderFixture(t, d, false)

	// A third component with no stock at all, so it can still be toggled.
	componentCategory := "component"
	fragile, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: fx.orgID, Name: "Ignition module", Type: "product", StockEnabled: 1,
		Category: &componentCategory, UnitCost: ptr(int64(2500)),
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if _, err := d.ReplaceBillOfMaterials(fx.finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: fragile.ID, QuantityPerUnit: 1},
	}, 1); err != nil {
		t.Fatalf("ReplaceBillOfMaterials: %v", err)
	}

	order, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: fx.orgID, OrderNumber: "PRO-0001",
		FinishedProductID: fx.finished.ID, Quantity: 1, Date: fx.date,
	})
	if err != nil {
		t.Fatalf("CreateProductionOrder: %v", err)
	}

	// UpdateProduct is a full replace, so every field the product should
	// keep has to be resent — sending Serialized alone would blank the name
	// and switch stockEnabled off, which coerces serialized straight back
	// to 0 and never reaches the branch under test.
	if _, err := d.UpdateProduct(fragile.ID, UpdateProductRequest{
		Name: fragile.Name, Type: fragile.Type, Category: ptr("component"),
		StockEnabled: 1, Serialized: 1,
	}); err != nil {
		t.Fatalf("UpdateProduct(serialized): %v", err)
	}
	reloaded, err := d.GetProduct(fragile.ID)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if reloaded.Serialized != 1 {
		t.Fatalf("fixture error: the component is not serialized after the toggle")
	}

	_, err = d.UpdateProductionOrderStatus(order.ID, "completed", nil)
	verr := assertValidationError(t, err, "serialized component at completion")
	if !strings.Contains(verr.Error(), "serialized component") {
		t.Errorf("error = %q, want it to mention 'serialized component'", verr.Error())
	}

	current, err := d.GetProductionOrder(order.ID)
	if err != nil {
		t.Fatalf("GetProductionOrder: %v", err)
	}
	if current.Status != "draft" {
		t.Errorf("status = %q, want it left at draft", current.Status)
	}
}

// resolveMovementCost 409s rather than consuming at a silent zero when a
// component has no cost basis at all — no average from a costed inflow and
// no unit cost of its own. The whole transition is refused before any stock
// is touched, the same shape as COGS on shipping.
func TestCompleteProductionOrderRejectsAComponentWithNoCostBasis(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := seedProductionOrderFixture(t, d, false)

	componentCategory := "component"
	uncosted, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: fx.orgID, Name: "Unpriced bracket", Type: "product", StockEnabled: 1,
		Category: &componentCategory,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	// Stock in, but with no unit cost — so the insufficient-stock guard
	// passes and execution reaches resolveMovementCost.
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: fx.orgID, ProductID: uncosted.ID, Type: "in", Quantity: 50,
	}); err != nil {
		t.Fatalf("CreateStockMovement: %v", err)
	}
	if _, err := d.ReplaceBillOfMaterials(fx.finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: uncosted.ID, QuantityPerUnit: 1},
	}, 1); err != nil {
		t.Fatalf("ReplaceBillOfMaterials: %v", err)
	}

	order, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: fx.orgID, OrderNumber: "PRO-0001",
		FinishedProductID: fx.finished.ID, Quantity: 1, Date: fx.date,
	})
	if err != nil {
		t.Fatalf("CreateProductionOrder: %v", err)
	}

	_, err = d.UpdateProductionOrderStatus(order.ID, "completed", nil)
	verr := assertValidationError(t, err, "no cost basis")
	// wrapCostBasisError reprefixes the neutral message per call site, so a
	// 409 always names the operation that actually failed.
	if !strings.Contains(verr.Error(), "cannot complete production order") {
		t.Errorf("error = %q, want it prefixed with the operation", verr.Error())
	}

	// Refused before anything moved.
	after, err := d.GetProduct(uncosted.ID)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if after.StockQuantity != 50 {
		t.Errorf("component stock = %v, want the untouched 50", after.StockQuantity)
	}
}

// ---------------------------------------------------------------------------
// Units of measure.
// ---------------------------------------------------------------------------

// isDefault is NOT NULL at the schema level, so a request that omits it
// entirely (a minimal body, or a direct API call) must default to 0 rather
// than hitting a raw constraint violation.
func TestCreateUnitOfMeasureDefaultsIsDefaultWhenOmitted(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-uom-nil-default"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	created, err := d.CreateUnitOfMeasure(CreateUnitOfMeasureRequest{
		OrganizationID: org.ID, Name: "testunit-omitted",
	})
	if err != nil {
		t.Fatalf("CreateUnitOfMeasure with no isDefault: %v", err)
	}
	if created.IsDefault == nil || *created.IsDefault != 0 {
		t.Errorf("isDefault = %v, want 0 — an omitted flag means 'not the default'", created.IsDefault)
	}

	// And the organization's seeded default must be untouched by it.
	units, err := d.GetUnitsOfMeasure(org.ID)
	if err != nil {
		t.Fatalf("GetUnitsOfMeasure: %v", err)
	}
	defaults := 0
	for _, u := range units {
		if u.IsDefault != nil && *u.IsDefault == 1 {
			defaults++
		}
	}
	if defaults != 1 {
		t.Errorf("default count = %d, want exactly 1 (the seeded one)", defaults)
	}
}

func TestCreateUnitOfMeasureRejectsBlankName(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-uom-blank"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	for _, name := range []string{"", "   ", "\t"} {
		_, err := d.CreateUnitOfMeasure(CreateUnitOfMeasureRequest{OrganizationID: org.ID, Name: name})
		assertValidationError(t, err, "blank name")
	}
}

// DeleteUnitOfMeasure reports found/not-found through its bool rather than
// erroring, so a missing id is a clean false — what the API maps to a 404.
func TestDeleteUnitOfMeasureOnAMissingIDReportsNotFound(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	deleted, err := d.DeleteUnitOfMeasure("no-such-unit-of-measure")
	if err != nil {
		t.Fatalf("DeleteUnitOfMeasure on a missing id should not error: %v", err)
	}
	if deleted {
		t.Error("deleted = true for an id that never existed")
	}
}

// An organization is allowed to have no default at all: unsetting the only
// default promotes nothing in its place. Recorded deliberately (audit
// 2026-09-14 F79 reviewed this and left it unchanged — isDefault is a plain
// Checkbox, so unsetting it is explicit user intent, and taxRates /
// payment_terms behave identically). This test pins that decision so a
// future change to it is a conscious one.
func TestUnsettingTheOnlyDefaultUnitOfMeasureLeavesNoDefault(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-uom-unset-default"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	units, err := d.GetUnitsOfMeasure(org.ID)
	if err != nil {
		t.Fatalf("GetUnitsOfMeasure: %v", err)
	}
	var defaultID string
	for _, u := range units {
		if u.IsDefault != nil && *u.IsDefault == 1 {
			defaultID = u.ID
		}
	}
	if defaultID == "" {
		t.Fatal("fixture error: the seeded set has no default")
	}

	if _, err := d.UpdateUnitOfMeasure(defaultID, UpdateUnitOfMeasureRequest{
		IsDefault: ptr(int64(0)),
	}); err != nil {
		t.Fatalf("UpdateUnitOfMeasure: %v", err)
	}

	after, err := d.GetUnitsOfMeasure(org.ID)
	if err != nil {
		t.Fatalf("GetUnitsOfMeasure after: %v", err)
	}
	for _, u := range after {
		if u.IsDefault != nil && *u.IsDefault == 1 {
			t.Fatalf("%q was promoted to default; unsetting must promote nothing", u.Name)
		}
	}
}

// The one-time backfill links an existing product's unitOfMeasureId only on
// an exact case-insensitive name match, leaving anything else NULL rather
// than guessing — CLAUDE.md describes all three of the gate, the
// idempotency and this matching rule, and none were tested.
func TestSeedDefaultUnitsOfMeasureBackfillsOnlyCaseInsensitiveExactMatches(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-uom-backfill"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	// CreateOrganization already seeded the list, so clear it to simulate a
	// pre-migration organization — the backfill's gate is COUNT(*) == 0.
	if _, err := d.DB.Exec(`DELETE FROM units_of_measure WHERE organizationId = ?`, org.ID); err != nil {
		t.Fatalf("clear units_of_measure: %v", err)
	}

	type probe struct {
		name, unit   string
		wantLinkedTo string
	}
	probes := []probe{
		{name: "Exact match", unit: "kg", wantLinkedTo: "kg"},
		{name: "Different case", unit: "KG", wantLinkedTo: "kg"},
		{name: "Mixed case", unit: "Piece", wantLinkedTo: "piece"},
		{name: "No match", unit: "pcs", wantLinkedTo: ""},
		{name: "Empty unit", unit: "", wantLinkedTo: ""},
	}
	ids := make([]string, len(probes))
	for i, pr := range probes {
		p, err := d.CreateProduct(CreateProductRequest{
			OrganizationID: org.ID, Name: pr.name, Type: "product", Unit: ptr(pr.unit),
		})
		if err != nil {
			t.Fatalf("CreateProduct %q: %v", pr.name, err)
		}
		ids[i] = p.ID
	}

	if err := d.SeedDefaultUnitsOfMeasureForAllOrganizations(); err != nil {
		t.Fatalf("SeedDefaultUnitsOfMeasureForAllOrganizations: %v", err)
	}

	byName := map[string]string{}
	units, err := d.GetUnitsOfMeasure(org.ID)
	if err != nil {
		t.Fatalf("GetUnitsOfMeasure: %v", err)
	}
	for _, u := range units {
		byName[u.Name] = u.ID
	}

	for i, pr := range probes {
		p, err := d.GetProduct(ids[i])
		if err != nil {
			t.Fatalf("GetProduct %q: %v", pr.name, err)
		}
		if pr.wantLinkedTo == "" {
			if p.UnitOfMeasureID != nil {
				t.Errorf("%s (unit %q): linked to %v, want left NULL rather than guessed",
					pr.name, pr.unit, *p.UnitOfMeasureID)
			}
			continue
		}
		want, ok := byName[pr.wantLinkedTo]
		if !ok {
			t.Fatalf("fixture error: seeded set has no %q row", pr.wantLinkedTo)
		}
		if p.UnitOfMeasureID == nil || *p.UnitOfMeasureID != want {
			t.Errorf("%s (unit %q): linked to %v, want the %q row",
				pr.name, pr.unit, p.UnitOfMeasureID, pr.wantLinkedTo)
		}
	}
}

// ---------------------------------------------------------------------------
// Bill of materials.
// ---------------------------------------------------------------------------

// A first save with no batchSize (or a nonsensical one) settles on 1 rather
// than storing a zero that every later scale-divide would divide by.
//
// This needs a finished product with NO version history: batchSize <= 0 on
// a product that already has versions deliberately *inherits* the latest
// version's batch size instead (so the product form's embedded BOM card,
// which has no batch UI and always sends 0, can't stomp what the dedicated
// drawer last saved). The fixture's own finished product already has a
// recipe, so reusing it would exercise the inheritance branch and pass for
// the wrong reason.
func TestReplaceBillOfMaterialsFirstSaveCoercesBatchSizeToOne(t *testing.T) {
	t.Parallel()
	for _, batch := range []int{0, -3} {
		d := newTestDB(t)
		fx := seedProductionOrderFixture(t, d, false)

		finishedCategory := "finished"
		fresh, err := d.CreateProduct(CreateProductRequest{
			OrganizationID: fx.orgID, Name: "Scooter", Type: "product", StockEnabled: 1,
			Category: &finishedCategory,
		})
		if err != nil {
			t.Fatalf("CreateProduct: %v", err)
		}
		if existing, err := d.GetBillOfMaterialsVersions(fresh.ID); err != nil || len(existing) != 0 {
			t.Fatalf("fixture error: fresh product already has %d versions (err=%v)", len(existing), err)
		}

		if _, err := d.ReplaceBillOfMaterials(fresh.ID, []CreateBillOfMaterialsLineRequest{
			{ComponentProductID: fx.componentA.ID, QuantityPerUnit: 2},
		}, batch); err != nil {
			t.Fatalf("ReplaceBillOfMaterials(batchSize=%d): %v", batch, err)
		}

		versions, err := d.GetBillOfMaterialsVersions(fresh.ID)
		if err != nil || len(versions) != 1 {
			t.Fatalf("GetBillOfMaterialsVersions: err=%v len=%d", err, len(versions))
		}
		if versions[0].BatchSize != 1 {
			t.Errorf("batchSize %d stored as %d, want coerced to 1", batch, versions[0].BatchSize)
		}
	}
}

// The counterpart to the above: on a product that DOES have history,
// batchSize <= 0 inherits the latest version's rather than resetting to 1.
func TestReplaceBillOfMaterialsInheritsBatchSizeOnAProductWithHistory(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := seedProductionOrderFixture(t, d, false)

	if _, err := d.ReplaceBillOfMaterials(fx.finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: fx.componentA.ID, QuantityPerUnit: 1},
	}, 6); err != nil {
		t.Fatalf("ReplaceBillOfMaterials(batchSize=6): %v", err)
	}
	// A caller with no batch UI sends 0 alongside a genuine line change.
	if _, err := d.ReplaceBillOfMaterials(fx.finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: fx.componentA.ID, QuantityPerUnit: 4},
	}, 0); err != nil {
		t.Fatalf("ReplaceBillOfMaterials(batchSize=0): %v", err)
	}

	versions, err := d.GetBillOfMaterialsVersions(fx.finished.ID)
	if err != nil {
		t.Fatalf("GetBillOfMaterialsVersions: %v", err)
	}
	newest := versions[0]
	for _, v := range versions {
		if v.VersionNumber > newest.VersionNumber {
			newest = v
		}
	}
	if newest.BatchSize != 6 {
		t.Errorf("batchSize = %d, want the inherited 6 — a 0 from a caller with no batch UI must not reset it",
			newest.BatchSize)
	}
}

// Restore is itself a ReplaceBillOfMaterials call using the restored
// version's lines, so the new version it creates must carry the *restored*
// batchSize, not whatever the latest version happened to be saved at.
func TestRestoreBillOfMaterialsVersionCarriesTheRestoredBatchSize(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := seedProductionOrderFixture(t, d, false)

	if _, err := d.ReplaceBillOfMaterials(fx.finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: fx.componentA.ID, QuantityPerUnit: 3},
	}, 3); err != nil {
		t.Fatalf("ReplaceBillOfMaterials v1: %v", err)
	}
	if _, err := d.ReplaceBillOfMaterials(fx.finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: fx.componentB.ID, QuantityPerUnit: 7},
	}, 10); err != nil {
		t.Fatalf("ReplaceBillOfMaterials v2: %v", err)
	}

	versions, err := d.GetBillOfMaterialsVersions(fx.finished.ID)
	if err != nil {
		t.Fatalf("GetBillOfMaterialsVersions: %v", err)
	}
	// seedProductionOrderFixture already saved a recipe, so the two saves
	// above are versions 2 and 3 — find the one to restore by its batch
	// size rather than assuming a version number.
	var target *BOMVersion
	for i := range versions {
		if versions[i].BatchSize == 3 {
			target = &versions[i]
		}
	}
	if target == nil {
		t.Fatalf("no version with batchSize 3 among %d versions", len(versions))
	}
	restoredFrom := target.VersionNumber

	if _, err := d.RestoreBillOfMaterialsVersion(fx.finished.ID, target.ID); err != nil {
		t.Fatalf("RestoreBillOfMaterialsVersion: %v", err)
	}

	after, err := d.GetBillOfMaterialsVersions(fx.finished.ID)
	if err != nil {
		t.Fatalf("GetBillOfMaterialsVersions after: %v", err)
	}
	newest := after[0]
	for _, v := range after {
		if v.VersionNumber > newest.VersionNumber {
			newest = v
		}
	}
	if newest.VersionNumber != len(versions)+1 {
		t.Errorf("restore produced version %d, want %d (restore creates, never rewrites)",
			newest.VersionNumber, len(versions)+1)
	}
	if newest.VersionNumber == restoredFrom {
		t.Error("restore rewrote the version it restored from instead of creating a new one")
	}
	if newest.BatchSize != 3 {
		t.Errorf("restored version's batchSize = %d, want the restored 3, not the latest 10", newest.BatchSize)
	}
}

// Clearing a recipe is a real change and must be recorded as one, so the
// history shows the recipe existed and was emptied rather than going silent.
func TestClearingABillOfMaterialsRecordsAVersion(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := seedProductionOrderFixture(t, d, false)

	before, err := d.GetBillOfMaterialsVersions(fx.finished.ID)
	if err != nil {
		t.Fatalf("GetBillOfMaterialsVersions: %v", err)
	}

	if _, err := d.ReplaceBillOfMaterials(fx.finished.ID, []CreateBillOfMaterialsLineRequest{}, 1); err != nil {
		t.Fatalf("ReplaceBillOfMaterials(empty): %v", err)
	}

	after, err := d.GetBillOfMaterialsVersions(fx.finished.ID)
	if err != nil {
		t.Fatalf("GetBillOfMaterialsVersions after: %v", err)
	}
	if len(after) != len(before)+1 {
		t.Fatalf("versions %d -> %d, want exactly one more for the clear", len(before), len(after))
	}

	lines, err := d.GetBillOfMaterials(fx.finished.ID)
	if err != nil {
		t.Fatalf("GetBillOfMaterials: %v", err)
	}
	if len(lines) != 0 {
		t.Errorf("live recipe has %d lines, want 0", len(lines))
	}
}

// Reordering the same component set is NOT detected as a change — the
// comparison is by component and quantity, not by position. Recorded as a
// known limitation (the audit's watch list) rather than a bug: pinning it
// means a future decision to make ordering significant has to change this
// test deliberately.
func TestReorderingBillOfMaterialsLinesIsNotRecordedAsAChange(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := seedProductionOrderFixture(t, d, false)

	if _, err := d.ReplaceBillOfMaterials(fx.finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: fx.componentA.ID, QuantityPerUnit: 1},
		{ComponentProductID: fx.componentB.ID, QuantityPerUnit: 2},
	}, 1); err != nil {
		t.Fatalf("ReplaceBillOfMaterials: %v", err)
	}
	before, err := d.GetBillOfMaterialsVersions(fx.finished.ID)
	if err != nil {
		t.Fatalf("GetBillOfMaterialsVersions: %v", err)
	}

	// Same set, same quantities, opposite order.
	if _, err := d.ReplaceBillOfMaterials(fx.finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: fx.componentB.ID, QuantityPerUnit: 2},
		{ComponentProductID: fx.componentA.ID, QuantityPerUnit: 1},
	}, 1); err != nil {
		t.Fatalf("ReplaceBillOfMaterials reordered: %v", err)
	}

	after, err := d.GetBillOfMaterialsVersions(fx.finished.ID)
	if err != nil {
		t.Fatalf("GetBillOfMaterialsVersions after: %v", err)
	}
	if len(after) != len(before) {
		t.Errorf("reordering recorded a new version (%d -> %d); ordering is deliberately not significant",
			len(before), len(after))
	}
}
