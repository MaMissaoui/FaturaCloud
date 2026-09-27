package db

import "fmt"

// organizations.inventoryValuation (migration 0093) — how an organization's
// stock is valued. InventoryValuationPerpetual is the Phase 7 behaviour
// (docs/inventory-cogs-integration.md): GRNI on receipt, COGS on shipment, a
// GL trace for every manual adjustment and production residual, and a 409
// for any stock-out without a cost basis. InventoryValuationQuantityOnly
// tracks stock by quantity alone for a business that keeps no unit costs:
// none of those entries post, no stock-out ever needs a cost, and a bill for
// a stock-enabled product is expensed instead of capitalized — the periodic
// method, where the accountant values closing stock at year end. Unit cost
// stays an optional, informational field in both modes (average cost is
// still derived whenever costed inflows exist).
const (
	InventoryValuationPerpetual    = "perpetual"
	InventoryValuationQuantityOnly = "quantity_only"
)

// normalizeInventoryValuation maps a stored value to the mode actually used:
// exactly "quantity_only" selects it, and anything else — NULL, "",
// "perpetual" or an unrecognized value — is perpetual, today's behaviour.
func normalizeInventoryValuation(v *string) string {
	if v != nil && *v == InventoryValuationQuantityOnly {
		return InventoryValuationQuantityOnly
	}
	return InventoryValuationPerpetual
}

// isQuantityOnlyInventory is the single predicate every inventory GL posting
// path checks before resolving a cost or building a line.
func isQuantityOnlyInventory(org *Organization) bool {
	return normalizeInventoryValuation(org.InventoryValuation) == InventoryValuationQuantityOnly
}

func validateInventoryValuation(v *string) error {
	if v == nil || *v == "" || *v == InventoryValuationPerpetual || *v == InventoryValuationQuantityOnly {
		return nil
	}
	return newValidationError("inventory valuation %q is not one of %s, %s",
		*v, InventoryValuationPerpetual, InventoryValuationQuantityOnly)
}

// checkInventoryValuationSwitch refuses to change the mode once the
// organization has recorded any stock or carries anything on its Inventory
// account. Either direction would leave the books inconsistent: going
// quantity-only strands an Inventory/GRNI balance nothing will ever clear
// (and a bill for goods received under perpetual would expense instead of
// clearing their GRNI), while going perpetual leaves stock on hand with no
// GL value, so the first COGS entry drives Inventory negative. Only an
// actual change is checked — the Organizations drawer re-sends the whole
// form on every save, and re-saving the current (normalized) mode must
// never fail for an organization with history.
func (d *Database) checkInventoryValuationSwitch(org *Organization, requested *string) error {
	if requested == nil {
		return nil
	}
	if normalizeInventoryValuation(requested) == normalizeInventoryValuation(org.InventoryValuation) {
		return nil
	}
	var movements int
	if err := d.DB.Get(&movements, `SELECT COUNT(*) FROM stockMovements WHERE organizationId = ?`, org.ID); err != nil {
		return fmt.Errorf("check_inventory_valuation_switch movements: %w", err)
	}
	if movements > 0 {
		return newValidationError("inventory valuation can only be changed before any stock movement is recorded")
	}
	if org.DefaultInventoryAccountID != nil {
		var lines int
		if err := d.DB.Get(&lines, `
			SELECT COUNT(*) FROM journal_lines jl
			JOIN journal_entries je ON je.id = jl.journalEntryId
			WHERE je.organizationId = ? AND je.status IN ('posted', 'reversed')
			      AND jl.accountId = ?`,
			org.ID, *org.DefaultInventoryAccountID,
		); err != nil {
			return fmt.Errorf("check_inventory_valuation_switch inventory lines: %w", err)
		}
		if lines > 0 {
			return newValidationError("inventory valuation can only be changed before anything is posted to the Inventory account")
		}
	}
	return nil
}
