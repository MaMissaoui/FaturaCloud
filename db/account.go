package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// ErrAccountInUse is returned by DeleteAccount when the account is still
// referenced by journal lines, an organization GL default, a tax rate's
// output/input tax account, or a product's revenue/expense account.
var ErrAccountInUse = errors.New("account is still referenced by journal entries or other records")

// ErrAccountHasChildren is returned by DeleteAccount when other accounts
// still point at it as their parent — deleting it would orphan them.
var ErrAccountHasChildren = errors.New("account has child accounts")

var accountTypes = map[string]bool{
	"asset":     true,
	"liability": true,
	"equity":    true,
	"revenue":   true,
	"expense":   true,
}

// Account mirrors the accounts table — a node in the chart of accounts.
// isGroup=1 rows are headers only, never postable; that's enforced by
// allocateAndFinalizeEntryTx (db/journal_entry.go) on every posting path,
// not by a DB constraint. normalBalance is deliberately not a column — it's
// a pure function of Type, computed by AccountNormalBalance below.
type Account struct {
	ID                 string  `db:"id"                 json:"id"`
	OrganizationID     string  `db:"organizationId"     json:"organizationId"`
	ParentID           *string `db:"parentId"            json:"parentId"`
	Code               string  `db:"code"                json:"code"`
	Name               string  `db:"name"                json:"name"`
	Type               string  `db:"type"                json:"type"`
	IsGroup            int     `db:"isGroup"             json:"isGroup"`
	IsActive           int     `db:"isActive"            json:"isActive"`
	DATEVAccountNumber *string `db:"datevAccountNumber"  json:"datevAccountNumber"`
	Description        *string `db:"description"         json:"description"`
	CreatedAt          int64   `db:"createdAt"           json:"createdAt"`
}

// AccountNormalBalance returns "debit" or "credit" for an account type —
// asset/expense accounts increase on the debit side, liability/equity/
// revenue accounts increase on the credit side. Computed, never stored, so
// it can never drift from Type.
func AccountNormalBalance(accountType string) string {
	if accountType == "asset" || accountType == "expense" {
		return "debit"
	}
	return "credit"
}

// CreateAccountRequest is the payload for creating a chart-of-accounts entry.
type CreateAccountRequest struct {
	ID                 string  `json:"id"`
	OrganizationID     string  `json:"organizationId"`
	ParentID           *string `json:"parentId"`
	Code               string  `json:"code"`
	Name               string  `json:"name"`
	Type               string  `json:"type"`
	IsGroup            int     `json:"isGroup"`
	IsActive           *int    `json:"isActive"`
	DATEVAccountNumber *string `json:"datevAccountNumber"`
	Description        *string `json:"description"`
}

// UpdateAccountRequest is the payload for updating a chart-of-accounts entry.
type UpdateAccountRequest struct {
	ParentID           *string `json:"parentId"`
	Code               string  `json:"code"`
	Name               string  `json:"name"`
	Type               string  `json:"type"`
	IsGroup            int     `json:"isGroup"`
	IsActive           int     `json:"isActive"`
	DATEVAccountNumber *string `json:"datevAccountNumber"`
	Description        *string `json:"description"`
}

func (d *Database) GetAccounts(organizationID string) ([]Account, error) {
	accounts := []Account{}
	err := d.DB.Select(&accounts,
		`SELECT * FROM accounts WHERE organizationId = ? ORDER BY code ASC`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_accounts: %w", err)
	}
	return accounts, nil
}

func (d *Database) GetAccount(accountID string) (*Account, error) {
	var account Account
	err := d.DB.Get(&account, `SELECT * FROM accounts WHERE id = ? LIMIT 1`, accountID)
	if err != nil {
		return nil, fmt.Errorf("get_account: %w", err)
	}
	return &account, nil
}

func (d *Database) CreateAccount(req CreateAccountRequest) (*Account, error) {
	if !accountTypes[req.Type] {
		return nil, newValidationError("invalid account type %q", req.Type)
	}
	if req.ID == "" {
		req.ID, _ = gonanoid.New()
	}
	isActive := 1
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	_, err := d.DB.Exec(
		`INSERT INTO accounts (id, organizationId, parentId, code, name, type, isGroup, isActive, datevAccountNumber, description)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.ParentID, req.Code, req.Name, req.Type, req.IsGroup, isActive,
		req.DATEVAccountNumber, req.Description,
	)
	if err != nil {
		if isDuplicateAccountCode(err) {
			return nil, newValidationError("an account with code %q already exists", req.Code)
		}
		return nil, fmt.Errorf("create_account: %w", err)
	}
	return d.GetAccount(req.ID)
}

// UpdateAccount refuses two changes DeleteAccount already guards against in
// its own way: retyping/regrouping an account that has any posted history
// (F51, 2026-08-13 audit) — allocateAndFinalizeEntryTx only enforces
// isGroup=1 accounts being non-postable at *post* time, not at *edit* time,
// so without this an account could be silently reclassified out from under
// every journal_lines row that already posted against it, retroactively
// changing past P&L/balance sheet numbers — and turning a group header with
// existing child accounts back into a leaf, which would leave those
// children pointing at a parent that's no longer structurally a group.
func (d *Database) UpdateAccount(accountID string, updates UpdateAccountRequest) (*Account, error) {
	if !accountTypes[updates.Type] {
		return nil, newValidationError("invalid account type %q", updates.Type)
	}

	current, err := d.GetAccount(accountID)
	if err != nil {
		return nil, err
	}

	if updates.Type != current.Type || updates.IsGroup != current.IsGroup {
		usage, err := d.GetAccountUsageCount(accountID)
		if err != nil {
			return nil, err
		}
		if usage > 0 {
			return nil, newValidationError(
				"cannot change type or group status of an account that has posted journal entries or other references — it would retroactively reclassify existing history",
			)
		}
	}
	if updates.IsGroup == 0 {
		children, err := d.getAccountChildCount(accountID)
		if err != nil {
			return nil, err
		}
		if children > 0 {
			return nil, newValidationError("cannot make this account a leaf — it still has child accounts under it")
		}
	}
	if updates.ParentID != nil {
		if *updates.ParentID == accountID {
			return nil, newValidationError("an account cannot be its own parent")
		}
		var exists int64
		if err := d.DB.Get(&exists, `SELECT COUNT(*) FROM accounts WHERE id = ?`, *updates.ParentID); err != nil {
			return nil, fmt.Errorf("update_account parent_check: %w", err)
		}
		if exists == 0 {
			return nil, newValidationError("parent account %q does not exist", *updates.ParentID)
		}
	}

	_, err = d.DB.Exec(
		`UPDATE accounts
		 SET parentId = ?, code = ?, name = ?, type = ?, isGroup = ?, isActive = ?,
		     datevAccountNumber = ?, description = ?
		 WHERE id = ?`,
		updates.ParentID, updates.Code, updates.Name, updates.Type, updates.IsGroup, updates.IsActive,
		updates.DATEVAccountNumber, updates.Description, accountID,
	)
	if err != nil {
		if isDuplicateAccountCode(err) {
			return nil, newValidationError("an account with code %q already exists", updates.Code)
		}
		return nil, fmt.Errorf("update_account: %w", err)
	}
	return d.GetAccount(accountID)
}

// isDuplicateAccountCode recognizes the raw SQLite unique-index violation on
// (organizationId, code). Mirrors isDuplicateSKU (db/product.go).
func isDuplicateAccountCode(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed") && strings.Contains(err.Error(), "accounts.code")
}

// accountReferencingColumns lists every {table, column} pair outside
// journal_lines that can point at an account: org-level GL defaults, a tax
// rate's output/input tax account, a product's revenue/expense account.
// journal_lines is counted separately below since it's keyed by accountId
// like every other domain's usage count, while these are one-off named FKs
// on their owning tables.
var accountReferencingOrganizationColumns = []string{
	"defaultArAccountId", "defaultApAccountId", "defaultRevenueAccountId",
	"defaultExpenseAccountId", "defaultCashAccountId", "fxGainAccountId",
	"fxLossAccountId", "retainedEarningsAccountId", "datevClearingAccountId",
	"defaultInventoryAccountId", "defaultGRNIAccountId",
	"defaultCOGSAccountId", "defaultInventoryAdjustmentAccountId",
}

// GetAccountUsageCount returns how many rows reference this account, so
// callers can decide whether it's safe to delete. Unlike GetTaxRateUsageCount
// (a single referencing column name across several tables), an account is
// referenced by many differently-named columns across several tables, so
// this is assembled by hand rather than from one generic list.
func (d *Database) GetAccountUsageCount(accountID string) (int64, error) {
	subqueries := []string{
		"(SELECT COUNT(*) FROM journal_lines WHERE accountId = ?)",
		"(SELECT COUNT(*) FROM taxRates WHERE outputTaxAccountId = ? OR inputTaxAccountId = ?)",
		"(SELECT COUNT(*) FROM products WHERE revenueAccountId = ? OR expenseAccountId = ?)",
		"(SELECT COUNT(*) FROM payments WHERE bankAccountId = ?)",
	}
	args := []any{accountID, accountID, accountID, accountID, accountID, accountID}
	for _, col := range accountReferencingOrganizationColumns {
		subqueries = append(subqueries, fmt.Sprintf("(SELECT COUNT(*) FROM organizations WHERE %s = ?)", col))
		args = append(args, accountID)
	}

	var count int64
	if err := d.DB.Get(&count, "SELECT "+strings.Join(subqueries, " + "), args...); err != nil {
		return 0, fmt.Errorf("get_account_usage_count: %w", err)
	}
	return count, nil
}

func (d *Database) getAccountChildCount(accountID string) (int64, error) {
	var count int64
	if err := d.DB.Get(&count, `SELECT COUNT(*) FROM accounts WHERE parentId = ?`, accountID); err != nil {
		return 0, fmt.Errorf("get_account_child_count: %w", err)
	}
	return count, nil
}

// DeleteAccount refuses to delete an account that has child accounts (would
// orphan them) or that's still referenced anywhere (would either fail on a
// foreign key or silently break the reference it once served).
func (d *Database) DeleteAccount(accountID string) (bool, error) {
	children, err := d.getAccountChildCount(accountID)
	if err != nil {
		return false, err
	}
	if children > 0 {
		return false, ErrAccountHasChildren
	}

	usage, err := d.GetAccountUsageCount(accountID)
	if err != nil {
		return false, err
	}
	if usage > 0 {
		return false, ErrAccountInUse
	}

	res, err := d.DB.Exec(`DELETE FROM accounts WHERE id = ?`, accountID)
	if err != nil {
		return false, fmt.Errorf("delete_account: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// defaultChartAccount is one row of a starter chart of accounts
// seedDefaultChartOfAccounts installs for a new organization.
// datevNumber is only set for a template whose codes ARE real DATEV/SKR
// account numbers (currently skr04ChartOfAccounts) — see datevAccountNumber
// on the accounts table. defaultRole, when set, wires this row to one of
// organizations.default*AccountId at seed time (see roleToID in
// seedDefaultChartOfAccounts) instead of a template hardcoding a specific
// code into that UPDATE, since the code for "the AR account" differs per
// template.
type defaultChartAccount struct {
	code, name, accountType, datevNumber, defaultRole string
	isGroup                                           bool
	parentCode                                        string // matches another row's code in the SAME chart — "" for a top-level group
}

// defaultChartOfAccounts is the minimal generic starter chart — not a
// country-specific import, same honesty stance as the e-invoice generator's
// Peppol decision (db/einvoice.go). Installed for every organization whose
// country isn't one of chartTemplates' curated, citable imports below.
var defaultChartOfAccounts = []defaultChartAccount{
	{code: "1000", name: "Assets", accountType: "asset", isGroup: true},
	{code: "1010", name: "Cash", accountType: "asset", parentCode: "1000"},
	{code: "1020", name: "Bank", accountType: "asset", parentCode: "1000", defaultRole: "cash"},
	{code: "1100", name: "Accounts Receivable", accountType: "asset", parentCode: "1000", defaultRole: "ar"},
	{code: "1200", name: "Input Tax Receivable", accountType: "asset", parentCode: "1000"},

	{code: "2000", name: "Liabilities", accountType: "liability", isGroup: true},
	{code: "2100", name: "Accounts Payable", accountType: "liability", parentCode: "2000", defaultRole: "ap"},
	{code: "2200", name: "Output Tax Payable", accountType: "liability", parentCode: "2000"},

	{code: "3000", name: "Equity", accountType: "equity", isGroup: true},
	{code: "3100", name: "Retained Earnings", accountType: "equity", parentCode: "3000", defaultRole: "retainedEarnings"},

	{code: "4000", name: "Revenue", accountType: "revenue", isGroup: true},
	{code: "4100", name: "Sales Revenue", accountType: "revenue", parentCode: "4000", defaultRole: "revenue"},
	{code: "4200", name: "Foreign Exchange Gain", accountType: "revenue", parentCode: "4000", defaultRole: "fxGain"},

	{code: "5000", name: "Expenses", accountType: "expense", isGroup: true},
	// Renamed from "Purchases / Cost of Goods Sold" once Phase 7 gave COGS
	// its own dedicated account (5150) — this one now only ever receives an
	// immediate-expense bill line for a non-stock-tracked product. Go-literal
	// rename only: an organization created before this change keeps whatever
	// its own 5100 row is already named (see phase7BaseAdditions below).
	{code: "5100", name: "General Purchases", accountType: "expense", parentCode: "5000", defaultRole: "expense"},
	{code: "5200", name: "Foreign Exchange Loss", accountType: "expense", parentCode: "5000", defaultRole: "fxLoss"},
}

// skr04ChartOfAccounts is a curated starter subset of Germany's SKR04
// (Standardkontenrahmen 04) — every code, name, and hierarchy position here
// is a real SKR04 account, cross-checked against the German-localization
// chart of accounts shipped by frappe/erpnext
// (erpnext/accounts/doctype/account/chart_of_accounts/verified/de_kontenplan_SKR04.json,
// an actively-maintained open-source ERP's DE dataset) — not the exhaustive
// official Kontenrahmen (which runs to several hundred accounts across
// specialized VAT/EU-trade/leasing variants no small organization needs on
// day one), the same "minimal, not exhaustive" scope defaultChartOfAccounts
// already has, just with real German numbering instead of invented one.
// Every leaf's code doubles as its datevNumber, since SKR04 codes ARE DATEV
// account numbers — uniformly 4 digits, satisfying the DATEV exporter's
// Sachkontenlänge rule (db/export_datev.go) out of the box for a German
// organization. Group headers ("1"/"2"/"3"/"4"/"5") are NOT real SKR04
// numbers — SKR04 has no single numbered header for "all assets" the way
// this table's isGroup rows work, so these are organizational-only, mirroring
// defaultChartOfAccounts' own synthetic 1000/2000/... headers; verified not
// to collide with any real SKR04 leaf (minimum real account number is 100).
var skr04ChartOfAccounts = []defaultChartAccount{
	{code: "1", name: "Aktiva", accountType: "asset", isGroup: true},
	{code: "1600", name: "Kasse", accountType: "asset", parentCode: "1", datevNumber: "1600"},
	{code: "1800", name: "Bank", accountType: "asset", parentCode: "1", datevNumber: "1800", defaultRole: "cash"},
	{code: "1200", name: "Forderungen aus Lieferungen und Leistungen", accountType: "asset", parentCode: "1", datevNumber: "1200", defaultRole: "ar"},
	{code: "1400", name: "Abziehbare Vorsteuer", accountType: "asset", parentCode: "1", datevNumber: "1400"},

	{code: "3", name: "Passiva – Verbindlichkeiten", accountType: "liability", isGroup: true},
	{code: "3300", name: "Verbindlichkeiten aus Lieferungen und Leistungen", accountType: "liability", parentCode: "3", datevNumber: "3300", defaultRole: "ap"},
	{code: "3800", name: "Umsatzsteuer", accountType: "liability", parentCode: "3", datevNumber: "3800"},

	{code: "2", name: "Passiva – Eigenkapital", accountType: "equity", isGroup: true},
	{code: "2970", name: "Gewinnvortrag vor Verwendung", accountType: "equity", parentCode: "2", datevNumber: "2970", defaultRole: "retainedEarnings"},

	{code: "4", name: "Erträge", accountType: "revenue", isGroup: true},
	{code: "4400", name: "Erlöse 19 % USt", accountType: "revenue", parentCode: "4", datevNumber: "4400", defaultRole: "revenue"},
	{code: "4840", name: "Erträge aus der Währungsumrechnung", accountType: "revenue", parentCode: "4", datevNumber: "4840", defaultRole: "fxGain"},

	{code: "5", name: "Aufwendungen", accountType: "expense", isGroup: true},
	{code: "5000", name: "Aufwendungen f. Roh-, Hilfs- und Betriebsstoffe und f. bezogene Waren", accountType: "expense", parentCode: "5", datevNumber: "5000", defaultRole: "expense"},
	{code: "6880", name: "Aufwendungen aus der Währungsumrechnung", accountType: "expense", parentCode: "5", datevNumber: "6880", defaultRole: "fxLoss"},
}

// pcgChartOfAccounts is a curated starter subset of France's Plan Comptable
// Général — every code, name, and hierarchy position (including the class
// headers "1000"/"2000"/.../"5000" below, which stand in for real PCG class
// numbers 1-7 without literally reusing them, since a PCG class like "4 -
// Comptes de tiers" mixes both AR and AP under one class and doesn't map
// 1:1 onto this table's single-type isGroup rows) is cross-checked against
// the Autorité des Normes Comptables' own PCG text as digitized by
// github.com/arrhes/PCG (versions/2026/pcg_2026.json, an annually
// republished, source-cited dataset), restricted to entries tagged
// "système minimal" in that dataset — the PCG's own smallest reporting
// tier, not the exhaustive ~860-account plan. France has no DATEV
// equivalent, so unlike skr04ChartOfAccounts there's no datevNumber here.
var pcgChartOfAccounts = []defaultChartAccount{
	{code: "1000", name: "Actif", accountType: "asset", isGroup: true},
	{code: "53", name: "Caisse", accountType: "asset", parentCode: "1000"},
	{code: "512", name: "Banques", accountType: "asset", parentCode: "1000", defaultRole: "cash"},
	{code: "411", name: "Clients", accountType: "asset", parentCode: "1000", defaultRole: "ar"},
	{code: "4456", name: "Taxes sur le chiffre d'affaires déductibles", accountType: "asset", parentCode: "1000"},

	{code: "2000", name: "Passif – Dettes", accountType: "liability", isGroup: true},
	{code: "401", name: "Fournisseurs", accountType: "liability", parentCode: "2000", defaultRole: "ap"},
	{code: "4457", name: "Taxes sur le chiffre d'affaires collectées", accountType: "liability", parentCode: "2000"},

	{code: "3000", name: "Passif – Capitaux propres", accountType: "equity", isGroup: true},
	{code: "110", name: "Report à nouveau - solde créditeur", accountType: "equity", parentCode: "3000", defaultRole: "retainedEarnings"},

	{code: "4000", name: "Produits", accountType: "revenue", isGroup: true},
	{code: "70", name: "Ventes de produits fabriqués, prestations de services, marchandises", accountType: "revenue", parentCode: "4000", defaultRole: "revenue"},
	{code: "766", name: "Gains de change financiers", accountType: "revenue", parentCode: "4000", defaultRole: "fxGain"},

	{code: "5000", name: "Charges", accountType: "expense", isGroup: true},
	{code: "60", name: "Achats (sauf 603)", accountType: "expense", parentCode: "5000", defaultRole: "expense"},
	{code: "666", name: "Pertes de change financières", accountType: "expense", parentCode: "5000", defaultRole: "fxLoss"},
}

// chartTemplates maps an organization's country — as collected by the New
// Organization form's free-text Select (src/utils/countries.tsx's country
// *name*; org creation collects no ISO country_code field at all, unlike
// db/einvoice.go's resolveEInvoiceProfile, which only reads CountryCode and
// only after it's set later via the Organizations list edit drawer) — to a
// curated, citable chart of accounts for that country. A country not listed
// here keeps defaultChartOfAccounts, the same generic starter chart every
// organization got before this feature existed — the DATEV exporter's
// "cite the source, state plainly what's unvalidated" stance, not the
// Peppol decision's "the claim can't be backed at all" one: every code
// below is real, sourced, and cited on its own chart, just not exhaustive.
var chartTemplates = map[string][]defaultChartAccount{
	"Germany": skr04ChartOfAccounts,
	"France":  pcgChartOfAccounts,
}

// resolveChartTemplate returns the chart to seed for an organization's
// country (nil-safe — a new organization can leave Country unset).
func resolveChartTemplate(country *string) []defaultChartAccount {
	if country != nil {
		if chart, ok := chartTemplates[*country]; ok {
			return chart
		}
	}
	return defaultChartOfAccounts
}

// phase7BaseAdditions are the four accounts Phase 7's inventory/COGS GL
// integration needs, common to every chart template regardless of country.
// Unlike the AR/AP/tax/revenue/expense accounts above, no country's real
// chart has one universally-agreed number for "the GRNI clearing account"
// or "the COGS account" — different firms number these differently even
// within German or French practice — so inventing a real-looking SKR04/PCG
// number for them would be exactly the false-conformance problem the
// e-invoice generator's Peppol decision refuses to make. Kept as one
// shared, honestly-generic set of codes across every template instead.
// parentCode is resolved dynamically, not hardcoded here, since each
// template's own group header codes differ — see phase7AdditionsFor (fresh
// organization, resolves against the chart slice being seeded) and
// seedInventoryAccountsTx (existing-organization backfill, resolves against
// whatever that organization's chart actually has in the database).
var phase7BaseAdditions = []defaultChartAccount{
	{code: "1150", name: "Inventory", accountType: "asset", defaultRole: "inventory"},
	{code: "2150", name: "Goods Received Not Invoiced", accountType: "liability", defaultRole: "grni"},
	{code: "5150", name: "Cost of Goods Sold", accountType: "expense", defaultRole: "cogs"},
	{code: "5900", name: "Inventory Adjustment", accountType: "expense", defaultRole: "inventoryAdjustment"},
}

// headerCodeForType returns the code of chart's isGroup row for accountType,
// or "" if the chart has none — used to parent phase7BaseAdditions under
// whichever chart template is actually being seeded.
func headerCodeForType(chart []defaultChartAccount, accountType string) string {
	for _, a := range chart {
		if a.isGroup && a.accountType == accountType {
			return a.code
		}
	}
	return ""
}

// phase7AdditionsFor returns phase7BaseAdditions with each row's parentCode
// resolved against chart — the template about to be seeded for a brand-new
// organization (see seedDefaultChartOfAccounts). Returns a copy; the shared
// phase7BaseAdditions slice itself is never mutated.
func phase7AdditionsFor(chart []defaultChartAccount) []defaultChartAccount {
	out := make([]defaultChartAccount, len(phase7BaseAdditions))
	for i, a := range phase7BaseAdditions {
		a.parentCode = headerCodeForType(chart, a.accountType)
		out[i] = a
	}
	return out
}

// seedDefaultChartOfAccounts inserts the starter chart for organizationID —
// resolveChartTemplate(country)'s curated import if one exists, otherwise
// the generic defaultChartOfAccounts — plus phase7BaseAdditions, and wires
// organizations.default*AccountId to the created rows via each row's
// defaultRole, so auto-posting (Phase 2) has somewhere to post to
// immediately regardless of which template was used. Runs on the given exec
// so callers can fold it into their own transaction (CreateOrganization) or
// run it standalone (the startup backfill in main.go).
func seedDefaultChartOfAccounts(exec sqlGetExecer, organizationID string, country *string) error {
	chart := resolveChartTemplate(country)
	full := append(append([]defaultChartAccount{}, chart...), phase7AdditionsFor(chart)...)

	codeToID := make(map[string]string, len(full))
	roleToID := make(map[string]string, 12)
	for _, a := range full {
		id, err := gonanoid.New()
		if err != nil {
			return fmt.Errorf("seed_default_chart_of_accounts new_id: %w", err)
		}
		codeToID[a.code] = id
		if a.defaultRole != "" {
			roleToID[a.defaultRole] = id
		}

		var parentID any
		if a.parentCode != "" {
			parentID = codeToID[a.parentCode]
		}
		isGroup := 0
		if a.isGroup {
			isGroup = 1
		}
		if _, err := exec.Exec(
			`INSERT INTO accounts (id, organizationId, parentId, code, name, type, isGroup, datevAccountNumber)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, organizationID, parentID, a.code, a.name, a.accountType, isGroup, nullableString(a.datevNumber),
		); err != nil {
			return fmt.Errorf("seed_default_chart_of_accounts insert %s: %w", a.code, err)
		}
	}

	if _, err := exec.Exec(
		`UPDATE organizations
		 SET defaultArAccountId = ?, defaultApAccountId = ?, defaultRevenueAccountId = ?,
		     defaultExpenseAccountId = ?, defaultCashAccountId = ?,
		     fxGainAccountId = ?, fxLossAccountId = ?, retainedEarningsAccountId = ?,
		     defaultInventoryAccountId = ?, defaultGRNIAccountId = ?,
		     defaultCOGSAccountId = ?, defaultInventoryAdjustmentAccountId = ?
		 WHERE id = ?`,
		nullableString(roleToID["ar"]), nullableString(roleToID["ap"]), nullableString(roleToID["revenue"]),
		nullableString(roleToID["expense"]), nullableString(roleToID["cash"]),
		nullableString(roleToID["fxGain"]), nullableString(roleToID["fxLoss"]), nullableString(roleToID["retainedEarnings"]),
		nullableString(roleToID["inventory"]), nullableString(roleToID["grni"]),
		nullableString(roleToID["cogs"]), nullableString(roleToID["inventoryAdjustment"]),
		organizationID,
	); err != nil {
		return fmt.Errorf("seed_default_chart_of_accounts wire_defaults: %w", err)
	}
	return nil
}

// seedInventoryAccountsTx installs phase7ChartAdditions for an organization
// that already has a chart of accounts (every organization that ran Phases
// 1-6 before Phase 7 shipped — see SeedInventoryAccountingDefaultsForAllOrganizations).
// Unlike seedDefaultChartOfAccounts, which builds parent ids from a
// codeToID map it populates in the same pass, this resolves each new row's
// parent by *account type* against the organization's existing chart —
// "the isGroup row of this type, earliest first" — rather than a hardcoded
// parentCode, since an organization seeded from skr04ChartOfAccounts or
// pcgChartOfAccounts has different group header codes than the generic
// chart's 1000/2000/5000 (and a manually-edited chart could have anything).
// A code collision (the organization already has its own account at, say,
// code "1150") skips just that one insert rather than failing the whole
// backfill; the organization can wire its own account to the new default
// column manually afterward.
func seedInventoryAccountsTx(exec sqlGetExecer, organizationID string) error {
	roleToID := map[string]string{}
	for _, a := range phase7BaseAdditions {
		var parentID *string
		var existing string
		err := exec.Get(&existing,
			`SELECT id FROM accounts WHERE organizationId = ? AND type = ? AND isGroup = 1 ORDER BY createdAt ASC LIMIT 1`,
			organizationID, a.accountType,
		)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("seed_inventory_accounts parent_lookup %s: %w", a.accountType, err)
		}
		if existing != "" {
			parentID = &existing
		}

		id, err := gonanoid.New()
		if err != nil {
			return fmt.Errorf("seed_inventory_accounts new_id: %w", err)
		}
		_, err = exec.Exec(
			`INSERT INTO accounts (id, organizationId, parentId, code, name, type, isGroup)
			 VALUES (?, ?, ?, ?, ?, ?, 0)`,
			id, organizationID, parentID, a.code, a.name, a.accountType,
		)
		if err != nil {
			if isDuplicateAccountCode(err) {
				continue
			}
			return fmt.Errorf("seed_inventory_accounts insert %s: %w", a.code, err)
		}
		roleToID[a.defaultRole] = id
	}

	// COALESCE, not a bare assignment: an organization that already wired
	// one of these columns manually (or from a partially-applied earlier
	// backfill attempt) keeps its own choice rather than being overwritten.
	// A code that collided above and was skipped has no entry in roleToID —
	// its column simply stays whatever it already was (likely NULL).
	if _, err := exec.Exec(
		`UPDATE organizations
		 SET defaultInventoryAccountId = COALESCE(defaultInventoryAccountId, ?),
		     defaultGRNIAccountId = COALESCE(defaultGRNIAccountId, ?),
		     defaultCOGSAccountId = COALESCE(defaultCOGSAccountId, ?),
		     defaultInventoryAdjustmentAccountId = COALESCE(defaultInventoryAdjustmentAccountId, ?)
		 WHERE id = ?`,
		nullableString(roleToID["inventory"]), nullableString(roleToID["grni"]),
		nullableString(roleToID["cogs"]), nullableString(roleToID["inventoryAdjustment"]),
		organizationID,
	); err != nil {
		return fmt.Errorf("seed_inventory_accounts wire_defaults: %w", err)
	}
	return nil
}

// nullableString turns an empty string (a code that collided and was
// skipped, so codeToID has no entry for it) into a real NULL rather than
// an empty-string value that COALESCE would treat as "already set".
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// SeedInventoryAccountingDefaultsForAllOrganizations installs Phase 7's
// four inventory/COGS accounts for every organization that doesn't have
// them yet — called once at startup (main.go), alongside
// SeedAccountingDefaultsForAllOrganizations. That function's account-count
// gate ("seed if this org has zero accounts") can't reach these: every
// organization that ran Phases 1-6 already has a non-empty chart, so it
// would never re-enter seeding for them. Gated on
// defaultInventoryAccountId IS NULL instead, and idempotent — safe to run
// on every startup.
func (d *Database) SeedInventoryAccountingDefaultsForAllOrganizations() error {
	// F62 (2026-08-13 audit): gated on *any* of the four columns being
	// NULL, not just defaultInventoryAccountId — seedInventoryAccountsTx
	// itself is already safe to call on a partially-wired organization
	// (its COALESCE keeps whatever's already set), but an organization
	// that had only defaultInventoryAccountId set manually (e.g. via the
	// Accounting card) while the other three stayed NULL would never be
	// selected here, and so never get the chance to have them backfilled.
	var orgIDs []string
	if err := d.DB.Select(&orgIDs, `
		SELECT id FROM organizations
		WHERE defaultInventoryAccountId IS NULL
		   OR defaultGRNIAccountId IS NULL
		   OR defaultCOGSAccountId IS NULL
		   OR defaultInventoryAdjustmentAccountId IS NULL`,
	); err != nil {
		return fmt.Errorf("seed_inventory_accounting_defaults list_organizations: %w", err)
	}
	for _, orgID := range orgIDs {
		if err := seedInventoryAccountsTx(d.DB, orgID); err != nil {
			return fmt.Errorf("seed_inventory_accounting_defaults %s: %w", orgID, err)
		}
	}
	return nil
}

// SeedAccountingDefaultsForAllOrganizations installs the default chart of
// accounts and journals for every organization that doesn't have any yet —
// called once at startup (main.go), mirroring EnsureFirstAdmin's
// count-then-seed shape (api/users.go), so upgrading an existing database
// backfills the new GL tables the same way a fresh install gets them.
func (d *Database) SeedAccountingDefaultsForAllOrganizations() error {
	var orgs []struct {
		ID      string  `db:"id"`
		Country *string `db:"country"`
	}
	if err := d.DB.Select(&orgs, `SELECT id, country FROM organizations`); err != nil {
		return fmt.Errorf("seed_accounting_defaults list_organizations: %w", err)
	}

	for _, org := range orgs {
		orgID := org.ID
		var accountCount int
		if err := d.DB.Get(&accountCount, `SELECT COUNT(*) FROM accounts WHERE organizationId = ?`, orgID); err != nil {
			return fmt.Errorf("seed_accounting_defaults count_accounts: %w", err)
		}
		if accountCount == 0 {
			if err := seedDefaultChartOfAccounts(d.DB, orgID, org.Country); err != nil {
				return fmt.Errorf("seed_accounting_defaults chart %s: %w", orgID, err)
			}
		}

		var journalCount int
		if err := d.DB.Get(&journalCount, `SELECT COUNT(*) FROM journals WHERE organizationId = ?`, orgID); err != nil {
			return fmt.Errorf("seed_accounting_defaults count_journals: %w", err)
		}
		if journalCount == 0 {
			if err := seedDefaultJournals(d.DB, orgID); err != nil {
				return fmt.Errorf("seed_accounting_defaults journals %s: %w", orgID, err)
			}
		}
	}
	return nil
}
