import isNumber from "lodash/isNumber";

// @ts-expect-error - Intl supportedValuesOf support?
export const currencies = Intl.supportedValuesOf("currency");

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

  return new Intl.NumberFormat(locale, {
    style: "currency",
    currency: currency,
    minimumFractionDigits: organization.minimum_fraction_digits,
  }).format(number);
};
