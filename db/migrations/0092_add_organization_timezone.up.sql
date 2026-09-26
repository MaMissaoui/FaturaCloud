-- The organization's IANA time zone (e.g. 'Africa/Tunis'), used everywhere
-- the server turns a stored Unix-ms timestamp into a calendar day: printed
-- document dates, FEC/DATEV and e-invoice dates, the date tokens in document
-- numbers, the Cash Book's daily register buckets and revenue-trend months
-- (orgLocation in db/timezone.go). Dates are stored as instants — either the
-- moment of entry (a form's dayjs() default) or local midnight (a date picked
-- in a DatePicker) — so a day can only be read back correctly in the zone
-- they were entered in. NULL/'' keeps the previous behaviour: UTC days.
ALTER TABLE organizations ADD COLUMN timezone TEXT;

-- One-time continuity backfill, the same shape as 0089/0091, limited to
-- countries with a single time zone. Everything else stays NULL (UTC, as
-- before) until set in the Organizations drawer. Afterwards the zone is
-- purely the explicit setting, never inferred from country.
UPDATE organizations SET timezone = CASE
    WHEN UPPER(TRIM(country_code)) = 'TN' OR LOWER(TRIM(country)) IN ('tunisia', 'tunisie') THEN 'Africa/Tunis'
    WHEN UPPER(TRIM(country_code)) = 'DZ' OR LOWER(TRIM(country)) IN ('algeria', 'algérie') THEN 'Africa/Algiers'
    WHEN UPPER(TRIM(country_code)) = 'MA' OR LOWER(TRIM(country)) IN ('morocco', 'maroc') THEN 'Africa/Casablanca'
    WHEN UPPER(TRIM(country_code)) = 'LY' OR LOWER(TRIM(country)) IN ('libya', 'libye') THEN 'Africa/Tripoli'
    WHEN UPPER(TRIM(country_code)) = 'EG' OR LOWER(TRIM(country)) IN ('egypt', 'égypte') THEN 'Africa/Cairo'
    WHEN UPPER(TRIM(country_code)) = 'FR' OR LOWER(TRIM(country)) = 'france' THEN 'Europe/Paris'
    WHEN UPPER(TRIM(country_code)) = 'DE' OR LOWER(TRIM(country)) IN ('germany', 'deutschland') THEN 'Europe/Berlin'
    WHEN UPPER(TRIM(country_code)) = 'AT' OR LOWER(TRIM(country)) IN ('austria', 'österreich', 'Österreich') THEN 'Europe/Vienna'
    WHEN UPPER(TRIM(country_code)) = 'CH' OR LOWER(TRIM(country)) IN ('switzerland', 'suisse', 'schweiz') THEN 'Europe/Zurich'
    WHEN UPPER(TRIM(country_code)) = 'BE' OR LOWER(TRIM(country)) IN ('belgium', 'belgique', 'belgië') THEN 'Europe/Brussels'
    WHEN UPPER(TRIM(country_code)) = 'NL' OR LOWER(TRIM(country)) IN ('netherlands', 'nederland') THEN 'Europe/Amsterdam'
    WHEN UPPER(TRIM(country_code)) = 'LU' OR LOWER(TRIM(country)) = 'luxembourg' THEN 'Europe/Luxembourg'
    WHEN UPPER(TRIM(country_code)) = 'LI' OR LOWER(TRIM(country)) = 'liechtenstein' THEN 'Europe/Vaduz'
    WHEN UPPER(TRIM(country_code)) = 'MC' OR LOWER(TRIM(country)) = 'monaco' THEN 'Europe/Monaco'
    WHEN UPPER(TRIM(country_code)) = 'IT' OR LOWER(TRIM(country)) IN ('italy', 'italia') THEN 'Europe/Rome'
    WHEN UPPER(TRIM(country_code)) = 'GB' OR LOWER(TRIM(country)) IN ('united kingdom', 'uk') THEN 'Europe/London'
    WHEN UPPER(TRIM(country_code)) = 'IE' OR LOWER(TRIM(country)) = 'ireland' THEN 'Europe/Dublin'
    WHEN UPPER(TRIM(country_code)) = 'PL' OR LOWER(TRIM(country)) IN ('poland', 'polska') THEN 'Europe/Warsaw'
  END
 WHERE timezone IS NULL;
