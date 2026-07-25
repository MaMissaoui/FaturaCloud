-- BT-118 VAT category code (UNTDID 5305: S=standard, Z=zero rated,
-- E=exempt, AE=reverse charge, ...). Standard rate is a safe default for
-- every existing tax rate. BT-120 exemption reason text is required by
-- EN 16931 when the category needs one (e.g. E, Z, O).
ALTER TABLE taxRates ADD COLUMN category_code TEXT NOT NULL DEFAULT 'S';
ALTER TABLE taxRates ADD COLUMN exemption_reason TEXT;
