-- Tunisia invoice support: timbre fiscal (fiscal stamp duty) and retenue à
-- la source (withholding tax), plus a per-organization invoice layout
-- selector so a non-default PDF template (starting with "tunisia") can be
-- chosen explicitly rather than inferred from country.
--
-- fiscalStampAmount is NOT NULL DEFAULT 0 cents: it's additive into
-- invoices.total (see validateInvoiceTotals), so every existing invoice
-- reads as "no stamp" without needing null-handling at every call site —
-- same convention as overdueCharge/total/taxTotal/subTotal.
--
-- withholdingTaxRate/withholdingTaxAmount are deliberately NOT part of
-- total/subTotal/taxTotal and are not posted to the GL in this change: a
-- withholding certificate changes how the invoiced amount is *discharged*
-- (part cash, part tax credit), not what's legally owed. They're
-- informational — computed and stored for display (the PDF's "Net à
-- recevoir" line) and audit, not enforced by validateInvoiceTotals. Both
-- nullable and set together, direct-assignment style (like dueDate), so
-- clearing withholding on an invoice is a real NULL, not a stored 0.
ALTER TABLE invoices ADD COLUMN fiscalStampAmount INTEGER NOT NULL DEFAULT 0;
ALTER TABLE invoices ADD COLUMN withholdingTaxRate REAL;
ALTER TABLE invoices ADD COLUMN withholdingTaxAmount INTEGER;

-- defaultFiscalStampAmount: the organization's usual stamp duty (Tunisia's
-- statutory timbre fiscal is a flat fee that changes by law from time to
-- time, so this is a plain editable default, not a hardcoded constant) —
-- prefills a new invoice's fiscalStampAmount, never enforced.
--
-- defaultStampDutyAccountId: same nullable, no-cascade convention as every
-- other default*AccountId column (0058) — an organization can use stamp
-- duty before this is wired up; auto-posting refuses with a 409, not a
-- 500, until it is. Must be added to accountReferencingOrganizationColumns
-- (db/account.go) and ResetOrganizationData's NULL-clear list (db/reset.go).
ALTER TABLE organizations ADD COLUMN defaultFiscalStampAmount INTEGER;
ALTER TABLE organizations ADD COLUMN defaultStampDutyAccountId TEXT REFERENCES accounts(id);

-- invoiceLayout: NULL/'' selects the original single-layout template
-- ("default"); any other value must match a key in the frontend's layout
-- registry (src/components/invoices/layouts.ts). Deliberately an explicit
-- setting, not inferred from organizations.country/country_code — a layout
-- is a presentation preference, not a jurisdiction mandate (unlike
-- resolveEInvoiceProfile's buyer-country resolution).
ALTER TABLE organizations ADD COLUMN invoiceLayout TEXT;
