package db

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func seedGermanInvoice(t *testing.T, d *Database) *Invoice {
	t.Helper()

	org, err := d.CreateOrganization(CreateOrganizationRequest{
		ID:          "org-xr",
		Name:        ptr("Muster GmbH"),
		Vatin:       ptr("DE123456789"),
		Phone:       ptr("+49 30 1234567"),
		Email:       ptr("billing@muster.example"),
		IBAN:        ptr("DE89370400440532013000"),
		BIC:         ptr("COBADEFFXXX"),
		Street:      ptr("Musterstraße"),
		HouseNumber: ptr("12"),
		PostalCode:  ptr("10115"),
		City:        ptr("Berlin"),
		CountryCode: ptr("DE"),
	})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	client, err := d.CreateClient(CreateClientRequest{
		ID:             "client-xr",
		OrganizationID: org.ID,
		Name:           ptr("Kunde AG"),
		Vatin:          ptr("DE987654321"),
		Street:         ptr("Kundenweg"),
		HouseNumber:    ptr("3"),
		PostalCode:     ptr("80331"),
		City:           ptr("München"),
		CountryCode:    ptr("DE"),
	})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	taxRate, err := d.CreateTaxRate(CreateTaxRateRequest{
		ID: "tax-xr", OrganizationID: org.ID, Name: "VAT 19%", Percentage: 19, CategoryCode: "S",
	})
	if err != nil {
		t.Fatalf("CreateTaxRate: %v", err)
	}

	invoice, err := d.CreateInvoice(CreateInvoiceRequest{
		ID:             "inv-xr",
		OrganizationID: org.ID,
		Number:         "INV-2026-001",
		ClientID:       client.ID,
		// Berlin (UTC+1) local midnight for 2025-01-15, not UTC midnight —
		// pins the timezone-rounding behavior in formatMillis (flooring in
		// UTC would wrongly read this back as 2025-01-14).
		Date:           1736895600000,
		Currency:       "EUR",
		BuyerReference: ptr("04011000-1234512345-06"),
		PaymentTerms:   ptr("Payable within 14 days"),
		SubTotal:       10000,
		TaxTotal:       1900,
		Total:          11900,
		LineItems: []CreateInvoiceLineItemRequest{
			{Description: ptr("Consulting services"), Quantity: 2, UnitPrice: 5000, TaxRate: &taxRate.ID},
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	return invoice
}

// This is a golden-file regression test, not an EN 16931 conformance check —
// there is no validator available in this environment (see GenerateEInvoice's
// doc comment). Validate the fixture output externally (e.g. the KoSIT
// validator) before treating this as proof the XML is accepted by a real
// recipient.
//
// A German buyer resolves to the XRechnung 3.0 profile (see
// resolveEInvoiceProfile), which is why this fixture requires a buyer
// reference.
func TestGenerateEInvoiceGoldenGermany(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	invoice := seedGermanInvoice(t, d)

	got, err := d.GenerateEInvoice(invoice.ID)
	if err != nil {
		t.Fatalf("GenerateEInvoice: %v", err)
	}

	want := `<?xml version="1.0" encoding="UTF-8"?>
<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
  <cbc:CustomizationID>urn:cen.eu:en16931:2017#compliant#urn:xoev-de:kosit:standard:xrechnung_3.0</cbc:CustomizationID>
  <cbc:ID>INV-2026-001</cbc:ID>
  <cbc:IssueDate>2025-01-15</cbc:IssueDate>
  <cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>
  <cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>
  <cbc:BuyerReference>04011000-1234512345-06</cbc:BuyerReference>
  <cac:AccountingSupplierParty>
    <cac:Party>
      <cac:PostalAddress>
        <cbc:StreetName>Musterstraße 12</cbc:StreetName>
        <cbc:CityName>Berlin</cbc:CityName>
        <cbc:PostalZone>10115</cbc:PostalZone>
        <cac:Country>
          <cbc:IdentificationCode>DE</cbc:IdentificationCode>
        </cac:Country>
      </cac:PostalAddress>
      <cac:PartyTaxScheme>
        <cbc:CompanyID>DE123456789</cbc:CompanyID>
        <cac:TaxScheme>
          <cbc:ID>VAT</cbc:ID>
        </cac:TaxScheme>
      </cac:PartyTaxScheme>
      <cac:PartyLegalEntity>
        <cbc:RegistrationName>Muster GmbH</cbc:RegistrationName>
      </cac:PartyLegalEntity>
      <cac:Contact>
        <cbc:Telephone>+49 30 1234567</cbc:Telephone>
        <cbc:ElectronicMail>billing@muster.example</cbc:ElectronicMail>
      </cac:Contact>
    </cac:Party>
  </cac:AccountingSupplierParty>
  <cac:AccountingCustomerParty>
    <cac:Party>
      <cac:PostalAddress>
        <cbc:StreetName>Kundenweg 3</cbc:StreetName>
        <cbc:CityName>München</cbc:CityName>
        <cbc:PostalZone>80331</cbc:PostalZone>
        <cac:Country>
          <cbc:IdentificationCode>DE</cbc:IdentificationCode>
        </cac:Country>
      </cac:PostalAddress>
      <cac:PartyTaxScheme>
        <cbc:CompanyID>DE987654321</cbc:CompanyID>
        <cac:TaxScheme>
          <cbc:ID>VAT</cbc:ID>
        </cac:TaxScheme>
      </cac:PartyTaxScheme>
      <cac:PartyLegalEntity>
        <cbc:RegistrationName>Kunde AG</cbc:RegistrationName>
      </cac:PartyLegalEntity>
    </cac:Party>
  </cac:AccountingCustomerParty>
  <cac:PaymentMeans>
    <cbc:PaymentMeansCode>58</cbc:PaymentMeansCode>
    <cac:PayeeFinancialAccount>
      <cbc:ID>DE89370400440532013000</cbc:ID>
      <cac:FinancialInstitutionBranch>
        <cbc:ID>COBADEFFXXX</cbc:ID>
      </cac:FinancialInstitutionBranch>
    </cac:PayeeFinancialAccount>
  </cac:PaymentMeans>
  <cac:PaymentTerms>
    <cbc:Note>Payable within 14 days</cbc:Note>
  </cac:PaymentTerms>
  <cac:TaxTotal>
    <cbc:TaxAmount currencyID="EUR">19.00</cbc:TaxAmount>
    <cac:TaxSubtotal>
      <cbc:TaxableAmount currencyID="EUR">100.00</cbc:TaxableAmount>
      <cbc:TaxAmount currencyID="EUR">19.00</cbc:TaxAmount>
      <cac:TaxCategory>
        <cbc:ID>S</cbc:ID>
        <cbc:Percent>19</cbc:Percent>
        <cac:TaxScheme>
          <cbc:ID>VAT</cbc:ID>
        </cac:TaxScheme>
      </cac:TaxCategory>
    </cac:TaxSubtotal>
  </cac:TaxTotal>
  <cac:LegalMonetaryTotal>
    <cbc:LineExtensionAmount currencyID="EUR">100.00</cbc:LineExtensionAmount>
    <cbc:TaxExclusiveAmount currencyID="EUR">100.00</cbc:TaxExclusiveAmount>
    <cbc:TaxInclusiveAmount currencyID="EUR">119.00</cbc:TaxInclusiveAmount>
    <cbc:PayableAmount currencyID="EUR">119.00</cbc:PayableAmount>
  </cac:LegalMonetaryTotal>
  <cac:InvoiceLine>
    <cbc:ID>1</cbc:ID>
    <cbc:InvoicedQuantity unitCode="C62">2</cbc:InvoicedQuantity>
    <cbc:LineExtensionAmount currencyID="EUR">100.00</cbc:LineExtensionAmount>
    <cac:Item>
      <cbc:Name>Consulting services</cbc:Name>
      <cac:ClassifiedTaxCategory>
        <cbc:ID>S</cbc:ID>
        <cbc:Percent>19</cbc:Percent>
        <cac:TaxScheme>
          <cbc:ID>VAT</cbc:ID>
        </cac:TaxScheme>
      </cac:ClassifiedTaxCategory>
    </cac:Item>
    <cac:Price>
      <cbc:PriceAmount currencyID="EUR">50.00</cbc:PriceAmount>
    </cac:Price>
  </cac:InvoiceLine>
</Invoice>`

	if strings.TrimSpace(string(got)) != want {
		t.Fatalf("GenerateEInvoice output mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// A French buyer has no country-specific profile, so this resolves to the
// generic EN 16931 core profile — even though the seller is German. This
// pins two things at once: profile resolution keys off the *buyer's*
// country (not the seller's), and the generic profile doesn't require (or
// emit) a buyer reference the way XRechnung does.
func TestGenerateEInvoiceGoldenGenericProfile(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)

	org, err := d.CreateOrganization(CreateOrganizationRequest{
		ID: "org-generic", Name: ptr("Muster GmbH"),
		Street: ptr("Musterstraße"), HouseNumber: ptr("12"),
		PostalCode: ptr("10115"), City: ptr("Berlin"), CountryCode: ptr("DE"),
	})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	client, err := d.CreateClient(CreateClientRequest{
		ID: "client-fr", OrganizationID: org.ID, Name: ptr("Client Français"),
		Street: ptr("Rue de Client"), HouseNumber: ptr("5"),
		PostalCode: ptr("75001"), City: ptr("Paris"), CountryCode: ptr("FR"),
	})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	taxRate, err := d.CreateTaxRate(CreateTaxRateRequest{
		ID: "tax-generic", OrganizationID: org.ID, Name: "VAT 20%", Percentage: 20, CategoryCode: "S",
	})
	if err != nil {
		t.Fatalf("CreateTaxRate: %v", err)
	}

	invoice, err := d.CreateInvoice(CreateInvoiceRequest{
		ID: "inv-generic", OrganizationID: org.ID, Number: "INV-2026-002", ClientID: client.ID,
		Date: 1736895600000, Currency: "EUR",
		SubTotal: 10000, TaxTotal: 2000, Total: 12000,
		LineItems: []CreateInvoiceLineItemRequest{
			{Description: ptr("Consulting services"), Quantity: 1, UnitPrice: 10000, TaxRate: &taxRate.ID},
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}

	got, err := d.GenerateEInvoice(invoice.ID)
	if err != nil {
		t.Fatalf("GenerateEInvoice: %v", err)
	}

	gotStr := string(got)
	if !strings.Contains(gotStr, "<cbc:CustomizationID>urn:cen.eu:en16931:2017</cbc:CustomizationID>") {
		t.Fatalf("expected the generic EN 16931 CustomizationID, got:\n%s", gotStr)
	}
	if strings.Contains(gotStr, "xrechnung") {
		t.Fatalf("did not expect the XRechnung CustomizationID for a French buyer, got:\n%s", gotStr)
	}
	if strings.Contains(gotStr, "cbc:BuyerReference") {
		t.Fatalf("did not expect a BuyerReference element when the profile doesn't require one, got:\n%s", gotStr)
	}
}

// A lowercase stored country code (e.g. written by a direct API call that
// bypasses the form's uppercase-on-change) must still resolve to the DE
// profile and emit an uppercase ISO 3166-1 code — normalizeCountryCode is
// what makes that true at both read sites (resolveEInvoiceProfile and
// buildParty).
func TestGenerateEInvoiceNormalizesCountryCodeCasing(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	invoice := seedGermanInvoice(t, d)

	// UpdateClientRequest overwrites every field verbatim (no COALESCE), so
	// the rest of the client's fields must be resupplied here too.
	if _, err := d.UpdateClient("client-xr", UpdateClientRequest{
		Name: ptr("Kunde AG"), Vatin: ptr("DE987654321"),
		Street: ptr("Kundenweg"), HouseNumber: ptr("3"),
		PostalCode: ptr("80331"), City: ptr("München"),
		CountryCode: ptr("de"),
	}); err != nil {
		t.Fatalf("UpdateClient: %v", err)
	}

	got, err := d.GenerateEInvoice(invoice.ID)
	if err != nil {
		t.Fatalf("GenerateEInvoice: %v", err)
	}

	gotStr := string(got)
	if !strings.Contains(gotStr, "xrechnung_3.0") {
		t.Fatalf("expected a lowercase client country code to still resolve to the XRechnung profile, got:\n%s", gotStr)
	}
	if strings.Contains(gotStr, "<cbc:IdentificationCode>de</cbc:IdentificationCode>") {
		t.Fatalf("expected the emitted country code to be uppercase, got:\n%s", gotStr)
	}
	if !strings.Contains(gotStr, "<cbc:IdentificationCode>DE</cbc:IdentificationCode>") {
		t.Fatalf("expected an uppercase DE country code, got:\n%s", gotStr)
	}
}

func TestGenerateEInvoiceRejectsIncompleteSeller(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	invoice := seedGermanInvoice(t, d)

	// Clear a mandatory seller field directly via the DB layer's own update
	// path (empty string, not nil, since UpdateOrganization COALESCEs nil).
	if _, err := d.UpdateOrganization("org-xr", UpdateOrganizationRequest{CountryCode: ptr("")}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}

	_, err := d.GenerateEInvoice(invoice.ID)
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError, got %T: %v", err, err)
	}
	if !strings.Contains(verr.Error(), "organization country code") {
		t.Fatalf("expected error to mention the missing field, got %q", verr.Error())
	}
}

func TestGenerateEInvoiceRejectsMissingLineTaxRate(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	invoice := seedGermanInvoice(t, d)

	if _, err := d.UpdateInvoice(invoice.ID, UpdateInvoiceRequest{
		LineItems: &[]CreateInvoiceLineItemRequest{
			{Description: ptr("Consulting services"), Quantity: 2, UnitPrice: 5000},
		},
		SubTotal: ptr(int64(10000)),
		TaxTotal: ptr(int64(0)),
		Total:    ptr(int64(10000)),
	}); err != nil {
		t.Fatalf("UpdateInvoice: %v", err)
	}

	_, err := d.GenerateEInvoice(invoice.ID)
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError, got %T: %v", err, err)
	}
	if !strings.Contains(verr.Error(), "line item 1 tax rate") {
		t.Fatalf("expected error to mention the missing tax rate, got %q", verr.Error())
	}
}

// TestGenerateEInvoiceCarriesInvoiceDiscount covers audit F142: an invoice
// discount must reach the XML as document-level allowances per VAT category,
// with every monetary total net of it, so PayableAmount equals the invoice's
// own total rather than the undiscounted one.
func TestGenerateEInvoiceCarriesInvoiceDiscount(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	seeded := seedGermanInvoice(t, d)
	reduced, err := d.CreateTaxRate(CreateTaxRateRequest{
		ID: "tax-xr-7", OrganizationID: seeded.OrganizationID, Name: "VAT 7%", Percentage: 7, CategoryCode: "S",
	})
	if err != nil {
		t.Fatalf("CreateTaxRate: %v", err)
	}
	standard := "tax-xr"

	// 19% group 30.00 and 7% group 10.00 (subtotal 40.00), discount 8.00:
	// allowances 6.00 / 2.00, taxable 24.00 / 8.00, tax 4.56 / 0.56.
	invoice, err := d.CreateInvoice(CreateInvoiceRequest{
		ID: "inv-xr-discount", OrganizationID: seeded.OrganizationID, Number: "INV-2026-002",
		ClientID: seeded.ClientID, Date: 1736895600000, Currency: "EUR",
		BuyerReference: ptr("04011000-1234512345-06"),
		SubTotal:       4000, DiscountAmount: 800, TaxTotal: 512, Total: 3712,
		LineItems: []CreateInvoiceLineItemRequest{
			{Description: ptr("Consulting"), Quantity: 3, UnitPrice: 1000, TaxRate: &standard},
			{Description: ptr("Books"), Quantity: 1, UnitPrice: 1000, TaxRate: &reduced.ID},
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}

	raw, err := d.GenerateEInvoice(invoice.ID)
	if err != nil {
		t.Fatalf("GenerateEInvoice: %v", err)
	}
	xml := string(raw)
	for _, want := range []string{
		`<cbc:LineExtensionAmount currencyID="EUR">40.00</cbc:LineExtensionAmount>`,
		`<cbc:TaxExclusiveAmount currencyID="EUR">32.00</cbc:TaxExclusiveAmount>`,
		`<cbc:TaxInclusiveAmount currencyID="EUR">37.12</cbc:TaxInclusiveAmount>`,
		`<cbc:AllowanceTotalAmount currencyID="EUR">8.00</cbc:AllowanceTotalAmount>`,
		`<cbc:PayableAmount currencyID="EUR">37.12</cbc:PayableAmount>`,
		`<cbc:TaxAmount currencyID="EUR">5.12</cbc:TaxAmount>`,
		`<cbc:TaxableAmount currencyID="EUR">24.00</cbc:TaxableAmount>`,
		`<cbc:TaxableAmount currencyID="EUR">8.00</cbc:TaxableAmount>`,
		`<cbc:Amount currencyID="EUR">6.00</cbc:Amount>`,
		`<cbc:Amount currencyID="EUR">2.00</cbc:Amount>`,
		`<cbc:ChargeIndicator>false</cbc:ChargeIndicator>`,
		`<cbc:AllowanceChargeReasonCode>95</cbc:AllowanceChargeReasonCode>`,
	} {
		if !strings.Contains(xml, want) {
			t.Errorf("e-invoice missing %s", want)
		}
	}
	if strings.Count(xml, "<cac:AllowanceCharge>") != 2 {
		t.Errorf("want one allowance per VAT category (2), got %d", strings.Count(xml, "<cac:AllowanceCharge>"))
	}
	// UBL schema order: AllowanceCharge comes before TaxTotal.
	if strings.Index(xml, "<cac:AllowanceCharge>") > strings.Index(xml, "<cac:TaxTotal>") {
		t.Error("cac:AllowanceCharge must precede cac:TaxTotal")
	}
	if payable := fmt.Sprintf("%d.%02d", invoice.Total/100, invoice.Total%100); !strings.Contains(xml, ">"+payable+"</cbc:PayableAmount>") {
		t.Errorf("PayableAmount should equal the invoice total %s", payable)
	}
}

// TestGenerateEInvoiceDiscountSweep pins audit F152. The discount splits
// that actually round — two and four VAT categories, across many discount
// values — must still give a PayableAmount equal to the invoice's own total
// (no fiscal stamp here; the e-invoice doesn't emit one), and every
// allowance must be non-negative, within its category's line total, and sum
// to AllowanceTotalAmount. TestGenerateEInvoiceCarriesInvoiceDiscount's
// split divides exactly, which is how the original fix got through.
func TestGenerateEInvoiceDiscountSweep(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	seeded := seedGermanInvoice(t, d)
	rateIDs := map[float64]string{19: "tax-xr"}
	for _, pct := range []float64{7, 5, 16} {
		tr, err := d.CreateTaxRate(CreateTaxRateRequest{
			OrganizationID: seeded.OrganizationID, Name: fmt.Sprintf("VAT %v%%", pct), Percentage: pct, CategoryCode: "S",
		})
		if err != nil {
			t.Fatalf("CreateTaxRate: %v", err)
		}
		rateIDs[pct] = tr.ID
	}
	type group struct {
		cents int64
		pct   float64
	}
	shapes := [][]group{
		{{123456, 19}, {45678, 7}},
		{{3000, 19}, {3000, 7}, {3000, 5}, {1000, 16}},
	}
	amountRe := regexp.MustCompile(`<cac:AllowanceCharge>.*?<cbc:Amount currencyID="EUR">(-?[0-9.]+)</cbc:Amount>`)
	payableRe := regexp.MustCompile(`<cbc:PayableAmount currencyID="EUR">([0-9.]+)</cbc:PayableAmount>`)
	n := 0
	for si, groups := range shapes {
		var subTotal int64
		for _, g := range groups {
			subTotal += g.cents
		}
		for discount := int64(1); discount <= 2000; discount += 37 {
			// The invoice's own tax: each group's exact net, rounded once —
			// validateInvoiceTotals' rule.
			var taxTotal int64
			var items []CreateInvoiceLineItemRequest
			for _, g := range groups {
				net := new(big.Rat).Sub(big.NewRat(g.cents, 1), big.NewRat(discount*g.cents, subTotal))
				tax := new(big.Rat).Mul(net, big.NewRat(int64(g.pct), 100))
				taxTotal += roundHalfUp(tax, 0).Num().Int64()
				id := rateIDs[g.pct]
				items = append(items, CreateInvoiceLineItemRequest{
					Description: ptr("Item"), Quantity: 1, UnitPrice: float64(g.cents), TaxRate: &id,
				})
			}
			n++
			total := subTotal - discount + taxTotal
			inv, err := d.CreateInvoice(CreateInvoiceRequest{
				ID: fmt.Sprintf("inv-sweep-%d", n), OrganizationID: seeded.OrganizationID,
				Number: fmt.Sprintf("SWEEP-%d", n), ClientID: seeded.ClientID, Date: 1736895600000, Currency: "EUR",
				BuyerReference: ptr("04011000-1234512345-06"),
				SubTotal:       subTotal, DiscountAmount: discount, TaxTotal: taxTotal, Total: total, LineItems: items,
			})
			if err != nil {
				t.Fatalf("shape %d discount %d: CreateInvoice: %v", si, discount, err)
			}
			raw, err := d.GenerateEInvoice(inv.ID)
			if err != nil {
				t.Fatalf("shape %d discount %d: GenerateEInvoice: %v", si, discount, err)
			}
			xml := strings.ReplaceAll(string(raw), "\n", "")
			if m := payableRe.FindStringSubmatch(xml); m == nil || m[1] != formatCents(total) {
				t.Fatalf("shape %d discount %d: PayableAmount %v, want %s", si, discount, m, formatCents(total))
			}
			allowances := regexp.MustCompile(`<cac:AllowanceCharge>`).Split(xml, -1)[1:]
			if len(allowances) != len(groups) {
				t.Fatalf("shape %d discount %d: %d allowances, want %d", si, discount, len(allowances), len(groups))
			}
			var sum int64
			for gi, a := range allowances {
				m := amountRe.FindStringSubmatch("<cac:AllowanceCharge>" + a)
				if m == nil {
					t.Fatalf("shape %d discount %d: allowance %d has no amount", si, discount, gi)
				}
				cents, err := strconv.ParseFloat(m[1], 64)
				if err != nil {
					t.Fatal(err)
				}
				c := int64(math.Round(cents * 100))
				if c < 0 || c > groups[gi].cents {
					t.Fatalf("shape %d discount %d: allowance %d = %d cents, want 0..%d", si, discount, gi, c, groups[gi].cents)
				}
				sum += c
			}
			if sum != discount {
				t.Fatalf("shape %d discount %d: allowances sum to %d, want %d", si, discount, sum, discount)
			}
		}
	}
}
