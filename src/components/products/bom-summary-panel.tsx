import type { CSSProperties, ReactNode } from "react";
import { useNavigate } from "react-router";
import { Alert, Button, Skeleton, Space, Tag, theme } from "antd";
import { useAtomValue } from "jotai";
import Decimal from "decimal.js";
import { Trans } from "@lingui/react/macro";
import { plural, t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";

import { GetBOMRecipeDetail, type BOMRecipeDetail } from "src/api";
import type { Product } from "src/types/models";
import { myOrgRoleSyncAtom, organizationAtom } from "src/atoms/organization";
import { themeAtom } from "src/atoms/generic";
import { isRouteAllowedForRole } from "src/layouts/role-menu";
import { formatOrgCents } from "src/utils/currencies";
import { unitLabel } from "src/utils/units";
import { useFetch } from "src/hooks/useFetch";
import { lateTextColor } from "src/components/master-data/aging";
import { formatQuantity } from "src/components/products/stock-tag";

// BomSummaryPanel is the Bill of Materials screen's right-hand panel: how many
// units the stock on hand allows and which part limits it, what the parts of
// one unit cost against the sale price, and every part with its stock and
// cost. It loads GET /api/products/{id}/bom/summary, again whenever
// reloadKey changes (a saved recipe). Quantities are for one finished unit,
// whatever batch size the recipe was entered at.
export default function BomSummaryPanel({
  product,
  qtyLocale,
  reloadKey,
  onEditRecipe,
}: {
  product: Product;
  qtyLocale: string;
  reloadKey: number;
  onEditRecipe: () => void;
}) {
  const { i18n } = useLingui();
  const { token } = theme.useToken();
  const navigate = useNavigate();
  const organization = useAtomValue(organizationAtom);
  const role = useAtomValue(myOrgRoleSyncAtom);
  const dark = useAtomValue(themeAtom) === "dark";
  const {
    data: recipe,
    loading,
    failed,
    reload,
  } = useFetch<BOMRecipeDetail | null>(
    [product.id, reloadKey],
    () => GetBOMRecipeDetail(product.id),
    null,
  );

  const money = (cents: number) => formatOrgCents(cents, organization, i18n.locale);
  const qty = (n: number) => formatQuantity(n, qtyLocale);
  const secondary: CSSProperties = { color: token.colorTextSecondary };
  const warm: CSSProperties = { color: lateTextColor(dark), fontWeight: 600 };
  const rule: CSSProperties = {
    borderTop: `1px solid ${token.colorBorderSecondary}`,
    paddingTop: 16,
  };

  const subtitle = [
    product.sku ? t`Code ${product.sku}` : "",
    recipe?.versionNumber ? t`recipe version ${recipe.versionNumber}` : "",
    recipe && recipe.componentCount > 0 ? t`quantities for one unit` : "",
  ]
    .filter(Boolean)
    .join(", ");

  // Only where CreateProductionOrder would accept the order: a recipe, a
  // stock-tracked product and no serialized component. A short or uncosted
  // part still allows a draft order, refused only on completion.
  const canProduce =
    !!recipe &&
    recipe.componentCount > 0 &&
    !!product.stockEnabled &&
    recipe.blockedReason !== "serialized-component" &&
    isRouteAllowedForRole(role, "/production-orders");

  const header = (
    <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
      <h2
        id="bom-summary-name"
        style={{ margin: 0, fontSize: 20, lineHeight: "28px", fontWeight: 600 }}
      >
        {product.name}
      </h2>
      {subtitle && <p style={{ margin: 0, ...secondary }}>{subtitle}</p>}
    </div>
  );

  const actions = (
    <Space wrap>
      {canProduce && (
        <Button
          onClick={() =>
            navigate("/production-orders/new", { state: { finishedProductId: product.id } })
          }
        >
          <Trans>Start production</Trans>
        </Button>
      )}
      <Button onClick={onEditRecipe}>
        {recipe && recipe.componentCount === 0 ? (
          <Trans>Create the recipe</Trans>
        ) : (
          <Trans>Edit the recipe</Trans>
        )}
      </Button>
    </Space>
  );

  if (failed) {
    return (
      <section aria-labelledby="bom-summary-name" style={column(20)}>
        {header}
        <Alert
          type="error"
          showIcon
          title={<Trans>The recipe could not be loaded.</Trans>}
          action={
            <Button size="small" onClick={reload}>
              <Trans>Retry</Trans>
            </Button>
          }
        />
      </section>
    );
  }
  if (!recipe) {
    return (
      <section aria-labelledby="bom-summary-name" style={column(20)}>
        {header}
        {loading && <Skeleton active paragraph={{ rows: 6 }} />}
      </section>
    );
  }

  if (recipe.componentCount === 0) {
    return (
      <section aria-labelledby="bom-summary-name" style={column(20)}>
        {header}
        <p style={{ margin: 0, ...secondary }}>
          <Trans>
            No recipe yet. Add the parts one unit is made of to see what it costs and how many you
            can build.
          </Trans>
        </p>
        {actions}
      </section>
    );
  }

  const unit = product.unit ? unitLabel(product.unit) : "";
  const limitLine = recipe.lines.find((l) => l.componentProductId === recipe.limitingComponentId);
  const limitName = recipe.limitingComponentName ?? "";
  const finishedNote =
    recipe.finishedStock > 0
      ? t`Finished units in stock: ${qty(recipe.finishedStock)}.`
      : t`No finished units in stock.`;

  let reason: ReactNode;
  switch (recipe.blockedReason) {
    case "":
      reason = limitLine
        ? t`Limited by ${limitName}, ${qty(limitLine.stockQuantity)} in stock.`
        : null;
      break;
    case "short":
      reason = t`Not enough ${limitName} in stock for one unit.`;
      break;
    case "stock-not-tracked":
      reason = t`Stock isn't tracked for this product, so a production order has nothing to produce into.`;
      break;
    case "serialized-component":
      reason = t`${limitName} is tracked by serial number, which a production order can't consume yet.`;
      break;
    case "uncosted-component":
      reason = t`${limitName} has no cost yet, so a production order can't value what it uses.`;
      break;
  }

  const price = product.price;
  const margin = recipe.cost != null ? price - recipe.cost : null;
  const marginPercent =
    margin != null && price > 0
      ? new Decimal(margin).mul(100).div(price).toDecimalPlaces(1).toNumber()
      : null;
  const figures: { label: string; value: ReactNode; note?: string; loss?: boolean }[] = [
    { label: t`Parts cost`, value: recipe.cost != null ? money(recipe.cost) : "—" },
    { label: t`Sale price excl. tax`, value: price > 0 ? money(price) : "—" },
    {
      label: t`Margin per unit`,
      value: margin != null && price > 0 ? money(margin) : "—",
      note:
        marginPercent != null
          ? `${marginPercent.toLocaleString(i18n.locale)} %`
          : recipe.cost == null && recipe.inventoryValuation !== "quantity_only"
            ? t`A part has no cost yet`
            : undefined,
      loss: recipe.belowCost,
    },
  ];

  return (
    <section aria-labelledby="bom-summary-name" style={column(20)}>
      {header}
      {actions}

      <div style={{ ...rule, ...column(6) }}>
        <p style={{ margin: 0, ...secondary }}>
          <Trans>Buildable now</Trans>
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
              ...(recipe.buildable === 0 ? { color: lateTextColor(dark) } : {}),
            }}
          >
            {recipe.buildable === 0 ? <Trans>None</Trans> : recipe.buildable}
          </span>
          {recipe.buildable > 0 && (
            <span style={secondary}>
              {unit || plural(recipe.buildable, { one: "unit", other: "units" })}
            </span>
          )}
        </p>
        <p style={{ margin: 0, ...secondary }}>
          {reason} {product.stockEnabled ? finishedNote : null}
        </p>
      </div>

      <dl
        style={{
          margin: 0,
          display: "grid",
          gridTemplateColumns: "repeat(3, minmax(0, 1fr))",
          gap: "14px 16px",
        }}
      >
        {figures.map(({ label, value, note, loss }) => (
          <div key={label} style={column(2)}>
            <dt style={{ ...secondary, fontSize: 13 }}>{label}</dt>
            <dd
              style={{
                margin: 0,
                fontWeight: 500,
                fontVariantNumeric: "tabular-nums",
                ...(loss ? warm : {}),
              }}
            >
              {value}
              {note && (
                <div
                  style={{
                    fontSize: 12,
                    fontWeight: 400,
                    ...(loss ? { color: lateTextColor(dark) } : secondary),
                  }}
                >
                  {note}
                </div>
              )}
            </dd>
          </div>
        ))}
      </dl>

      <div style={{ ...rule, ...column(8) }}>
        <h3 style={{ margin: 0, fontSize: 15, fontWeight: 600 }}>
          <Trans>Parts for one unit</Trans>
        </h3>
        <table
          style={{ width: "100%", borderCollapse: "collapse", fontVariantNumeric: "tabular-nums" }}
        >
          <thead>
            <tr>
              <th style={{ ...headCell(token), textAlign: "left" }}>
                <Trans>Part</Trans>
              </th>
              <th style={{ ...headCell(token), textAlign: "right" }}>
                <Trans>Qty</Trans>
              </th>
              <th style={{ ...headCell(token), textAlign: "right" }}>
                <Trans>In stock</Trans>
              </th>
              {recipe.inventoryValuation !== "quantity_only" && (
                <th style={{ ...headCell(token), textAlign: "right" }}>
                  <Trans>Cost</Trans>
                </th>
              )}
            </tr>
          </thead>
          <tbody>
            {recipe.lines.map((line) => {
              const limiting = line.componentProductId === recipe.limitingComponentId;
              const rowStyle: CSSProperties = limiting ? { background: token.colorWarningBg } : {};
              return (
                <tr key={line.componentProductId} style={rowStyle}>
                  <td style={cell(token)}>
                    <span style={{ overflowWrap: "anywhere" }}>{line.componentName}</span>
                    {limiting && (
                      <Tag color="warning" style={{ marginInlineStart: 8, marginInlineEnd: 0 }}>
                        <Trans>Limit</Trans>
                      </Tag>
                    )}
                  </td>
                  <td style={{ ...cell(token), textAlign: "right", whiteSpace: "nowrap" }}>
                    {qty(line.quantityPerUnit)}
                    {line.componentUnit && (
                      <span style={secondary}> {unitLabel(line.componentUnit)}</span>
                    )}
                  </td>
                  <td
                    style={{
                      ...cell(token),
                      textAlign: "right",
                      whiteSpace: "nowrap",
                      ...(line.buildable === 0 ? warm : {}),
                    }}
                  >
                    {qty(line.stockQuantity)}
                  </td>
                  {recipe.inventoryValuation !== "quantity_only" && (
                    <td style={{ ...cell(token), textAlign: "right", whiteSpace: "nowrap" }}>
                      {line.lineCost != null ? money(line.lineCost) : "—"}
                    </td>
                  )}
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </section>
  );
}

const column = (gap: number): CSSProperties => ({ display: "flex", flexDirection: "column", gap });

type Token = ReturnType<typeof theme.useToken>["token"];

const cell = (token: Token): CSSProperties => ({
  padding: "8px 6px",
  borderBottom: `1px solid ${token.colorBorderSecondary}`,
  verticalAlign: "top",
});

const headCell = (token: Token): CSSProperties => ({
  padding: "0 6px 6px",
  borderBottom: `1px solid ${token.colorBorderSecondary}`,
  color: token.colorTextSecondary,
  fontSize: 13,
  fontWeight: 400,
  whiteSpace: "nowrap",
});
