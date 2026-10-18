package db

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestClientAddressLine(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		client Client
		want   string
	}{
		{"street and number", Client{Street: ptr("Avenue Habib Bourguiba"), HouseNumber: ptr("12")}, "Avenue Habib Bourguiba 12"},
		{"street only", Client{Street: ptr("Rue de Marseille")}, "Rue de Marseille"},
		{"Cash Book address only", Client{Address: ptr("Cité El Khadra, bloc 4")}, "Cité El Khadra, bloc 4"},
		{"street wins over the free-text address", Client{Street: ptr("Rue de Rome"), Address: ptr("près de la poste")}, "Rue de Rome"},
		{"blank street falls back", Client{Street: ptr("  "), Address: ptr(" Sfax centre ")}, "Sfax centre"},
		{"nothing", Client{}, ""},
	}
	for _, c := range cases {
		if got := clientAddressLine(c.client); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// Every built-in layout with a client block prints the address line, so a
// client entered through the Clients form (street) and a Cash Book customer
// (free-text address) both get one. The Tunisia invoice used to print only
// the free-text address and the others only the street.
func TestClientLayoutsPrintTheAddressLine(t *testing.T) {
	t.Parallel()
	for _, layout := range []map[string][]byte{embeddedDefaultTemplates, embeddedTunisiaTemplates} {
		for _, docType := range []string{"invoice", "order", "delivery"} {
			f, err := excelize.OpenReader(bytes.NewReader(layout[docType]))
			if err != nil {
				t.Fatalf("open %s: %v", docType, err)
			}
			rows, _ := f.GetRows(f.GetSheetName(0))
			_ = f.Close()
			found := false
			for _, row := range rows {
				for _, cell := range row {
					if strings.Contains(cell, "{{client.street}}") || strings.Contains(cell, "{{client.address}}") {
						t.Errorf("%s template prints %q instead of {{client.addressLine}}", docType, cell)
					}
					found = found || strings.TrimSpace(cell) == "{{client.addressLine}}"
				}
			}
			if !found {
				t.Errorf("%s template has no {{client.addressLine}} line", docType)
			}
		}
	}

	for _, c := range []struct {
		client Client
		want   string
	}{
		{Client{Name: ptr("Form client"), Street: ptr("Avenue Habib Bourguiba"), HouseNumber: ptr("12")}, "Avenue Habib Bourguiba 12"},
		{Client{Name: ptr("Counter customer"), Address: ptr("Cité El Khadra, bloc 4")}, "Cité El Khadra, bloc 4"},
	} {
		client, want := c.client, c.want
		out, _, err := FillInvoiceTemplate(invoiceTunisiaTemplate, testInvoice(), nil, testOrg(), client, nil, nil, "")
		if err != nil {
			t.Fatalf("fill: %v", err)
		}
		f, err := excelize.OpenReader(bytes.NewReader(out))
		if err != nil {
			t.Fatalf("open filled: %v", err)
		}
		rows, _ := f.GetRows(f.GetSheetName(0))
		_ = f.Close()
		found := false
		for _, row := range rows {
			for _, cell := range row {
				found = found || strings.TrimSpace(cell) == want
			}
		}
		if !found {
			t.Errorf("Tunisia invoice for %s doesn't print %q", *client.Name, want)
		}
	}
}
