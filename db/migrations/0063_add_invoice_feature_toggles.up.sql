-- Decouples Tunisia invoice support (0062) from the invoice PDF layout: the
-- fiscal stamp and withholding tax fields were gated on
-- organizations.invoiceLayout = 'tunisia', which meant an organization
-- could only use them by also switching its whole PDF template — two
-- unrelated decisions (which template renders vs. which financial fields
-- an invoice has) coupled into one. These two boolean toggles are the
-- generic gate instead; invoiceLayout stays purely a template choice.
-- Same INTEGER 0/1 convention as isDefault/isGroup/isSystem elsewhere.
ALTER TABLE organizations ADD COLUMN fiscalStampEnabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE organizations ADD COLUMN withholdingTaxEnabled INTEGER NOT NULL DEFAULT 0;
