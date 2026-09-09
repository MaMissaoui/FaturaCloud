package main

import (
	"fmt"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// setupOrganization creates (or, with --reset, recreates) the demo
// organization and records the ids every later step needs (org id, cash
// account for payments). Importing db's own request/response types instead
// of hand-building map[string]interface{} bodies means a field rename on
// the server is a compile error here, not a silently-wrong wire payload —
// see cmd/seed-demo/README.md's "why HTTP, and why db types" note.
func (s *Seeder) setupOrganization() error {
	if s.cfg.Reset {
		var orgs []db.Organization
		if err := s.c.Get("/api/organizations", &orgs); err != nil {
			return fmt.Errorf("list organizations: %w", err)
		}
		for _, o := range orgs {
			if o.Name != nil && *o.Name == s.cfg.OrgName {
				s.log.Printf("seed-demo: --reset: deleting existing organization %q (%s)", s.cfg.OrgName, o.ID)
				if err := s.c.Delete("/api/organizations/" + o.ID); err != nil {
					return fmt.Errorf("delete existing organization %s: %w", o.ID, err)
				}
				break
			}
		}
	}

	req := db.CreateOrganizationRequest{
		Name:                  strPtr(s.cfg.OrgName),
		Code:                  strPtr("DEMO"),
		Country:               strPtr("Germany"), // -> SKR04 chart of accounts template
		CountryCode:           strPtr("DE"),
		Currency:              strPtr("EUR"),
		MinimumFractionDigits: int64Ptr(2),
		DueDays:               int64Ptr(14),
		DateFormat:            strPtr("DD/MM/YYYY"),
		Email:                 strPtr("billing@demo-organization.example"),
		Phone:                 strPtr("+49 30 5550100"),
		Street:                strPtr("Musterstraße"),
		HouseNumber:           strPtr("12"),
		PostalCode:            strPtr("10115"),
		City:                  strPtr("Berlin"),
		Vatin:                 strPtr("DE111222333"),
		BankName:              strPtr("Demo Bank AG"),
		IBAN:                  strPtr("DE89370400440532013000"),
		InvoiceNumberFormat:   strPtr("INV-{YYYY}-{NNNN}"),
	}

	var org db.Organization
	if err := s.c.Post("/api/organizations", req, &org); err != nil {
		return fmt.Errorf("create organization: %w", err)
	}
	s.orgID = org.ID
	if org.DefaultCashAccountID == nil {
		return fmt.Errorf("newly created organization has no default cash account — cannot record payments")
	}
	s.cashAccountID = *org.DefaultCashAccountID
	s.log.Printf("seed-demo: organization %q ready (%s)", s.cfg.OrgName, s.orgID)
	return nil
}

// setupMasterData seeds tax rates, then vendors, clients and the product
// catalog — everything the daily simulation in sales.go/purchasing.go picks
// from at random. Order matters only for tax rates (products reference
// them).
func (s *Seeder) setupMasterData() error {
	if err := s.setupTaxRates(); err != nil {
		return fmt.Errorf("tax rates: %w", err)
	}
	if err := s.setupVendors(); err != nil {
		return fmt.Errorf("vendors: %w", err)
	}
	if err := s.setupClients(); err != nil {
		return fmt.Errorf("clients: %w", err)
	}
	if err := s.setupProducts(); err != nil {
		return fmt.Errorf("products: %w", err)
	}
	s.log.Printf("seed-demo: master data ready — %d vendors, %d clients, %d products",
		len(s.vendors), len(s.clients), len(s.products))
	return nil
}

func (s *Seeder) setupTaxRates() error {
	// A tax rate needs an output (liability) and input (asset) VAT account
	// before UpdateInvoiceState/UpdateIncomingInvoiceState can post GL for
	// any line item referencing it — 409s with "no output tax account
	// configured" otherwise. seedDefaultChartOfAccounts's SKR04 template
	// (this org's Country is "Germany") always creates code 3800
	// "Umsatzsteuer" (output) and 1400 "Abziehbare Vorsteuer" (input) — see
	// db/account.go.
	var accounts []db.Account
	if err := s.c.Get("/api/organizations/"+s.orgID+"/accounts", &accounts); err != nil {
		return fmt.Errorf("list accounts: %w", err)
	}
	var outputTaxAccountID, inputTaxAccountID string
	for _, a := range accounts {
		switch a.Code {
		case "3800":
			outputTaxAccountID = a.ID
		case "1400":
			inputTaxAccountID = a.ID
		}
	}
	if outputTaxAccountID == "" || inputTaxAccountID == "" {
		return fmt.Errorf("could not find SKR04 VAT accounts (code 3800/1400) on the new organization's chart of accounts")
	}

	create := func(name string, percent float64, isDefault bool, category string) (taxRateRef, error) {
		req := db.CreateTaxRateRequest{
			OrganizationID:     s.orgID,
			Name:               name,
			Percentage:         percent,
			CategoryCode:       category,
			OutputTaxAccountID: &outputTaxAccountID,
			InputTaxAccountID:  &inputTaxAccountID,
		}
		if isDefault {
			req.IsDefault = int64Ptr(1)
		}
		var rate db.TaxRate
		if err := s.c.Post("/api/tax-rates", req, &rate); err != nil {
			return taxRateRef{}, err
		}
		return taxRateRef{id: rate.ID, percent: rate.Percentage}, nil
	}

	var err error
	if s.standardTax, err = create("Standard rate", 19, true, "S"); err != nil {
		return err
	}
	if s.reducedTax, err = create("Reduced rate", 7, false, "S"); err != nil {
		return err
	}
	if s.zeroTax, err = create("Zero-rated", 0, false, "Z"); err != nil {
		return err
	}
	return nil
}

func (s *Seeder) setupVendors() error {
	for i := 0; i < s.profile.Vendors; i++ {
		name := s.rng.CompanyName()
		city, plz := s.rng.City()
		req := db.CreateVendorRequest{
			OrganizationID: s.orgID,
			Name:           strPtr(name),
			Vatin:          strPtr(s.rng.VATIN()),
			Emails:         strPtr(fmt.Sprintf(`["%s"]`, s.rng.Email(name))),
			Phone:          strPtr(fmt.Sprintf("+49 %d %d", s.rng.IntRange(30, 891), s.rng.IntRange(100000, 999999))),
			Street:         strPtr(s.rng.Street()),
			HouseNumber:    strPtr(fmt.Sprintf("%d", s.rng.IntRange(1, 200))),
			PostalCode:     strPtr(plz),
			City:           strPtr(city),
			CountryCode:    strPtr("DE"),
		}
		var v db.Vendor
		if err := s.c.Post("/api/vendors", req, &v); err != nil {
			return fmt.Errorf("vendor %q: %w", name, err)
		}
		s.vendors = append(s.vendors, vendorRef{id: v.ID, name: name})
		s.stats.Vendors++
	}
	return nil
}

func (s *Seeder) setupClients() error {
	for i := 0; i < s.profile.Clients; i++ {
		name := s.rng.ClientName()
		city, plz := s.rng.City()
		req := db.CreateClientRequest{
			OrganizationID: s.orgID,
			Name:           strPtr(name),
			Vatin:          strPtr(s.rng.VATIN()),
			Phone:          strPtr(fmt.Sprintf("+49 %d %d", s.rng.IntRange(30, 891), s.rng.IntRange(100000, 999999))),
			Street:         strPtr(s.rng.Street()),
			HouseNumber:    strPtr(fmt.Sprintf("%d", s.rng.IntRange(1, 200))),
			PostalCode:     strPtr(plz),
			City:           strPtr(city),
			CountryCode:    strPtr("DE"),
		}
		var cl db.Client
		if err := s.c.Post("/api/clients", req, &cl); err != nil {
			return fmt.Errorf("client %q: %w", name, err)
		}
		s.clients = append(s.clients, clientRef{id: cl.ID, name: name})
		s.stats.Clients++
	}
	return nil
}

// setupProducts creates one Product per catalog.go entry (services first,
// then stock-tracked goods), each with a randomized price/cost within its
// template's range and a randomly assigned tax rate (mostly standard, a
// minority reduced/zero — the same rough split a real catalog has).
func (s *Seeder) setupProducts() error {
	pick := func() string {
		switch {
		case s.rng.Chance(0.08):
			return s.zeroTax.id
		case s.rng.Chance(0.15):
			return s.reducedTax.id
		default:
			return s.standardTax.id
		}
	}

	create := func(entry productCatalogEntry) error {
		price := s.rng.Int64Range(entry.priceCentsLo, entry.priceCentsHi)
		var costPtr *int64
		cost := int64(0)
		if entry.stockEnabled {
			factor := s.rng.Float64Range(entry.costFactorLo, entry.costFactorHi)
			cost = int64(float64(price) * factor)
			costPtr = int64Ptr(cost)
		}
		taxID := pick()
		typ := "service"
		var unitPtr *string
		stockEnabled := 0
		if entry.stockEnabled {
			typ = "product"
			unitPtr = strPtr(entry.unit)
			stockEnabled = 1
		}
		req := db.CreateProductRequest{
			OrganizationID: s.orgID,
			Name:           entry.name,
			SKU:            strPtr(productSKU(entry.name, s.rng)),
			Price:          price,
			UnitCost:       costPtr,
			Unit:           unitPtr,
			Type:           typ,
			TaxRateID:      strPtr(taxID),
			StockEnabled:   stockEnabled,
		}
		var p db.Product
		if err := s.c.Post("/api/products", req, &p); err != nil {
			return fmt.Errorf("product %q: %w", entry.name, err)
		}
		s.products = append(s.products, productRef{
			id: p.ID, name: entry.name, unit: entry.unit, stockEnabled: entry.stockEnabled,
			priceCents: price, costCents: cost, taxRateID: taxID,
			qtyLo: entry.qtyLo, qtyHi: entry.qtyHi,
		})
		s.stats.Products++
		return nil
	}

	for _, entry := range serviceCatalog {
		if err := create(entry); err != nil {
			return err
		}
	}
	for _, entry := range productCatalog {
		if err := create(entry); err != nil {
			return err
		}
	}
	return nil
}

func productSKU(name string, rng *Rand) string {
	return fmt.Sprintf("%s-%04d", slugPrefix(name), rng.IntRange(1000, 9999))
}

func slugPrefix(name string) string {
	s := slugify(name)
	if len(s) > 8 {
		s = s[:8]
	}
	return s
}
