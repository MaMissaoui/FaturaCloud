package main

// Product catalog for --scenario retail: a small Tunisian home-appliance
// retailer. Every entry has category "" (never "finished"/"component") —
// unlike catalog.go's motorcycle catalog, this business buys and sells the
// *same* physical good, so "" is the only category value that passes both
// sales.go's sellableProducts (excludes "component") and purchasing.go's
// stockProducts (excludes "finished") filters. Prices are in the
// organization's own functional currency (TND cents, since --scenario
// retail is always run with --country Tunisia --currency TND), plausible
// rather than sourced from a real price list — the same "plausible, not
// authoritative" spirit catalog.go's countryLocale generators already use.

// applianceVariant is one size/capacity/model point within a category —
// each becomes its own productCatalogEntry. Unlike catalog.go's
// displacementClass (one shared 5-way dimension every motorcycle/component
// is generated across), appliance categories don't share a single sizing
// axis, so each category owns its own variant list.
type applianceVariant struct {
	name                       string
	priceCentsLo, priceCentsHi int64
}

// brandTier is a price multiplier applied on top of a variant's own
// priceCentsLo/Hi, standing in for "which brand/quality tier" without this
// tool inventing fake brand names — a real appliance shop's shelf has
// several price points for the same size/capacity (economy/standard/
// premium), and a nil tiers list (small appliances below) just means "one
// entry per variant, no tiering" rather than every category needing one.
type brandTier struct {
	label      string
	multiplier float64
}

var majorApplianceTiers = []brandTier{
	{"Économique", 0.85},
	{"Standard", 1.0},
	{"Premium", 1.3},
}

type applianceCategory struct {
	name                       string
	unit                       string
	costFactorLo, costFactorHi float64
	qtyLo, qtyHi               int
	variants                   []applianceVariant
	// tiers is empty for small appliances (their variant list already
	// distinguishes basic/mid/high-power products by name) and
	// majorApplianceTiers for the big-ticket categories, where a size like
	// "Single-door 180L" plausibly comes in several brand/quality tiers at
	// once.
	tiers []brandTier
}

// applianceCategories is this tool's only hand-picked list for the retail
// scenario — extending the catalog (a new category, a new size) means
// editing one entry here. costFactorLo/Hi is deliberately tighter and
// higher (0.70-0.88) than the motorcycle catalog's component tiers
// (0.45-0.75): a retailer's margin on a resold appliance is a wholesale
// markup, not the value added by assembling parts into a finished good.
var applianceCategories = []applianceCategory{
	{
		name: "Réfrigérateur", unit: "pcs", costFactorLo: 0.78, costFactorHi: 0.88, qtyLo: 1, qtyHi: 1,
		tiers: majorApplianceTiers,
		variants: []applianceVariant{
			{"1 porte 180L", 80000, 130000},
			{"1 porte 250L", 130000, 190000},
			{"2 portes 320L", 190000, 280000},
			{"2 portes 420L", 280000, 380000},
			{"Side-by-side 500L", 380000, 550000},
		},
	},
	{
		name: "Congélateur", unit: "pcs", costFactorLo: 0.78, costFactorHi: 0.87, qtyLo: 1, qtyHi: 1,
		tiers: majorApplianceTiers,
		variants: []applianceVariant{
			{"Coffre 100L", 60000, 95000},
			{"Coffre 200L", 95000, 150000},
			{"Armoire 250L", 150000, 220000},
		},
	},
	{
		name: "Machine à laver", unit: "pcs", costFactorLo: 0.76, costFactorHi: 0.86, qtyLo: 1, qtyHi: 1,
		tiers: majorApplianceTiers,
		variants: []applianceVariant{
			{"Ouverture dessus 6kg", 90000, 140000},
			{"Ouverture dessus 8kg", 140000, 190000},
			{"Hublot 7kg", 190000, 240000},
			{"Hublot 9kg", 240000, 280000},
		},
	},
	{
		name: "Climatiseur", unit: "pcs", costFactorLo: 0.75, costFactorHi: 0.85, qtyLo: 1, qtyHi: 1,
		tiers: majorApplianceTiers,
		variants: []applianceVariant{
			{"Split 9000 BTU", 120000, 170000},
			{"Split 12000 BTU", 170000, 230000},
			{"Split 18000 BTU", 230000, 310000},
			{"Split 24000 BTU", 310000, 380000},
		},
	},
	{
		name: "Téléviseur", unit: "pcs", costFactorLo: 0.72, costFactorHi: 0.85, qtyLo: 1, qtyHi: 1,
		tiers: majorApplianceTiers,
		variants: []applianceVariant{
			{`LED 32"`, 50000, 90000},
			{`LED 43"`, 90000, 140000},
			{`4K 50"`, 140000, 220000},
			{`4K 55"`, 220000, 320000},
			{`4K 65"`, 320000, 450000},
		},
	},
	{
		name: "Four à micro-ondes", unit: "pcs", costFactorLo: 0.70, costFactorHi: 0.83, qtyLo: 1, qtyHi: 1,
		tiers: majorApplianceTiers,
		variants: []applianceVariant{
			{"Solo 20L", 25000, 40000},
			{"Gril 25L", 40000, 60000},
			{"Convection 30L", 60000, 85000},
		},
	},
	{
		name: "Cuisinière gaz/électrique", unit: "pcs", costFactorLo: 0.74, costFactorHi: 0.85, qtyLo: 1, qtyHi: 1,
		tiers: majorApplianceTiers,
		variants: []applianceVariant{
			{"Cuisinière gaz 4 feux", 60000, 95000},
			{"Cuisinière gaz 5 feux", 95000, 130000},
			{"Cuisinière électrique avec four", 130000, 180000},
		},
	},
	{
		name: "Chauffe-eau", unit: "pcs", costFactorLo: 0.74, costFactorHi: 0.85, qtyLo: 1, qtyHi: 1,
		tiers: majorApplianceTiers,
		variants: []applianceVariant{
			{"Électrique 30L", 25000, 40000},
			{"Électrique 50L", 40000, 60000},
			{"Chauffe-eau instantané au gaz", 60000, 90000},
		},
	},
	{
		name: "Lave-vaisselle", unit: "pcs", costFactorLo: 0.76, costFactorHi: 0.86, qtyLo: 1, qtyHi: 1,
		tiers: majorApplianceTiers,
		variants: []applianceVariant{
			{"Compact 6 couverts", 90000, 130000},
			{"Standard 12 couverts", 130000, 180000},
		},
	},
	{
		name: "Mixeur", unit: "pcs", costFactorLo: 0.55, costFactorHi: 0.72, qtyLo: 1, qtyHi: 2,
		variants: []applianceVariant{
			{"Basique", 4000, 7000},
			{"Milieu de gamme", 7000, 12000},
			{"Haute puissance", 12000, 20000},
		},
	},
	{
		name: "Bouilloire électrique", unit: "pcs", costFactorLo: 0.55, costFactorHi: 0.70, qtyLo: 1, qtyHi: 3,
		variants: []applianceVariant{
			{"Basique 1.5L", 3000, 5000},
			{"Sans fil 1.7L", 5000, 8000},
		},
	},
	{
		name: "Grille-pain", unit: "pcs", costFactorLo: 0.55, costFactorHi: 0.70, qtyLo: 1, qtyHi: 2,
		variants: []applianceVariant{
			{"2 tranches", 3500, 6000},
			{"4 tranches", 6000, 10000},
		},
	},
	{
		name: "Cafetière", unit: "pcs", costFactorLo: 0.58, costFactorHi: 0.73, qtyLo: 1, qtyHi: 2,
		variants: []applianceVariant{
			{"Filtre, basique", 6000, 10000},
			{"Filtre, programmable", 10000, 16000},
			{"Machine à espresso", 16000, 30000},
		},
	},
	{
		name: "Robot ménager", unit: "pcs", costFactorLo: 0.58, costFactorHi: 0.74, qtyLo: 1, qtyHi: 1,
		variants: []applianceVariant{
			{"Compact", 8000, 14000},
			{"Standard", 14000, 24000},
			{"Multifonction", 24000, 40000},
		},
	},
	{
		name: "Batteur", unit: "pcs", costFactorLo: 0.60, costFactorHi: 0.75, qtyLo: 1, qtyHi: 1,
		variants: []applianceVariant{
			{"Batteur à main", 6000, 11000},
			{"Batteur sur socle, basique", 25000, 40000},
			{"Batteur sur socle, premium", 40000, 70000},
		},
	},
	{
		name: "Extracteur de jus", unit: "pcs", costFactorLo: 0.58, costFactorHi: 0.73, qtyLo: 1, qtyHi: 1,
		variants: []applianceVariant{
			{"Presse-agrumes", 4000, 7000},
			{"Centrifugeuse", 10000, 18000},
			{"Extracteur à froid", 18000, 35000},
		},
	},
	{
		name: "Ventilateur", unit: "pcs", costFactorLo: 0.55, costFactorHi: 0.72, qtyLo: 1, qtyHi: 2,
		variants: []applianceVariant{
			{"De table", 4000, 8000},
			{"Sur pied", 8000, 15000},
			{"Plafonnier", 15000, 28000},
		},
	},
	{
		name: "Aspirateur", unit: "pcs", costFactorLo: 0.62, costFactorHi: 0.77, qtyLo: 1, qtyHi: 1,
		variants: []applianceVariant{
			{"Avec sac, basique", 15000, 25000},
			{"Sans sac, standard", 25000, 45000},
			{"Sans fil, cyclonique", 45000, 80000},
		},
	},
	{
		name: "Fer à repasser", unit: "pcs", costFactorLo: 0.55, costFactorHi: 0.70, qtyLo: 1, qtyHi: 2,
		variants: []applianceVariant{
			{"Fer sec", 3000, 5000},
			{"Fer à vapeur", 5000, 9000},
			{"Centrale vapeur", 9000, 18000},
		},
	},
	{
		name: "Fontaine à eau", unit: "pcs", costFactorLo: 0.65, costFactorHi: 0.80, qtyLo: 1, qtyHi: 1,
		variants: []applianceVariant{
			{"Bonbonne dessus, chaud/froid", 15000, 25000},
			{"Bonbonne dessous, chaud/froid", 25000, 40000},
			{"Avec réfrigérateur intégré", 40000, 65000},
		},
	},
}

// buildApplianceCatalog expands applianceCategories into one
// productCatalogEntry per variant — the "tier + template expansion"
// pattern catalog.go's buildComponentCatalog already uses, but with each
// category's own variant list rather than one shared dimension. category
// is deliberately left "" on every entry — see this file's own doc
// comment for why that (not "finished") is what makes both sides of the
// business work.
func buildApplianceCatalog() []productCatalogEntry {
	var out []productCatalogEntry
	for _, cat := range applianceCategories {
		tiers := cat.tiers
		if len(tiers) == 0 {
			tiers = []brandTier{{"", 1.0}} // no tiering — one entry per variant
		}
		for _, v := range cat.variants {
			for _, tier := range tiers {
				name := cat.name + " — " + v.name
				if tier.label != "" {
					name += " (" + tier.label + ")"
				}
				out = append(out, productCatalogEntry{
					name:         name,
					unit:         cat.unit,
					stockEnabled: true,
					priceCentsLo: int64(float64(v.priceCentsLo) * tier.multiplier),
					priceCentsHi: int64(float64(v.priceCentsHi) * tier.multiplier),
					costFactorLo: cat.costFactorLo, costFactorHi: cat.costFactorHi,
					qtyLo: cat.qtyLo, qtyHi: cat.qtyHi,
				})
			}
		}
	}
	return out
}

// retailServiceCatalog is the small set of services a home-appliance
// counter plausibly sells alongside physical goods on the same Cash Book
// sale — delivery, installation, warranty, repair visits. Not counted
// against the "120-150 products" target (these are services, Type
// "service", not stock-tracked products).
var retailServiceCatalog = []productCatalogEntry{
	{name: "Livraison à domicile", priceCentsLo: 3000, priceCentsHi: 8000, qtyLo: 1, qtyHi: 1},
	{name: "Service d'installation", priceCentsLo: 4000, priceCentsHi: 12000, qtyLo: 1, qtyHi: 1},
	{name: "Extension de garantie (1 an)", priceCentsLo: 5000, priceCentsHi: 20000, qtyLo: 1, qtyHi: 1},
	{name: "Extension de garantie (2 ans)", priceCentsLo: 9000, priceCentsHi: 35000, qtyLo: 1, qtyHi: 1},
	{name: "Visite de réparation / entretien", priceCentsLo: 3000, priceCentsHi: 15000, qtyLo: 1, qtyHi: 1},
	{name: "Enlèvement de l'ancien appareil", priceCentsLo: 1500, priceCentsHi: 4000, qtyLo: 1, qtyHi: 1},
}
