import Decimal from "decimal.js";

// Configure Decimal.js for financial calculations
Decimal.set({
  precision: 20,
  rounding: Decimal.ROUND_HALF_UP,
});

/**
 * Convert cents to currency units (e.g., dollars)
 * @param cents - Amount in cents (smallest currency unit)
 * @param precision - Number of decimal places (default: 2)
 * @returns Amount in currency units
 */
export function centsToUnits(cents: number, precision: number = 2): number {
  return divideDecimal(cents, Math.pow(10, precision));
}

/**
 * Convert currency units to cents
 * @param units - Amount in currency units (e.g., dollars)
 * @param precision - Number of decimal places (default: 2)
 * @returns Amount in cents (smallest currency unit)
 */
export function unitsToCents(units: number, precision: number = 2): number {
  return Math.round(multiplyDecimal(units, Math.pow(10, precision)));
}

/**
 * Format cents as currency string
 * @param cents - Amount in cents
 * @param currency - Currency code (e.g., 'USD')
 * @param locale - Locale for formatting
 * @returns Formatted currency string
 */
// Several CLDR locales (fr-*, and others) legitimately use a narrow/thin
// no-break space (U+202F, occasionally U+2009) as Intl.NumberFormat's
// thousands grouping separator — real, correct Unicode. But it's a
// reproducible browser rendering bug (confirmed live in this app: same
// font/size, only the text color differs) that collapses it to zero
// visible width in some contexts (e.g. antd's colorError red used for
// overdue/outstanding amounts), making a grouped amount look ungrouped.
// Normalizing to an ordinary space sidesteps the bug entirely. Mirrors
// src/utils/currencies.tsx's identical helper — deliberately not shared
// across the two files for one regex line, to avoid coupling them.
const normalizeGroupingSpace = (s: string) => s.replace(/[  ]/g, " ");

export function formatCents(cents: number, currency: string, locale: string): string {
  const units = centsToUnits(cents);
  // A blank/invalid currency code must never crash the caller — see
  // getFormattedNumber in src/utils/currencies.tsx for the same guard and
  // why (Intl.NumberFormat throws a RangeError otherwise).
  try {
    return normalizeGroupingSpace(
      new Intl.NumberFormat(locale, {
        style: "currency",
        currency: currency,
      }).format(units),
    );
  } catch {
    return normalizeGroupingSpace(new Intl.NumberFormat(locale).format(units));
  }
}

/**
 * Multiply two numbers with precise decimal arithmetic
 * @param a - First number
 * @param b - Second number
 * @returns Result as a number
 */
export function multiplyDecimal(a: number | string, b: number | string): number {
  return new Decimal(a).times(b).toNumber();
}

/**
 * Divide two numbers with precise decimal arithmetic
 * @param a - Dividend
 * @param b - Divisor
 * @returns Result as a number
 */
export function divideDecimal(a: number | string, b: number | string): number {
  return new Decimal(a).div(b).toNumber();
}

/**
 * Add two numbers with precise decimal arithmetic
 * @param a - First number
 * @param b - Second number
 * @returns Result as a number
 */
export function addDecimal(a: number | string, b: number | string): number {
  return new Decimal(a).plus(b).toNumber();
}

/**
 * Subtract two numbers with precise decimal arithmetic
 * @param a - First number
 * @param b - Second number
 * @returns Result as a number
 */
export function subtractDecimal(a: number | string, b: number | string): number {
  return new Decimal(a).minus(b).toNumber();
}

/**
 * Calculate tax amount with precise decimal arithmetic.
 *
 * Deliberately always rounds to 2 decimal places regardless of the
 * organization's configured "Decimal places" (minimum_fraction_digits) or
 * the invoice's currency (e.g. JPY): storage stays cents (× 100) everywhere,
 * always — decimals are a display-only concern, applied by the formatter at
 * render time, never by the arithmetic that produces the stored total. This
 * mirrors db/invoice_totals.go's ratToCents on the Go side, which the server
 * cross-checks this against; changing this would desync the two.
 * @param amount - Base amount
 * @param percentage - Tax percentage (e.g., 20 for 20%)
 * @returns Tax amount as a number
 */
export function calculateTax(amount: number | string, percentage: number | string): number {
  return new Decimal(amount)
    .times(percentage)
    .div(100)
    .toDecimalPlaces(2, Decimal.ROUND_HALF_UP)
    .toNumber();
}

/**
 * Back out the tax-exclusive (net) unit price from a tax-inclusive (gross)
 * one, rounded to 2 decimal places like every other stored price. Used by
 * gross-price entry screens (Cash Book) to convert what a cashier types
 * into the net unitPrice CreateCashSaleRequest — and every other
 * document's line items — actually stores. Round the result once here and
 * reuse it everywhere downstream (totals math, the submitted payload):
 * recomputing from the unrounded value in one place and the rounded cents
 * in another drifts apart once quantity amplifies the sub-cent gap, and
 * db/invoice_totals.go's validateInvoiceTotals requires an exact match.
 * @param gross - Tax-inclusive unit price
 * @param percentage - Tax percentage (e.g., 19 for 19%)
 */
export function netFromGross(gross: number | string, percentage: number | string): number {
  const pct = new Decimal(percentage || 0);
  const net = pct.isZero() ? new Decimal(gross) : new Decimal(gross).div(pct.div(100).plus(1));
  return net.toDecimalPlaces(2, Decimal.ROUND_HALF_UP).toNumber();
}

/**
 * The inverse of netFromGross — used to prefill a gross-price field from a
 * product's stored net price when it's selected on a gross-price screen.
 * @param net - Tax-exclusive unit price
 * @param percentage - Tax percentage (e.g., 19 for 19%)
 */
export function grossFromNet(net: number | string, percentage: number | string): number {
  const pct = new Decimal(percentage || 0);
  const gross = pct.isZero() ? new Decimal(net) : new Decimal(net).times(pct.div(100).plus(1));
  return gross.toDecimalPlaces(2, Decimal.ROUND_HALF_UP).toNumber();
}
