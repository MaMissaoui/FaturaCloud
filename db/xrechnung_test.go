package db

import (
	"errors"
	"strings"
	"testing"
)

func seedXRechnungInvoice(t *testing.T, d *Database) *Invoice {
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
// there is no validator available in this environment (see GenerateXRechnung's
// doc comment). Validate the fixture output externally (e.g. the KoSIT
// validator) before treating this as proof the XML is accepted by a real
// recipient.
func TestGenerateXRechnungGolden(t *testing.T) {
	d := newTestDB(t)
	invoice := seedXRechnungInvoice(t, d)

	got, err := d.GenerateXRechnung(invoice.ID)
	if err != nil {
		t.Fatalf("GenerateXRechnung: %v", err)
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
		t.Fatalf("GenerateXRechnung output mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestGenerateXRechnungRejectsIncompleteSeller(t *testing.T) {
	d := newTestDB(t)
	invoice := seedXRechnungInvoice(t, d)

	// Clear a mandatory seller field directly via the DB layer's own update
	// path (empty string, not nil, since UpdateOrganization COALESCEs nil).
	if _, err := d.UpdateOrganization("org-xr", UpdateOrganizationRequest{CountryCode: ptr("")}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}

	_, err := d.GenerateXRechnung(invoice.ID)
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError, got %T: %v", err, err)
	}
	if !strings.Contains(verr.Error(), "organization country code") {
		t.Fatalf("expected error to mention the missing field, got %q", verr.Error())
	}
}

func TestGenerateXRechnungRejectsMissingLineTaxRate(t *testing.T) {
	d := newTestDB(t)
	invoice := seedXRechnungInvoice(t, d)

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

	_, err := d.GenerateXRechnung(invoice.ID)
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError, got %T: %v", err, err)
	}
	if !strings.Contains(verr.Error(), "line item 1 tax rate") {
		t.Fatalf("expected error to mention the missing tax rate, got %q", verr.Error())
	}
}
