package db

import (
	"fmt"
	"math/big"
)

// costMovement is one row of a product's movement history, in replay order.
type costMovement struct {
	Quantity float64 `db:"quantity"`
	UnitCost *int64  `db:"unitCost"`
}

// recomputeAverageCostTx derives a product's weighted-average unit cost by
// replaying its whole movement history, and writes the result to
// products.unitCost.
//
// This mirrors how stockQuantity is always recomputed as SUM(quantity) rather
// than adjusted in place: the stored cost stays a pure function of the movement
// history, so repeated recomputation can never drift and a cancelled receipt
// unwinds by the same rules that applied it.
//
// Replay is standard weighted average, tracking running quantity and total
// value: a costed inflow adds q*unitCost to value, an uncosted inflow adds
// q*currentAverage (leaving the average untouched), and any outflow removes
// |q|*currentAverage (also leaving the average untouched).
//
// Note that cancelling a receipt does NOT restore the previous average, and
// that is correct: cancellation inserts a reversing movement, and a reversal
// removes quantity at the *current* average. After +10@100, +10@200, -5, -10
// the average runs 100 -> 150 -> 150 -> 150, leaving 5 units validly valued at
// 150. Changing the replay to return 100 would replace standard weighted-average
// costing with something non-standard.
//
// If the product has no costed inflow at all, products.unitCost is left exactly
// as it is — a manually entered cost stays the user's until real purchase data
// exists, which keeps every pre-existing product unchanged.
func recomputeAverageCostTx(exec sqlSelectExecer, productID string) error {
	movements := []costMovement{}
	// createdAt is a second-resolution TEXT timestamp, so it ties for movements
	// written in the same transaction; rowid breaks those ties in insertion
	// order and makes the replay deterministic.
	if err := exec.Select(&movements, `
		SELECT quantity, unitCost
		FROM stockMovements
		WHERE productId = ?
		ORDER BY createdAt ASC, rowid ASC`,
		productID,
	); err != nil {
		return fmt.Errorf("recompute_average_cost select: %w", err)
	}

	hasCostedInflow := false
	for _, m := range movements {
		if m.Quantity > 0 && m.UnitCost != nil {
			hasCostedInflow = true
			break
		}
	}
	if !hasCostedInflow {
		return nil
	}

	quantity := new(big.Rat)
	value := new(big.Rat)
	average := new(big.Rat)

	for _, m := range movements {
		qty, err := floatToRat(m.Quantity)
		if err != nil {
			return fmt.Errorf("recompute_average_cost quantity: %w", err)
		}

		switch {
		case m.Quantity > 0 && m.UnitCost != nil:
			value.Add(value, new(big.Rat).Mul(qty, new(big.Rat).SetInt64(*m.UnitCost)))
			quantity.Add(quantity, qty)
		default:
			// Uncosted inflows and every outflow move at the running average,
			// which leaves the average itself unchanged.
			value.Add(value, new(big.Rat).Mul(qty, average))
			quantity.Add(quantity, qty)
		}

		if quantity.Sign() > 0 {
			average = new(big.Rat).Quo(value, quantity)
		} else {
			// Stock is exhausted (or negative): keep the last known average
			// rather than resetting it, and zero the value so the next costed
			// receipt starts cleanly from its own price.
			value = new(big.Rat)
			quantity = new(big.Rat)
		}
	}

	cents := roundHalfUp(average, 0)
	if _, err := exec.Exec(
		`UPDATE products SET unitCost = ? WHERE id = ?`,
		cents.Num().Int64(), productID,
	); err != nil {
		return fmt.Errorf("recompute_average_cost update: %w", err)
	}
	return nil
}

// sqlSelectExecer is satisfied by both *sqlx.DB and *sqlx.Tx, letting
// recomputeAverageCostTx run inside a caller's transaction while still reading
// the movement history it needs.
type sqlSelectExecer interface {
	sqlExecer
	Select(dest any, query string, args ...any) error
}
