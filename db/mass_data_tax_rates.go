package db

import (
	"strconv"
	"strings"
)

// taxRatesMassDataSpec is tax rates' massDataSpec (db/mass_data.go).
// "Output/Input Tax Account" columns hold the account's Code, the same
// human-editable-reference convention accountsMassDataSpec uses for parent
// accounts, rather than a raw account id.
type taxRatesMassDataSpec struct{}

func (taxRatesMassDataSpec) Headers() []string {
	return []string{
		"ID", "Name", "Description", "Percentage", "Default",
		"Category Code", "Exemption Reason",
		"Output Tax Account Code", "Input Tax Account Code", "DATEV BU Key",
	}
}

func (taxRatesMassDataSpec) ExportRows(d *Database, organizationID string) ([][]string, error) {
	rates, err := d.GetTaxRates(organizationID)
	if err != nil {
		return nil, err
	}
	accounts, err := d.GetAccounts(organizationID)
	if err != nil {
		return nil, err
	}
	codeByID := make(map[string]string, len(accounts))
	for _, a := range accounts {
		codeByID[a.ID] = a.Code
	}
	rows := make([][]string, len(rates))
	for i, t := range rates {
		outputCode, inputCode := "", ""
		if t.OutputTaxAccountID != nil {
			outputCode = codeByID[*t.OutputTaxAccountID]
		}
		if t.InputTaxAccountID != nil {
			inputCode = codeByID[*t.InputTaxAccountID]
		}
		rows[i] = []string{
			t.ID, t.Name, strOrEmpty(t.Description), formatPercentage(t.Percentage), int64BoolCell(t.IsDefault),
			t.CategoryCode, strOrEmpty(t.ExemptionReason),
			outputCode, inputCode, strOrEmpty(t.DatevBuKey),
		}
	}
	return rows, nil
}

type taxRatesImportContext struct {
	accountCodeToID map[string]string
}

func (taxRatesMassDataSpec) NewImportContext(d *Database, organizationID string) (any, error) {
	accounts, err := d.GetAccounts(organizationID)
	if err != nil {
		return nil, err
	}
	ctx := &taxRatesImportContext{accountCodeToID: make(map[string]string, len(accounts))}
	for _, a := range accounts {
		ctx.accountCodeToID[a.Code] = a.ID
	}
	return ctx, nil
}

func (taxRatesMassDataSpec) resolveAccountCode(ctx *taxRatesImportContext, code string) (*string, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, nil
	}
	id, ok := ctx.accountCodeToID[code]
	if !ok {
		return nil, newValidationError("account code %q not found", code)
	}
	return &id, nil
}

func (s taxRatesMassDataSpec) ImportRow(d *Database, organizationID string, ctxAny any, cells []string) (action, identifier string, err error) {
	ctx := ctxAny.(*taxRatesImportContext)

	id := strings.TrimSpace(cells[0])
	name := strings.TrimSpace(cells[1])
	identifier = name
	if name == "" {
		return "", identifier, newValidationError("name is required")
	}
	percentage, err := cellFloat(cells[3])
	if err != nil {
		return "", identifier, newValidationError("percentage: %v", err)
	}
	isDefault := cellToInt64BoolPtr(cells[4])
	categoryCode := strings.TrimSpace(cells[5])

	outputAccountID, err := s.resolveAccountCode(ctx, cells[7])
	if err != nil {
		return "", identifier, err
	}
	inputAccountID, err := s.resolveAccountCode(ctx, cells[8])
	if err != nil {
		return "", identifier, err
	}

	fields := func(dst *CreateTaxRateRequest) {
		dst.Name = name
		dst.Description = cellStr(cells[2])
		dst.Percentage = percentage
		dst.IsDefault = isDefault
		dst.CategoryCode = categoryCode
		dst.ExemptionReason = cellStr(cells[6])
		dst.OutputTaxAccountID = outputAccountID
		dst.InputTaxAccountID = inputAccountID
		dst.DatevBuKey = cellStr(cells[9])
	}

	if id == "" {
		req := CreateTaxRateRequest{OrganizationID: organizationID}
		fields(&req)
		if _, err := d.CreateTaxRate(req); err != nil {
			return "", identifier, err
		}
		return "created", identifier, nil
	}

	existing, err := d.GetTaxRate(id)
	if err != nil {
		return "", identifier, newValidationError("tax rate id %q not found", id)
	}
	if existing.OrganizationID != organizationID {
		return "", identifier, newValidationError("tax rate id %q belongs to a different organization", id)
	}
	// CreateTaxRate defaults a blank category code to "S" itself;
	// UpdateTaxRate's CategoryCode is a *string that must already be valid
	// when non-nil, so an emptied cell needs the same default applied here
	// rather than sending a bare "" through.
	if categoryCode == "" {
		categoryCode = "S"
	}
	var create CreateTaxRateRequest
	fields(&create)
	update := UpdateTaxRateRequest{
		Name: &create.Name, Description: create.Description, Percentage: &create.Percentage, IsDefault: create.IsDefault,
		CategoryCode: &categoryCode, ExemptionReason: create.ExemptionReason,
		OutputTaxAccountID: create.OutputTaxAccountID, InputTaxAccountID: create.InputTaxAccountID, DatevBuKey: create.DatevBuKey,
	}
	if _, err := d.UpdateTaxRate(id, update); err != nil {
		return "", identifier, err
	}
	return "updated", identifier, nil
}

// formatPercentage renders 19 as "19" and 7.5 as "7.5" — the minimal
// decimal representation, not a fixed 2-decimal string, since Percentage
// is a plain float64 with no cents-style storage convention behind it.
func formatPercentage(p float64) string {
	return strconv.FormatFloat(p, 'f', -1, 64)
}

// ExportTaxRatesXLSX/ImportTaxRatesXLSX are the public entry points for tax
// rates' Excel mass-maintenance download/upload (api/mass_data.go).
func (d *Database) ExportTaxRatesXLSX(organizationID string) ([]byte, error) {
	return exportMassDataXLSX(taxRatesMassDataSpec{}, d, organizationID)
}

func (d *Database) ImportTaxRatesXLSX(organizationID string, content []byte) (*MassDataImportResult, error) {
	return importMassDataXLSX(taxRatesMassDataSpec{}, d, organizationID, content)
}
