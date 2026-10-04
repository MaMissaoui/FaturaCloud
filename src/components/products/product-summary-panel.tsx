import type { CSSProperties, ReactNode } from "react";
import { Link } from "react-router";
import { Alert, Button, Skeleton, Space, theme } from "antd";
import { useAtomValue } from "jotai";
import Decimal from "decimal.js";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";

import { GetProductSummary, type ProductMovement, type ProductSummary } from "src/api";
import type { Product, TaxRate } from "src/types/models";
import { myOrgRoleSyncAtom, organizationAtom } from "src/atoms/organization";
import { isRouteAllowedForRole } from "src/layouts/role-menu";
import { formatOrgCents } from "src/utils/currencies";
import { unitLabel } from "src/utils/units";
import { useFetch } from "src/hooks/useFetch";
import { useDateFormatter } from "src/utils/date";
import { formatQuantity, StockTag } from "src/components/products/stock-tag";

// The screen a movement's document opens on, by its kind.
const DOCUMENT_PATH: Record<Exclude<ProductMovement["documentKind"], "">, string> = {
  invoice: "/invoices",
  delivery: "/deliveries",
  receipt: "/inbound-deliveries",
  production: "/production-orders",
};

// ProductSummaryPanel is the Products screen's right-hand panel: what is in
// stock, the price with and without tax, the average cost and the margin it
// leaves, the latest stock movements with the document and counterparty
// behind each, and the product's details. It loads
// GET /api/products/{id}/summary each time the picked product changes.
// Under quantity-only valuation nothing is valued, so cost and margin show
// "—" rather than a figure the organization never tracks.
export default function ProductSummaryPanel({
  product,
  familyName,
  taxRate,
  quantityOnly,
  lowStockThreshold,
  qtyLocale,
  onEdit,
}: {
  product: Product;
  familyName: string | null;
  taxRate: TaxRate | null;
  quantityOnly: boolean;
  lowStockThreshold: number;
  qtyLocale: string;
  onEdit: () => void;
}) {
  const { i18n } = useLingui();
  const { token } = theme.useToken();
  const organization = useAtomValue(organizationAtom);
  const role = useAtomValue(myOrgRoleSyncAtom);
  const formatDate = useDateFormatter();
  const {
    data: summary,
    loading,
    failed,
    reload,
  } = useFetch<ProductSummary | null>([product.id], () => GetProductSummary(product.id), null);

  const money = (cents: number) => formatOrgCents(cents, organization, i18n.locale);
  const secondary: CSSProperties = { color: token.colorTextSecondary };
  const rule: CSSProperties = {
    borderTop: `1px solid ${token.colorBorderSecondary}`,
    paddingTop: 16,
  };
  const stocked = !!product.stockEnabled;
  const unit = product.unit ? unitLabel(product.unit) : "";
  const qty = (n: number) => formatQuantity(n, qtyLocale);

  const grossPrice = taxRate
    ? new Decimal(product.price)
        .mul(new Decimal(100).plus(taxRate.percentage))
        .div(100)
        .toDecimalPlaces(0, Decimal.ROUND_HALF_UP)
        .toNumber()
    : null;
  const cost = quantityOnly ? null : product.unitCost;
  const margin = cost != null && product.price > 0 ? product.price - cost : null;
  const marginPercent =
    margin != null
      ? new Decimal(margin).mul(100).div(product.price).toDecimalPlaces(1).toNumber()
      : null;

  const subtitle = [
    familyName ? t`Family ${familyName}` : "",
    product.sku ? t`code ${product.sku}` : "",
  ]
    .filter(Boolean)
    .join(", ");

  const figures: { label: string; value: ReactNode; note?: string }[] = [
    { label: t`Price excl. tax`, value: money(product.price) },
    {
      label: t`Price incl. tax`,
      value: grossPrice != null ? money(grossPrice) : "—",
      note: taxRate ? t`VAT ${taxRate.percentage} %` : t`No tax rate`,
    },
    { label: t`Average cost`, value: cost != null ? money(cost) : "—" },
    {
      label: t`Margin per unit`,
      value: margin != null ? money(margin) : "—",
      note: marginPercent != null ? `${marginPercent.toLocaleString(i18n.locale)} %` : undefined,
    },
  ];

  const details: { label: string; value: ReactNode }[] = [
    { label: t`Unit`, value: unit || null },
    ...(stocked
      ? [
          {
            label: t`Serial numbers`,
            value: product.serialized ? t`Tracked` : t`Not tracked`,
          },
        ]
      : []),
    {
      label: t`Category`,
      value:
        product.category === "finished"
          ? t`Finished good`
          : product.category === "component"
            ? t`Component`
            : null,
    },
    ...(summary?.lastVendor ? [{ label: t`Last vendor`, value: summary.lastVendor.name }] : []),
  ];

  return (
    <section
      aria-labelledby="product-summary-name"
      style={{ display: "flex", flexDirection: "column", gap: 20 }}
    >
      <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
        <h2
          id="product-summary-name"
          style={{ margin: 0, fontSize: 20, lineHeight: "28px", fontWeight: 600 }}
        >
          {product.name}
        </h2>
        {subtitle && <p style={{ margin: 0, ...secondary }}>{subtitle}</p>}
      </div>

      <Space wrap>
        {product.type === "product" && isRouteAllowedForRole(role, "/purchase-orders") && (
          <Link to="/purchase-orders/new">
            <Button>
              <Trans>New purchase order</Trans>
            </Button>
          </Link>
        )}
        <Button onClick={onEdit}>
          <Trans>Edit</Trans>
        </Button>
      </Space>

      <div style={{ ...rule, display: "flex", flexDirection: "column", gap: 6 }}>
        {stocked ? (
          <>
            <p style={{ margin: 0, ...secondary }}>
              <Trans>In stock</Trans>
            </p>
            <p
              style={{
                margin: 0,
                display: "flex",
                flexWrap: "wrap",
                alignItems: "baseline",
                gap: "4px 10px",
              }}
            >
              <span
                style={{
                  fontSize: 28,
                  lineHeight: 1.1,
                  fontWeight: 600,
                  fontVariantNumeric: "tabular-nums",
                }}
              >
                {qty(product.stockQuantity)}
              </span>
              {unit && <span style={secondary}>{unit}</span>}
              {product.stockQuantity <= lowStockThreshold && (
                <StockTag quantity={product.stockQuantity} threshold={lowStockThreshold} />
              )}
            </p>
          </>
        ) : (
          <p style={{ margin: 0, ...secondary }}>
            {product.type === "service" ? (
              <Trans>A service: no stock is kept.</Trans>
            ) : (
              <Trans>Stock is not tracked for this product.</Trans>
            )}
          </p>
        )}
      </div>

      <dl
        style={{
          margin: 0,
          display: "grid",
          gridTemplateColumns: "repeat(2, minmax(0, 1fr))",
          gap: "14px 16px",
        }}
      >
        {figures.map(({ label, value, note }) => (
          <div key={label} style={{ display: "flex", flexDirection: "column", gap: 2 }}>
            <dt style={{ ...secondary, fontSize: 13 }}>{label}</dt>
            <dd style={{ margin: 0, fontWeight: 500, fontVariantNumeric: "tabular-nums" }}>
              {value}
              {note && <div style={{ ...secondary, fontSize: 12, fontWeight: 400 }}>{note}</div>}
            </dd>
          </div>
        ))}
      </dl>

      {stocked && failed && (
        <Alert
          type="error"
          showIcon
          title={<Trans>The stock movements could not be loaded.</Trans>}
          action={
            <Button size="small" onClick={reload}>
              <Trans>Retry</Trans>
            </Button>
          }
        />
      )}
      {stocked && loading && !summary && <Skeleton active paragraph={{ rows: 4 }} />}

      {stocked && summary && (
        <div style={{ ...rule, display: "flex", flexDirection: "column", gap: 8 }}>
          <h3 style={{ margin: 0, fontSize: 15, fontWeight: 600 }}>
            <Trans>Latest movements</Trans>
          </h3>
          {summary.recentMovements.length === 0 ? (
            <p style={{ margin: 0, ...secondary }}>
              <Trans>No stock movement yet.</Trans>
            </p>
          ) : (
            <table
              style={{
                width: "100%",
                borderCollapse: "collapse",
                fontVariantNumeric: "tabular-nums",
              }}
            >
              <tbody>
                {summary.recentMovements.map((m) => {
                  const base = m.documentKind ? DOCUMENT_PATH[m.documentKind] : null;
                  const label = m.reference || t`Manual movement`;
                  const doc =
                    base && m.documentId && isRouteAllowedForRole(role, base) ? (
                      <Link to={`${base}/${m.documentId}`}>{label}</Link>
                    ) : (
                      label
                    );
                  return (
                    <tr key={m.id}>
                      <td style={{ ...cell(token), whiteSpace: "nowrap", ...secondary }}>
                        {m.date ? formatDate(m.date) : ""}
                      </td>
                      <td style={cell(token)}>
                        {doc}
                        {m.counterpartyName && (
                          <div style={{ ...secondary, fontSize: 12 }}>{m.counterpartyName}</div>
                        )}
                      </td>
                      <td
                        style={{
                          ...cell(token),
                          textAlign: "right",
                          whiteSpace: "nowrap",
                          fontWeight: 500,
                        }}
                      >
                        {m.quantity > 0 ? `+${qty(m.quantity)}` : `−${qty(-m.quantity)}`}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
          {summary.movementCount > summary.recentMovements.length && (
            <p style={{ margin: 0, ...secondary }}>
              {t`${summary.movementCount} movements in total.`}{" "}
              {isRouteAllowedForRole(role, "/inventory") && (
                <Link to="/inventory">
                  <Trans>Open the stock ledger</Trans>
                </Link>
              )}
            </p>
          )}
        </div>
      )}

      <dl
        style={{
          ...rule,
          margin: 0,
          display: "grid",
          gridTemplateColumns: "max-content 1fr",
          gap: "8px 16px",
        }}
      >
        {details
          .filter((row) => row.value)
          .map(({ label, value }) => (
            <div key={label} style={{ display: "contents" }}>
              <dt style={secondary}>{label}</dt>
              <dd style={{ margin: 0, overflowWrap: "anywhere" }}>{value}</dd>
            </div>
          ))}
        {product.description && (
          <div style={{ display: "contents" }}>
            <dt style={secondary}>
              <Trans>Description</Trans>
            </dt>
            <dd style={{ margin: 0, overflowWrap: "anywhere" }}>{product.description}</dd>
          </div>
        )}
      </dl>
    </section>
  );
}

type Token = ReturnType<typeof theme.useToken>["token"];

const cell = (token: Token): CSSProperties => ({
  padding: "8px 6px",
  borderBottom: `1px solid ${token.colorBorderSecondary}`,
  verticalAlign: "top",
});
