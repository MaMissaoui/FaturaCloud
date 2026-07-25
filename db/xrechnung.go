package db

import (
	"encoding/xml"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"
)

// GenerateXRechnung renders an invoice as an EN 16931 / XRechnung-conformant
// UBL 2.1 Invoice document.
//
// This is deliberately minimal: it maps only the BT fields the schema has
// real columns for (see the 0034-0037 migrations) and does not attempt the
// full set of EN 16931 / XRechnung business rules (BR-*, BR-DE-*) — there is
// no validator available in this environment to check against, so inventing
// extra structure would be unverifiable. Validate the output against the
// KoSIT validator (or an equivalent EN 16931 validator) before relying on it
// for real B2G submission.
func (d *Database) GenerateXRechnung(invoiceID string) ([]byte, error) {
	invoice, err := d.GetInvoice(invoiceID)
	if err != nil {
		return nil, fmt.Errorf("xrechnung get_invoice: %w", err)
	}
	lineItems, err := d.GetInvoiceLineItems(invoiceID)
	if err != nil {
		return nil, fmt.Errorf("xrechnung get_line_items: %w", err)
	}
	client, err := d.GetClient(invoice.ClientID)
	if err != nil {
		return nil, fmt.Errorf("xrechnung get_client: %w", err)
	}
	org, err := d.GetOrganization(invoice.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("xrechnung get_organization: %w", err)
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
			return nil, fmt.Errorf("xrechnung get_tax_rate: %w", err)
		}
		taxRates[*item.TaxRate] = rate
	}

	if err := validateXRechnungCompleteness(invoice, lineItems, client, org); err != nil {
		return nil, err
	}

	ubl, err := buildUBLInvoice(invoice, lineItems, client, org, taxRates)
	if err != nil {
		return nil, err
	}

	out, err := xml.MarshalIndent(ubl, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("xrechnung marshal: %w", err)
	}
	return append([]byte(xml.Header), out...), nil
}

// validateXRechnungCompleteness checks the BT fields that are mandatory in
// EN 16931 and have a real column to read from, and reports every missing
// one at once (rather than failing on the first) so a user can fix them in
// one pass instead of one 409 at a time.
func validateXRechnungCompleteness(invoice *Invoice, lineItems []InvoiceLineItem, client *Client, org *Organization) error {
	var missing []string

	require := func(v *string, label string) {
		if v == nil || strings.TrimSpace(*v) == "" {
			missing = append(missing, label)
		}
	}

	require(invoice.BuyerReference, "invoice buyer reference")
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
		return newValidationError("cannot generate XRechnung, missing: %s", strings.Join(missing, "; "))
	}
	return nil
}

// UBL/CEN customization identifier for XRechnung 3.0 (BT-24), the only
// specification identifier this generator claims conformance to.
const xrechnungCustomizationID = "urn:cen.eu:en16931:2017#compliant#urn:xoev-de:kosit:standard:xrechnung_3.0"

// Commercial invoice, UNCL1001 code 380.
const invoiceTypeCodeCommercial = "380"

func buildUBLInvoice(invoice *Invoice, lineItems []InvoiceLineItem, client *Client, org *Organization, taxRates map[string]*TaxRate) (*ublInvoice, error) {
	currency := invoice.Currency

	lines := make([]ublInvoiceLine, len(lineItems))
	taxableByRate := map[string]*big.Rat{}
	var taxRateOrder []string
	subtotal := new(big.Rat)

	for i, item := range lineItems {
		qty, err := floatToRat(item.Quantity)
		if err != nil {
			return nil, fmt.Errorf("xrechnung line %d quantity: %w", i+1, err)
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
			return nil, fmt.Errorf("xrechnung tax rate %q percentage: %w", taxRateID, err)
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
		CustomizationID:      xrechnungCustomizationID,
		ID:                   invoice.Number,
		IssueDate:            formatMillis(invoice.Date),
		InvoiceTypeCode:      invoiceTypeCodeCommercial,
		DocumentCurrencyCode: currency,
		BuyerReference:       *invoice.BuyerReference,
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
			Country:    ublCountry{IdentificationCode: countryCode},
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
	BuyerReference       string `xml:"cbc:BuyerReference"`

	AccountingSupplierParty ublPartyWrapper `xml:"cac:AccountingSupplierParty"`
	AccountingCustomerParty ublPartyWrapper `xml:"cac:AccountingCustomerParty"`

	PaymentMeans *ublPaymentMeans `xml:"cac:PaymentMeans,omitempty"`
	PaymentTerms *ublPaymentTerms `xml:"cac:PaymentTerms,omitempty"`

	TaxTotal           ublTaxTotal      `xml:"cac:TaxTotal"`
	LegalMonetaryTotal ublMonetaryTotal `xml:"cac:LegalMonetaryTotal"`

	InvoiceLine []ublInvoiceLine `xml:"cac:InvoiceLine"`
}
