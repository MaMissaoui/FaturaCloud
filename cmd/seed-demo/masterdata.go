package main

import (
	"fmt"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// orgProfile is everything about the seeded organization that varies by
// --country beyond Currency (its own independent flag): the address/VATIN/
// bank placeholder data, and — functionally load-bearing, not just cosmetic
// — the ISO country code and which chart-of-accounts VAT account codes
// setupTaxRates must wire tax rates to (db/account.go's resolveChartTemplate
// picks a chart by this same Country string; a tax rate needs an
// output/input VAT account before any invoice referencing it can post GL,
// see setupTaxRates below).
type orgProfile struct {
	countryCode                 string
	email, phone                string
	street, houseNumber         string
	postalCode, city            string
	vatin, bankName, iban       string
	dueDays                     int64
	outputTaxCode, inputTaxCode string
}

// orgProfiles has one entry per --country this tool has been taught real
// placeholder data and VAT account codes for. "Germany" was this tool's
// original (and only) shape; "Tunisia" was added alongside the --country/
// --currency flags for seeding a Tunisia-based organization. Client/vendor
// generation (setupVendors/setupClients) still uses German-flavored phone
// numbers, VATIN format, and city names (catalog.go's cities/VATIN helpers)
// regardless of --country — a deliberate, not yet closed, scope boundary:
// full per-country business-partner realism would mean per-country city/
// phone/VATIN generators too, not just the organization's own profile.
var orgProfiles = map[string]orgProfile{
	"Germany": {
		countryCode: "DE",
		email:       "billing@demo-organization.example", phone: "+49 30 5550100",
		street: "Musterstraße", houseNumber: "12", postalCode: "10115", city: "Berlin",
		vatin: "DE111222333", bankName: "Demo Bank AG", iban: "DE89370400440532013000",
		dueDays:       14,
		outputTaxCode: "3800", inputTaxCode: "1400", // SKR04: Umsatzsteuer / Abziehbare Vorsteuer
	},
	"Tunisia": {
		countryCode: "TN",
		email:       "billing@demo-organization.example", phone: "+216 71 234 567",
		street: "Zone Industrielle", houseNumber: "Lot 12", postalCode: "2013", city: "Ben Arous",
		vatin: "0987654X/A/M/000", bankName: "Banque Démo Tunisie", iban: "TN5904018104004942711234",
		dueDays:       30,
		outputTaxCode: "2200", inputTaxCode: "1200", // no Tunisia-specific chart template yet — falls back to db/account.go's defaultChartOfAccounts
	},
}

// genericOrgProfile is the fallback for any --country besides the two
// above: blank address/VATIN/bank fields rather than guessed ones (the same
// "absence over a false claim" convention as this app's e-invoice generator
// — see CLAUDE.md's db/einvoice.go note), but still the correct VAT account
// codes for whatever chart resolveChartTemplate actually resolves to for an
// unrecognized country (db/account.go: anything besides "Germany"/"France"
// falls back to defaultChartOfAccounts, which is what these codes match —
// note this fallback would be wrong for --country France specifically,
// since the PCG chart uses 4457/4456 instead; add a "France" entry above if
// that combination is ever needed).
var genericOrgProfile = orgProfile{
	dueDays:       30,
	outputTaxCode: "2200", inputTaxCode: "1200",
}

func orgProfileFor(country string) orgProfile {
	if p, ok := orgProfiles[country]; ok {
		return p
	}
	return genericOrgProfile
}

// setupOrganization creates (or, with --reset, recreates) the demo
// organization and records the ids every later step needs (org id, cash
// account for payments). Importing db's own request/response types instead
// of hand-building map[string]interface{} bodies means a field rename on
// the server is a compile error here, not a silently-wrong wire payload —
// see cmd/seed-demo/README.md's "why HTTP, and why db types" note.
func (s *Seeder) setupOrganization() error {
	s.orgProfile = orgProfileFor(s.cfg.Country)
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

	p := s.orgProfile
	req := db.CreateOrganizationRequest{
		Name:                  strPtr(s.cfg.OrgName),
		Code:                  strPtr("DEMO"),
		Country:               strPtr(s.cfg.Country), // -> chart-of-accounts template, db/account.go's resolveChartTemplate
		CountryCode:           nonEmptyStrPtr(p.countryCode),
		Currency:              strPtr(s.cfg.Currency),
		MinimumFractionDigits: int64Ptr(2),
		DueDays:               int64Ptr(p.dueDays),
		DateFormat:            strPtr("DD/MM/YYYY"),
		Email:                 nonEmptyStrPtr(p.email),
		Phone:                 nonEmptyStrPtr(p.phone),
		Street:                nonEmptyStrPtr(p.street),
		HouseNumber:           nonEmptyStrPtr(p.houseNumber),
		PostalCode:            nonEmptyStrPtr(p.postalCode),
		City:                  nonEmptyStrPtr(p.city),
		Vatin:                 nonEmptyStrPtr(p.vatin),
		BankName:              nonEmptyStrPtr(p.bankName),
		IBAN:                  nonEmptyStrPtr(p.iban),
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
	// configured" otherwise. Which codes to look for depends on which chart
	// of accounts db/account.go's resolveChartTemplate picked for this
	// org's Country — s.orgProfile.outputTaxCode/inputTaxCode (set in
	// setupOrganization) is that chart's own pair, not a hardcoded one.
	var accounts []db.Account
	if err := s.c.Get("/api/organizations/"+s.orgID+"/accounts", &accounts); err != nil {
		return fmt.Errorf("list accounts: %w", err)
	}
	var outputTaxAccountID, inputTaxAccountID string
	for _, a := range accounts {
		switch a.Code {
		case s.orgProfile.outputTaxCode:
			outputTaxAccountID = a.ID
		case s.orgProfile.inputTaxCode:
			inputTaxAccountID = a.ID
		}
	}
	if outputTaxAccountID == "" || inputTaxAccountID == "" {
		return fmt.Errorf("could not find VAT accounts (code %s/%s) on the new organization's chart of accounts",
			s.orgProfile.outputTaxCode, s.orgProfile.inputTaxCode)
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

// setupVendors and setupClients still generate German-flavored phone
// numbers, VATIN format, and city names (catalog.go's City/VATIN helpers)
// regardless of --country — see orgProfiles' doc comment above for why
// that's a known, not yet closed, scope boundary. CountryCode itself does
// follow the resolved orgProfile, since (unlike those cosmetic details) it
// feeds real business logic elsewhere (e.g. db/einvoice.go's e-invoice
// profile resolution keys off a client's own CountryCode).
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
			CountryCode:    nonEmptyStrPtr(s.orgProfile.countryCode),
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
			CountryCode:    nonEmptyStrPtr(s.orgProfile.countryCode),
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

	// usedSKUs guards against productSKU's random 4-digit suffix colliding
	// on the products.sku UNIQUE(organizationId, sku) constraint —
	// slugPrefix truncates to 8 characters, so every displacement variant
	// of a componentTemplate/motorcycle model line (catalog.go) shares one
	// prefix (e.g. "Radiator (50cc)".."Radiator (650cc)" all slug to
	// "radiator"), and with 300+ products drawn from a 9000-value space per
	// prefix group, an unretried collision became likely rather than rare
	// once the catalog grew from ~34 to 310 entries.
	usedSKUs := make(map[string]bool, len(serviceCatalog)+len(productCatalog))
	uniqueSKU := func(name string) string {
		for {
			sku := productSKU(name, s.rng)
			if !usedSKUs[sku] {
				usedSKUs[sku] = true
				return sku
			}
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
		var categoryPtr *string
		if entry.category != "" {
			categoryPtr = strPtr(entry.category)
		}
		req := db.CreateProductRequest{
			OrganizationID: s.orgID,
			Name:           entry.name,
			SKU:            strPtr(uniqueSKU(entry.name)),
			Price:          price,
			UnitCost:       costPtr,
			Unit:           unitPtr,
			Type:           typ,
			Category:       categoryPtr,
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
			qtyLo: entry.qtyLo, qtyHi: entry.qtyHi, category: entry.category,
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
