package db

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"
)

// ProductSummaryList is the batch overview for the Products screen: the
// headline (stock value at average cost, units in stock) and the filter chip
// counts. The list itself stays the paginated GET .../products.
type ProductSummaryList struct {
	ItemCount    int `json:"itemCount"`
	StockTracked int `json:"stockTracked"`
	Services     int `json:"services"`
	// OutOfStock and LowStock count stock-tracked products at or below 0 and
	// at or below LowStockThreshold (out of stock included) — the same rule
	// as the Dashboard's low-stock panel and GetProducts' Stock filter.
	OutOfStock        int     `json:"outOfStock"`
	LowStock          int     `json:"lowStock"`
	LowStockThreshold float64 `json:"lowStockThreshold"`
	// StockValue and Units are getStockValuation's Total and Units, so the
	// headline equals the Dashboard's stock figure by construction.
	StockValue int64   `json:"stockValue"`
	Units      float64 `json:"units"`
	// InventoryValuation is the organization's mode ("perpetual" or
	// "quantity_only"); under quantity_only nothing is valued, so the screen
	// shows units and no cost or margin.
	InventoryValuation string `json:"inventoryValuation"`
}

// ProductMovement is one stock movement on the product panel, resolved to the
// document that caused it: its date, kind, number and counterparty.
type ProductMovement struct {
	ID       string  `json:"id"`
	Date     int64   `json:"date"`
	Type     string  `json:"type"`
	Quantity float64 `json:"quantity"`
	// DocumentKind is "invoice", "delivery" (outbound), "receipt" (inbound
	// delivery), "production" or "" (a manual movement, a stock count, or a
	// reference that matches no single document).
	DocumentKind string  `json:"documentKind"`
	DocumentID   *string `json:"documentId"`
	Reference    *string `json:"reference"`
	// CounterpartyKind is "client", "vendor" or "". The API strips the
	// counterparty for a role that may not see it (api/product_summary.go).
	CounterpartyKind string  `json:"counterpartyKind"`
	CounterpartyID   *string `json:"counterpartyId"`
	CounterpartyName *string `json:"counterpartyName"`
}

// ProductLastVendor is the vendor of the most recent approved or paid
// incoming invoice with a line for the product.
type ProductLastVendor struct {
	VendorID          string `db:"vendorId"          json:"vendorId"`
	Name              string `db:"name"              json:"name"`
	Date              int64  `db:"date"              json:"date"`
	IncomingInvoiceID string `db:"incomingInvoiceId" json:"incomingInvoiceId"`
}

// ProductSummary is the single-product detail view.
type ProductSummary struct {
	ProductID       string             `json:"productId"`
	MovementCount   int                `json:"movementCount"`
	RecentMovements []ProductMovement  `json:"recentMovements"`
	LastVendor      *ProductLastVendor `json:"lastVendor"`
}

// productRecentMovements is how many movements the panel lists.
const productRecentMovements = 5

func (d *Database) GetProductSummaries(organizationID string) (ProductSummaryList, error) {
	org, err := d.GetOrganization(organizationID)
	if err != nil {
		return ProductSummaryList{}, err
	}
	if !org.MasterDataSummaries {
		return ProductSummaryList{}, errSummariesSwitchedOff
	}

	out := ProductSummaryList{
		LowStockThreshold:  LowStockThreshold,
		InventoryValuation: normalizeInventoryValuation(org.InventoryValuation),
	}
	var counts struct {
		ItemCount    int `db:"itemCount"`
		StockTracked int `db:"stockTracked"`
		Services     int `db:"services"`
		OutOfStock   int `db:"outOfStock"`
		LowStock     int `db:"lowStock"`
	}
	if err := d.DB.Get(&counts, `
		SELECT COUNT(*) AS itemCount,
		       COALESCE(SUM(stockEnabled = 1), 0) AS stockTracked,
		       COALESCE(SUM(type = 'service'), 0) AS services,
		       COALESCE(SUM(stockEnabled = 1 AND stockQuantity <= 0), 0) AS outOfStock,
		       COALESCE(SUM(stockEnabled = 1 AND stockQuantity <= ?), 0) AS lowStock
		FROM products
		WHERE organizationId = ?`,
		LowStockThreshold, organizationID,
	); err != nil {
		return ProductSummaryList{}, fmt.Errorf("get_product_summaries counts: %w", err)
	}
	out.ItemCount = counts.ItemCount
	out.StockTracked = counts.StockTracked
	out.Services = counts.Services
	out.OutOfStock = counts.OutOfStock
	out.LowStock = counts.LowStock

	valuation, err := d.getStockValuation(organizationID)
	if err != nil {
		return ProductSummaryList{}, fmt.Errorf("get_product_summaries: %w", err)
	}
	out.StockValue = valuation.Total
	out.Units = valuation.Units
	return out, nil
}

// productMovementRow is one stockMovements row with every document it might
// belong to joined in. A movement names its document by sourceDocumentId
// (Cash Book sales, serialized lines, production orders) or only by its
// reference, the delivery number (ordinary delivery and receipt lines,
// including a cancelled delivery's "in" restore). The reference is matched
// against both delivery tables regardless of the movement's type, and only
// when exactly one delivery in the organization carries that number:
// numbering formats are configurable, so a number can repeat. A delivery's
// client is its order's, else its own (outboundDeliverySelect's rule).
type productMovementRow struct {
	ID               string  `db:"id"`
	Type             string  `db:"type"`
	Quantity         float64 `db:"quantity"`
	Reference        *string `db:"reference"`
	CreatedAt        *string `db:"createdAt"`
	SourceDocumentID *string `db:"sourceDocumentId"`

	InvoiceID       *string `db:"invoiceId"`
	InvoiceDate     *int64  `db:"invoiceDate"`
	InvoiceClientID *string `db:"invoiceClientId"`

	OutID       *string `db:"outId"`
	OutDate     *int64  `db:"outDate"`
	OutClientID *string `db:"outClientId"`

	InID       *string `db:"inId"`
	InDate     *int64  `db:"inDate"`
	InVendorID *string `db:"inVendorId"`

	ProdID   *string `db:"prodId"`
	ProdDate *int64  `db:"prodDate"`

	OutRefID       *string `db:"outRefId"`
	OutRefCount    *int    `db:"outRefCount"`
	OutRefDate     *int64  `db:"outRefDate"`
	OutRefClientID *string `db:"outRefClientId"`

	InRefID       *string `db:"inRefId"`
	InRefCount    *int    `db:"inRefCount"`
	InRefDate     *int64  `db:"inRefDate"`
	InRefVendorID *string `db:"inRefVendorId"`
}

func (d *Database) GetProductSummary(organizationID, productID string) (*ProductSummary, error) {
	org, err := d.GetOrganization(organizationID)
	if err != nil {
		return nil, err
	}
	if !org.MasterDataSummaries {
		return nil, errSummariesSwitchedOff
	}
	product, err := d.GetProduct(productID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	if product.OrganizationID != organizationID {
		return nil, sql.ErrNoRows
	}

	rows := []productMovementRow{}
	if err := d.DB.Select(&rows, `
		SELECT sm.id, sm.type, sm.quantity, sm.reference, sm.createdAt, sm.sourceDocumentId,
		       inv.id AS invoiceId, inv.date AS invoiceDate, inv.clientId AS invoiceClientId,
		       od.id AS outId, od.deliveryDate AS outDate, COALESCE(odo.clientId, od.clientId) AS outClientId,
		       ind.id AS inId, ind.deliveryDate AS inDate, ind.vendorId AS inVendorId,
		       po.id AS prodId, po.date AS prodDate,
		       odr.id AS outRefId, odr.n AS outRefCount, odr.deliveryDate AS outRefDate, odr.clientId AS outRefClientId,
		       idr.id AS inRefId, idr.n AS inRefCount, idr.deliveryDate AS inRefDate, idr.vendorId AS inRefVendorId
		FROM stockMovements sm
		LEFT JOIN invoices inv ON inv.id = sm.sourceDocumentId AND inv.organizationId = sm.organizationId
		LEFT JOIN outbound_deliveries od ON od.id = sm.sourceDocumentId AND od.organizationId = sm.organizationId
		LEFT JOIN orders odo ON odo.id = od.orderId
		LEFT JOIN inbound_deliveries ind ON ind.id = sm.sourceDocumentId AND ind.organizationId = sm.organizationId
		LEFT JOIN production_orders po ON po.id = sm.sourceDocumentId AND po.organizationId = sm.organizationId
		LEFT JOIN (
			SELECT d.deliveryNumber, MIN(d.id) AS id, COUNT(*) AS n,
			       MIN(d.deliveryDate) AS deliveryDate, MIN(COALESCE(o.clientId, d.clientId)) AS clientId
			FROM outbound_deliveries d LEFT JOIN orders o ON o.id = d.orderId
			WHERE d.organizationId = ? GROUP BY d.deliveryNumber
		) odr ON sm.sourceDocumentId IS NULL AND odr.deliveryNumber = sm.reference
		LEFT JOIN (
			SELECT deliveryNumber, MIN(id) AS id, COUNT(*) AS n,
			       MIN(deliveryDate) AS deliveryDate, MIN(vendorId) AS vendorId
			FROM inbound_deliveries WHERE organizationId = ? GROUP BY deliveryNumber
		) idr ON sm.sourceDocumentId IS NULL AND idr.deliveryNumber = sm.reference
		WHERE sm.organizationId = ? AND sm.productId = ?`,
		organizationID, organizationID, organizationID, productID,
	); err != nil {
		return nil, fmt.Errorf("get_product_summary movements: %w", err)
	}

	movements := make([]ProductMovement, 0, len(rows))
	for _, r := range rows {
		movements = append(movements, resolveProductMovement(r))
	}
	// Newest document first; the id breaks ties so the order is stable.
	sort.SliceStable(movements, func(i, j int) bool {
		if movements[i].Date != movements[j].Date {
			return movements[i].Date > movements[j].Date
		}
		return movements[i].ID > movements[j].ID
	})
	summary := &ProductSummary{ProductID: productID, MovementCount: len(movements)}
	if len(movements) > productRecentMovements {
		movements = movements[:productRecentMovements]
	}
	if err := d.fillMovementCounterpartyNames(movements); err != nil {
		return nil, err
	}
	summary.RecentMovements = movements

	var last ProductLastVendor
	err = d.DB.Get(&last, `
		SELECT ii.vendorId AS vendorId, v.name AS name, ii.date AS date, ii.id AS incomingInvoiceId
		FROM incoming_invoices ii
		JOIN vendors v ON v.id = ii.vendorId
		WHERE ii.organizationId = ? AND ii.state IN ('approved', 'paid')
		  AND EXISTS (SELECT 1 FROM incoming_invoice_line_items li
		              WHERE li.incomingInvoiceId = ii.id AND li.productId = ?)
		ORDER BY ii.date DESC, ii.id DESC
		LIMIT 1`,
		organizationID, productID,
	)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("get_product_summary last vendor: %w", err)
	}
	if err == nil {
		summary.LastVendor = &last
	}
	return summary, nil
}

// resolveProductMovement turns a joined row into a ProductMovement: the
// document a sourceDocumentId points at wins, then an unambiguous delivery
// number; the date is that document's date, else the movement's createdAt.
func resolveProductMovement(r productMovementRow) ProductMovement {
	m := ProductMovement{ID: r.ID, Type: r.Type, Quantity: r.Quantity, Reference: r.Reference}
	var date *int64
	switch {
	case r.InvoiceID != nil:
		m.DocumentKind, m.DocumentID, date = "invoice", r.InvoiceID, r.InvoiceDate
		m.CounterpartyKind, m.CounterpartyID = "client", r.InvoiceClientID
	case r.OutID != nil:
		m.DocumentKind, m.DocumentID, date = "delivery", r.OutID, r.OutDate
		m.CounterpartyKind, m.CounterpartyID = "client", r.OutClientID
	case r.InID != nil:
		m.DocumentKind, m.DocumentID, date = "receipt", r.InID, r.InDate
		m.CounterpartyKind, m.CounterpartyID = "vendor", r.InVendorID
	case r.ProdID != nil:
		m.DocumentKind, m.DocumentID, date = "production", r.ProdID, r.ProdDate
	case r.SourceDocumentID == nil && r.OutRefID != nil && r.InRefID == nil && derefInt(r.OutRefCount) == 1:
		m.DocumentKind, m.DocumentID, date = "delivery", r.OutRefID, r.OutRefDate
		m.CounterpartyKind, m.CounterpartyID = "client", r.OutRefClientID
	case r.SourceDocumentID == nil && r.InRefID != nil && r.OutRefID == nil && derefInt(r.InRefCount) == 1:
		m.DocumentKind, m.DocumentID, date = "receipt", r.InRefID, r.InRefDate
		m.CounterpartyKind, m.CounterpartyID = "vendor", r.InRefVendorID
	}
	if m.CounterpartyID == nil || *m.CounterpartyID == "" {
		m.CounterpartyKind, m.CounterpartyID = "", nil
	}
	if date != nil {
		m.Date = *date
	} else if r.CreatedAt != nil {
		// SQLite's datetime('now') default: UTC, second precision.
		if t, err := time.Parse("2006-01-02 15:04:05", *r.CreatedAt); err == nil {
			m.Date = t.UnixMilli()
		}
	}
	return m
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// fillMovementCounterpartyNames looks up the client and vendor names for the
// few movements the panel shows.
func (d *Database) fillMovementCounterpartyNames(movements []ProductMovement) error {
	for i := range movements {
		m := &movements[i]
		if m.CounterpartyID == nil {
			continue
		}
		table := "clients"
		if m.CounterpartyKind == "vendor" {
			table = "vendors"
		}
		var name sql.NullString
		err := d.DB.Get(&name, "SELECT name FROM "+table+" WHERE id = ?", *m.CounterpartyID)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("get_product_summary counterparty: %w", err)
		}
		if name.Valid && name.String != "" {
			m.CounterpartyName = &name.String
		}
	}
	return nil
}
