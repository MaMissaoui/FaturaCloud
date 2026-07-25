package db

import (
	"encoding/xml"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"
)

// eInvoiceProfile is a jurisdiction-specific set of EN 16931 UBL export
// rules: which CEN customization identifier (BT-24) to claim, and which
// extra fields that specification mandates on top of core EN 16931.
//
// Deliberately only two profiles exist: "DE" (XRechnung 3.0, which this
// generator has every field for) and a generic EN 16931 core profile for
// everywhere else. Peppol BIS Billing 3.0 (which would cover GB/US/most
// Peppol-network recipients) is NOT modeled here even though it's also
// EN 16931-based UBL — Peppol BIS mandates cbc:EndpointID with a scheme ID
// on both parties (BT-34/BT-49), which has no column and no reliable
// scheme to infer. Claiming a CustomizationID for a profile whose mandatory
// elements this generator can't actually produce would be a false
// conformance claim, not a missing-field 409 — worse than not offering the
// profile. Add it once EndpointID + scheme IDs are modeled.
type eInvoiceProfile struct {
	CustomizationID       string
	RequireBuyerReference bool
}

var deXRechnungProfile = eInvoiceProfile{
	CustomizationID:       "urn:cen.eu:en16931:2017#compliant#urn:xoev-de:kosit:standard:xrechnung_3.0",
	RequireBuyerReference: true,
}

// genericEN16931Profile claims only core EN 16931 conformance, with none of
// XRechnung's extra German B2G rules (no mandatory buyer reference). This is
// what every country other than Germany gets — including France, the UK,
// and the US, none of which have a CIUS this generator implements. It's a
// deliberately conservative default: valid EN 16931 UBL, no invented
// country-specific extension.
var genericEN16931Profile = eInvoiceProfile{
	CustomizationID:       "urn:cen.eu:en16931:2017",
	RequireBuyerReference: false,
}

var countryEInvoiceProfiles = map[string]eInvoiceProfile{
	"DE": deXRechnungProfile,
}

// resolveEInvoiceProfile picks the profile by the *buyer's* country, falling
// back to the seller's when the buyer's is unset. XRechnung/B2G e-invoicing
// mandates are triggered by the recipient's jurisdiction, not the issuer's —
// a German seller invoicing a French buyer does not owe that buyer an
// XRechnung, and the buyer reference (Leitweg-ID) it would require is a
// buyer-side routing identifier (mirrored by default_buyer_reference living
// on clients, not organizations). An unrecognized or missing country code
// resolves to the generic core profile rather than failing here — validated
// as a normal missing-field 409 by validateEInvoiceCompleteness instead.
func resolveEInvoiceProfile(client *Client, org *Organization) eInvoiceProfile {
	countryCode := ""
	if client.CountryCode != nil && *client.CountryCode != "" {
		countryCode = *client.CountryCode
	} else if org.CountryCode != nil && *org.CountryCode != "" {
		countryCode = *org.CountryCode
	}
	if profile, ok := countryEInvoiceProfiles[normalizeCountryCode(countryCode)]; ok {
		return profile
	}
	return genericEN16931Profile
}

// GenerateEInvoice renders an invoice as an EN 16931-conformant UBL 2.1
// Invoice document, using the export profile resolved for the buyer's
// country (see resolveEInvoiceProfile).
//
// This is deliberately minimal: it maps only the BT fields the schema has
// real columns for (see the 0034-0037 migrations) and does not attempt the
// full set of EN 16931 business rules (BR-*), or any country's CIUS-specific
// ones (BR-DE-* etc.) beyond the one BT-10 rule XRechnung adds — there is no
// validator available in this environment to check against, so inventing
// extra structure would be unverifiable. Validate the output against the
// KoSIT validator (or an equivalent EN 16931 validator) before relying on it
// for real B2G submission.
func (d *Database) GenerateEInvoice(invoiceID string) ([]byte, error) {
	invoice, err := d.GetInvoice(invoiceID)
	if err != nil {
		return nil, fmt.Errorf("einvoice get_invoice: %w", err)
	}
	lineItems, err := d.GetInvoiceLineItems(invoiceID)
	if err != nil {
		return nil, fmt.Errorf("einvoice get_line_items: %w", err)
	}
	client, err := d.GetClient(invoice.ClientID)
	if err != nil {
		return nil, fmt.Errorf("einvoice get_client: %w", err)
	}
	org, err := d.GetOrganization(invoice.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("einvoice get_organization: %w", err)
	}

	taxRates := map[string]*TaxRate{}
	for _, item := range lineItems {
		if item.TaxRate == nil || *item.TaxRate == "" {
			continue
		}
		if _, ok := taxRates[*item.TaxRate]; ok {
			continue
		}
		rate, err := d.GetTaxRate(*item.TaxRate)
		if err != nil {
			return nil, fmt.Errorf("einvoice get_tax_rate: %w", err)
		}
		taxRates[*item.TaxRate] = rate
	}

	profile := resolveEInvoiceProfile(client, org)

	if err := validateEInvoiceCompleteness(profile, invoice, lineItems, client, org); err != nil {
		return nil, err
	}

	ubl, err := buildUBLInvoice(profile, invoice, lineItems, client, org, taxRates)
	if err != nil {
		return nil, err
	}

	out, err := xml.MarshalIndent(ubl, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("einvoice marshal: %w", err)
	}
	return append([]byte(xml.Header), out...), nil
}

// validateEInvoiceCompleteness checks the BT fields that are mandatory for
// the resolved profile and have a real column to read from, and reports
// every missing one at once (rather than failing on the first) so a user
// can fix them in one pass instead of one 409 at a time.
func validateEInvoiceCompleteness(profile eInvoiceProfile, invoice *Invoice, lineItems []InvoiceLineItem, client *Client, org *Organization) error {
	var missing []string

	require := func(v *string, label string) {
		if v == nil || strings.TrimSpace(*v) == "" {
			missing = append(missing, label)
		}
	}

	if profile.RequireBuyerReference {
		require(invoice.BuyerReference, "invoice buyer reference")
	}
	require(org.Name, "organization name")
	require(org.Street, "organization street")
	require(org.PostalCode, "organization postal code")
	require(org.City, "organization city")
	require(org.CountryCode, "organization country code")
	require(client.Name, "client name")
	require(client.Street, "client street")
	require(client.PostalCode, "client postal code")
	require(client.City, "client city")
	require(client.CountryCode, "client country code")

	if len(lineItems) == 0 {
		missing = append(missing, "at least one line item")
	}
	for i, item := range lineItems {
		if item.Description == nil || strings.TrimSpace(*item.Description) == "" {
			missing = append(missing, fmt.Sprintf("line item %d description", i+1))
		}
		if item.TaxRate == nil || *item.TaxRate == "" {
			missing = append(missing, fmt.Sprintf("line item %d tax rate", i+1))
		}
	}

	if len(missing) > 0 {
		return newValidationError("cannot generate e-invoice, missing: %s", strings.Join(missing, "; "))
	}
	return nil
}

// Commercial invoice, UNCL1001 code 380.
const invoiceTypeCodeCommercial = "380"

func buildUBLInvoice(profile eInvoiceProfile, invoice *Invoice, lineItems []InvoiceLineItem, client *Client, org *Organization, taxRates map[string]*TaxRate) (*ublInvoice, error) {
	currency := invoice.Currency

	lines := make([]ublInvoiceLine, len(lineItems))
	taxableByRate := map[string]*big.Rat{}
	var taxRateOrder []string
	subtotal := new(big.Rat)

	for i, item := range lineItems {
		qty, err := floatToRat(item.Quantity)
		if err != nil {
			return nil, fmt.Errorf("einvoice line %d quantity: %w", i+1, err)
		}
		price := new(big.Rat).SetInt64(item.UnitPrice)
		priceUnits := new(big.Rat).Quo(price, hundred)
		lineNet := new(big.Rat).Mul(qty, priceUnits)
		subtotal.Add(subtotal, lineNet)

		rate := taxRates[*item.TaxRate]
		if _, ok := taxableByRate[*item.TaxRate]; !ok {
			taxableByRate[*item.TaxRate] = new(big.Rat)
			taxRateOrder = append(taxRateOrder, *item.TaxRate)
		}
		taxableByRate[*item.TaxRate].Add(taxableByRate[*item.TaxRate], lineNet)

		lines[i] = ublInvoiceLine{
			ID:                  strconv.Itoa(i + 1),
			InvoicedQuantity:    ublQuantity{UnitCode: "C62", Value: formatFloat(item.Quantity)},
			LineExtensionAmount: ublAmount{CurrencyID: currency, Value: formatRat(lineNet)},
			Item:                ublItem{Name: strings.TrimSpace(*item.Description), ClassifiedTaxCategory: taxCategoryFor(rate)},
			Price:               ublPrice{PriceAmount: ublAmount{CurrencyID: currency, Value: formatCents(item.UnitPrice)}},
		}
	}

	taxTotalAmount := new(big.Rat)
	subtotals := make([]ublTaxSubtotal, 0, len(taxRateOrder))
	for _, taxRateID := range taxRateOrder {
		rate := taxRates[taxRateID]
		taxable := taxableByRate[taxRateID]
		pct, err := floatToRat(rate.Percentage)
		if err != nil {
			return nil, fmt.Errorf("einvoice tax rate %q percentage: %w", taxRateID, err)
		}
		tax := new(big.Rat).Mul(taxable, pct)
		tax.Quo(tax, hundred)
		tax = roundHalfUp(tax, 2)
		taxTotalAmount.Add(taxTotalAmount, tax)

		cat := taxCategoryFor(rate)
		if rate.ExemptionReason != nil && *rate.ExemptionReason != "" {
			cat.TaxExemptionReason = *rate.ExemptionReason
		}

		subtotals = append(subtotals, ublTaxSubtotal{
			TaxableAmount: ublAmount{CurrencyID: currency, Value: formatRat(taxable)},
			TaxAmount:     ublAmount{CurrencyID: currency, Value: formatRat(tax)},
			TaxCategory:   cat,
		})
	}

	total := new(big.Rat).Add(subtotal, taxTotalAmount)

	inv := &ublInvoice{
		Xmlns:                "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2",
		XmlnsCac:             "urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2",
		XmlnsCbc:             "urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2",
		CustomizationID:      profile.CustomizationID,
		ID:                   invoice.Number,
		IssueDate:            formatMillis(invoice.Date),
		InvoiceTypeCode:      invoiceTypeCodeCommercial,
		DocumentCurrencyCode: currency,
		AccountingSupplierParty: ublPartyWrapper{Party: buildParty(
			*org.Name, org.Street, org.HouseNumber, *org.PostalCode, *org.City, *org.CountryCode,
			org.Vatin, org.Phone, org.Email,
		)},
		AccountingCustomerParty: ublPartyWrapper{Party: buildParty(
			*client.Name, client.Street, client.HouseNumber, *client.PostalCode, *client.City, *client.CountryCode,
			client.Vatin, client.Phone, nil,
		)},
		TaxTotal: ublTaxTotal{
			TaxAmount:   ublAmount{CurrencyID: currency, Value: formatRat(taxTotalAmount)},
			TaxSubtotal: subtotals,
		},
		LegalMonetaryTotal: ublMonetaryTotal{
			LineExtensionAmount: ublAmount{CurrencyID: currency, Value: formatRat(subtotal)},
			TaxExclusiveAmount:  ublAmount{CurrencyID: currency, Value: formatRat(subtotal)},
			TaxInclusiveAmount:  ublAmount{CurrencyID: currency, Value: formatRat(total)},
			PayableAmount:       ublAmount{CurrencyID: currency, Value: formatRat(total)},
		},
		InvoiceLine: lines,
	}

	if invoice.BuyerReference != nil && *invoice.BuyerReference != "" {
		inv.BuyerReference = *invoice.BuyerReference
	}
	if invoice.DueDate != nil {
		inv.DueDate = formatMillis(*invoice.DueDate)
	}
	if invoice.PaymentTerms != nil && *invoice.PaymentTerms != "" {
		inv.PaymentTerms = &ublPaymentTerms{Note: *invoice.PaymentTerms}
	}
	if org.IBAN != nil && *org.IBAN != "" {
		account := ublFinancialAccount{ID: *org.IBAN}
		if org.BIC != nil && *org.BIC != "" {
			account.FinancialInstitutionBranch = &ublFinancialInstitutionBranch{ID: *org.BIC}
		}
		inv.PaymentMeans = &ublPaymentMeans{
			PaymentMeansCode:      "58",
			PayeeFinancialAccount: account,
		}
	}

	return inv, nil
}

func taxCategoryFor(rate *TaxRate) ublTaxCategory {
	return ublTaxCategory{
		ID:        rate.CategoryCode,
		Percent:   formatFloat(rate.Percentage),
		TaxScheme: ublTaxScheme{ID: "VAT"},
	}
}

// normalizeCountryCode upper-cases and trims a country code before it's used
// for profile lookup or emitted as BT-40/BT-55 (ISO 3166-1 alpha-2, which is
// uppercase by spec). Nothing enforces casing when the column is written —
// e.g. a direct API call bypassing the form's uppercase-on-change — so both
// read sites normalize defensively rather than trusting stored casing.
func normalizeCountryCode(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// buildParty maps seller/buyer fields shared by db.Organization and db.Client
// onto a UBL Party. House number, when present, is folded into StreetName —
// UBL's PostalAddress has no dedicated building-number element.
func buildParty(name string, street, houseNumber *string, postalCode, city, countryCode string, vatin, phone, email *string) ublParty {
	streetName := ""
	if street != nil {
		streetName = strings.TrimSpace(*street)
	}
	if houseNumber != nil && strings.TrimSpace(*houseNumber) != "" {
		streetName = strings.TrimSpace(streetName + " " + *houseNumber)
	}

	party := ublParty{
		PostalAddress: ublAddress{
			StreetName: streetName,
			CityName:   city,
			PostalZone: postalCode,
			Country:    ublCountry{IdentificationCode: normalizeCountryCode(countryCode)},
		},
		PartyLegalEntity: ublPartyLegalEntity{RegistrationName: name},
	}

	if vatin != nil && *vatin != "" {
		party.PartyTaxScheme = &ublPartyTaxScheme{
			CompanyID: *vatin,
			TaxScheme: ublTaxScheme{ID: "VAT"},
		}
	}

	if (phone != nil && *phone != "") || (email != nil && *email != "") {
		contact := &ublContact{}
		if phone != nil {
			contact.Telephone = *phone
		}
		if email != nil {
			contact.ElectronicMail = *email
		}
		party.Contact = contact
	}

	return party
}

// formatMillis renders a stored date as a calendar date (BT-2/BT-9 want a
// date, not an instant). Invoice/due dates are written by the frontend as
// local midnight (dayjs().valueOf()) with no timezone recorded, and this
// runs server-side where the local zone is typically UTC (e.g. in Docker)
// and essentially never matches the browser's. Flooring in UTC would read
// back the wrong calendar day for any positive UTC offset — local midnight
// in Berlin (UTC+1) is 23:00 UTC the day before. Rounding to the nearest UTC
// day instead recovers the intended date for any zone within ±12h of UTC,
// which covers every real timezone this app's locales (en/de/fr) target.
func formatMillis(ms int64) string {
	return time.UnixMilli(ms).UTC().Add(12 * time.Hour).Format("2006-01-02")
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func formatRat(r *big.Rat) string {
	return r.FloatString(2)
}

// formatCents renders a cents amount (int64) as a fixed 2-decimal string,
// e.g. 12345 -> "123.45".
func formatCents(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	s := fmt.Sprintf("%d.%02d", cents/100, cents%100)
	if neg {
		s = "-" + s
	}
	return s
}

// --- UBL 2.1 Invoice structs ---
//
// Field order matches the UBL schema's element sequence — encoding/xml
// emits fields in declaration order, and UBL is sequence-based (not a free
// bag of elements), so reordering these fields changes the schema this
// produces. Element names use literal "cac:"/"cbc:" prefixes rather than
// Go's namespace machinery, matching the prefixes declared on the root
// element's xmlns:cac/xmlns:cbc attributes.

type ublAmount struct {
	CurrencyID string `xml:"currencyID,attr"`
	Value      string `xml:",chardata"`
}

type ublQuantity struct {
	UnitCode string `xml:"unitCode,attr"`
	Value    string `xml:",chardata"`
}

type ublCountry struct {
	IdentificationCode string `xml:"cbc:IdentificationCode"`
}

type ublAddress struct {
	StreetName string     `xml:"cbc:StreetName"`
	CityName   string     `xml:"cbc:CityName"`
	PostalZone string     `xml:"cbc:PostalZone"`
	Country    ublCountry `xml:"cac:Country"`
}

type ublTaxScheme struct {
	ID string `xml:"cbc:ID"`
}

type ublPartyTaxScheme struct {
	CompanyID string       `xml:"cbc:CompanyID"`
	TaxScheme ublTaxScheme `xml:"cac:TaxScheme"`
}

type ublPartyLegalEntity struct {
	RegistrationName string `xml:"cbc:RegistrationName"`
}

type ublContact struct {
	Telephone      string `xml:"cbc:Telephone,omitempty"`
	ElectronicMail string `xml:"cbc:ElectronicMail,omitempty"`
}

type ublParty struct {
	PostalAddress    ublAddress          `xml:"cac:PostalAddress"`
	PartyTaxScheme   *ublPartyTaxScheme  `xml:"cac:PartyTaxScheme,omitempty"`
	PartyLegalEntity ublPartyLegalEntity `xml:"cac:PartyLegalEntity"`
	Contact          *ublContact         `xml:"cac:Contact,omitempty"`
}

type ublPartyWrapper struct {
	Party ublParty `xml:"cac:Party"`
}

type ublFinancialInstitutionBranch struct {
	ID string `xml:"cbc:ID"`
}

type ublFinancialAccount struct {
	ID                         string                         `xml:"cbc:ID"`
	FinancialInstitutionBranch *ublFinancialInstitutionBranch `xml:"cac:FinancialInstitutionBranch,omitempty"`
}

type ublPaymentMeans struct {
	PaymentMeansCode      string              `xml:"cbc:PaymentMeansCode"`
	PayeeFinancialAccount ublFinancialAccount `xml:"cac:PayeeFinancialAccount"`
}

type ublPaymentTerms struct {
	Note string `xml:"cbc:Note"`
}

type ublTaxCategory struct {
	ID                 string       `xml:"cbc:ID"`
	Percent            string       `xml:"cbc:Percent"`
	TaxExemptionReason string       `xml:"cbc:TaxExemptionReason,omitempty"`
	TaxScheme          ublTaxScheme `xml:"cac:TaxScheme"`
}

type ublTaxSubtotal struct {
	TaxableAmount ublAmount      `xml:"cbc:TaxableAmount"`
	TaxAmount     ublAmount      `xml:"cbc:TaxAmount"`
	TaxCategory   ublTaxCategory `xml:"cac:TaxCategory"`
}

type ublTaxTotal struct {
	TaxAmount   ublAmount        `xml:"cbc:TaxAmount"`
	TaxSubtotal []ublTaxSubtotal `xml:"cac:TaxSubtotal"`
}

type ublMonetaryTotal struct {
	LineExtensionAmount ublAmount `xml:"cbc:LineExtensionAmount"`
	TaxExclusiveAmount  ublAmount `xml:"cbc:TaxExclusiveAmount"`
	TaxInclusiveAmount  ublAmount `xml:"cbc:TaxInclusiveAmount"`
	PayableAmount       ublAmount `xml:"cbc:PayableAmount"`
}

type ublItem struct {
	Name                  string         `xml:"cbc:Name"`
	ClassifiedTaxCategory ublTaxCategory `xml:"cac:ClassifiedTaxCategory"`
}

type ublPrice struct {
	PriceAmount ublAmount `xml:"cbc:PriceAmount"`
}

type ublInvoiceLine struct {
	ID                  string      `xml:"cbc:ID"`
	InvoicedQuantity    ublQuantity `xml:"cbc:InvoicedQuantity"`
	LineExtensionAmount ublAmount   `xml:"cbc:LineExtensionAmount"`
	Item                ublItem     `xml:"cac:Item"`
	Price               ublPrice    `xml:"cac:Price"`
}

type ublInvoice struct {
	XMLName xml.Name `xml:"Invoice"`

	Xmlns    string `xml:"xmlns,attr"`
	XmlnsCac string `xml:"xmlns:cac,attr"`
	XmlnsCbc string `xml:"xmlns:cbc,attr"`

	CustomizationID      string `xml:"cbc:CustomizationID"`
	ID                   string `xml:"cbc:ID"`
	IssueDate            string `xml:"cbc:IssueDate"`
	DueDate              string `xml:"cbc:DueDate,omitempty"`
	InvoiceTypeCode      string `xml:"cbc:InvoiceTypeCode"`
	DocumentCurrencyCode string `xml:"cbc:DocumentCurrencyCode"`
	BuyerReference       string `xml:"cbc:BuyerReference,omitempty"`

	AccountingSupplierParty ublPartyWrapper `xml:"cac:AccountingSupplierParty"`
	AccountingCustomerParty ublPartyWrapper `xml:"cac:AccountingCustomerParty"`

	PaymentMeans *ublPaymentMeans `xml:"cac:PaymentMeans,omitempty"`
	PaymentTerms *ublPaymentTerms `xml:"cac:PaymentTerms,omitempty"`

	TaxTotal           ublTaxTotal      `xml:"cac:TaxTotal"`
	LegalMonetaryTotal ublMonetaryTotal `xml:"cac:LegalMonetaryTotal"`

	InvoiceLine []ublInvoiceLine `xml:"cac:InvoiceLine"`
}
