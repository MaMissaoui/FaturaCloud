package db

import (
	"fmt"
	"sort"
	"testing"
)

// TestNullableForeignKeysAreClassified reads the live schema and fails when
// a nullable foreign-key column is missing from nullableFKClassification.
//
// Same tripwire idiom as TestVendorDocumentCountCoversEveryReference and
// TestTaxRateUsageCountCoversEveryReference: the point is not that the map
// is correct today, it is that adding a nullable FK column tomorrow forces
// a decision about empty-string ids instead of silently inheriting F94.
func TestNullableForeignKeysAreClassified(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)

	var tables []string
	if err := d.DB.Select(&tables, `
		SELECT name FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
		ORDER BY name`); err != nil {
		t.Fatalf("list tables: %v", err)
	}

	live := map[string]bool{}
	for _, table := range tables {
		type fkRow struct {
			ID    int     `db:"id"`
			Seq   int     `db:"seq"`
			Table string  `db:"table"`
			From  string  `db:"from"`
			To    *string `db:"to"`
			OnUpd string  `db:"on_update"`
			OnDel string  `db:"on_delete"`
			Match string  `db:"match"`
		}
		fks := []fkRow{}
		if err := d.DB.Select(&fks, fmt.Sprintf(`PRAGMA foreign_key_list(%q)`, table)); err != nil {
			t.Fatalf("foreign_key_list(%s): %v", table, err)
		}
		if len(fks) == 0 {
			continue
		}

		type colRow struct {
			CID     int     `db:"cid"`
			Name    string  `db:"name"`
			Type    string  `db:"type"`
			NotNull int     `db:"notnull"`
			Dflt    *string `db:"dflt_value"`
			PK      int     `db:"pk"`
		}
		cols := []colRow{}
		if err := d.DB.Select(&cols, fmt.Sprintf(`PRAGMA table_info(%q)`, table)); err != nil {
			t.Fatalf("table_info(%s): %v", table, err)
		}
		notNull := make(map[string]bool, len(cols))
		for _, c := range cols {
			notNull[c.Name] = c.NotNull == 1
		}

		for _, fk := range fks {
			if notNull[fk.From] {
				continue // a NOT NULL FK can't hold "unset" at all
			}
			live[table+"."+fk.From] = true
		}
	}

	if len(live) == 0 {
		t.Fatal("read no nullable foreign keys from the schema — the tripwire would pass vacuously")
	}

	var missing, stale []string
	for col := range live {
		if _, ok := nullableFKClassification[col]; !ok {
			missing = append(missing, col)
		}
	}
	for col := range nullableFKClassification {
		if !live[col] {
			stale = append(stale, col)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)

	for _, col := range missing {
		t.Errorf("nullable foreign key %s is not in nullableFKClassification — decide whether its "+
			"Create/Update normalizes an empty-string id (fkNormalized), whether it is server-set "+
			"(fkServerSet), or whether nil already means \"don't touch\" (fkExplicitClear). See F94.", col)
	}
	for _, col := range stale {
		t.Errorf("nullableFKClassification lists %s, which is no longer a nullable foreign key — "+
			"remove it so the map keeps describing the real schema", col)
	}
}

// The regression itself, per document type: an empty-string optional FK id
// must be accepted as "unset" rather than reaching the INSERT and failing
// as a raw foreign-key violation.
func TestEmptyOptionalForeignKeyIsTreatedAsUnset(t *testing.T) {
	t.Parallel()

	t.Run("production order import", func(t *testing.T) {
		t.Parallel()
		d := newTestDB(t)
		fx := seedProductionOrderFixture(t, d, false)

		order, err := d.CreateProductionOrder(CreateProductionOrderRequest{
			OrganizationID: fx.orgID, OrderNumber: "PRO-0001",
			FinishedProductID: fx.finished.ID, Quantity: 1, Date: fx.date,
			ImportID: ptr(""),
		})
		if err != nil {
			t.Fatalf("empty importId should mean unset: %v", err)
		}
		if order.ImportID != nil {
			t.Errorf("importId = %v, want nil", *order.ImportID)
		}
	})

	t.Run("purchase order vendor and import", func(t *testing.T) {
		t.Parallel()
		d := newTestDB(t)
		org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-empty-fk-po"})
		if err != nil {
			t.Fatalf("CreateOrganization: %v", err)
		}
		po, err := d.CreatePurchaseOrder(CreatePurchaseOrderRequest{
			OrganizationID: org.ID, OrderNumber: "PO-0001", OrderDate: 1700000000000,
			VendorID: ptr(""), ImportID: ptr(""),
			LineItems: []CreatePurchaseOrderLineItemRequest{
				{Description: "Free text line", Quantity: 1, UnitPrice: 100,
					ProductID: ptr(""), TaxRate: ptr("")},
			},
		})
		if err != nil {
			t.Fatalf("empty vendorId/importId/productId/taxRate should mean unset: %v", err)
		}
		if po.VendorID != nil || po.ImportID != nil {
			t.Errorf("vendorId = %v, importId = %v, want both nil", po.VendorID, po.ImportID)
		}
		lines, err := d.GetPurchaseOrderLineItems(po.ID)
		if err != nil || len(lines) != 1 {
			t.Fatalf("GetPurchaseOrderLineItems: err=%v len=%d", err, len(lines))
		}
		if lines[0].ProductID != nil || lines[0].TaxRate != nil {
			t.Errorf("line productId = %v, taxRate = %v, want both nil", lines[0].ProductID, lines[0].TaxRate)
		}
	})

	t.Run("order client and line product", func(t *testing.T) {
		t.Parallel()
		d := newTestDB(t)
		org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-empty-fk-order"})
		if err != nil {
			t.Fatalf("CreateOrganization: %v", err)
		}
		order, err := d.CreateOrder(CreateOrderRequest{
			OrganizationID: org.ID, OrderNumber: "SO-0001", OrderDate: 1700000000000,
			ClientID: ptr(""),
			LineItems: []CreateOrderLineItemRequest{
				{Description: "Free text line", Quantity: 1, UnitPrice: 100, ProductID: ptr("")},
			},
		})
		if err != nil {
			t.Fatalf("empty clientId/productId should mean unset: %v", err)
		}
		if order.ClientID != nil {
			t.Errorf("clientId = %v, want nil", *order.ClientID)
		}
	})

	t.Run("product optional links", func(t *testing.T) {
		t.Parallel()
		d := newTestDB(t)
		org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-empty-fk-product"})
		if err != nil {
			t.Fatalf("CreateOrganization: %v", err)
		}
		p, err := d.CreateProduct(CreateProductRequest{
			OrganizationID: org.ID, Name: "Thing", Type: "service",
			TaxRateID: ptr(""), RevenueAccountID: ptr(""), ExpenseAccountID: ptr(""),
			UnitOfMeasureID: ptr(""),
		})
		if err != nil {
			t.Fatalf("empty optional product links should mean unset: %v", err)
		}
		if p.TaxRateID != nil || p.RevenueAccountID != nil || p.ExpenseAccountID != nil || p.UnitOfMeasureID != nil {
			t.Errorf("want all nil, got taxRate=%v revenue=%v expense=%v uom=%v",
				p.TaxRateID, p.RevenueAccountID, p.ExpenseAccountID, p.UnitOfMeasureID)
		}
	})

	t.Run("account parent", func(t *testing.T) {
		t.Parallel()
		d := newTestDB(t)
		org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-empty-fk-account"})
		if err != nil {
			t.Fatalf("CreateOrganization: %v", err)
		}
		a, err := d.CreateAccount(CreateAccountRequest{
			OrganizationID: org.ID, Code: "9999", Name: "Probe", Type: "asset",
			ParentID: ptr(""),
		})
		if err != nil {
			t.Fatalf("empty parentId should mean unset: %v", err)
		}
		if a.ParentID != nil {
			t.Errorf("parentId = %v, want nil", *a.ParentID)
		}
	})

	t.Run("goods receipt vendor and purchase order", func(t *testing.T) {
		t.Parallel()
		d := newTestDB(t)
		org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-empty-fk-receipt"})
		if err != nil {
			t.Fatalf("CreateOrganization: %v", err)
		}
		r, err := d.CreateInboundDelivery(CreateInboundDeliveryRequest{
			OrganizationID: org.ID, DeliveryNumber: "GR-0001", DeliveryDate: 1700000000000,
			PurchaseOrderID: ptr(""), VendorID: ptr(""),
			LineItems: []CreateInboundDeliveryLineItemRequest{
				{Description: "Free text line", Quantity: 1,
					PurchaseOrderLineItemID: ptr(""), ProductID: ptr("")},
			},
		})
		if err != nil {
			t.Fatalf("empty receipt links should mean unset: %v", err)
		}
		if r.PurchaseOrderID != nil || r.VendorID != nil {
			t.Errorf("purchaseOrderId = %v, vendorId = %v, want both nil", r.PurchaseOrderID, r.VendorID)
		}
	})

	t.Run("outbound delivery client and order", func(t *testing.T) {
		t.Parallel()
		d := newTestDB(t)
		org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-empty-fk-delivery"})
		if err != nil {
			t.Fatalf("CreateOrganization: %v", err)
		}
		del, err := d.CreateDelivery(CreateDeliveryRequest{
			OrganizationID: org.ID, DeliveryNumber: "DN-0001", DeliveryDate: 1700000000000,
			OrderID: ptr(""), ClientID: ptr(""),
			LineItems: []CreateDeliveryLineItemRequest{
				{Description: "Free text line", Quantity: 1,
					OrderLineItemID: ptr(""), ProductID: ptr("")},
			},
		})
		if err != nil {
			t.Fatalf("empty delivery links should mean unset: %v", err)
		}
		if del.OrderID != nil || del.OwnClientID != nil {
			t.Errorf("orderId = %v, ownClientId = %v, want both nil", del.OrderID, del.OwnClientID)
		}
	})
}

// nilIfEmptyID must leave a genuine id and a genuine nil alone — it only
// ever collapses the empty string.
func TestNilIfEmptyID(t *testing.T) {
	t.Parallel()
	if got := nilIfEmptyID(nil); got != nil {
		t.Errorf("nil -> %v, want nil", *got)
	}
	if got := nilIfEmptyID(ptr("")); got != nil {
		t.Errorf("empty string -> %q, want nil", *got)
	}
	real := "abc123"
	got := nilIfEmptyID(&real)
	if got == nil || *got != real {
		t.Errorf("%q -> %v, want it unchanged", real, got)
	}
	// Whitespace is NOT collapsed: it isn't a plausible "unset" spelling
	// from a JSON client, and silently accepting it would hide a real bad
	// id behind a clean success.
	if got := nilIfEmptyID(ptr(" ")); got == nil || *got != " " {
		t.Errorf("a whitespace id was collapsed to nil; only \"\" should be")
	}
}
