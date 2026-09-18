package db

import "strings"

// accountsMassDataSpec is the chart of accounts' massDataSpec
// (db/mass_data.go). "Parent Account" is a human-editable column holding
// the parent's Code (accounts.code is unique per organization, and is
// already how DATEV/the rest of this app names an account outside its own
// id) rather than its raw id.
type accountsMassDataSpec struct{}

func (accountsMassDataSpec) Headers() []string {
	return []string{
		"ID", "Code", "Name", "Type", "Group Account", "Active",
		"Parent Account Code", "DATEV Account Number", "Description",
	}
}

func (accountsMassDataSpec) ExportRows(d *Database, organizationID string) ([][]string, error) {
	accounts, err := d.GetAccounts(organizationID)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]string, len(accounts))
	for _, a := range accounts {
		byID[a.ID] = a.Code
	}
	rows := make([][]string, len(accounts))
	for i, a := range accounts {
		parentCode := ""
		if a.ParentID != nil {
			parentCode = byID[*a.ParentID]
		}
		rows[i] = []string{
			a.ID, a.Code, a.Name, a.Type, boolCell(a.IsGroup == 1), boolCell(a.IsActive == 1),
			parentCode, strOrEmpty(a.DATEVAccountNumber), strOrEmpty(a.Description),
		}
	}
	return rows, nil
}

// accountsImportContext maps an account code (case-insensitive) to its id —
// seeded from every account that exists before the import starts, then
// updated after each created row so a later row in the same file can name
// a parent that this same import just created, not only a pre-existing one.
type accountsImportContext struct {
	codeToID map[string]string
}

func (accountsMassDataSpec) NewImportContext(d *Database, organizationID string) (any, error) {
	accounts, err := d.GetAccounts(organizationID)
	if err != nil {
		return nil, err
	}
	ctx := &accountsImportContext{codeToID: make(map[string]string, len(accounts))}
	for _, a := range accounts {
		ctx.codeToID[a.Code] = a.ID
	}
	return ctx, nil
}

func (accountsMassDataSpec) ImportRow(d *Database, organizationID string, ctxAny any, cells []string) (action, identifier string, err error) {
	ctx := ctxAny.(*accountsImportContext)

	id := strings.TrimSpace(cells[0])
	code := strings.TrimSpace(cells[1])
	name := strings.TrimSpace(cells[2])
	accountType := strings.TrimSpace(cells[3])
	identifier = code
	if identifier != "" && name != "" {
		identifier = code + " " + name
	} else if name != "" {
		identifier = name
	}
	if code == "" {
		return "", identifier, newValidationError("code is required")
	}
	if name == "" {
		return "", identifier, newValidationError("name is required")
	}

	var parentID *string
	if parentCode := strings.TrimSpace(cells[6]); parentCode != "" {
		resolved, ok := ctx.codeToID[parentCode]
		if !ok {
			return "", identifier, newValidationError("parent account code %q not found", parentCode)
		}
		parentID = &resolved
	}

	isGroup := intCell(cellBool(cells[4]))
	isActive := intCell(cellBool(cells[5]))

	if id == "" {
		req := CreateAccountRequest{
			OrganizationID:     organizationID,
			ParentID:           parentID,
			Code:               code,
			Name:               name,
			Type:               accountType,
			IsGroup:            isGroup,
			IsActive:           &isActive,
			DATEVAccountNumber: cellStr(cells[7]),
			Description:        cellStr(cells[8]),
		}
		created, err := d.CreateAccount(req)
		if err != nil {
			return "", identifier, err
		}
		ctx.codeToID[code] = created.ID
		return "created", identifier, nil
	}

	existing, err := d.GetAccount(id)
	if err != nil {
		return "", identifier, newValidationError("account id %q not found", id)
	}
	if existing.OrganizationID != organizationID {
		return "", identifier, newValidationError("account id %q belongs to a different organization", id)
	}
	update := UpdateAccountRequest{
		ParentID:           parentID,
		Code:               code,
		Name:               name,
		Type:               accountType,
		IsGroup:            isGroup,
		IsActive:           isActive,
		DATEVAccountNumber: cellStr(cells[7]),
		Description:        cellStr(cells[8]),
	}
	if _, err := d.UpdateAccount(id, update); err != nil {
		return "", identifier, err
	}
	ctx.codeToID[code] = id
	return "updated", identifier, nil
}

// ExportAccountsXLSX/ImportAccountsXLSX are the public entry points for the
// chart of accounts' Excel mass-maintenance download/upload
// (api/mass_data.go).
func (d *Database) ExportAccountsXLSX(organizationID string) ([]byte, error) {
	return exportMassDataXLSX(accountsMassDataSpec{}, d, organizationID)
}

func (d *Database) ImportAccountsXLSX(organizationID string, content []byte) (*MassDataImportResult, error) {
	return importMassDataXLSX(accountsMassDataSpec{}, d, organizationID, content)
}
