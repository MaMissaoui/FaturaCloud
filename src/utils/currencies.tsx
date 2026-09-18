import isNumber from "lodash/isNumber";

// @ts-expect-error - Intl supportedValuesOf support?
export const currencies = Intl.supportedValuesOf("currency");

// Curated organizations.country_code -> BCP-47 locale, for grouping/decimal
// SEPARATOR conventions only (comma vs. period vs. space) — a distinct axis
// from minimumFractionDigits (how many decimal places), which every caller
// below already layers on top independently. Deliberately not exhaustive:
// verified per entry against a real Intl.NumberFormat run rather than
// guessed (e.g. "en-TN"/"de-TN" silently drop the TN region and fall back
// to en-US/de-DE-style formatting — only a real language+region pairing
// CLDR actually tailors, like "fr-TN", changes anything), the same
// cite-what's-verified stance db/account.go's chart templates and
// db/einvoice.go's Peppol refusal both take. A country not listed here
// returns null — "no override for this org, use the caller's own default"
// (today's i18n.locale-driven behavior) — rather than guessing at a
// convention this table hasn't actually checked.
//
// This is about the ORGANIZATION's own convention, not the viewer's UI
// language — the same "the org's own setting is authoritative" precedent
// organizations.minimum_fraction_digits already established. A Tunisian
// organization's numbers should look Tunisian regardless of whether the
// person viewing them has their own UI language set to English or German.
const countryNumberLocale: Record<string, string> = {
  // German-speaking
  DE: "de-DE",
  AT: "de-AT",
  CH: "de-CH",
  // French-speaking (Europe)
  FR: "fr-FR",
  BE: "fr-BE",
  LU: "fr-LU",
  // French-speaking Maghreb — see cmd/seed-demo's countryLocale and
  // db/CLAUDE.md's Tunisia invoice support for this app's existing French-
  // for-business-documents stance in the region
  TN: "fr-TN",
  MA: "fr-MA",
  DZ: "fr-DZ",
  // English-speaking
  US: "en-US",
  GB: "en-GB",
  IE: "en-IE",
  CA: "en-CA",
  AU: "en-AU",
  NZ: "en-NZ",
  // Other common, unambiguous business locales (verified: none of these
  // switch to a non-Latin digit system, unlike e.g. ar-SA/ar-EG)
  ES: "es-ES",
  IT: "it-IT",
  PT: "pt-PT",
  NL: "nl-NL",
  PL: "pl-PL",
  RU: "ru-RU",
  TR: "tr-TR",
};

/**
 * The BCP-47 locale to format an organization's numbers/currency amounts
 * with, derived from its country rather than the viewer's own UI language —
 * null if the country isn't in the curated table above, meaning the caller
 * should fall back to its own default (usually i18n.locale).
 */
export const numberFormatLocale = (countryCode?: string | null): string | null =>
  countryCode ? (countryNumberLocale[countryCode] ?? null) : null;

export const getCurrencySymbol = (locale: string, currency: string) => {
  const numberFormat = new Intl.NumberFormat(locale, { style: "currency", currency });

  const parts = numberFormat.formatToParts(1);
  const partValues = parts.map((p) => p.value);
  return partValues[0];
};

/**
 * The number of decimal places a currency is conventionally displayed with
 * (2 for EUR/USD, 0 for JPY, 3 for BHD/KWD, …), read from the platform's own
 * ICU currency data via Intl rather than a hand-maintained table. Used to
 * derive a sensible default for "Decimal places" when a currency is picked —
 * the field stays a plain number afterwards so the user can still override it.
 */
export const getDefaultFractionDigits = (currency: string, locale: string = "en") => {
  try {
    return new Intl.NumberFormat(locale, { style: "currency", currency }).resolvedOptions()
      .minimumFractionDigits;
  } catch {
    return 2;
  }
};

export const getFormattedNumber = (
  number: number,
  currency: string,
  locale: string,
  organization: any,
) => {
  if (!isNumber(number)) return "-";

  // A record's own currency should always be set, but a blank/invalid code
  // (bad data, an incomplete import) must never crash the whole page —
  // Intl.NumberFormat throws a RangeError on anything it doesn't recognize,
  // and an uncaught throw inside a table cell's render aborts the entire
  // React tree with no fallback UI. Fall back to the organization's own
  // currency first (the documented "blank means the org's own" convention),
  // then to a plain unstyled number as a last resort so this can never throw.
  const effectiveCurrency = currency || organization?.currency;
  // The organization's own country drives separator style (comma vs.
  // period vs. space), not the viewer's own UI language — falls back to
  // the passed-in locale (normally i18n.locale) for a country outside the
  // curated table. See numberFormatLocale's doc comment.
  const effectiveLocale = numberFormatLocale(organization?.country_code) ?? locale;
  try {
    return new Intl.NumberFormat(effectiveLocale, {
      style: "currency",
      currency: effectiveCurrency,
      minimumFractionDigits: organization.minimum_fraction_digits,
    }).format(number);
  } catch {
    return new Intl.NumberFormat(effectiveLocale, {
      minimumFractionDigits: organization.minimum_fraction_digits,
    }).format(number);
  }
};
