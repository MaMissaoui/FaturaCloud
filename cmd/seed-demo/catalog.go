package main

import (
	"fmt"
	"math/rand/v2"
)

// This file is the only place with hand-picked word lists — every other file
// asks a *Rand for a name/city/product rather than embedding its own guesses,
// so extending the demo data's variety (a new product category, a new city)
// only ever means editing one list here.

// cityEntry is one entry in a locale's cities list — a name plus a
// plausible postal code, picked together so the two never drift apart the
// way independently-random name/code fields would.
type cityEntry struct {
	name string
	plz  string
}

// countryLocale is every hand-picked word list/generator setupVendors and
// setupClients (masterdata.go) draw from for a given --country — company
// names, person names, cities, streets, and the phone/VATIN formats real
// businesses in that country actually use. This is deliberately a
// different (larger) set of concerns than orgProfile (masterdata.go): that
// type is the *organization's own* address/VAT-account profile, this one
// is the shape of the *other* businesses (clients/vendors) it trades with —
// see localeFor's doc comment for why every --country still gets one
// rather than only Germany/Tunisia.
type countryLocale struct {
	companyStems, companySuffixes, vendorFocus []string
	personFirstNames, personLastNames          []string
	cities                                     []cityEntry
	streetNames                                []string
	phone                                      func(rr *Rand) string
	vatin                                      func(rr *Rand) string
}

var localeGermany = countryLocale{
	companyStems: []string{
		"Nordlicht", "Rheinblick", "Alpenwerk", "Havelstern", "Elbmarkt", "Baltisch",
		"Schwarzwald", "Havelland", "Spreewerk", "Odertal", "Isarquell", "Mainstrom",
		"Nordkontor", "Maschinenwerk", "Feinwerk", "Metallbau", "Solartech", "Bauzentrum",
		"Logistik", "Handwerk", "Systemtechnik", "Datenwerk", "Industriebau", "Werkstoff",
		"Kupfer", "Silber", "Granit", "Quarz", "Bernstein", "Zeder",
	},
	companySuffixes: []string{
		"GmbH", "GmbH & Co. KG", "AG", "KG", "e.K.", "Handels GmbH", "Systems GmbH",
		"Solutions AG", "Service GmbH", "Vertrieb GmbH",
	},
	vendorFocus: []string{
		"Metallwaren", "Elektronik", "Bürobedarf", "Verpackung", "Werkzeuge",
		"Baustoffe", "Textilien", "Chemie", "Maschinenteile", "IT-Zubehör",
	},
	personFirstNames: []string{
		"Anna", "Ben", "Clara", "David", "Elif", "Finn", "Greta", "Hannah", "Ismail",
		"Jonas", "Katrin", "Lukas", "Mira", "Noah", "Olga", "Paul", "Quentin", "Rosa",
		"Sven", "Theresa", "Uwe", "Vera", "Wilhelm", "Yasmin", "Zoe",
	},
	personLastNames: []string{
		"Müller", "Schmidt", "Schneider", "Fischer", "Weber", "Meyer", "Wagner",
		"Becker", "Hoffmann", "Schulz", "Koch", "Bauer", "Richter", "Klein", "Wolf",
		"Neumann", "Schwarz", "Zimmermann", "Braun", "Krüger",
	},
	cities: []cityEntry{
		{"Berlin", "10115"}, {"Munich", "80331"}, {"Hamburg", "20095"},
		{"Cologne", "50667"}, {"Frankfurt", "60311"}, {"Stuttgart", "70173"},
		{"Düsseldorf", "40213"}, {"Leipzig", "04109"}, {"Dortmund", "44135"},
		{"Essen", "45127"}, {"Bremen", "28195"}, {"Dresden", "01067"},
		{"Hanover", "30159"}, {"Nuremberg", "90402"}, {"Mannheim", "68159"},
	},
	streetNames: []string{
		"Hauptstraße", "Bahnhofstraße", "Industriering", "Gewerbepark", "Am Kanal",
		"Marktplatz", "Werkstraße", "Lindenallee", "Schulstraße", "Rosenweg",
		"Fabrikstraße", "Kirchweg", "Talstraße", "Bergstraße", "Ringstraße",
	},
	phone: func(rr *Rand) string {
		return fmt.Sprintf("+49 %d %d", rr.IntRange(30, 891), rr.IntRange(100000, 999999))
	},
	vatin: func(rr *Rand) string {
		return fmt.Sprintf("DE%09d", rr.IntRange(100000000, 999999999))
	},
}

// localeTunisia backs client/vendor generation for --country Tunisia —
// added alongside orgProfiles' own Tunisia entry so a Tunisia-seeded
// organization's master data actually reads as Tunisian rather than
// German placeholder text with a Tunisian address bolted onto the
// organization alone. companySuffixes leans on the legal forms that
// actually appear on Tunisian trade registers (SARL/SUARL — a
// single-member SARL is common there — and Ets for a smaller family
// business, "Établissement"), and vatin mirrors the org profile's own
// "Matricule Fiscal" shape (7 digits + a check letter + the category
// codes) rather than reusing DE's VATIN format — the client/vendor form
// relabels this field "MF" specifically when countryCode is "TN" (see
// src/components/clients/form.tsx).
var localeTunisia = countryLocale{
	companyStems: []string{
		"Carthage", "Maghreb", "Méditerranée", "Zaghouan", "Kairouan", "Sahel",
		"Jasmin", "Oliveraie", "Ariana", "Kerkennah", "Sidi Bou", "Technopole",
		"Nord-Sud", "Atlas", "Ichkeul", "Jerba", "Utique", "Bizerte Nord",
	},
	companySuffixes: []string{
		"SARL", "SUARL", "SA", "Ets", "& Fils", "Groupe",
	},
	vendorFocus: []string{
		"Mécanique", "Électronique", "Import-Export", "Industrie", "Métallurgie",
		"Plastique", "Textile", "Logistique", "Négoce", "Matériaux",
	},
	personFirstNames: []string{
		"Ahmed", "Mohamed", "Amine", "Karim", "Sami", "Walid", "Yassine", "Mehdi",
		"Nizar", "Anis", "Amira", "Ines", "Sarra", "Rania", "Nour", "Yosra",
		"Emna", "Salma", "Wafa", "Dorra",
	},
	personLastNames: []string{
		"Trabelsi", "Bouazizi", "Jlassi", "Gharbi", "Cherif", "Hammami",
		"Mansour", "Zaidi", "Khelifi", "Mejri", "Sassi", "Bouzid", "Karoui",
		"Slim", "Ben Salah", "Ayari", "Ferjani", "Guesmi", "Chaabane", "Rekik",
	},
	cities: []cityEntry{
		{"Tunis", "1000"}, {"Sfax", "3000"}, {"Sousse", "4000"},
		{"Bizerte", "7000"}, {"Gabès", "6000"}, {"Ariana", "2080"},
		{"Monastir", "5000"}, {"Nabeul", "8000"}, {"Kairouan", "3100"},
		{"Gafsa", "2100"}, {"Médenine", "4100"}, {"Kasserine", "1200"},
	},
	streetNames: []string{
		"Avenue Habib Bourguiba", "Rue de Marseille", "Avenue Mohamed V",
		"Rue Ibn Khaldoun", "Zone Industrielle", "Rue de la Liberté",
		"Avenue de la République", "Rue du 18 Janvier", "Rue Farhat Hached",
		"Avenue Taïeb Mhiri",
	},
	phone: func(rr *Rand) string {
		// Real Tunisian numbers are 8 digits total (2+3+3), matching the
		// org's own profile phone (+216 71 234 567, masterdata.go).
		return fmt.Sprintf("+216 %02d %03d %03d", rr.IntRange(20, 99), rr.IntRange(100, 999), rr.IntRange(100, 999))
	},
	vatin: func(rr *Rand) string {
		letters := "ABCDEHMNPT"
		letter := letters[rr.IntRange(0, len(letters)-1)]
		return fmt.Sprintf("%07d%c/A/M/000", rr.IntRange(1000000, 9999999), letter)
	},
}

// locales maps --country to its countryLocale; localeFor falls back to
// localeGermany for any country besides the two above — the same
// deliberate, not-yet-closed scope boundary orgProfiles documents for the
// organization's own address data (masterdata.go's genericOrgProfile).
var locales = map[string]countryLocale{
	"Germany": localeGermany,
	"Tunisia": localeTunisia,
}

func localeFor(country string) countryLocale {
	if l, ok := locales[country]; ok {
		return l
	}
	return localeGermany
}

// productCatalogEntry is a template for a product/service the demo org
// sells and/or buys. priceCents is what a client is charged; costCents is
// what a vendor charges (only meaningful for stockEnabled==true — a service
// has no purchasing side in this demo). qtyLo/qtyHi bounds how many units of
// this line a single invoice/order plausibly carries — critical for a
// service: "24 units of Annual support contract" on one invoice isn't
// realistic the way "24 hours of consulting" is, and without a per-entry
// range every service defaulted to the same hours-style spread, occasionally
// producing a five-figure invoice for one line of a fixed-fee item.
type productCatalogEntry struct {
	name         string
	unit         string // "" for a service
	stockEnabled bool
	priceCentsLo int64
	priceCentsHi int64
	costFactorLo float64 // cost as a fraction of price, e.g. 0.55
	costFactorHi float64
	qtyLo, qtyHi int
	// category mirrors products.category (db/product.go) — "finished" or
	// "component", "" for a service or anything left unclassified. Drives
	// both CreateProductRequest.Category and, more importantly, which side
	// of the business a generator is allowed to pick the product for: a
	// component is bought from a vendor and never sold directly, a
	// finished good is sold and never purchased — see sales.go/
	// purchasing.go's category filters, which mirror the same
	// finished/component split the real frontend product pickers apply
	// (CLAUDE.md's "products.category" note).
	category string
	// displacement is the displacementClass.label this entry was generated
	// at ("50cc".."650cc"), set only for "finished"/"component" entries —
	// production.go's assembly step groups both sides by this value, since
	// a component is only ever fit for a motorcycle of its own displacement
	// class (a "125cc" engine block doesn't go into a "650cc" frame).
	displacement string
}

var serviceCatalog = []productCatalogEntry{
	{name: "Consulting — strategy workshop (per hour)", priceCentsLo: 9000, priceCentsHi: 18000, qtyLo: 2, qtyHi: 16},
	{name: "Consulting — implementation support (per hour)", priceCentsLo: 7000, priceCentsHi: 14000, qtyLo: 2, qtyHi: 24},
	{name: "On-site technical support (per hour)", priceCentsLo: 6000, priceCentsHi: 11000, qtyLo: 1, qtyHi: 12},
	{name: "Software maintenance — monthly plan", priceCentsLo: 25000, priceCentsHi: 90000, qtyLo: 1, qtyHi: 1},
	{name: "System integration — fixed fee", priceCentsLo: 150000, priceCentsHi: 800000, qtyLo: 1, qtyHi: 1},
	{name: "Staff training session (half day)", priceCentsLo: 40000, priceCentsHi: 90000, qtyLo: 1, qtyHi: 2},
	{name: "Custom reporting setup", priceCentsLo: 60000, priceCentsHi: 200000, qtyLo: 1, qtyHi: 1},
	{name: "Annual support contract", priceCentsLo: 300000, priceCentsHi: 1200000, qtyLo: 1, qtyHi: 1},
	{name: "Installation service", priceCentsLo: 20000, priceCentsHi: 60000, qtyLo: 1, qtyHi: 3},
	{name: "Logistics coordination (per shipment)", priceCentsLo: 8000, priceCentsHi: 25000, qtyLo: 1, qtyHi: 4},
}

// displacementClass is the one dimension both the finished-motorcycle and
// component catalogs below are generated across — a part or model sized for
// a bigger engine costs proportionally more. multiplier scales a tier's
// baseline (125cc) price range.
type displacementClass struct {
	label      string
	multiplier float64
}

var displacementClasses = []displacementClass{
	{"50cc", 0.5}, {"125cc", 1.0}, {"250cc", 1.8}, {"400cc", 2.8}, {"650cc", 4.0},
}

// motorcycleModelLines × displacementClasses gives the 25 finished-goods
// entries a motorcycle assembler like "Atlas Moto Assemblage SARL" sells —
// see buildFinishedMotorcycleCatalog below. Not every line/displacement
// pairing is a real-world product a manufacturer would actually offer (a
// 650cc "Scooter" is a stretch), but distinct, plausible-sounding SKUs
// matter more here than a fully curated model lineup would.
var motorcycleModelLines = []string{"Roadster", "Cruiser", "Adventure", "Scrambler", "Scooter"}

// buildFinishedMotorcycleCatalog returns one entry per (model line,
// displacement) pair — 5x5 = 25, matching issue-driven demand for "around
// 25 finished products". costFactor is higher than a typical resold good
// (0.65-0.8, not the 0.4-0.7 range componentTiers below use): a finished
// motorcycle's cost is dominated by the parts and labor that went into
// assembling it, not a wholesale markup.
func buildFinishedMotorcycleCatalog() []productCatalogEntry {
	const baseLo, baseHi int64 = 180000, 280000 // 125cc baseline, EUR cents
	out := make([]productCatalogEntry, 0, len(motorcycleModelLines)*len(displacementClasses))
	for _, line := range motorcycleModelLines {
		for _, c := range displacementClasses {
			out = append(out, productCatalogEntry{
				name:         fmt.Sprintf("Atlas %s %s", line, c.label),
				unit:         "pcs",
				stockEnabled: true,
				priceCentsLo: int64(float64(baseLo) * c.multiplier),
				priceCentsHi: int64(float64(baseHi) * c.multiplier),
				costFactorLo: 0.65, costFactorHi: 0.8,
				qtyLo: 1, qtyHi: 3,
				category:     "finished",
				displacement: c.label,
			})
		}
	}
	return out
}

// componentTier is a shared price/cost/quantity bucket a componentTemplate
// picks by rough complexity — cheaper to maintain and reason about than a
// unique price range hand-tuned per one of 55 part names. qtyLo/qtyHi is a
// purchase-order line's bulk restocking quantity (small hardware ordered in
// the hundreds, an engine block ordered a handful at a time).
type componentTier struct {
	priceCentsLo, priceCentsHi int64
	costFactorLo, costFactorHi float64
	qtyLo, qtyHi               int
}

var (
	tierEngineCore      = componentTier{25000, 120000, 0.6, 0.75, 1, 6}
	tierMajorAssembly   = componentTier{8000, 35000, 0.55, 0.7, 2, 15}
	tierMidComponent    = componentTier{2000, 9000, 0.5, 0.65, 5, 40}
	tierElectricalSmall = componentTier{800, 4500, 0.45, 0.62, 10, 80}
	tierSmallHardware   = componentTier{250, 1200, 0.45, 0.6, 20, 200}
)

type componentTemplate struct {
	name string
	tier componentTier
}

// componentTemplates are the parts that go into assembling one of the
// motorcycles above — bought from vendors (purchasing.go's stockProducts
// excludes "finished" goods, the mirror image of sales.go excluding
// "component" ones), never sold directly to a client. 55 templates x 5
// displacementClasses = 275 rows, the middle of the requested 250-300
// component range.
var componentTemplates = []componentTemplate{
	{"Engine block", tierEngineCore},
	{"Cylinder head", tierEngineCore},
	{"Crankshaft", tierEngineCore},
	{"Gearbox housing", tierEngineCore},
	{"Frame chassis", tierEngineCore},

	{"Front fork assembly", tierMajorAssembly},
	{"Rear shock absorber", tierMajorAssembly},
	{"Swingarm", tierMajorAssembly},
	{"Radiator", tierMajorAssembly},
	{"Carburetor", tierMajorAssembly},
	{"Starter motor", tierMajorAssembly},
	{"Stator/alternator", tierMajorAssembly},
	{"Wiring harness", tierMajorAssembly},
	{"Fuel tank", tierMajorAssembly},
	{"Wheel rim", tierMajorAssembly},

	{"Piston kit", tierMidComponent},
	{"Connecting rod", tierMidComponent},
	{"Camshaft", tierMidComponent},
	{"Timing chain", tierMidComponent},
	{"Fuel injector", tierMidComponent},
	{"Fuel pump", tierMidComponent},
	{"Clutch plate set", tierMidComponent},
	{"Drive chain", tierMidComponent},
	{"Chain sprocket set", tierMidComponent},
	{"Exhaust header pipe", tierMidComponent},
	{"Muffler/silencer", tierMidComponent},
	{"Cooling fan", tierMidComponent},
	{"Oil pump", tierMidComponent},
	{"Brake caliper", tierMidComponent},
	{"Brake disc", tierMidComponent},
	{"Master cylinder", tierMidComponent},
	{"Wheel hub", tierMidComponent},
	{"Wheel bearing set", tierMidComponent},
	{"Battery", tierMidComponent},
	{"Voltage regulator", tierMidComponent},

	{"Ignition switch", tierElectricalSmall},
	{"Ignition coil", tierElectricalSmall},
	{"Headlight assembly", tierElectricalSmall},
	{"Tail light assembly", tierElectricalSmall},
	{"Turn signal set", tierElectricalSmall},
	{"Speedometer cluster", tierElectricalSmall},
	{"Horn", tierElectricalSmall},
	{"Handlebar", tierElectricalSmall},
	{"Handlebar grip set", tierElectricalSmall},
	{"Mirror set", tierElectricalSmall},

	{"Spark plug", tierSmallHardware},
	{"Clutch cable", tierSmallHardware},
	{"Brake cable", tierSmallHardware},
	{"Brake lever", tierSmallHardware},
	{"Brake pad set", tierSmallHardware},
	{"Air filter", tierSmallHardware},
	{"Oil filter", tierSmallHardware},
	{"Tire", tierSmallHardware},
	{"Inner tube", tierSmallHardware},
	{"Spoke set", tierSmallHardware},
}

// buildComponentCatalog expands componentTemplates across every
// displacementClass, scaling each tier's baseline price range by the
// class's multiplier — the same "bigger engine, pricier part" scaling
// buildFinishedMotorcycleCatalog uses for the vehicles these parts build.
func buildComponentCatalog() []productCatalogEntry {
	out := make([]productCatalogEntry, 0, len(componentTemplates)*len(displacementClasses))
	for _, t := range componentTemplates {
		for _, c := range displacementClasses {
			out = append(out, productCatalogEntry{
				name:         fmt.Sprintf("%s (%s)", t.name, c.label),
				unit:         "pcs",
				stockEnabled: true,
				priceCentsLo: int64(float64(t.tier.priceCentsLo) * c.multiplier),
				priceCentsHi: int64(float64(t.tier.priceCentsHi) * c.multiplier),
				costFactorLo: t.tier.costFactorLo, costFactorHi: t.tier.costFactorHi,
				qtyLo: t.tier.qtyLo, qtyHi: t.tier.qtyHi,
				category:     "component",
				displacement: c.label,
			})
		}
	}
	return out
}

// productCatalog is every physical good "Atlas Moto Assemblage SARL" deals
// in: 25 finished motorcycles it sells, plus 275 components it buys from
// vendors to assemble them — see setupProducts (masterdata.go) for how
// category then drives which side of the business (sales vs. purchasing)
// a generator is allowed to pick each one for.
var productCatalog = append(buildFinishedMotorcycleCatalog(), buildComponentCatalog()...)

// foreignVendorEntry is one of the fixed overseas suppliers Imports (F114)
// bring components in from — deliberately not generated from countryLocale
// the way local vendors are (that abstraction is keyed to the
// *organization's* --country, e.g. Tunisia, and has no notion of "a foreign
// counterparty regardless of the org's own country"). A short, explicit
// list is simpler and clearer here than stretching that abstraction to fit.
type foreignVendorEntry struct {
	name, city, postalCode string
}

// foreignVendorCatalog names real Chinese manufacturing/export hubs and
// plausible trading-company names — setupForeignVendors (masterdata.go)
// picks --vendors from this list (see maybeStartImport/purchasing.go's
// createImportLinkedPurchaseOrder for how these are the only vendors ever
// used for an Import-linked purchase order, never ordinary restocking).
var foreignVendorCatalog = []foreignVendorEntry{
	{"Shenzhen Hongda Motorcycle Parts Co., Ltd.", "Shenzhen", "518000"},
	{"Guangzhou Feilong Auto Parts Manufacturing Co., Ltd.", "Guangzhou", "510000"},
	{"Ningbo Zhongce Precision Components Co., Ltd.", "Ningbo", "315000"},
	{"Yiwu Xinyuan Trading Co., Ltd.", "Yiwu", "322000"},
	{"Dongguan Junhe Metal Products Co., Ltd.", "Dongguan", "523000"},
	{"Wuxi Taihu Machinery Manufacturing Co., Ltd.", "Wuxi", "214000"},
	{"Chongqing Jialing Powertrain Co., Ltd.", "Chongqing", "400000"},
	{"Tianjin Bohai Electrical Components Co., Ltd.", "Tianjin", "300000"},
}

// foreignVendorPhone/foreignVendorVATIN are a minimal, self-contained
// format for the two identifier fields every vendor needs — a +86 mobile
// pattern and a plausible 18-digit Chinese Unified Social Credit Code
// shape, neither validated against any real registry (same "plausible, not
// authoritative" spirit as countryLocale's own vatin generators).
func (rr *Rand) foreignVendorPhone() string {
	return fmt.Sprintf("+86 1%d %d", rr.IntRange(30, 89), rr.IntRange(10000000, 99999999))
}

func (rr *Rand) foreignVendorVATIN() string {
	const alnum = "0123456789ABCDEFGHJKLMNPQRTUWXY"
	b := make([]byte, 18)
	for i := range b {
		b[i] = alnum[rr.r.IntN(len(alnum))]
	}
	return string(b)
}

// Rand wraps math/rand/v2's PCG source seeded deterministically, plus the
// small helpers every generator in this tool needs (weighted picks, business
// names, money ranges). Everything in cmd/seed-demo goes through one *Rand
// so a given --seed always reproduces the same dataset end to end. locale
// is resolved once from --country at construction (localeFor) and read by
// every name/city/street/phone/VATIN helper below — see countryLocale's
// doc comment for why this is a separate concern from orgProfile.
type Rand struct {
	r      *rand.Rand
	locale countryLocale
}

func NewRand(seed uint64, country string) *Rand {
	return &Rand{r: rand.New(rand.NewPCG(seed, seed^0x9E3779B97F4A7C15)), locale: localeFor(country)}
}

// IntRange returns a value in [lo, hi] inclusive.
func (rr *Rand) IntRange(lo, hi int) int {
	if hi <= lo {
		return lo
	}
	return lo + rr.r.IntN(hi-lo+1)
}

func (rr *Rand) Int64Range(lo, hi int64) int64 {
	if hi <= lo {
		return lo
	}
	return lo + rr.r.Int64N(hi-lo+1)
}

func (rr *Rand) Float64Range(lo, hi float64) float64 {
	return lo + rr.r.Float64()*(hi-lo)
}

// Chance returns true with probability p (0..1).
func (rr *Rand) Chance(p float64) bool { return rr.r.Float64() < p }

// Pick is a free function, not a method, because Go methods can't take their
// own type parameters — every call site reads Pick(rr, items) instead of
// Pick(rr, items).
func Pick[T any](rr *Rand, items []T) T { return items[rr.r.IntN(len(items))] }

// Shuffle permutes items in place (Fisher-Yates via the shared source).
func Shuffle[T any](rr *Rand, items []T) {
	rr.r.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
}

func (rr *Rand) CompanyName() string {
	return fmt.Sprintf("%s %s %s", Pick(rr, rr.locale.companyStems), Pick(rr, rr.locale.vendorFocus), Pick(rr, rr.locale.companySuffixes))
}

func (rr *Rand) ClientName() string {
	// Mostly companies (B2B is the common case for this app's document
	// types), a minority of person-style names for variety.
	if rr.Chance(0.85) {
		return rr.CompanyName()
	}
	return Pick(rr, rr.locale.personFirstNames) + " " + Pick(rr, rr.locale.personLastNames)
}

func (rr *Rand) City() (name, plz string) {
	c := Pick(rr, rr.locale.cities)
	return c.name, c.plz
}

func (rr *Rand) Street() string {
	return fmt.Sprintf("%s %d", Pick(rr, rr.locale.streetNames), rr.IntRange(1, 180))
}

// Phone and VATIN dispatch on the resolved locale — see countryLocale's
// phone/vatin fields. setupVendors/setupClients (masterdata.go) call these
// instead of hand-formatting a German-shaped string themselves, which is
// what previously made every country's client/vendor data look German
// regardless of --country.
func (rr *Rand) Phone() string { return rr.locale.phone(rr) }
func (rr *Rand) VATIN() string { return rr.locale.vatin(rr) }

func (rr *Rand) Email(company string) string {
	return fmt.Sprintf("contact@%s.example", slugify(company))
}

func slugify(s string) string {
	out := make([]rune, 0, len(s))
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			out = append(out, r)
			prevDash = false
		case r >= 'A' && r <= 'Z':
			out = append(out, r+32)
			prevDash = false
		default:
			if !prevDash && len(out) > 0 {
				out = append(out, '-')
				prevDash = true
			}
		}
	}
	for len(out) > 0 && out[len(out)-1] == '-' {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return "demo"
	}
	if len(out) > 24 {
		out = out[:24]
	}
	return string(out)
}
