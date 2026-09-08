package db

import (
	"database/sql"
	"errors"
	"fmt"
	"math/big"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// Import models a consolidated China shipment (F114) — a container carrying
// several vendors' purchase orders, with customs and freight assessed on the
// shipment as a whole and one exchange rate applying to it, not to each
// vendor PO in isolation.
type Import struct {
	ID             string `db:"id"             json:"id"`
	OrganizationID string `db:"organizationId" json:"organizationId"`
	ImportNumber   string `db:"importNumber"   json:"importNumber"`
	Date           int64  `db:"date"           json:"date"`
	// Currency/ExchangeRate are a *prefill default* for purchase orders
	// linked to this import — never converted or posted themselves. Each
	// linked PO/receipt still stores and freezes its own currency/
	// exchangeRate (db/exchange_rate.go); see prefillImportExchangeRate on
	// the frontend for how the New PO form applies this.
	Currency         *string `db:"currency"         json:"currency"`
	ExchangeRate     *string `db:"exchangeRate"     json:"exchangeRate"`
	ExchangeRateDate *int64  `db:"exchangeRateDate" json:"exchangeRateDate"`
	// Both cents in the organization's own functional currency — see the
	// migration 0066 comment for why these aren't foreign-currency amounts.
	FreightCost int64   `db:"freightCost" json:"freightCost"`
	CustomsCost int64   `db:"customsCost" json:"customsCost"`
	Notes       *string `db:"notes"       json:"notes"`
	CreatedAt   int64   `db:"createdAt"   json:"createdAt"`
}

type CreateImportRequest struct {
	ID               string   `json:"id"`
	OrganizationID   string   `json:"organizationId"`
	ImportNumber     string   `json:"importNumber"`
	Date             int64    `json:"date"`
	Currency         *string  `json:"currency"`
	ExchangeRate     *float64 `json:"exchangeRate"`
	ExchangeRateDate *int64   `json:"exchangeRateDate"`
	FreightCost      float64  `json:"freightCost"` // cents sent from frontend
	CustomsCost      float64  `json:"customsCost"` // cents sent from frontend
	Notes            *string  `json:"notes"`
}

type UpdateImportRequest struct {
	ImportNumber     *string  `json:"importNumber"`
	Date             *int64   `json:"date"`
	Currency         *string  `json:"currency"`
	ExchangeRate     *float64 `json:"exchangeRate"`
	ExchangeRateDate *int64   `json:"exchangeRateDate"`
	FreightCost      *float64 `json:"freightCost"`
	CustomsCost      *float64 `json:"customsCost"`
	Notes            *string  `json:"notes"`
}

func (d *Database) GetImports(organizationID string) ([]Import, error) {
	imports := []Import{}
	err := d.DB.Select(&imports, `
		SELECT * FROM imports WHERE organizationId = ? ORDER BY date DESC, createdAt DESC`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_imports: %w", err)
	}
	return imports, nil
}

func (d *Database) GetImport(id string) (*Import, error) {
	var imp Import
	err := d.DB.Get(&imp, `SELECT * FROM imports WHERE id = ? LIMIT 1`, id)
	if err != nil {
		return nil, fmt.Errorf("get_import: %w", err)
	}
	return &imp, nil
}

// NextImportNumber proposes the next "IMP-%04d" number, continuing from the
// highest in use rather than COUNT(*)+1 — same shape as
// NextPurchaseOrderNumber. SUBSTR starts at 5 because the prefix "IMP-" is
// four characters.
func (d *Database) NextImportNumber(organizationID string) string {
	var maxNumber sql.NullInt64
	_ = d.DB.Get(&maxNumber, `
		SELECT MAX(CAST(SUBSTR(importNumber, 5) AS INTEGER))
		FROM imports
		WHERE organizationId = ? AND importNumber LIKE 'IMP-%'`,
		organizationID,
	)
	return fmt.Sprintf("IMP-%04d", maxNumber.Int64+1)
}

func (d *Database) CreateImport(req CreateImportRequest) (*Import, error) {
	if req.ID == "" {
		req.ID, _ = gonanoid.New()
	}
	org, err := d.GetOrganization(req.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("create_import organization: %w", err)
	}
	exchangeRate, err := resolveExchangeRateForSave(
		orgCurrencyOrDefault(org), "", nil, req.Currency, req.ExchangeRate,
	)
	if err != nil {
		return nil, err
	}

	_, err = d.DB.Exec(`
		INSERT INTO imports (id, organizationId, importNumber, date, currency, exchangeRate,
		                      exchangeRateDate, freightCost, customsCost, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.ImportNumber, req.Date, req.Currency, exchangeRate,
		req.ExchangeRateDate, roundCents(req.FreightCost), roundCents(req.CustomsCost), req.Notes,
	)
	if err != nil {
		return nil, fmt.Errorf("create_import: %w", err)
	}
	return d.GetImport(req.ID)
}

func (d *Database) UpdateImport(id string, updates UpdateImportRequest) (*Import, error) {
	current, err := d.GetImport(id)
	if err != nil {
		return nil, fmt.Errorf("update_import fetch current: %w", err)
	}
	org, err := d.GetOrganization(current.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("update_import organization: %w", err)
	}
	currentCurrency := ""
	if current.Currency != nil {
		currentCurrency = *current.Currency
	}
	exchangeRate, err := resolveExchangeRateForSave(
		orgCurrencyOrDefault(org), currentCurrency, current.ExchangeRate,
		updates.Currency, updates.ExchangeRate,
	)
	if err != nil {
		return nil, err
	}

	var freightCost, customsCost *int64
	if updates.FreightCost != nil {
		v := roundCents(*updates.FreightCost)
		freightCost = &v
	}
	if updates.CustomsCost != nil {
		v := roundCents(*updates.CustomsCost)
		customsCost = &v
	}

	_, err = d.DB.Exec(`
		UPDATE imports
		SET importNumber     = COALESCE(?, importNumber),
		    date             = COALESCE(?, date),
		    currency         = ?,
		    exchangeRate     = ?,
		    exchangeRateDate = ?,
		    freightCost      = COALESCE(?, freightCost),
		    customsCost      = COALESCE(?, customsCost),
		    notes            = COALESCE(?, notes)
		WHERE id = ?`,
		updates.ImportNumber, updates.Date, updates.Currency, exchangeRate, updates.ExchangeRateDate,
		freightCost, customsCost, updates.Notes,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("update_import: %w", err)
	}
	return d.GetImport(id)
}

// ErrImportInUse is returned by DeleteImport when the import is still
// referenced by a purchase order. Deleting it anyway would either fail on a
// foreign key or orphan that order's landed cost allocation.
var ErrImportInUse = errors.New("import is still referenced by one or more purchase orders")

func (d *Database) GetImportPurchaseOrderCount(importID string) (int64, error) {
	var count int64
	if err := d.DB.Get(&count,
		`SELECT COUNT(*) FROM purchase_orders WHERE importId = ?`, importID,
	); err != nil {
		return 0, fmt.Errorf("get_import_purchase_order_count: %w", err)
	}
	return count, nil
}

func (d *Database) DeleteImport(id string) (bool, error) {
	count, err := d.GetImportPurchaseOrderCount(id)
	if err != nil {
		return false, err
	}
	if count > 0 {
		return false, ErrImportInUse
	}

	res, err := d.DB.Exec(`DELETE FROM imports WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("delete_import: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ImportSummary is the read-only, computed-on-read view of what an import's
// landed cost allocation will look like — the same number
// db/gl_posting.go's applyLandedCost derives at receiving time, surfaced
// here so the Imports page can show it before anything is ever received.
type ImportSummary struct {
	TotalCommittedValue int64   `json:"totalCommittedValue"` // Σ non-cancelled linked POs, org-currency cents
	FreightCost         int64   `json:"freightCost"`
	CustomsCost         int64   `json:"customsCost"`
	LandedCostRate      float64 `json:"landedCostRate"` // (freight+customs) / totalCommittedValue, 0 if undistributable
	PurchaseOrderCount  int64   `json:"purchaseOrderCount"`
}

func (d *Database) GetImportSummary(importID string) (*ImportSummary, error) {
	imp, err := d.GetImport(importID)
	if err != nil {
		return nil, fmt.Errorf("get_import_summary import: %w", err)
	}

	totalValue, poCount, err := totalCommittedPOValueForImport(d.DB, importID)
	if err != nil {
		return nil, err
	}
	totalCents := roundHalfUp(totalValue, 0).Num().Int64()

	rate := landedCostRate(imp.FreightCost+imp.CustomsCost, totalValue)
	rateFloat, _ := rate.Float64()

	return &ImportSummary{
		TotalCommittedValue: totalCents,
		FreightCost:         imp.FreightCost,
		CustomsCost:         imp.CustomsCost,
		LandedCostRate:      rateFloat,
		PurchaseOrderCount:  poCount,
	}, nil
}

// totalCommittedPOValueForImport sums Σ(quantity × unitPrice) across every
// non-cancelled purchase order linked to importID, converting each PO's own
// line items to organization-currency cents via that PO's own frozen
// exchangeRate (nil/absent = 1, i.e. already in the org's currency) — a
// cancelled PO isn't committed spend, so it's excluded the same way
// grniClearedQtyForPOLine's approved/paid filter excludes what hasn't
// actually reached the ledger. Returns the value as an exact *big.Rat
// (rounded only by the caller) so landedCostRate's division stays exact,
// plus the number of distinct non-cancelled POs contributing to it.
func totalCommittedPOValueForImport(exec sqlSelectExecer, importID string) (*big.Rat, int64, error) {
	var rows []struct {
		PurchaseOrderID string  `db:"purchaseOrderId"`
		ExchangeRate    *string `db:"exchangeRate"`
		Quantity        float64 `db:"quantity"`
		UnitPrice       int64   `db:"unitPrice"`
	}
	if err := exec.Select(&rows, `
		SELECT poli.purchaseOrderId AS purchaseOrderId, po.exchangeRate AS exchangeRate,
		       poli.quantity AS quantity, poli.unitPrice AS unitPrice
		FROM purchase_order_line_items poli
		JOIN purchase_orders po ON po.id = poli.purchaseOrderId
		WHERE po.importId = ? AND po.status != 'cancelled'`,
		importID,
	); err != nil {
		return nil, 0, fmt.Errorf("total_committed_po_value_for_import: %w", err)
	}

	total := new(big.Rat)
	poIDs := map[string]bool{}
	for _, r := range rows {
		rate, err := parseExchangeRate(r.ExchangeRate)
		if err != nil {
			return nil, 0, err
		}
		converted := convertCents(r.UnitPrice, rate)
		qty, err := floatToRat(r.Quantity)
		if err != nil {
			return nil, 0, fmt.Errorf("total_committed_po_value_for_import: invalid quantity")
		}
		total.Add(total, new(big.Rat).Mul(qty, new(big.Rat).SetInt64(converted)))
		poIDs[r.PurchaseOrderID] = true
	}
	return total, int64(len(poIDs)), nil
}

// landedCostRate is (freightCost+customsCost) / totalValue, or the zero
// rational when totalValue is zero — an import with nothing committed
// against it yet (or every linked PO cancelled) has nothing to allocate the
// cost across, which is a state to display as "0%", not an error.
func landedCostRate(freightAndCustomsCents int64, totalValue *big.Rat) *big.Rat {
	if totalValue.Sign() == 0 {
		return new(big.Rat)
	}
	return new(big.Rat).Quo(new(big.Rat).SetInt64(freightAndCustomsCents), totalValue)
}

// importIDForPOLine resolves the import (if any) that owns the purchase
// order poLineID belongs to.
func importIDForPOLine(exec sqlGetExecer, poLineID string) (*string, error) {
	var importID sql.NullString
	err := exec.Get(&importID, `
		SELECT po.importId
		FROM purchase_order_line_items poli
		JOIN purchase_orders po ON po.id = poli.purchaseOrderId
		WHERE poli.id = ?`,
		poLineID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("import_id_for_po_line: %w", err)
	}
	if !importID.Valid {
		return nil, nil
	}
	return &importID.String, nil
}
