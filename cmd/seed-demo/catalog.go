package main

import (
	"fmt"
	"math/rand/v2"
)

// This file is the only place with hand-picked word lists — every other file
// asks a *Rand for a name/city/product rather than embedding its own guesses,
// so extending the demo data's variety (a new product category, a new city)
// only ever means editing one list here.

var companyStems = []string{
	"Nordlicht", "Rheinblick", "Alpenwerk", "Havelstern", "Elbmarkt", "Baltisch",
	"Schwarzwald", "Havelland", "Spreewerk", "Odertal", "Isarquell", "Mainstrom",
	"Nordkontor", "Maschinenwerk", "Feinwerk", "Metallbau", "Solartech", "Bauzentrum",
	"Logistik", "Handwerk", "Systemtechnik", "Datenwerk", "Industriebau", "Werkstoff",
	"Kupfer", "Silber", "Granit", "Quarz", "Bernstein", "Zeder",
}

var companySuffixes = []string{
	"GmbH", "GmbH & Co. KG", "AG", "KG", "e.K.", "Handels GmbH", "Systems GmbH",
	"Solutions AG", "Service GmbH", "Vertrieb GmbH",
}

var vendorFocus = []string{
	"Metallwaren", "Elektronik", "Bürobedarf", "Verpackung", "Werkzeuge",
	"Baustoffe", "Textilien", "Chemie", "Maschinenteile", "IT-Zubehör",
}

var personFirstNames = []string{
	"Anna", "Ben", "Clara", "David", "Elif", "Finn", "Greta", "Hannah", "Ismail",
	"Jonas", "Katrin", "Lukas", "Mira", "Noah", "Olga", "Paul", "Quentin", "Rosa",
	"Sven", "Theresa", "Uwe", "Vera", "Wilhelm", "Yasmin", "Zoe",
}

var personLastNames = []string{
	"Müller", "Schmidt", "Schneider", "Fischer", "Weber", "Meyer", "Wagner",
	"Becker", "Hoffmann", "Schulz", "Koch", "Bauer", "Richter", "Klein", "Wolf",
	"Neumann", "Schwarz", "Zimmermann", "Braun", "Krüger",
}

var cities = []struct {
	name string
	plz  string
}{
	{"Berlin", "10115"}, {"Munich", "80331"}, {"Hamburg", "20095"},
	{"Cologne", "50667"}, {"Frankfurt", "60311"}, {"Stuttgart", "70173"},
	{"Düsseldorf", "40213"}, {"Leipzig", "04109"}, {"Dortmund", "44135"},
	{"Essen", "45127"}, {"Bremen", "28195"}, {"Dresden", "01067"},
	{"Hanover", "30159"}, {"Nuremberg", "90402"}, {"Mannheim", "68159"},
}

var streetNames = []string{
	"Hauptstraße", "Bahnhofstraße", "Industriering", "Gewerbepark", "Am Kanal",
	"Marktplatz", "Werkstraße", "Lindenallee", "Schulstraße", "Rosenweg",
	"Fabrikstraße", "Kirchweg", "Talstraße", "Bergstraße", "Ringstraße",
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
				category: "finished",
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
				category: "component",
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

// Rand wraps math/rand/v2's PCG source seeded deterministically, plus the
// small helpers every generator in this tool needs (weighted picks, business
// names, money ranges). Everything in cmd/seed-demo goes through one *Rand
// so a given --seed always reproduces the same dataset end to end.
type Rand struct {
	r *rand.Rand
}

func NewRand(seed uint64) *Rand {
	return &Rand{r: rand.New(rand.NewPCG(seed, seed^0x9E3779B97F4A7C15))}
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
	return fmt.Sprintf("%s %s %s", Pick(rr, companyStems), Pick(rr, vendorFocus), Pick(rr, companySuffixes))
}

func (rr *Rand) ClientName() string {
	// Mostly companies (B2B is the common case for this app's document
	// types), a minority of person-style names for variety.
	if rr.Chance(0.85) {
		return rr.CompanyName()
	}
	return Pick(rr, personFirstNames) + " " + Pick(rr, personLastNames)
}

func (rr *Rand) City() (name, plz string) {
	c := Pick(rr, cities)
	return c.name, c.plz
}

func (rr *Rand) Street() string {
	return fmt.Sprintf("%s %d", Pick(rr, streetNames), rr.IntRange(1, 180))
}

func (rr *Rand) VATIN() string {
	return fmt.Sprintf("DE%09d", rr.IntRange(100000000, 999999999))
}

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
