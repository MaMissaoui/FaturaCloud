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
export function formatCents(cents: number, currency: string, locale: string): string {
  const units = centsToUnits(cents);
  return new Intl.NumberFormat(locale, {
    style: "currency",
    currency: currency,
  }).format(units);
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
