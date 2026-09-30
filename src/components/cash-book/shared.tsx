import { useState, type ReactNode } from "react";
import { theme, Typography } from "antd";
import { t } from "@lingui/core/macro";
import type { CashMovementDetail } from "src/api";

// A new customer picked in the "New customer" modal below — kept as local
// draft state, not created via a separate API call, until the whole sale is
// submitted. This is what makes CreateCashSale's client+invoice+payment
// creation genuinely atomic (see db/cash_sale.go's CreateCashSale doc
// comment) rather than a client-creation call followed by a separate sale.
export interface NewClientDraft {
  name: string;
  // Mandatory, like the modal field (a walk-in is identified by phone).
  phone: string;
  phone2?: string;
  phone3?: string;
  address?: string;
  identity_number?: string;
  iban?: string;
  guarantor?: string;
}

// Labels/colors for CashMovementDetail.kind — see
// db/cash_movement_details.go's CashMovementDetail doc comment for what
// each one means and why it's computed from payment history, not the
// invoice's current state.
export const movementKindLabel = (kind: CashMovementDetail["kind"]) => {
  switch (kind) {
    case "sale":
      return t`Sale`;
    case "loan":
      return t`Loan (deposit)`;
    case "repayment":
      return t`Loan repayment`;
    case "withdrawal":
      return t`Withdrawal`;
  }
};
export const movementKindColor = (kind: CashMovementDetail["kind"]) => {
  switch (kind) {
    case "sale":
      return "green";
    case "loan":
      return "gold";
    case "repayment":
      return "blue";
    case "withdrawal":
      return "default";
  }
};

// One-line customer summary — the same identifying fields the search results
// show (customer no., phone, CIN, IBAN, address). Shared by the search list
// and the loan-status customer filter so a cashier can tell two same-named
// customers apart without leaving the report.
export const clientDetailLine = (c: any): string => {
  const address =
    c.address ||
    [
      [c.house_number, c.street].filter(Boolean).join(" "),
      [c.postal_code, c.city].filter(Boolean).join(" "),
    ]
      .filter(Boolean)
      .join(", ");
  return [
    c.code ? `${t`Customer no.`} ${c.code}` : null,
    [c.phone, c.phone2, c.phone3].filter(Boolean).join(" / ") || null,
    c.identity_number ? `${t`CIN`} ${c.identity_number}` : null,
    c.iban,
    address || null,
    c.guarantor ? `${t`Guarantor`}: ${c.guarantor}` : null,
  ]
    .filter(Boolean)
    .join(" · ");
};

// One selectable customer in the Cash Book pick-list. A grid rather than
// List.Item.Meta: the name and its identifying fields on the left, a
// fixed-width right rail for the open-loan amount so the figures line up as a
// real column, and a single row height so a screenful holds ~11 results
// instead of 8. A `listbox` option driven from the search field's arrow keys —
// the row itself isn't focusable (the combobox is the single tab stop).
export const CashBookCustomerRow = ({
  name,
  meta,
  outstanding,
  moneyText,
  active,
  id,
  ariaLabel,
  title,
  onSelect,
  onHover,
}: {
  name: ReactNode;
  meta: ReactNode;
  outstanding: number;
  moneyText: string;
  active: boolean;
  id?: string;
  ariaLabel: string;
  title?: string;
  onSelect: () => void;
  onHover?: () => void;
}) => {
  const {
    token: {
      colorPrimary,
      colorTextSecondary,
      colorWarningText,
      colorBorderSecondary,
      controlItemBgHover,
      controlItemBgActive,
      borderRadius,
    },
  } = theme.useToken();
  const [hovered, setHovered] = useState(false);

  return (
    <div
      id={id}
      role="option"
      aria-selected={active}
      aria-label={ariaLabel}
      title={title}
      onClick={onSelect}
      onMouseEnter={() => {
        setHovered(true);
        onHover?.();
      }}
      onMouseLeave={() => setHovered(false)}
      style={{
        display: "grid",
        gridTemplateColumns: "minmax(0, 1fr) max-content",
        columnGap: 16,
        alignItems: "center",
        minHeight: 56,
        padding: "8px 12px",
        borderBottom: `1px solid ${colorBorderSecondary}`,
        // The grid's left track is minmax(0, 1fr) so this row's own min-width
        // never lets a long name push the money rail off-screen.
        minWidth: 0,
        borderRadius,
        cursor: "pointer",
        transition: "background-color 120ms ease",
        background: active ? controlItemBgActive : hovered ? controlItemBgHover : undefined,
        outline: active ? `2px solid ${colorPrimary}` : undefined,
        outlineOffset: active ? -2 : undefined,
      }}
    >
      <div style={{ minWidth: 0 }}>
        <div
          style={{
            fontSize: 15,
            fontWeight: 600,
            lineHeight: 1.35,
            whiteSpace: "nowrap",
            overflow: "hidden",
            textOverflow: "ellipsis",
          }}
        >
          {name}
        </div>
        <div
          style={{
            display: "flex",
            flexWrap: "wrap",
            gap: 8,
            marginTop: 2,
            fontSize: 13,
            lineHeight: 1.4,
            color: colorTextSecondary,
          }}
        >
          {meta}
        </div>
      </div>
      {/* The rail keeps its width even with nothing to show, so the amounts
          above and below it stay in one column; a zero balance renders
          nothing rather than "0,00". */}
      <div
        style={{
          minWidth: 132,
          display: "flex",
          flexDirection: "column",
          alignItems: "flex-end",
          gap: 2,
        }}
      >
        {outstanding > 0 ? (
          <>
            <span style={{ fontSize: 11, letterSpacing: ".02em", color: colorTextSecondary }}>
              {t`Open loan`}
            </span>
            <span
              style={{
                fontSize: 15,
                fontWeight: 600,
                color: colorWarningText,
                fontVariantNumeric: "tabular-nums",
              }}
            >
              {moneyText}
            </span>
          </>
        ) : null}
      </div>
    </div>
  );
};

// The identifiers that actually tell two same-named customers apart, in
// priority order: the customer number (the organization's own unique key),
// then the mobile number, then the CIN. IBAN/address/guarantor stay available
// through the row's title tooltip rather than competing for the one line.
export const customerIdentifiers = (c: any, highlight: (text: string) => ReactNode): ReactNode => (
  <>
    {c.code ? (
      <Typography.Text code style={{ fontSize: 12 }}>
        {String(c.code)}
      </Typography.Text>
    ) : null}
    {[c.phone, c.phone2, c.phone3].filter(Boolean).length ? (
      <span style={{ fontVariantNumeric: "tabular-nums", fontWeight: 500 }}>
        {highlight([c.phone, c.phone2, c.phone3].filter(Boolean).join(" / "))}
      </span>
    ) : null}
    {c.identity_number ? (
      <span>
        {t`CIN`} {highlight(String(c.identity_number))}
      </span>
    ) : null}
  </>
);
