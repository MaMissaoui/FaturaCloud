import { useMemo, useState } from "react";
import { Link, useNavigate } from "react-router";
import {
  Alert,
  Badge,
  Button,
  Col,
  Empty,
  Row,
  Table,
  Tag,
  theme,
  Tooltip,
  Typography,
} from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import Decimal from "decimal.js";
import { Trans } from "@lingui/react/macro";
import { plural, t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { BuildOutlined, PlusOutlined } from "@ant-design/icons";

import type { Product } from "src/types/models";
import { organizationAtom, organizationIdAtom } from "src/atoms/organization";
import { productsAtom, setProductsAtom } from "src/atoms/product";
import { themeAtom } from "src/atoms/generic";
import { GetBOMOverview, GetBOMSummaries, type BOMOverview, type BOMRecipeSummary } from "src/api";
import BOMEditorDrawer from "src/components/products/bom-editor-drawer";
import BomSummaryPanel from "src/components/products/bom-summary-panel";
import FilterChips from "src/components/master-data/filter-chips";
import ListWithPanel from "src/components/master-data/list-with-panel";
import { lateTextColor } from "src/components/master-data/aging";
import { useSummariesEnabled } from "src/components/master-data/use-summaries-enabled";
import { useWideScreen } from "src/components/master-data/use-wide-screen";
import PageHeader from "src/components/page-header";
import { formatOrgCents, numberFormatLocale } from "src/utils/currencies";
import { unitLabel } from "src/utils/units";
import { useFetch } from "src/hooks/useFetch";

type RecipeFilter = "all" | "below" | "blocked" | "none";

// A focused surface for maintaining a finished product's recipe — the
// product edit drawer (src/components/products/form.tsx) still has the
// same "Bill of Materials" card for editing one while already in that
// record, but this is the one place to see every finished product's
// recipe status at a glance (which ones still have none defined, most
// useful right after a batch of new finished goods is created) and jump
// straight into editing without opening the full product form first.
//
// With master data summaries on (admin/power_user, the only roles with this
// screen), each recipe also shows its parts cost against the sale price and
// how many units the stock on hand allows, and a picked recipe opens
// BomSummaryPanel beside the list.
const BillOfMaterials = () => {
  const { i18n } = useLingui();
  const { token } = theme.useToken();
  const navigate = useNavigate();
  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);
  const dark = useAtomValue(themeAtom) === "dark";
  const products = useAtomValue(productsAtom);
  const setProducts = useSetAtom(setProductsAtom);

  const [search, setSearch] = useState("");
  const [chip, setChip] = useState<RecipeFilter>("all");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  // Bumped after a recipe is saved, so the open panel reloads too.
  const [panelReload, setPanelReload] = useState(0);
  const qtyLocale = numberFormatLocale(organization?.country_code) ?? i18n.locale;

  const finishedProducts = useMemo(
    () => products.filter((p) => p.category === "finished"),
    [products],
  );

  // Client-side filter — the full finished-product list is already loaded
  // via productsAtom (same source the "New recipe" picker and the Products
  // page's component/finished pickers already rely on), so a search box
  // here doesn't need its own server round trip the way Products' paginated
  // list does.
  const filteredProducts = useMemo(() => {
    const term = search.trim().toLowerCase();
    if (!term) return finishedProducts;
    return finishedProducts.filter(
      (p) => p.name.toLowerCase().includes(term) || (p.sku ?? "").toLowerCase().includes(term),
    );
  }, [finishedProducts, search]);

  const wide = useWideScreen();

  // Summaries on: the products and the overview load together. A failed
  // overview (the setting switched off meanwhile) falls back to the plain
  // list below rather than showing an error.
  const summariesEnabled = useSummariesEnabled("bill-of-materials");
  const {
    data: overview,
    failed: overviewFailed,
    reload: reloadOverview,
  } = useFetch<BOMOverview | null>(
    summariesEnabled && organizationId ? [organizationId] : null,
    () => Promise.all([setProducts(), GetBOMOverview(organizationId!)]).then(([, o]) => o),
    null,
  );
  const showSummaries = summariesEnabled && !overviewFailed;

  // A failed summaries fetch used to leave summaries as {} and let every
  // finished product fall through to the "None / no recipe defined yet"
  // warning badge — telling the user correct recipes are missing (the F85
  // failure mode). Tracked separately so a failed load shows a real error
  // instead of a per-row false claim, while a genuinely-zero-count product
  // still keeps its true empty-state badge.
  const {
    data: summaries,
    loading,
    failed,
    reload: refresh,
  } = useFetch<Record<string, number>>(
    !showSummaries && organizationId ? [organizationId] : null,
    () =>
      Promise.all([setProducts(), GetBOMSummaries(organizationId!)]).then(([, rows]) =>
        Object.fromEntries(rows.map((r) => [r.finishedProductId, r.componentCount])),
      ),
    {},
  );

  const recipeById = useMemo(
    () => new Map((overview?.recipes ?? []).map((r) => [r.productId, r])),
    [overview],
  );

  const margin = (p: Product, r: BOMRecipeSummary | undefined) =>
    r?.cost != null && p.price > 0 ? p.price - r.cost : null;

  const shown = useMemo(() => {
    if (!showSummaries) return filteredProducts;
    const recipe = (p: Product) => recipeById.get(p.id);
    switch (chip) {
      case "below":
        return filteredProducts
          .filter((p) => recipe(p)?.belowCost)
          .sort((a, b) => (margin(a, recipe(a)) ?? 0) - (margin(b, recipe(b)) ?? 0));
      case "blocked":
        return filteredProducts.filter((p) => {
          const r = recipe(p);
          return !!r && r.componentCount > 0 && r.buildable === 0;
        });
      case "none":
        return filteredProducts.filter((p) => recipe(p)?.blockedReason === "no-recipe");
      default:
        return filteredProducts;
    }
  }, [showSummaries, chip, filteredProducts, recipeById]);

  const openEditor = (productId?: string) =>
    navigate("/bill-of-materials", { state: { bomModal: true, productId } });
  const pick = (record: Product) =>
    showSummaries ? setSelectedId(record.id) : openEditor(record.id);
  const onSaved = () => {
    if (showSummaries) {
      reloadOverview();
      setPanelReload((n) => n + 1);
    } else {
      refresh();
    }
  };

  const rowProps = (record: Product) => ({
    onClick: () => pick(record),
    onKeyDown: (e: React.KeyboardEvent) => {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        pick(record);
      }
    },
    style: { cursor: "pointer" },
    tabIndex: 0,
    role: "button",
    "aria-pressed": showSummaries ? record.id === selectedId : undefined,
  });

  const newRecipeButton = (
    <Button type="primary" icon={<PlusOutlined />} onClick={() => openEditor()}>
      <Trans>New recipe</Trans>
    </Button>
  );

  // Summaries off: the plain list, as before the redesign.
  if (!showSummaries) {
    return (
      <>
        <PageHeader
          icon={<BuildOutlined />}
          title={<Trans>Bill of Materials</Trans>}
          search={{
            placeholder: t`Search by name or SKU`,
            value: search,
            onChange: setSearch,
            allowClear: true,
          }}
          actions={newRecipeButton}
        />

        {failed && (
          <Alert
            style={{ marginTop: 16 }}
            type="error"
            showIcon
            message={<Trans>Couldn't load the bill-of-materials summaries</Trans>}
            action={
              <Button size="small" onClick={refresh}>
                <Trans>Retry</Trans>
              </Button>
            }
          />
        )}

        <Row style={{ marginTop: 16 }}>
          <Col span={24}>
            <Table
              dataSource={shown}
              rowKey="id"
              loading={loading}
              pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
              locale={{
                emptyText: search ? (
                  <Trans>No finished-good products match "{search}"</Trans>
                ) : (
                  <Trans>
                    No finished-good products yet — set a product's category to "Finished good" to
                    define a recipe for it.
                  </Trans>
                ),
              }}
              onRow={rowProps}
            >
              <Table.Column
                title={<Trans>Name</Trans>}
                dataIndex="name"
                key="name"
                sorter={(a: Product, b: Product) => (a.name ?? "").localeCompare(b.name ?? "")}
                defaultSortOrder="ascend"
                render={(name: string, record: Product) => (
                  <Link
                    to="/bill-of-materials"
                    state={{ bomModal: true, productId: record.id }}
                    onClick={(e) => e.stopPropagation()}
                  >
                    {name}
                  </Link>
                )}
              />
              <Table.Column
                title={<Trans>SKU</Trans>}
                dataIndex="sku"
                key="sku"
                sorter={(a: Product, b: Product) => (a.sku ?? "").localeCompare(b.sku ?? "")}
              />
              <Table.Column
                title={<Trans>Unit</Trans>}
                dataIndex="unit"
                sorter={(a: Product, b: Product) => (a.unit ?? "").localeCompare(b.unit ?? "")}
                key="unit"
                render={(unit: string | null) => (unit ? unitLabel(unit) : "—")}
              />
              <Table.Column
                title={<Trans>Components</Trans>}
                sorter={(a: Product, b: Product) => (summaries[a.id] ?? 0) - (summaries[b.id] ?? 0)}
                key="components"
                align="center"
                render={(p: Product) => {
                  // A failed summaries load must not read as "this product has
                  // no recipe" — show nothing rather than the warning badge.
                  if (failed) return "—";
                  const count = summaries[p.id] ?? 0;
                  return count > 0 ? (
                    <Badge status="success" text={count} />
                  ) : (
                    <Tooltip title={t`No recipe defined yet`}>
                      <Badge status="warning" text={t`None`} />
                    </Tooltip>
                  );
                }}
              />
            </Table>
          </Col>
        </Row>
        <BOMEditorDrawer onSaved={onSaved} />
      </>
    );
  }

  const money = (cents: number) => formatOrgCents(cents, organization, i18n.locale);
  const warm = { color: lateTextColor(dark), fontWeight: 600 };
  const selected = selectedId ? (finishedProducts.find((p) => p.id === selectedId) ?? null) : null;

  // What needs acting on, in a sentence each: the worst-selling recipe, and
  // the part that blocks the unbuildable ones when it is the same part.
  const belowCost = overview?.belowCost ?? 0;
  const notBuildable = overview?.notBuildable ?? 0;
  const worst = finishedProducts
    .map((p) => ({ p, m: margin(p, recipeById.get(p.id)) }))
    .filter(({ p, m }) => m != null && recipeById.get(p.id)?.belowCost)
    .sort((a, b) => (a.m ?? 0) - (b.m ?? 0))[0];
  const blocked = (overview?.recipes ?? []).filter(
    (r) => r.componentCount > 0 && r.buildable === 0,
  );
  const sharedShortPart =
    blocked.length > 0 &&
    blocked.every(
      (r) =>
        r.blockedReason === "short" && r.limitingComponentId === blocked[0].limitingComponentId,
    )
      ? blocked[0].limitingComponentName
      : null;

  const findings: string[] = [];
  if (belowCost > 0) {
    findings.push(
      plural(belowCost, {
        one: "# product sells for less than its parts cost.",
        other: "# products sell for less than their parts cost.",
      }) +
        (worst?.m != null
          ? " " + t`${worst.p.name} loses ${money(-worst.m)} on each unit sold.`
          : ""),
    );
  }
  if (notBuildable > 0) {
    findings.push(
      sharedShortPart
        ? plural(notBuildable, {
            one: `# product can't be built: there isn't enough ${sharedShortPart} left.`,
            other: `# products can't be built: there isn't enough ${sharedShortPart} left.`,
          })
        : plural(notBuildable, {
            one: "# product can't be built with the stock on hand.",
            other: "# products can't be built with the stock on hand.",
          }),
    );
  }

  const emptyText = search ? (
    <Empty description={<Trans>No finished products match your search</Trans>} />
  ) : (
    <Empty
      description={
        <Trans>
          No finished products yet. Set a product's category to "Finished good" to give it a recipe.
        </Trans>
      }
    />
  );

  const list = (
    <Table
      dataSource={shown}
      rowKey="id"
      loading={!overview}
      pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
      locale={{ emptyText }}
      onRow={rowProps}
      rowClassName={(record: Product) =>
        record.id === selectedId ? "master-data-selected-row" : ""
      }
      size="middle"
      scroll={{ x: "max-content" }}
    >
      <Table.Column
        title={<Trans>Finished product</Trans>}
        key="name"
        sorter={(a: Product, b: Product) => (a.name ?? "").localeCompare(b.name ?? "")}
        defaultSortOrder="ascend"
        render={(p: Product) => (
          <Tooltip title={p.name}>
            <Typography.Link
              ellipsis
              style={{ maxWidth: 240 }}
              onClick={(e) => {
                e.stopPropagation();
                pick(p);
              }}
            >
              {p.name}
            </Typography.Link>
          </Tooltip>
        )}
      />
      <Table.Column
        title={<Trans>Parts cost</Trans>}
        key="cost"
        align="right"
        sorter={(a: Product, b: Product) =>
          (recipeById.get(a.id)?.cost ?? 0) - (recipeById.get(b.id)?.cost ?? 0)
        }
        render={(p: Product) => {
          const r = recipeById.get(p.id);
          if (r?.blockedReason === "no-recipe") {
            return <Tag color="warning">{t`No recipe`}</Tag>;
          }
          return (
            <span style={{ whiteSpace: "nowrap" }}>{r?.cost != null ? money(r.cost) : "—"}</span>
          );
        }}
      />
      {/* Only from 1440px, where the panel beside the list leaves room; the margin carries the comparison and the panel shows the price. */}
      {wide && (
        <Table.Column
          title={<Trans>Sale price</Trans>}
          key="price"
          align="right"
          sorter={(a: Product, b: Product) => a.price - b.price}
          render={(p: Product) => (
            <span style={{ whiteSpace: "nowrap" }}>{p.price > 0 ? money(p.price) : "—"}</span>
          )}
        />
      )}
      <Table.Column
        title={<Trans>Margin</Trans>}
        key="margin"
        align="right"
        sorter={(a: Product, b: Product) =>
          (margin(a, recipeById.get(a.id)) ?? 0) - (margin(b, recipeById.get(b.id)) ?? 0)
        }
        render={(p: Product) => {
          const r = recipeById.get(p.id);
          const m = margin(p, r);
          if (m == null) return "—";
          const percent = new Decimal(m).mul(100).div(p.price).toDecimalPlaces(1).toNumber();
          return (
            <span style={{ whiteSpace: "nowrap", ...(r?.belowCost ? warm : {}) }}>
              {`${percent.toLocaleString(i18n.locale)} %`}
            </span>
          );
        }}
      />
      <Table.Column
        title={<Trans>Buildable</Trans>}
        key="buildable"
        align="right"
        fixed="right"
        sorter={(a: Product, b: Product) =>
          (recipeById.get(a.id)?.buildable ?? 0) - (recipeById.get(b.id)?.buildable ?? 0)
        }
        render={(p: Product) => {
          const r = recipeById.get(p.id);
          if (!r || r.blockedReason === "no-recipe") return "";
          return r.buildable === 0 ? (
            <span style={{ whiteSpace: "nowrap", ...warm }}>
              <Trans>None</Trans>
            </span>
          ) : (
            <span style={{ fontWeight: 500, fontVariantNumeric: "tabular-nums" }}>
              {r.buildable.toLocaleString(qtyLocale)}
            </span>
          );
        }}
      />
    </Table>
  );

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
      <PageHeader
        icon={<BuildOutlined />}
        title={<Trans>Bill of Materials</Trans>}
        search={{
          placeholder: t`Name or code`,
          value: search,
          onChange: setSearch,
          allowClear: true,
        }}
        actions={newRecipeButton}
      />
      {overview && (
        <p style={{ margin: "-12px 0 0", color: token.colorTextSecondary }}>
          {t`${plural(overview.finishedCount, {
            one: "# finished product",
            other: "# finished products",
          })}, ${plural(overview.recipeCount, {
            one: "# with a recipe",
            other: "# with a recipe",
          })}`}
        </p>
      )}

      {findings.length > 0 && (
        <div
          style={{
            background: token.colorWarningBg,
            borderRadius: token.borderRadius,
            padding: "14px 16px",
            display: "flex",
            flexDirection: "column",
            gap: 6,
            maxWidth: 880,
          }}
        >
          {findings.map((f) => (
            <p key={f} style={{ margin: 0 }}>
              {f}
            </p>
          ))}
        </div>
      )}

      <div
        style={{
          borderTop: `1px solid ${token.colorBorderSecondary}`,
          paddingTop: 20,
        }}
      >
        <FilterChips<RecipeFilter>
          ariaLabel={t`Filter recipes`}
          value={chip}
          onChange={setChip}
          chips={[
            { key: "all", label: <Trans>All</Trans>, count: overview?.finishedCount },
            { key: "below", label: <Trans>Sold below cost</Trans>, count: belowCost },
            { key: "blocked", label: <Trans>Can't be built</Trans>, count: notBuildable },
            ...(overview && overview.noRecipe > 0
              ? [
                  {
                    key: "none" as const,
                    label: <Trans>No recipe</Trans>,
                    count: overview.noRecipe,
                  },
                ]
              : []),
          ]}
        />
      </div>

      <ListWithPanel
        list={list}
        panelOpen={!!selected}
        onClosePanel={() => setSelectedId(null)}
        panel={
          selected ? (
            <BomSummaryPanel
              key={selected.id}
              product={selected}
              qtyLocale={qtyLocale}
              reloadKey={panelReload}
              onEditRecipe={() => openEditor(selected.id)}
            />
          ) : (
            <p style={{ margin: 0, color: token.colorTextSecondary }}>
              <Trans>
                Select a finished product to see its parts, what they cost and how many you can
                build.
              </Trans>
            </p>
          )
        }
      />

      <BOMEditorDrawer onSaved={onSaved} />
    </div>
  );
};

export default BillOfMaterials;
