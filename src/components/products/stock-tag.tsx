import type { ReactNode } from "react";
import { Tag } from "antd";
import { t } from "@lingui/core/macro";

// Stock quantities are displayed with the organization's country-derived
// locale (falling back to the viewer's UI language) so a fractional value
// renders "2,5" not "2.5" on French/German organizations; whole numbers stay
// clean ("5") and fractions are capped at 2 decimals.
export const formatQuantity = (qty: number, locale: string) =>
  new Intl.NumberFormat(locale, {
    minimumFractionDigits: 0,
    maximumFractionDigits: 2,
  }).format(qty);

// The out-of-stock colour: the aging ramp's darkest step (aging.ts), solid,
// so the white label reads in light and dark mode alike.
const OUT_OF_STOCK = "#7A2E0E";

// StockTag marks a stock-tracked product that is out of stock or running low
// (at or below the low-stock threshold, the Dashboard's rule). children
// replaces the default "N or less" label of a low product, e.g. with its
// quantity in a list cell.
export function StockTag({
  quantity,
  threshold,
  children,
}: {
  quantity: number;
  threshold: number;
  children?: ReactNode;
}) {
  if (quantity <= 0) {
    return (
      <Tag color={OUT_OF_STOCK} variant="solid" style={{ marginInlineEnd: 0 }}>
        {t`Out of stock`}
      </Tag>
    );
  }
  if (quantity <= threshold) {
    return (
      <Tag color="orange" style={{ marginInlineEnd: 0 }}>
        {children ?? t`${threshold} or less`}
      </Tag>
    );
  }
  return <>{children}</>;
}
