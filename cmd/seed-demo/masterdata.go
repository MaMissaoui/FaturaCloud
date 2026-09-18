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
	// invoiceNumberFormat overrides the default "INV-{year}-{number}" (see
	// setupOrganization) — "" means use the default. Invoice numbering has
	// no {number:N} padding token (unlike documentNumberFormats below): the
	// server's own validateInvoiceNumberFormat (db/organization.go) doesn't
	// recognize it.
	invoiceNumberFormat string
	// invoiceNumberPrefix is the prefix seeder.go's local invoiceNum
	// numberer uses ("" means "INV") — kept in sync with invoiceNumberFormat
	// by hand, since CreateInvoice (unlike CreateOrder/CreatePurchaseOrder/…)
	// has no server-side auto-generation to defer to: it inserts req.Number
	// literally, so this tool must still number invoices itself, and the
	// prefix it uses should visibly match what the org's own
	// invoiceNumberFormat setting says.
	invoiceNumberPrefix string
	// documentNumberFormats overrides db/document_number.go's
	// documentNumberDefaults for this country, keyed by documentType ("order",
	// "purchase_order", "delivery", "inbound_delivery", "production_order") —
	// nil means every type keeps the server's own default. Set via
	// setupDocumentNumberSettings (PUT .../document-number-settings/{type})
	// right after the organization exists, since (unlike invoiceNumberFormat)
	// this isn't a CreateOrganizationRequest field.
	documentNumberFormats map[string]string
	// paymentTermsFormat is a fmt.Sprintf verb taking dueDays, used as the
	// invoice's own free-text PaymentTerms line (sales.go's
	// createDirectInvoice/createInvoiceForOrder) — "" means the default
	// "Net %d days".
	paymentTermsFormat string
	// taxRateStandardName/taxRateReducedName/taxRateZeroName override
	// setupTaxRates' default English names ("Standard rate", "Reduced rate",
	// "Zero-rated") — "" means keep the default for that one rate.
	taxRateStandardName, taxRateReducedName, taxRateZeroName string
}

// orgProfiles has one entry per --country this tool has been taught real
// placeholder data and VAT account codes for. "Germany" was this tool's
// original (and only) shape; "Tunisia" was added alongside the --country/
// --currency flags for seeding a Tunisia-based organization. Client/vendor
// generation (setupVendors/setupClients) draws its own company/person
// names, cities, phone numbers, and VATIN format from catalog.go's
// countryLocale/localeFor for the same --country — the two profiles stay
// separate types (orgProfile here is only the organization's own address/
// VAT-account data) but are resolved from the same --country flag, so a
// Tunisia-seeded organization's vendors and clients read as Tunisian too,
// not German placeholder data with a Tunisian address bolted onto the
// organization alone.
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
		// French/Tunisian document-numbering conventions (Facture, Commande,
		// Bon de Commande, Bon de Livraison, Bon de Réception, Ordre de
		// Fabrication) instead of the tool's English defaults — see
		// setupOrganization/setupDocumentNumberSettings below.
		invoiceNumberFormat: "FAC-{year}-{number}", invoiceNumberPrefix: "FAC",
		documentNumberFormats: map[string]string{
			"order":            "CMD-{year}-{number:3}",
			"purchase_order":   "BC-{year}-{number:4}",
			"delivery":         "BL-{year}-{number:4}",
			"inbound_delivery": "BR-{year}-{number:4}",
			"production_order": "OF-{year}-{number:4}",
		},
		paymentTermsFormat:  "Paiement à %d jours",
		taxRateStandardName: "Taux normal", taxRateReducedName: "Taux réduit", taxRateZeroName: "Taux zéro",
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
		// {year}/{number} are the real generator's tokens (src/utils/invoice.ts,
		// db/cash_sale.go's Go port) — {YYYY}/{NNNN} aren't recognized and
		// used to render as a literal, unsubstituted invoice number on every
		// document this tool created. p.invoiceNumberFormat overrides this
		// per-country (e.g. Tunisia's "FAC-{year}-{number}").
		InvoiceNumberFormat: strPtr(orDefault(p.invoiceNumberFormat, "INV-{year}-{number}")),
	}

	var org db.Organization
	if err := s.c.Post("/api/organizations", req, &org); err != nil {
		return fmt.Errorf("create organization: %w", err)
	}
	s.orgID = org.ID
	if err := s.setupDocumentNumberSettings(); err != nil {
		return fmt.Errorf("document number settings: %w", err)
	}
	if org.DefaultCashAccountID == nil {
		return fmt.Errorf("newly created organization has no default cash account — cannot record payments")
	}
	s.cashAccountID = *org.DefaultCashAccountID
	if s.scenario.hasImports {
		// db/gl_posting.go's applyLandedCost (F114) credits this account when
		// receiving a purchase order linked to an import — seedDefaultChartOfAccounts
		// should always set it via importCostAdditions, but verify rather than
		// let every import-linked receipt in this run 409 one at a time until
		// the 25-error abort threshold (seeder.go's maybeAbort) trips partway
		// through an otherwise-successful multi-minute run.
		if org.DefaultImportCostsPayableAccountID == nil {
			return fmt.Errorf("newly created organization has no default import-costs-payable account — cannot receive a purchase order linked to an import")
		}
	}
	if s.scenario.hasCashBookSales {
		if err := s.setupCashRegisterAccount(); err != nil {
			return fmt.Errorf("cash register account: %w", err)
		}
		// cash_movements_retail.go's petty-cash withdrawal posts an
		// undocumented expense against this account.
		if org.DefaultExpenseAccountID == nil {
			return fmt.Errorf("newly created organization has no default expense account — cannot record a petty-cash withdrawal")
		}
		s.expenseAccountID = *org.DefaultExpenseAccountID
	}
	s.log.Printf("seed-demo: organization %q ready (%s)", s.cfg.OrgName, s.orgID)
	return nil
}

// setupDocumentNumberSettings applies s.orgProfile.documentNumberFormats (if
// any) via PUT .../document-number-settings/{documentType} — one call per
// overridden type, right after the organization exists. A country with no
// override (documentNumberFormats == nil, e.g. Germany/generic) leaves every
// type on the server's own default (db/document_number.go's
// documentNumberDefaults), unchanged from before this existed.
func (s *Seeder) setupDocumentNumberSettings() error {
	for documentType, format := range s.orgProfile.documentNumberFormats {
		req := db.UpdateDocumentNumberSettingRequest{Format: format}
		if err := s.c.Put("/api/organizations/"+s.orgID+"/document-number-settings/"+documentType, req, nil); err != nil {
			return fmt.Errorf("%s number format %q: %w", documentType, format, err)
		}
	}
	return nil
}

// orDefault returns v, or fallback if v is "" — the same "absence over a
// false claim" convention nonEmptyStrPtr uses just below, for a plain string
// value rather than a *string field.
func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// setupCashRegisterAccount wires organizations.defaultCashRegisterAccountId
// to the chart's own literal "Cash"/till leaf account — nothing does this
// automatically (unlike defaultCashAccountId, which every chart template
// wires to Bank via defaultRole: "cash" — see CLAUDE.md's cash register
// account note and db/account.go's chart definitions). This is exactly the
// one manual step a real admin has to do in Organization settings →
// Accounting, done here so Cash Book sales/withdrawals land somewhere the
// balance widget and Daily Cash Movements report actually watch.
func (s *Seeder) setupCashRegisterAccount() error {
	var accounts []db.Account
	if err := s.c.Get("/api/organizations/"+s.orgID+"/accounts", &accounts); err != nil {
		return fmt.Errorf("list accounts: %w", err)
	}
	var registerAccountID string
	for _, a := range accounts {
		if a.Code == "1010" { // "Cash" — the generic chart's till account (Tunisia has no dedicated chart template yet)
			registerAccountID = a.ID
			break
		}
	}
	if registerAccountID == "" {
		return fmt.Errorf(`could not find a "1010" (Cash) account on the new organization's chart of accounts`)
	}
	if err := s.c.Put("/api/organizations/"+s.orgID, db.UpdateOrganizationRequest{
		DefaultCashRegisterAccountID: &registerAccountID,
	}, nil); err != nil {
		return fmt.Errorf("set defaultCashRegisterAccountId: %w", err)
	}
	s.registerAccountID = registerAccountID
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
	if s.scenario.hasImports {
		if err := s.setupForeignVendors(); err != nil {
			return fmt.Errorf("foreign vendors: %w", err)
		}
	}
	if s.scenario.hasCashBookSales {
		// The client base grows organically via Cash Book's inline
		// NewClient creation instead — see cash_book_sales.go and
		// seeder.go's targetClientCount. Seeding a batch of clients up
		// front here would both double-count against that target and
		// give every walk-in "customer" an implausible pre-existing
		// account before their first purchase.
		s.log.Printf("seed-demo: skipping batch client setup — %s scenario grows its %d clients organically via Cash Book", s.scenario.name, s.targetClientCount)
	} else {
		if err := s.setupClients(); err != nil {
			return fmt.Errorf("clients: %w", err)
		}
	}
	if err := s.setupProducts(); err != nil {
		return fmt.Errorf("products: %w", err)
	}
	s.log.Printf("seed-demo: master data ready — %d vendors (+%d foreign), %d clients, %d products",
		len(s.vendors), len(s.foreignVendors), len(s.clients), len(s.products))
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
	if s.standardTax, err = create(orDefault(s.orgProfile.taxRateStandardName, "Standard rate"), 19, true, "S"); err != nil {
		return err
	}
	if s.reducedTax, err = create(orDefault(s.orgProfile.taxRateReducedName, "Reduced rate"), 7, false, "S"); err != nil {
		return err
	}
	if s.zeroTax, err = create(orDefault(s.orgProfile.taxRateZeroName, "Zero-rated"), 0, false, "Z"); err != nil {
		return err
	}
	return nil
}

// setupVendors and setupClients generate company/person names, city names,
// phone numbers, and VATIN format from the locale catalog.go resolves for
// --country (countryLocale/localeFor) — "Germany" and "Tunisia" have
// dedicated locales, anything else falls back to Germany's, the same
// scope boundary orgProfiles documents for the organization's own address
// data. CountryCode itself always followed the resolved orgProfile even
// before this, since (unlike those cosmetic details) it feeds real
// business logic elsewhere (e.g. db/einvoice.go's e-invoice profile
// resolution keys off a client's own CountryCode).
func (s *Seeder) setupVendors() error {
	for i := 0; i < s.vendorCount(); i++ {
		name := s.rng.CompanyName()
		city, plz := s.rng.City()
		req := db.CreateVendorRequest{
			OrganizationID: s.orgID,
			Name:           strPtr(name),
			Vatin:          strPtr(s.rng.VATIN()),
			Emails:         strPtr(fmt.Sprintf(`["%s"]`, s.rng.Email(name))),
			Phone:          strPtr(s.rng.Phone()),
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

// setupForeignVendors creates the fixed handful of overseas suppliers
// (catalog.go's foreignVendorCatalog) an Import (F114) sources components
// from — 5-7 of them, picked without replacement so a run never duplicates
// one. Their documents are priced in USD (DefaultCurrency, a prefill the
// real app's purchase-order form already reads — see CLAUDE.md's imports.go
// note) and never selected for ordinary local restocking (purchasing.go's
// createPurchaseOrder draws only from s.vendors); see
// createImportLinkedPurchaseOrder for the only path that uses them.
func (s *Seeder) setupForeignVendors() error {
	pool := append([]foreignVendorEntry(nil), foreignVendorCatalog...)
	Shuffle(s.rng, pool)
	n := s.rng.IntRange(5, 7)
	if n > len(pool) {
		n = len(pool)
	}
	for _, entry := range pool[:n] {
		req := db.CreateVendorRequest{
			OrganizationID:  s.orgID,
			Name:            strPtr(entry.name),
			Vatin:           strPtr(s.rng.foreignVendorVATIN()),
			Emails:          strPtr(fmt.Sprintf(`["%s"]`, s.rng.Email(entry.name))),
			Phone:           strPtr(s.rng.foreignVendorPhone()),
			Street:          strPtr(s.rng.Street()),
			HouseNumber:     strPtr(fmt.Sprintf("%d", s.rng.IntRange(1, 200))),
			PostalCode:      strPtr(entry.postalCode),
			City:            strPtr(entry.city),
			CountryCode:     strPtr("CN"),
			DefaultCurrency: strPtr("USD"),
		}
		var v db.Vendor
		if err := s.c.Post("/api/vendors", req, &v); err != nil {
			return fmt.Errorf("foreign vendor %q: %w", entry.name, err)
		}
		s.foreignVendors = append(s.foreignVendors, vendorRef{id: v.ID, name: entry.name, currency: "USD"})
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
			Phone:          strPtr(s.rng.Phone()),
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
	// prefix (e.g. "Radiateur (50cc)".."Radiateur (650cc)" all slug to
	// "radiateu"), and with 300+ products drawn from a 9000-value space per
	// prefix group, an unretried collision became likely rather than rare
	// once the catalog grew from ~34 to 310 entries.
	usedSKUs := make(map[string]bool, len(s.scenario.serviceCatalog)+len(s.scenario.productCatalog))
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
			displacement: entry.displacement,
		})
		s.stats.Products++
		return nil
	}

	for _, entry := range s.scenario.serviceCatalog {
		if err := create(entry); err != nil {
			return err
		}
	}
	for _, entry := range s.scenario.productCatalog {
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
