package db

import "strings"

// productsMassDataSpec is products' massDataSpec (db/mass_data.go). Tax
// Rate/Unit of Measure resolve by Name (both unique enough in practice for
// a business to recognize and type back), Revenue/Expense Account by Code —
// the same human-editable-reference convention every other mass-data spec
// in this file uses instead of a raw internal id.
type productsMassDataSpec struct{}

func (productsMassDataSpec) Headers() []string {
	return []string{
		"ID", "Name", "Description", "SKU", "Type", "Category",
		"Price", "Unit Cost", "Unit (legacy text)", "Unit of Measure",
		"Tax Rate", "Stock Enabled", "Serialized",
		"Revenue Account Code", "Expense Account Code",
	}
}

func (productsMassDataSpec) ExportRows(d *Database, organizationID string) ([][]string, error) {
	products, _, err := d.GetProducts(organizationID, ProductListOptions{})
	if err != nil {
		return nil, err
	}
	taxRates, err := d.GetTaxRates(organizationID)
	if err != nil {
		return nil, err
	}
	taxRateNameByID := make(map[string]string, len(taxRates))
	for _, t := range taxRates {
		taxRateNameByID[t.ID] = t.Name
	}
	units, err := d.GetUnitsOfMeasure(organizationID)
	if err != nil {
		return nil, err
	}
	unitNameByID := make(map[string]string, len(units))
	for _, u := range units {
		unitNameByID[u.ID] = u.Name
	}
	accounts, err := d.GetAccounts(organizationID)
	if err != nil {
		return nil, err
	}
	accountCodeByID := make(map[string]string, len(accounts))
	for _, a := range accounts {
		accountCodeByID[a.ID] = a.Code
	}

	rows := make([][]string, len(products))
	for i, p := range products {
		taxRateName, unitOfMeasureName, revenueCode, expenseCode := "", "", "", ""
		if p.TaxRateID != nil {
			taxRateName = taxRateNameByID[*p.TaxRateID]
		}
		if p.UnitOfMeasureID != nil {
			unitOfMeasureName = unitNameByID[*p.UnitOfMeasureID]
		}
		if p.RevenueAccountID != nil {
			revenueCode = accountCodeByID[*p.RevenueAccountID]
		}
		if p.ExpenseAccountID != nil {
			expenseCode = accountCodeByID[*p.ExpenseAccountID]
		}
		rows[i] = []string{
			p.ID, p.Name, strOrEmpty(p.Description), strOrEmpty(p.SKU), p.Type, strOrEmpty(p.Category),
			centsCell(p.Price), centsCellPtr(p.UnitCost), strOrEmpty(p.Unit), unitOfMeasureName,
			taxRateName, boolCell(p.StockEnabled == 1), boolCell(p.Serialized == 1),
			revenueCode, expenseCode,
		}
	}
	return rows, nil
}

type productsImportContext struct {
	taxRateNameToID map[string]string
	unitNameToID    map[string]string
	accountCodeToID map[string]string
}

func (productsMassDataSpec) NewImportContext(d *Database, organizationID string) (any, error) {
	taxRates, err := d.GetTaxRates(organizationID)
	if err != nil {
		return nil, err
	}
	units, err := d.GetUnitsOfMeasure(organizationID)
	if err != nil {
		return nil, err
	}
	accounts, err := d.GetAccounts(organizationID)
	if err != nil {
		return nil, err
	}
	ctx := &productsImportContext{
		taxRateNameToID: make(map[string]string, len(taxRates)),
		unitNameToID:    make(map[string]string, len(units)),
		accountCodeToID: make(map[string]string, len(accounts)),
	}
	for _, t := range taxRates {
		ctx.taxRateNameToID[t.Name] = t.ID
	}
	for _, u := range units {
		ctx.unitNameToID[u.Name] = u.ID
	}
	for _, a := range accounts {
		ctx.accountCodeToID[a.Code] = a.ID
	}
	return ctx, nil
}

func resolveByLookup(lookup map[string]string, value, what string) (*string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	id, ok := lookup[value]
	if !ok {
		return nil, newValidationError("%s %q not found", what, value)
	}
	return &id, nil
}

func (productsMassDataSpec) ImportRow(d *Database, organizationID string, ctxAny any, cells []string) (action, identifier string, err error) {
	ctx := ctxAny.(*productsImportContext)

	id := strings.TrimSpace(cells[0])
	name := strings.TrimSpace(cells[1])
	identifier = name
	if name == "" {
		return "", identifier, newValidationError("name is required")
	}
	productType := strings.TrimSpace(cells[4])
	if productType == "" {
		productType = "service"
	}
	price, err := cellToCents(cells[6])
	if err != nil {
		return "", identifier, newValidationError("price: %v", err)
	}
	unitCost, err := cellToCentsPtr(cells[7])
	if err != nil {
		return "", identifier, newValidationError("unit cost: %v", err)
	}
	unitOfMeasureID, err := resolveByLookup(ctx.unitNameToID, cells[9], "unit of measure")
	if err != nil {
		return "", identifier, err
	}
	taxRateID, err := resolveByLookup(ctx.taxRateNameToID, cells[10], "tax rate")
	if err != nil {
		return "", identifier, err
	}
	revenueAccountID, err := resolveByLookup(ctx.accountCodeToID, cells[13], "revenue account code")
	if err != nil {
		return "", identifier, err
	}
	expenseAccountID, err := resolveByLookup(ctx.accountCodeToID, cells[14], "expense account code")
	if err != nil {
		return "", identifier, err
	}
	stockEnabled := intCell(cellBool(cells[11]))
	serialized := intCell(cellBool(cells[12]))

	fields := func(dst *CreateProductRequest) {
		dst.Name = name
		dst.Description = cellStr(cells[2])
		dst.SKU = cellStr(cells[3])
		dst.Type = productType
		dst.Category = cellStr(cells[5])
		dst.Price = price
		dst.UnitCost = unitCost
		dst.Unit = cellStr(cells[8])
		dst.UnitOfMeasureID = unitOfMeasureID
		dst.TaxRateID = taxRateID
		dst.StockEnabled = stockEnabled
		dst.Serialized = serialized
		dst.RevenueAccountID = revenueAccountID
		dst.ExpenseAccountID = expenseAccountID
	}

	if id == "" {
		req := CreateProductRequest{OrganizationID: organizationID}
		fields(&req)
		if _, err := d.CreateProduct(req); err != nil {
			return "", identifier, err
		}
		return "created", identifier, nil
	}

	existing, err := d.GetProduct(id)
	if err != nil {
		return "", identifier, newValidationError("product id %q not found", id)
	}
	if existing.OrganizationID != organizationID {
		return "", identifier, newValidationError("product id %q belongs to a different organization", id)
	}
	var create CreateProductRequest
	fields(&create)
	update := UpdateProductRequest{
		Name: create.Name, Description: create.Description, SKU: create.SKU, Price: create.Price,
		UnitCost: create.UnitCost, Unit: create.Unit, UnitOfMeasureID: create.UnitOfMeasureID,
		Type: create.Type, Category: create.Category, TaxRateID: create.TaxRateID,
		StockEnabled: create.StockEnabled, Serialized: create.Serialized,
		RevenueAccountID: create.RevenueAccountID, ExpenseAccountID: create.ExpenseAccountID,
	}
	if _, err := d.UpdateProduct(id, update); err != nil {
		return "", identifier, err
	}
	return "updated", identifier, nil
}

// ExportProductsXLSX/ImportProductsXLSX are the public entry points for
// products' Excel mass-maintenance download/upload (api/mass_data.go).
func (d *Database) ExportProductsXLSX(organizationID string) ([]byte, error) {
	return exportMassDataXLSX(productsMassDataSpec{}, d, organizationID)
}

func (d *Database) ImportProductsXLSX(organizationID string, content []byte) (*MassDataImportResult, error) {
	return importMassDataXLSX(productsMassDataSpec{}, d, organizationID, content)
}
