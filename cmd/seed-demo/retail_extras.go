package main

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// The retail features the daily simulation doesn't reach on its own: product
// families, the paper loan register brought into the app on the first day, a
// yearly import of air conditioners and fans ahead of the summer, and the
// stock write-offs of the last day.

// retailFamilies groups the appliance kinds into the product families the
// Products screen filters by. Services are deliberately left without one.
var retailFamilies = []struct {
	name  string
	kinds []string
}{
	{"Froid", []string{"Réfrigérateur", "Congélateur"}},
	{"Lavage", []string{"Machine à laver", "Lave-vaisselle"}},
	{"Climatisation et ventilation", []string{"Climatiseur", "Ventilateur", "Climatisation professionnelle"}},
	{"Image et son", []string{"Téléviseur"}},
	{"Cuisson", []string{"Four à micro-ondes", "Cuisinière gaz/électrique"}},
	{"Eau chaude et fontaines", []string{"Chauffe-eau", "Fontaine à eau"}},
	{"Petit électroménager", []string{
		"Mixeur", "Bouilloire électrique", "Grille-pain", "Cafetière", "Robot ménager",
		"Batteur", "Extracteur de jus", "Aspirateur", "Fer à repasser",
	}},
}

// setupRetailFamilies creates the families before the products, so
// setupProducts can file each product under its family (familyIDFor).
func (s *Seeder) setupRetailFamilies() error {
	s.familyByKind = map[string]string{}
	for _, f := range retailFamilies {
		var family db.ProductFamily
		if err := s.c.Post("/api/product-families", db.CreateProductFamilyRequest{OrganizationID: s.orgID, Name: f.name}, &family); err != nil {
			return fmt.Errorf("product family %s: %w", f.name, err)
		}
		for _, kind := range f.kinds {
			s.familyByKind[kind] = family.ID
		}
	}
	return nil
}

// --- the paper loan register ------------------------------------------------

// importOpeningLoans brings the shop's paper loan book into the app on the
// run's first day (the cutover), through the same Excel import an owner uses
// (POST .../loan-imports): customers who bought on credit in the year before,
// with what they had paid so far. Most of them then pay the rest off at the
// counter over the following months, line by line; a few never come back.
func (s *Seeder) importOpeningLoans(cutover time.Time) error {
	f := excelize.NewFile()
	defer f.Close() //nolint:errcheck
	sheet := f.GetSheetName(0)
	headers := []string{
		"Réf. prêt (Loan ref) *", "Date de vente (Sale date) *", "Client (Customer) *", "CIN",
		"Téléphone (Phone)", "Téléphone 2 (Phone 2)", "Adresse (Address)", "Garant (Guarantor)",
		"Article (Product) *", "Quantité (Quantity)", "Montant (Amount) *", "Déjà payé (Paid to date)",
		"Dernier paiement (Last payment)", "Note",
	}
	rows := [][]any{toAny(headers)}

	n := max(8, int(math.Round(30*s.cfg.VolumeScale)))
	refs := map[string]bool{}
	for i := 0; i < n; i++ {
		ref := fmt.Sprintf("C%d/P%02d", 1+i/12, 1+i%12)
		refs[ref] = true
		sale := cutover.AddDate(0, 0, -s.rng.IntRange(20, 330))
		name, phone := s.newRetailCustomer()
		cin := fmt.Sprintf("%08d", s.rng.IntRange(1000000, 14999999))
		var phoneCell string
		if phone != nil {
			phoneCell = *phone
		}
		city, _ := s.rng.City()
		guarantor := ""
		if s.rng.Chance(0.4) {
			guarantor = s.rng.RetailCustomerName()
		}
		var items []productRef
		for _, p := range s.stockProducts() {
			if p.priceCents >= bigTicketThresholdCents && !p.serialized {
				items = append(items, p)
			}
		}
		lines := 1
		if s.rng.Chance(0.3) {
			lines = 2
		}
		var total int64
		var lineRows [][]any
		for j := 0; j < lines; j++ {
			p := Pick(s.rng, items)
			// The register holds the price as sold, tax included.
			amount := int64(math.Round(float64(p.priceCents)*(1+s.taxPercentFor(p.taxRateID)/100)/100)) * 100
			total += amount
			lineRows = append(lineRows, []any{p.name, 1, centsCell(amount)})
		}
		paid := total * int64(s.rng.IntRange(15, 75)) / 100 / 100 * 100
		last := sale.AddDate(0, 0, s.rng.IntRange(10, int(cutover.Sub(sale).Hours()/24)))
		if last.After(cutover) {
			last = cutover
		}
		for j, lr := range lineRows {
			row := []any{ref, "", "", "", "", "", "", "", lr[0], lr[1], lr[2], "", "", ""}
			if j == 0 {
				row[1] = sale.Format("02/01/2006")
				row[2], row[3], row[4], row[6], row[7] = name, cin, phoneCell, city, guarantor
				row[11], row[12] = centsCell(paid), last.Format("02/01/2006")
			}
			rows = append(rows, row)
		}
	}
	for r, row := range rows {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+1)
			if err := f.SetCellValue(sheet, cell, v); err != nil {
				return fmt.Errorf("loan register cell %s: %w", cell, err)
			}
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return fmt.Errorf("loan register file: %w", err)
	}

	cutoverDay := time.Date(cutover.Year(), cutover.Month(), cutover.Day(), 12, 0, 0, 0, time.UTC).UnixMilli()
	var report db.LoanImportReport
	if err := s.c.PostFile("/api/organizations/"+s.orgID+"/loan-imports",
		map[string]string{"cutoverDate": fmt.Sprintf("%d", cutoverDay), "dryRun": "false"},
		"registre-credits.xlsx", buf.Bytes(), &report); err != nil {
		return fmt.Errorf("import loan register: %w", err)
	}
	if !report.Imported {
		msgs := []string{}
		for _, p := range report.Problems {
			msgs = append(msgs, fmt.Sprintf("row %d %s: %s", p.Row, p.Severity, p.Message))
		}
		return fmt.Errorf("loan register not imported: %s", strings.Join(msgs, "; "))
	}
	s.stats.OpeningLoans = report.Loans

	// The imported customers become ordinary customers of the shop, and their
	// loans are collected like any other.
	var status []db.LoanStatusRow
	if err := s.c.Get("/api/organizations/"+s.orgID+"/reports/loan-status", &status); err != nil {
		return fmt.Errorf("read imported loans: %w", err)
	}
	seenClient := map[string]bool{}
	seenLoan := map[string]bool{}
	for _, r := range status {
		if !refs[r.InvoiceNumber] {
			continue
		}
		if !seenClient[r.ClientID] {
			seenClient[r.ClientID] = true
			s.clients = append(s.clients, clientRef{id: r.ClientID, name: r.ClientName})
		}
		if seenLoan[r.InvoiceID] {
			continue
		}
		seenLoan[r.InvoiceID] = true
		switch {
		case s.rng.Chance(0.55): // paid off in one visit
			s.scheduleLoanCollection(businessDaysLater(cutover, s.rng.IntRange(10, 90)), r.InvoiceID, r.ClientID, 1)
		case s.rng.Chance(0.7): // in two visits
			first := businessDaysLater(cutover, s.rng.IntRange(10, 60))
			s.scheduleLoanCollection(first, r.InvoiceID, r.ClientID, 0.5)
			s.scheduleLoanCollection(businessDaysLater(first, s.rng.IntRange(30, 90)), r.InvoiceID, r.ClientID, 1)
		default: // never came back
		}
	}
	return nil
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, v := range ss {
		out[i] = v
	}
	return out
}

// centsCell writes cents the way the register does: dinars with a decimal
// comma.
func centsCell(cents int64) string {
	return strings.Replace(fmt.Sprintf("%d.%02d", cents/100, cents%100), ".", ",", 1)
}

// --- the yearly import -------------------------------------------------------

// retailForeignVendors are the overseas manufacturers the shop imports air
// conditioners and fans from, priced in US dollars.
var retailForeignVendors = []foreignVendorEntry{
	{"Ningbo Haitian Home Appliances Co., Ltd.", "Ningbo", "315000"},
	{"Foshan Meilun Electric Co., Ltd.", "Foshan", "528000"},
}

func (s *Seeder) setupRetailForeignVendors() error {
	for _, entry := range retailForeignVendors {
		var v db.Vendor
		if err := s.c.Post("/api/vendors", db.CreateVendorRequest{
			OrganizationID: s.orgID, Name: strPtr(entry.name), Vatin: strPtr(s.rng.foreignVendorVATIN()),
			Phone: strPtr(s.rng.foreignVendorPhone()), City: strPtr(entry.city), PostalCode: strPtr(entry.postalCode),
			CountryCode: strPtr("CN"), DefaultCurrency: strPtr("USD"),
		}, &v); err != nil {
			return fmt.Errorf("foreign vendor %q: %w", entry.name, err)
		}
		s.foreignVendors = append(s.foreignVendors, vendorRef{id: v.ID, name: entry.name, currency: "USD"})
		s.stats.Vendors++
	}
	return nil
}

// maybeStartSeasonalImport places, on the first Monday of March, the year's
// container of air conditioners and fans: one purchase order per
// manufacturer on one import, received in April with its freight and customs
// spread over the goods' cost (db/gl_posting.go's applyLandedCost).
func (s *Seeder) maybeStartSeasonalImport(day time.Time) error {
	if day.Month() != time.March || day.Weekday() != time.Monday || day.Day() > 7 || len(s.foreignVendors) < 2 {
		return nil
	}
	var imp db.Import
	if err := s.c.Post("/api/imports", db.CreateImportRequest{
		OrganizationID: s.orgID, ImportNumber: s.importNum.next(day.Year()), Date: midnightUTC(day),
		FreightCost: 50000, CustomsCost: 20000, Notes: strPtr("Conteneur climatiseurs et ventilateurs — saison d'été"),
	}, &imp); err != nil {
		return fmt.Errorf("create import: %w", err)
	}
	s.stats.Imports++
	ref := &importRef{id: imp.ID, number: imp.ImportNumber, createdOn: day}
	rate := s.usdExchangeRateForOrgCurrency()
	for i, kind := range []string{"Climatiseur", "Ventilateur"} {
		vendor := s.foreignVendors[i]
		var products []productRef
		for _, p := range s.stockProducts() {
			if p.kind == kind {
				products = append(products, p)
			}
		}
		Shuffle(s.rng, products)
		products = products[:min(len(products), 6)]
		var reqLines []db.CreatePurchaseOrderLineItemRequest
		var lines []purchaseLine
		for _, p := range products {
			qty := float64(s.rng.IntRange(4, 10))
			// Priced in dollars: the catalog cost converted at the day's rate.
			usdCents := int64(math.Round(float64(p.costCents) / rate * 0.9))
			reqLines = append(reqLines, db.CreatePurchaseOrderLineItemRequest{
				ProductID: strPtr(p.id), Description: p.name, Quantity: qty, UnitPrice: float64(usdCents), Unit: strPtr(p.unit),
			})
			lines = append(lines, purchaseLine{productID: p.id, productName: p.name, unit: p.unit, quantity: qty,
				unitCost: usdCents, currency: "USD", exchangeRate: rate})
			ref.committedValueCents += int64(qty * float64(p.costCents))
			s.onOrder[p.id] += qty
		}
		var po db.PurchaseOrder
		if err := s.c.Post("/api/purchase-orders", db.CreatePurchaseOrderRequest{
			OrganizationID: s.orgID, VendorID: &vendor.id, Status: "draft", OrderDate: midnightUTC(day),
			LineItems: reqLines, Currency: strPtr("USD"), ExchangeRate: float64Ptr(rate), ImportID: &imp.ID,
		}, &po); err != nil {
			return fmt.Errorf("import purchase order: %w", err)
		}
		s.stats.PurchaseOrders++
		if err := s.c.Patch("/api/purchase-orders/"+po.ID+"/status", map[string]string{"status": "confirmed"}, nil); err != nil {
			return fmt.Errorf("confirm import PO %s: %w", po.OrderNumber, err)
		}
		var serverLines []db.PurchaseOrderLineItem
		if err := s.c.Get("/api/purchase-orders/"+po.ID+"/line-items", &serverLines); err != nil {
			return fmt.Errorf("read back import PO %s: %w", po.OrderNumber, err)
		}
		for j := range lines {
			lines[j].poLineID = serverLines[j].ID
		}
		receiveDay := businessDaysLater(day, s.rng.IntRange(25, 35))
		if receiveDay.After(s.cfg.EndDate) {
			continue
		}
		s.sched.Schedule(receiveDay, func() error { return s.receivePurchaseOrder(receiveDay, po, vendor, lines) })
	}
	// Freight and customs are known once the container is booked, before it
	// arrives — set them before the first receipt spreads them.
	finalize := businessDaysLater(day, 10)
	if !finalize.After(s.cfg.EndDate) {
		s.sched.Schedule(finalize, func() error { return s.finalizeImportCosts(ref) })
	}
	return nil
}

// --- the last day ------------------------------------------------------------

// recordStockWriteOffs records, on the run's last day, an appliance damaged
// in handling, taken out of stock by hand. Manual stock movements are dated
// when they're recorded, so they belong on the last day (today), not back in
// the simulated past.
func (s *Seeder) recordStockWriteOffs() error {
	var big []productRef
	for _, p := range s.stockProducts() {
		if s.onHand(p.id) >= 2 && !p.serialized && p.priceCents >= bigTicketThresholdCents {
			big = append(big, p)
		}
	}
	moves := []struct {
		pool            []productRef
		kind, note, ref string
	}{
		{big, "out", "Casse — appareil endommagé lors de la manutention", "CASSE-01"},
	}
	for _, m := range moves {
		if len(m.pool) == 0 {
			continue
		}
		p := Pick(s.rng, m.pool)
		if err := s.c.Post("/api/stock-movements", db.CreateStockMovementRequest{
			OrganizationID: s.orgID, ProductID: p.id, Type: m.kind, Quantity: 1,
			Note: strPtr(m.note), Reference: strPtr(m.ref),
		}, nil); err != nil {
			return fmt.Errorf("stock %s for %s: %w", m.kind, p.name, err)
		}
		s.adjustOnHand(p.id, -1)
	}
	return nil
}

// recordStockCount counts the small-appliance shelf on the run's last day
// through the Inventory screen's Excel round trip: export the count sheet
// (GET .../stock-movements/export), fill Counted Quantity for the "Petit
// électroménager" products — every one as the sheet says except one found
// short and one found over — and upload it (POST .../stock-movements/import),
// which posts the two differences as count movements and reports the rest
// unchanged. Rows outside that family are left blank, which the import skips.
func (s *Seeder) recordStockCount() error {
	content, err := s.c.GetBytes("/api/organizations/" + s.orgID + "/stock-movements/export")
	if err != nil {
		return fmt.Errorf("export stock count sheet: %w", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return fmt.Errorf("open stock count sheet: %w", err)
	}
	defer f.Close() //nolint:errcheck
	sheet := f.GetSheetName(0)
	rows, err := f.GetRows(sheet)
	if err != nil {
		return fmt.Errorf("read stock count sheet: %w", err)
	}
	family := s.familyByKind["Mixeur"]
	type countRow struct {
		row     int
		id      string
		current float64
	}
	var counted []countRow
	for i, r := range rows {
		if i == 0 || len(r) < 4 {
			continue
		}
		p, ok := s.productByID(r[0])
		if !ok || s.familyByKind[p.kind] != family {
			continue
		}
		current, err := strconv.ParseFloat(strings.ReplaceAll(r[3], ",", "."), 64)
		if err != nil || current < 1 {
			continue
		}
		counted = append(counted, countRow{row: i + 1, id: p.id, current: current})
	}
	if len(counted) < 2 {
		return nil
	}
	short, over := s.rng.IntRange(0, len(counted)-1), -1
	for over < 0 || over == short {
		over = s.rng.IntRange(0, len(counted)-1)
	}
	for i, c := range counted {
		count := c.current
		note := ""
		switch i {
		case short:
			count--
			note = "Une unité manquante au comptage"
		case over:
			count++
			note = "Une unité retrouvée en réserve"
		}
		if err := f.SetCellValue(sheet, fmt.Sprintf("E%d", c.row), count); err != nil {
			return fmt.Errorf("fill stock count: %w", err)
		}
		if note != "" {
			if err := f.SetCellValue(sheet, fmt.Sprintf("G%d", c.row), note); err != nil {
				return fmt.Errorf("fill stock count: %w", err)
			}
		}
		// The sheet's own quantity is the server's: keep the mirror on it.
		s.adjustOnHand(c.id, count-s.onHand(c.id))
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return fmt.Errorf("write stock count sheet: %w", err)
	}
	var result db.MassDataImportResult
	if err := s.c.PostFile("/api/organizations/"+s.orgID+"/stock-movements/import", nil, "comptage-petit-electromenager.xlsx", buf.Bytes(), &result); err != nil {
		return fmt.Errorf("upload stock count: %w", err)
	}
	if result.Failed > 0 {
		return fmt.Errorf("stock count upload: %d rows failed: %+v", result.Failed, result.Rows)
	}
	s.log.Printf("seed-demo: stock count: %d counted, %d adjusted, %d unchanged", len(counted), result.Updated+result.Created, result.Unchanged)
	return nil
}
