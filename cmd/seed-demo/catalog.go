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

var productCatalog = []productCatalogEntry{
	{name: "Steel bracket, galvanized", unit: "pcs", stockEnabled: true, priceCentsLo: 350, priceCentsHi: 1200, costFactorLo: 0.45, costFactorHi: 0.65},
	{name: "Aluminium profile, 2m", unit: "pcs", stockEnabled: true, priceCentsLo: 1200, priceCentsHi: 4500, costFactorLo: 0.5, costFactorHi: 0.7},
	{name: "Cable duct, 40x60mm", unit: "m", stockEnabled: true, priceCentsLo: 450, priceCentsHi: 900, costFactorLo: 0.4, costFactorHi: 0.6},
	{name: "Hex bolt set M8 (100pcs)", unit: "pcs", stockEnabled: true, priceCentsLo: 800, priceCentsHi: 1800, costFactorLo: 0.5, costFactorHi: 0.65},
	{name: "Industrial hinge, heavy duty", unit: "pcs", stockEnabled: true, priceCentsLo: 1500, priceCentsHi: 3500, costFactorLo: 0.5, costFactorHi: 0.7},
	{name: "PVC conduit, 25mm", unit: "m", stockEnabled: true, priceCentsLo: 200, priceCentsHi: 550, costFactorLo: 0.4, costFactorHi: 0.55},
	{name: "Circuit breaker, 16A", unit: "pcs", stockEnabled: true, priceCentsLo: 900, priceCentsHi: 2200, costFactorLo: 0.5, costFactorHi: 0.68},
	{name: "LED panel light, 60x60cm", unit: "pcs", stockEnabled: true, priceCentsLo: 3500, priceCentsHi: 9000, costFactorLo: 0.45, costFactorHi: 0.6},
	{name: "Safety helmet, EN397", unit: "pcs", stockEnabled: true, priceCentsLo: 1200, priceCentsHi: 2800, costFactorLo: 0.4, costFactorHi: 0.55},
	{name: "Work gloves, cut resistant", unit: "pair", stockEnabled: true, priceCentsLo: 600, priceCentsHi: 1500, costFactorLo: 0.4, costFactorHi: 0.55},
	{name: "Pallet wrap, 500mm x 300m", unit: "roll", stockEnabled: true, priceCentsLo: 800, priceCentsHi: 1600, costFactorLo: 0.45, costFactorHi: 0.6},
	{name: "Corrugated shipping box, large", unit: "pcs", stockEnabled: true, priceCentsLo: 150, priceCentsHi: 450, costFactorLo: 0.4, costFactorHi: 0.6},
	{name: "Network switch, 24-port", unit: "pcs", stockEnabled: true, priceCentsLo: 12000, priceCentsHi: 38000, costFactorLo: 0.55, costFactorHi: 0.72},
	{name: "Ethernet cable, Cat6, 10m", unit: "pcs", stockEnabled: true, priceCentsLo: 800, priceCentsHi: 1800, costFactorLo: 0.4, costFactorHi: 0.55},
	{name: "UPS battery backup, 1500VA", unit: "pcs", stockEnabled: true, priceCentsLo: 15000, priceCentsHi: 32000, costFactorLo: 0.55, costFactorHi: 0.7},
	{name: "Server rack, 42U", unit: "pcs", stockEnabled: true, priceCentsLo: 45000, priceCentsHi: 120000, costFactorLo: 0.55, costFactorHi: 0.72},
	{name: "Industrial fan, 400mm", unit: "pcs", stockEnabled: true, priceCentsLo: 4000, priceCentsHi: 9500, costFactorLo: 0.45, costFactorHi: 0.62},
	{name: "Hydraulic hose, 5m", unit: "pcs", stockEnabled: true, priceCentsLo: 2500, priceCentsHi: 6000, costFactorLo: 0.5, costFactorHi: 0.68},
	{name: "Ball bearing, 6205-2RS", unit: "pcs", stockEnabled: true, priceCentsLo: 300, priceCentsHi: 900, costFactorLo: 0.45, costFactorHi: 0.6},
	{name: "Toolbox, steel, lockable", unit: "pcs", stockEnabled: true, priceCentsLo: 3500, priceCentsHi: 8000, costFactorLo: 0.45, costFactorHi: 0.62},
	{name: "Office chair, ergonomic", unit: "pcs", stockEnabled: true, priceCentsLo: 12000, priceCentsHi: 32000, costFactorLo: 0.5, costFactorHi: 0.65},
	{name: "Desk, adjustable height", unit: "pcs", stockEnabled: true, priceCentsLo: 25000, priceCentsHi: 55000, costFactorLo: 0.5, costFactorHi: 0.68},
	{name: "Filing cabinet, 4-drawer", unit: "pcs", stockEnabled: true, priceCentsLo: 15000, priceCentsHi: 32000, costFactorLo: 0.5, costFactorHi: 0.65},
	{name: "Printer toner cartridge", unit: "pcs", stockEnabled: true, priceCentsLo: 4500, priceCentsHi: 11000, costFactorLo: 0.5, costFactorHi: 0.7},
	{name: "Label printer roll, 100m", unit: "roll", stockEnabled: true, priceCentsLo: 900, priceCentsHi: 2200, costFactorLo: 0.45, costFactorHi: 0.6},
}

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
