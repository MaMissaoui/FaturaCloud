-- The organization's document language: which language the printed
-- amount-in-words line ("Arrêtée la présente facture à la somme de ...",
-- migration 0088) is written in — 'en', 'de' or 'fr'. NULL (and anything
-- unrecognized) falls back by layout: French on the Tunisian layout, English
-- on the default one (documentLanguageFor in db/amount_in_words_lang.go). It
-- does not translate a template's static labels.
ALTER TABLE organizations ADD COLUMN documentLanguage TEXT;

-- One-time continuity backfill, the same shape as 0089's: until now the line
-- was always French, which is right for Tunisian organizations, so they keep
-- it explicitly. German- and French-speaking countries get their language;
-- every other organization stays NULL and prints English from here on —
-- that switch away from French is the point of this change. (SQLite's LOWER
-- only folds ASCII, hence 'Österreich' spelled out.) Afterwards the
-- language is purely the explicit setting, never inferred from country.
UPDATE organizations SET documentLanguage = 'fr'
 WHERE documentLayout = 'tunisia'
    OR UPPER(TRIM(country_code)) IN ('TN', 'FR', 'BE', 'LU', 'MC', 'MA', 'DZ')
    OR LOWER(TRIM(country)) IN ('tunisia', 'tunisie', 'france', 'belgium', 'belgique', 'luxembourg', 'monaco', 'morocco', 'maroc', 'algeria', 'algérie');

UPDATE organizations SET documentLanguage = 'de'
 WHERE documentLanguage IS NULL
   AND (UPPER(TRIM(country_code)) IN ('DE', 'AT', 'LI')
        OR LOWER(TRIM(country)) IN ('germany', 'deutschland', 'austria', 'österreich', 'Österreich', 'liechtenstein'));
