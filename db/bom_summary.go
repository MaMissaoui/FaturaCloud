package db

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"math/big"
)

// Why a finished product can't be built right now — the first rule of
// CreateProductionOrder / UpdateProductionOrderStatus it would trip.
const (
	BOMBlockedNoRecipe            = "no-recipe"
	BOMBlockedStockNotTracked     = "stock-not-tracked"
	BOMBlockedSerializedComponent = "serialized-component"
	BOMBlockedUncostedComponent   = "uncosted-component"
	BOMBlockedShort               = "short"
)

// BOMOverview is the Bill of Materials screen's batch view: one row per
// finished product with its recipe cost and how many units the stock on hand
// allows, plus the filter chip counts.
type BOMOverview struct {
	FinishedCount int `json:"finishedCount"`
	RecipeCount   int `json:"recipeCount"`
	NoRecipe      int `json:"noRecipe"`
	// BelowCost counts recipes whose sale price (excl. tax) is lower than
	// their parts cost; NotBuildable counts recipes that can't be built now
	// (a product with no recipe is counted in NoRecipe instead).
	BelowCost    int `json:"belowCost"`
	NotBuildable int `json:"notBuildable"`
	// InventoryValuation is the organization's mode; under quantity_only
	// nothing is valued, so every cost is null.
	InventoryValuation string             `json:"inventoryValuation"`
	Recipes            []BOMRecipeSummary `json:"recipes"`
}

// BOMRecipeSummary is one finished product's recipe on the list.
type BOMRecipeSummary struct {
	ProductID      string `json:"productId"`
	ComponentCount int    `json:"componentCount"`
	// Cost is the parts cost of one finished unit at each component's
	// average cost, rounded to the cent. Null when any component has no cost
	// yet, and always under quantity-only valuation.
	Cost      *int64 `json:"cost"`
	BelowCost bool   `json:"belowCost"`
	// Buildable is how many whole units a production order could consume
	// stock for right now: the largest N whose every component line passes
	// the completion check (roundBOMQuantity(qpu×N) <= stock). 0 whenever
	// BlockedReason is set.
	Buildable     int64  `json:"buildable"`
	BlockedReason string `json:"blockedReason"`
	// LimitingComponent is the component that caps Buildable, or the one
	// behind BlockedReason (the serialized or uncosted component, the part
	// that is short).
	LimitingComponentID   *string `json:"limitingComponentId"`
	LimitingComponentName *string `json:"limitingComponentName"`
}

// BOMRecipeLine is one component of the recipe panel, for one finished unit.
type BOMRecipeLine struct {
	ComponentProductID string  `json:"componentProductId"`
	ComponentName      string  `json:"componentName"`
	ComponentSKU       *string `json:"componentSku"`
	ComponentUnit      *string `json:"componentUnit"`
	QuantityPerUnit    float64 `json:"quantityPerUnit"`
	StockQuantity      float64 `json:"stockQuantity"`
	Serialized         bool    `json:"serialized"`
	// UnitCost is the component's average cost; LineCost is QuantityPerUnit
	// × UnitCost rounded to the cent. Both null under quantity-only valuation.
	UnitCost *int64 `json:"unitCost"`
	LineCost *int64 `json:"lineCost"`
	// Buildable is how many finished units this line's stock alone allows.
	Buildable int64 `json:"buildable"`
}

// BOMRecipeDetail is the recipe panel: the row's figures plus every line,
// the latest recipe version and the finished units already in stock.
type BOMRecipeDetail struct {
	BOMRecipeSummary
	VersionNumber      *int            `json:"versionNumber"`
	FinishedStock      float64         `json:"finishedStock"`
	InventoryValuation string          `json:"inventoryValuation"`
	Lines              []BOMRecipeLine `json:"lines"`
}

type bomFinishedRow struct {
	ID            string  `db:"id"`
	Price         int64   `db:"price"`
	StockEnabled  int     `db:"stockEnabled"`
	StockQuantity float64 `db:"stockQuantity"`
}

type bomComponentRow struct {
	FinishedProductID  string  `db:"finishedProductId"`
	ComponentProductID string  `db:"componentProductId"`
	QuantityPerUnit    float64 `db:"quantityPerUnit"`
	Name               string  `db:"name"`
	SKU                *string `db:"sku"`
	Unit               *string `db:"unit"`
	StockQuantity      float64 `db:"stockQuantity"`
	Serialized         int     `db:"serialized"`
	UnitCost           *int64  `db:"unitCost"`
}

const bomComponentSelect = `
	SELECT bom.finishedProductId, bom.componentProductId, bom.quantityPerUnit,
	       p.name, p.sku, p.unit, p.stockQuantity, p.serialized, p.unitCost
	FROM bill_of_materials bom
	JOIN products p ON p.id = bom.componentProductId`

func (d *Database) GetBOMOverview(organizationID string) (BOMOverview, error) {
	org, err := d.GetOrganization(organizationID)
	if err != nil {
		return BOMOverview{}, err
	}
	if !org.MasterDataSummaries {
		return BOMOverview{}, errSummariesSwitchedOff
	}
	quantityOnly := isQuantityOnlyInventory(org)

	finished := []bomFinishedRow{}
	if err := d.DB.Select(&finished, `
		SELECT id, price, stockEnabled, stockQuantity
		FROM products
		WHERE organizationId = ? AND category = 'finished'
		ORDER BY name COLLATE NOCASE, id`,
		organizationID,
	); err != nil {
		return BOMOverview{}, fmt.Errorf("get_bom_overview products: %w", err)
	}
	components := []bomComponentRow{}
	if err := d.DB.Select(&components, bomComponentSelect+`
		WHERE bom.organizationId = ?
		ORDER BY bom.createdAt ASC, bom.rowid ASC`,
		organizationID,
	); err != nil {
		return BOMOverview{}, fmt.Errorf("get_bom_overview lines: %w", err)
	}
	byProduct := map[string][]bomComponentRow{}
	for _, c := range components {
		byProduct[c.FinishedProductID] = append(byProduct[c.FinishedProductID], c)
	}

	out := BOMOverview{
		FinishedCount:      len(finished),
		InventoryValuation: normalizeInventoryValuation(org.InventoryValuation),
		Recipes:            make([]BOMRecipeSummary, 0, len(finished)),
	}
	for _, p := range finished {
		summary, _ := computeBOMRecipe(p, byProduct[p.ID], quantityOnly)
		switch {
		case summary.BlockedReason == BOMBlockedNoRecipe:
			out.NoRecipe++
		case summary.Buildable == 0:
			out.NotBuildable++
		}
		if summary.ComponentCount > 0 {
			out.RecipeCount++
		}
		if summary.BelowCost {
			out.BelowCost++
		}
		out.Recipes = append(out.Recipes, summary)
	}
	return out, nil
}

// GetBOMRecipeDetail returns one product's recipe panel. A product of another
// organization is sql.ErrNoRows.
func (d *Database) GetBOMRecipeDetail(organizationID, productID string) (*BOMRecipeDetail, error) {
	org, err := d.GetOrganization(organizationID)
	if err != nil {
		return nil, err
	}
	if !org.MasterDataSummaries {
		return nil, errSummariesSwitchedOff
	}
	quantityOnly := isQuantityOnlyInventory(org)

	var p bomFinishedRow
	if err := d.DB.Get(&p, `
		SELECT id, price, stockEnabled, stockQuantity
		FROM products
		WHERE id = ? AND organizationId = ?`,
		productID, organizationID,
	); err != nil {
		return nil, err
	}
	components := []bomComponentRow{}
	if err := d.DB.Select(&components, bomComponentSelect+`
		WHERE bom.finishedProductId = ? AND bom.organizationId = ?
		ORDER BY bom.createdAt ASC, bom.rowid ASC`,
		productID, organizationID,
	); err != nil {
		return nil, fmt.Errorf("get_bom_recipe_detail lines: %w", err)
	}
	summary, lines := computeBOMRecipe(p, components, quantityOnly)

	detail := &BOMRecipeDetail{
		BOMRecipeSummary:   summary,
		FinishedStock:      p.StockQuantity,
		InventoryValuation: normalizeInventoryValuation(org.InventoryValuation),
		Lines:              lines,
	}
	var version int
	err = d.DB.Get(&version, `
		SELECT versionNumber FROM bill_of_materials_versions
		WHERE finishedProductId = ?
		ORDER BY versionNumber DESC LIMIT 1`,
		productID,
	)
	switch {
	case err == nil:
		detail.VersionNumber = &version
	case !errors.Is(err, sql.ErrNoRows):
		return nil, fmt.Errorf("get_bom_recipe_detail version: %w", err)
	}
	return detail, nil
}

// computeBOMRecipe works out one finished product's parts cost and buildable
// units, applying the production order rules in the order a user would meet
// them: no recipe, the finished product doesn't track stock
// (CreateProductionOrder), a serialized component (CreateProductionOrder),
// an uncosted component under perpetual valuation (completion's cost
// basis), then a component short of stock (completion's stock check).
func computeBOMRecipe(p bomFinishedRow, components []bomComponentRow, quantityOnly bool) (BOMRecipeSummary, []BOMRecipeLine) {
	summary := BOMRecipeSummary{ProductID: p.ID, ComponentCount: len(components)}
	lines := make([]BOMRecipeLine, 0, len(components))

	total := new(big.Rat)
	allCosted := true
	var serialized, uncosted *bomComponentRow
	limit := -1
	for i := range components {
		c := &components[i]
		line := BOMRecipeLine{
			ComponentProductID: c.ComponentProductID,
			ComponentName:      c.Name,
			ComponentSKU:       c.SKU,
			ComponentUnit:      c.Unit,
			QuantityPerUnit:    c.QuantityPerUnit,
			StockQuantity:      c.StockQuantity,
			Serialized:         c.Serialized == 1,
			Buildable:          maxBuildable(c.QuantityPerUnit, c.StockQuantity),
		}
		if c.Serialized == 1 && serialized == nil {
			serialized = c
		}
		if c.UnitCost == nil {
			allCosted = false
			if uncosted == nil {
				uncosted = c
			}
		} else if !quantityOnly {
			// floatToRat fails only on NaN/Inf, which a stored quantity
			// never is; a failure counts the line as uncosted.
			if qty, err := floatToRat(c.QuantityPerUnit); err == nil {
				lineCost := new(big.Rat).Mul(qty, new(big.Rat).SetInt64(*c.UnitCost))
				total.Add(total, lineCost)
				cents := roundHalfUp(lineCost, 0).Num().Int64()
				unitCost := *c.UnitCost
				line.UnitCost, line.LineCost = &unitCost, &cents
			} else {
				allCosted = false
			}
		}
		// The first line with the fewest units is the limit (recipe order).
		if limit < 0 || line.Buildable < lines[limit].Buildable {
			limit = i
		}
		lines = append(lines, line)
	}

	if !quantityOnly && allCosted && len(components) > 0 {
		cost := roundHalfUp(total, 0).Num().Int64()
		summary.Cost = &cost
		summary.BelowCost = p.Price > 0 && p.Price < cost
	}

	blockOn := func(reason string, c *bomComponentRow) {
		summary.BlockedReason = reason
		if c != nil {
			id, name := c.ComponentProductID, c.Name
			summary.LimitingComponentID, summary.LimitingComponentName = &id, &name
		}
	}
	switch {
	case len(components) == 0:
		blockOn(BOMBlockedNoRecipe, nil)
	case p.StockEnabled != 1:
		blockOn(BOMBlockedStockNotTracked, nil)
	case serialized != nil:
		blockOn(BOMBlockedSerializedComponent, serialized)
	case !quantityOnly && uncosted != nil:
		blockOn(BOMBlockedUncostedComponent, uncosted)
	default:
		summary.Buildable = lines[limit].Buildable
		limiting := components[limit]
		id, name := limiting.ComponentProductID, limiting.Name
		summary.LimitingComponentID, summary.LimitingComponentName = &id, &name
		if summary.Buildable == 0 {
			summary.BlockedReason = BOMBlockedShort
		}
	}
	return summary, lines
}

// maxBuildable is the largest whole N for which a production order's
// completion check passes on this line: roundBOMQuantity(qpu×N), the line's
// stored total, is no more than the component's stock. Negative stock allows
// nothing. The floor is only a first guess, nudged both ways because the
// stored total is rounded to 4 decimals.
func maxBuildable(qpu, stock float64) int64 {
	if qpu <= 0 || stock <= 0 {
		return 0
	}
	n := int64(math.Floor(stock/qpu + 1e-9))
	for n > 0 && roundBOMQuantity(qpu*float64(n)) > stock {
		n--
	}
	for roundBOMQuantity(qpu*float64(n+1)) <= stock {
		n++
	}
	return n
}
