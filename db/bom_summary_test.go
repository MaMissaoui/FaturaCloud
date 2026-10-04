package db

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
)

// TestBOMBuildableMatchesProductionOrders ties the buildable count to the
// real production path rather than to its formula: an order for exactly
// Buildable units completes, one for Buildable+1 is refused for stock.
func TestBOMBuildableMatchesProductionOrders(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	f := seedProductionOrderFixture(t, d, false)

	// A fractional line: 0.3333 per unit with 10 in stock allows 30 units
	// (30 × 0.3333 = 9.999), fewer than the 50 the other two lines allow.
	componentCategory := "component"
	bolt, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: f.orgID, Name: "Bolt kit", Type: "product", StockEnabled: 1,
		Category: &componentCategory, UnitCost: ptr(int64(3000)),
	})
	if err != nil {
		t.Fatalf("CreateProduct bolt: %v", err)
	}
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: f.orgID, ProductID: bolt.ID, Type: "in", Quantity: 10,
	}); err != nil {
		t.Fatalf("CreateStockMovement bolt: %v", err)
	}
	if _, err := d.ReplaceBillOfMaterials(f.finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: f.componentA.ID, QuantityPerUnit: 1},
		{ComponentProductID: f.componentB.ID, QuantityPerUnit: 2},
		{ComponentProductID: bolt.ID, QuantityPerUnit: 0.3333},
	}, 1); err != nil {
		t.Fatalf("ReplaceBillOfMaterials: %v", err)
	}

	detail, err := d.GetBOMRecipeDetail(f.orgID, f.finished.ID)
	if err != nil {
		t.Fatalf("GetBOMRecipeDetail: %v", err)
	}
	if detail.Buildable != 30 || detail.BlockedReason != "" {
		t.Fatalf("Buildable = %d (%q), want 30", detail.Buildable, detail.BlockedReason)
	}
	if detail.LimitingComponentID == nil || *detail.LimitingComponentID != bolt.ID {
		t.Errorf("LimitingComponentID = %v, want the bolt kit", detail.LimitingComponentID)
	}
	// 1 × 100.00 + 2 × 50.00 + 0.3333 × 30.00 = 209.999 → 210.00.
	if detail.Cost == nil || *detail.Cost != 21000 {
		t.Errorf("Cost = %v, want 21000", detail.Cost)
	}
	if got := []int64{detail.Lines[0].Buildable, detail.Lines[1].Buildable, detail.Lines[2].Buildable}; got[0] != 100 || got[1] != 50 || got[2] != 30 {
		t.Errorf("line buildable = %v, want [100 50 30]", got)
	}
	if detail.VersionNumber == nil || *detail.VersionNumber != 2 {
		t.Errorf("VersionNumber = %v, want 2", detail.VersionNumber)
	}

	tooMany, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: f.orgID, FinishedProductID: f.finished.ID, Quantity: float64(detail.Buildable + 1), Date: f.date,
	})
	if err != nil {
		t.Fatalf("CreateProductionOrder(buildable+1): %v", err)
	}
	if _, err := d.UpdateProductionOrderStatus(tooMany.ID, "completed", nil); err == nil ||
		!strings.Contains(err.Error(), `insufficient stock for "Bolt kit"`) {
		t.Fatalf("completing buildable+1 units: err = %v, want insufficient stock for the bolt kit", err)
	}
	exact, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: f.orgID, FinishedProductID: f.finished.ID, Quantity: float64(detail.Buildable), Date: f.date,
	})
	if err != nil {
		t.Fatalf("CreateProductionOrder(buildable): %v", err)
	}
	if _, err := d.UpdateProductionOrderStatus(exact.ID, "completed", nil); err != nil {
		t.Fatalf("completing exactly buildable units: %v", err)
	}

	after, err := d.GetBOMRecipeDetail(f.orgID, f.finished.ID)
	if err != nil {
		t.Fatalf("GetBOMRecipeDetail after: %v", err)
	}
	if after.Buildable != 0 || after.BlockedReason != BOMBlockedShort {
		t.Errorf("after building: Buildable = %d (%q), want 0 short", after.Buildable, after.BlockedReason)
	}
	if after.FinishedStock != 30 {
		t.Errorf("FinishedStock = %v, want 30", after.FinishedStock)
	}
}

func TestMaxBuildable(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		qpu, stock float64
		want       int64
	}{
		{1, 53, 53},
		{2, 100, 50},
		{0.3333, 1, 3},
		{0.3333, 10, 30},
		{0.5, 0.4999, 0},
		{0.1, 0.3, 3}, // 0.3/0.1 is 2.9999999999999996 in floats
		{1, -3, 0},
		{1, 0, 0},
	} {
		if got := maxBuildable(tc.qpu, tc.stock); got != tc.want {
			t.Errorf("maxBuildable(%v, %v) = %d, want %d", tc.qpu, tc.stock, got, tc.want)
		}
	}
}

func TestComputeBOMRecipeBlockedReasons(t *testing.T) {
	t.Parallel()
	cost := func(c int64) *int64 { return &c }
	part := func(id string, stock float64, unitCost *int64, serialized int) bomComponentRow {
		return bomComponentRow{ComponentProductID: id, Name: id, QuantityPerUnit: 1, StockQuantity: stock, UnitCost: unitCost, Serialized: serialized}
	}
	tracked := bomFinishedRow{ID: "f", Price: 1000, StockEnabled: 1}

	for _, tc := range []struct {
		name         string
		finished     bomFinishedRow
		parts        []bomComponentRow
		quantityOnly bool
		wantReason   string
		wantLimit    string
		wantBuild    int64
		wantCost     *int64
		wantBelow    bool
	}{
		{name: "no recipe", finished: tracked, wantReason: BOMBlockedNoRecipe},
		{name: "finished not tracked", finished: bomFinishedRow{ID: "f", Price: 1000},
			parts: []bomComponentRow{part("a", 5, cost(100), 0)}, wantReason: BOMBlockedStockNotTracked, wantCost: cost(100)},
		{name: "serialized component", finished: tracked,
			parts: []bomComponentRow{part("a", 5, cost(100), 0), part("s", 5, cost(100), 1)}, wantReason: BOMBlockedSerializedComponent, wantLimit: "s", wantCost: cost(200)},
		{name: "uncosted component", finished: tracked,
			parts: []bomComponentRow{part("a", 5, cost(100), 0), part("u", 5, nil, 0)}, wantReason: BOMBlockedUncostedComponent, wantLimit: "u"},
		{name: "uncosted is fine under quantity-only", finished: tracked, quantityOnly: true,
			parts: []bomComponentRow{part("a", 5, cost(100), 0), part("u", 3, nil, 0)}, wantLimit: "u", wantBuild: 3},
		{name: "negative stock is short", finished: tracked,
			parts: []bomComponentRow{part("a", 5, cost(100), 0), part("n", -2, cost(100), 0)}, wantReason: BOMBlockedShort, wantLimit: "n", wantCost: cost(200)},
		{name: "first of equal limits wins", finished: tracked,
			parts: []bomComponentRow{part("a", 4, cost(600), 0), part("b", 4, cost(600), 0)}, wantLimit: "a", wantBuild: 4, wantCost: cost(1200), wantBelow: true},
		{name: "unpriced is never below cost", finished: bomFinishedRow{ID: "f", StockEnabled: 1},
			parts: []bomComponentRow{part("a", 4, cost(600), 0)}, wantLimit: "a", wantBuild: 4, wantCost: cost(600)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, lines := computeBOMRecipe(tc.finished, tc.parts, tc.quantityOnly)
			if got.BlockedReason != tc.wantReason || got.Buildable != tc.wantBuild {
				t.Errorf("reason/buildable = %q/%d, want %q/%d", got.BlockedReason, got.Buildable, tc.wantReason, tc.wantBuild)
			}
			limit := ""
			if got.LimitingComponentID != nil {
				limit = *got.LimitingComponentID
			}
			if limit != tc.wantLimit {
				t.Errorf("limit = %q, want %q", limit, tc.wantLimit)
			}
			if (got.Cost == nil) != (tc.wantCost == nil) || (got.Cost != nil && *got.Cost != *tc.wantCost) {
				t.Errorf("cost = %v, want %v", got.Cost, tc.wantCost)
			}
			if got.BelowCost != tc.wantBelow {
				t.Errorf("BelowCost = %v, want %v", got.BelowCost, tc.wantBelow)
			}
			if len(lines) != len(tc.parts) {
				t.Errorf("%d lines, want %d", len(lines), len(tc.parts))
			}
			if tc.quantityOnly {
				for _, l := range lines {
					if l.UnitCost != nil || l.LineCost != nil {
						t.Errorf("line %s carries a cost under quantity-only", l.ComponentProductID)
					}
				}
			}
		})
	}
}

func TestBOMOverview(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	f := seedProductionOrderFixture(t, d, false)
	finishedCategory := "finished"
	// Priced below its 200.00 parts cost.
	if _, err := d.DB.Exec(`UPDATE products SET price = 15000 WHERE id = ?`, f.finished.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: f.orgID, Name: "Unplanned", Type: "product", StockEnabled: 1, Category: &finishedCategory,
	}); err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	short, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: f.orgID, Name: "Big bike", Type: "product", StockEnabled: 1, Category: &finishedCategory,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if _, err := d.ReplaceBillOfMaterials(short.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: f.componentA.ID, QuantityPerUnit: 101},
	}, 1); err != nil {
		t.Fatalf("ReplaceBillOfMaterials: %v", err)
	}

	overview, err := d.GetBOMOverview(f.orgID)
	if err != nil {
		t.Fatalf("GetBOMOverview: %v", err)
	}
	if overview.FinishedCount != 3 || overview.RecipeCount != 2 || overview.NoRecipe != 1 ||
		overview.BelowCost != 1 || overview.NotBuildable != 1 {
		t.Errorf("counts = %+v", overview)
	}
	// Ordered by name: Big bike, Motorcycle, Unplanned.
	if len(overview.Recipes) != 3 || overview.Recipes[0].ProductID != short.ID || overview.Recipes[1].ProductID != f.finished.ID {
		t.Fatalf("recipes = %+v", overview.Recipes)
	}
	if r := overview.Recipes[1]; r.Buildable != 50 || !r.BelowCost || r.Cost == nil || *r.Cost != 20000 {
		t.Errorf("Motorcycle = %+v", r)
	}
	if overview.InventoryValuation != InventoryValuationPerpetual {
		t.Errorf("InventoryValuation = %q", overview.InventoryValuation)
	}
}

func TestBOMSummariesSwitchAndOrgScope(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	f := seedProductionOrderFixture(t, d, false)
	other, err := d.CreateOrganization(CreateOrganizationRequest{ID: "bom-other"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := d.GetBOMRecipeDetail(other.ID, f.finished.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("cross-org detail err = %v, want sql.ErrNoRows", err)
	}

	off := false
	if _, err := d.UpdateOrganization(f.orgID, UpdateOrganizationRequest{MasterDataSummaries: &off}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}
	if _, err := d.GetBOMOverview(f.orgID); !errors.Is(err, errSummariesSwitchedOff) {
		t.Errorf("overview err = %v, want switched off", err)
	}
	if _, err := d.GetBOMRecipeDetail(f.orgID, f.finished.ID); !errors.Is(err, errSummariesSwitchedOff) {
		t.Errorf("detail err = %v, want switched off", err)
	}
}
