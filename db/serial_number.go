package db

import (
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	gonanoid "github.com/matoous/go-nanoid/v2"
)

// isDuplicateSerialNumber recognizes the raw SQLite unique-index violation on
// (organizationId, productId, serialNumber). Mirrors isDuplicateSKU.
func isDuplicateSerialNumber(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed") &&
		strings.Contains(err.Error(), "serialNumber")
}

// SerialNumber mirrors the product_serial_numbers table. One row per
// physical unit ever registered — never deleted once created, since a
// unit's identity persists across receive/ship/return cycles.
type SerialNumber struct {
	ID             string  `db:"id"             json:"id"`
	OrganizationID string  `db:"organizationId" json:"organizationId"`
	ProductID      string  `db:"productId"      json:"productId"`
	SerialNumber   string  `db:"serialNumber"   json:"serialNumber"`
	CreatedAt      *string `db:"createdAt"      json:"createdAt"`
	// InStock is 0/1, computed on read from the sign of the unit's most
	// recent linked stockMovements row — the same "computed on read, never
	// stored" rule products.stockQuantity/unitCost already follow, so a
	// cancel/delete elsewhere can never leave this stale.
	InStock int `db:"inStock" json:"inStock"`
}

// serialStatusCTE computes each serial's current in-stock status from the
// sign of its most recent linked stockMovements row (not its `type` string,
// which carries no CHECK constraint) in one windowed pass — shared by
// GetProductSerialNumbers (whole-product listing) and lookupSerialNumbersTx
// (batch lookup by serial string), so neither pays an N+1 query per unit.
const serialStatusCTE = `
	WITH latest_serial_movement AS (
		SELECT serialNumberId, quantity,
		       ROW_NUMBER() OVER (
		         PARTITION BY serialNumberId ORDER BY createdAt DESC, rowid DESC
		       ) AS rn
		FROM stockMovements
		WHERE serialNumberId IS NOT NULL
	)`

// GetProductSerialNumbers returns every serial ever registered for a
// product, newest-first by serial string, each with its current in-stock
// status.
func (d *Database) GetProductSerialNumbers(productID string) ([]SerialNumber, error) {
	rows := []SerialNumber{}
	err := d.DB.Select(&rows, serialStatusCTE+`
		SELECT sn.*,
		       CASE WHEN COALESCE(m.quantity, 0) > 0 THEN 1 ELSE 0 END AS inStock
		FROM product_serial_numbers sn
		LEFT JOIN latest_serial_movement m ON m.serialNumberId = sn.id AND m.rn = 1
		WHERE sn.productId = ?
		ORDER BY sn.serialNumber ASC`,
		productID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_product_serial_numbers: %w", err)
	}
	return rows, nil
}

// serialLookup is one resolved serial: its row id and current in-stock sign.
type serialLookup struct {
	ID      string `db:"id"`
	InStock bool   `db:"inStock"`
}

// lookupSerialNumbersTx resolves a batch of serial strings for one product
// to {id, inStock} in a single query, keyed by the serial string. A serial
// string absent from the result map is simply unregistered.
func lookupSerialNumbersTx(exec sqlSelectExecer, productID string, serials []string) (map[string]serialLookup, error) {
	result := make(map[string]serialLookup, len(serials))
	if len(serials) == 0 {
		return result, nil
	}

	query, args, err := sqlx.In(serialStatusCTE+`
		SELECT sn.id AS id, sn.serialNumber AS serialNumber,
		       CASE WHEN COALESCE(m.quantity, 0) > 0 THEN 1 ELSE 0 END AS inStock
		FROM product_serial_numbers sn
		LEFT JOIN latest_serial_movement m ON m.serialNumberId = sn.id AND m.rn = 1
		WHERE sn.productId = ? AND sn.serialNumber IN (?)`,
		productID, serials,
	)
	if err != nil {
		return nil, fmt.Errorf("lookup_serial_numbers build: %w", err)
	}

	rows := []struct {
		ID           string `db:"id"`
		SerialNumber string `db:"serialNumber"`
		InStock      int    `db:"inStock"`
	}{}
	if err := exec.Select(&rows, query, args...); err != nil {
		return nil, fmt.Errorf("lookup_serial_numbers: %w", err)
	}
	for _, r := range rows {
		result[r.SerialNumber] = serialLookup{ID: r.ID, InStock: r.InStock == 1}
	}
	return result, nil
}

// serialInStockByIDTx computes current in-stock status for a batch of
// serial row ids (as opposed to lookupSerialNumbersTx, which resolves by
// serial *string* within one product) — used by cancel-reversal, which
// already has the exact serialNumberId values a document posted and needs
// to confirm each is still in stock before reversing it.
func serialInStockByIDTx(exec sqlSelectExecer, serialIDs []string) (map[string]bool, error) {
	result := make(map[string]bool, len(serialIDs))
	if len(serialIDs) == 0 {
		return result, nil
	}

	query, args, err := sqlx.In(serialStatusCTE+`
		SELECT sn.id AS id,
		       CASE WHEN COALESCE(m.quantity, 0) > 0 THEN 1 ELSE 0 END AS inStock
		FROM product_serial_numbers sn
		LEFT JOIN latest_serial_movement m ON m.serialNumberId = sn.id AND m.rn = 1
		WHERE sn.id IN (?)`,
		serialIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("serial_in_stock_by_id build: %w", err)
	}

	rows := []struct {
		ID      string `db:"id"`
		InStock int    `db:"inStock"`
	}{}
	if err := exec.Select(&rows, query, args...); err != nil {
		return nil, fmt.Errorf("serial_in_stock_by_id: %w", err)
	}
	for _, r := range rows {
		result[r.ID] = r.InStock == 1
	}
	return result, nil
}

// getOrCreateSerialNumbersTx resolves serials that already exist and
// creates rows for the rest, returning serial string -> id. Used only by
// the receive path, where a fresh serial is the normal case and a
// returning serial (from a prior ship-out) is also valid — the caller
// checks in-stock status separately before calling this.
func getOrCreateSerialNumbersTx(exec sqlSelectExecer, orgID, productID string, serials []string) (map[string]string, error) {
	lookups, err := lookupSerialNumbersTx(exec, productID, serials)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string, len(serials))
	for _, s := range serials {
		if lu, ok := lookups[s]; ok {
			result[s] = lu.ID
			continue
		}
		id, _ := gonanoid.New()
		if _, err := exec.Exec(
			`INSERT INTO product_serial_numbers (id, organizationId, productId, serialNumber) VALUES (?, ?, ?, ?)`,
			id, orgID, productID, s,
		); err != nil {
			// The caller already validated `s` wasn't found by
			// lookupSerialNumbersTx above; this backstop only fires on a
			// genuine race (unexpected in this app's single-writer SQLite
			// setup, but a plain 500 here would be a confusing failure mode).
			if isDuplicateSerialNumber(err) {
				return nil, newValidationError("serial %q was just registered by another request — try again", s)
			}
			return nil, fmt.Errorf("get_or_create_serial_number: %w", err)
		}
		result[s] = id
	}
	return result, nil
}

// dedupeStrings trims and deduplicates a slice of strings, dropping blanks
// and preserving first-seen order.
func dedupeStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
