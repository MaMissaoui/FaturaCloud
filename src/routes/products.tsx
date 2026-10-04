import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { Product, TaxRate } from "src/types/models";
import { Link, useLocation, useNavigate } from "react-router";
import {
  App,
  Badge,
  Button,
  Col,
  Empty,
  Row,
  Select,
  Space,
  Table,
  Tag,
  theme,
  Tooltip,
  Typography,
} from "antd";
import type { TableProps } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { AppstoreOutlined } from "@ant-design/icons";
import debounce from "lodash/debounce";

import { myOrgRoleSyncAtom, organizationAtom, organizationIdAtom } from "src/atoms/organization";
import { taxRatesAtom, setTaxRatesAtom } from "src/atoms/tax-rate";
import { productFamiliesAtom, setProductFamiliesAtom } from "src/atoms/product-family";
import { GetProductSummaries, GetProducts, type ProductSummaryList } from "src/api";
import ProductForm from "src/components/products/form";
import ProductSummaryPanel from "src/components/products/product-summary-panel";
import { formatQuantity, StockTag } from "src/components/products/stock-tag";
import MassDataExcelActions from "src/components/mass-data/mass-data-excel-actions";
import FilterChips from "src/components/master-data/filter-chips";
import HeadlineFigure from "src/components/master-data/headline-figure";
import ListWithPanel from "src/components/master-data/list-with-panel";
import { useSummariesEnabled } from "src/components/master-data/use-summaries-enabled";
import PageHeader from "src/components/page-header";
import { dashboardWidgetsForRole } from "src/layouts/role-menu";
import { formatOrgCents, numberFormatLocale } from "src/utils/currencies";
import { unitLabel } from "src/utils/units";
import { useFetch } from "src/hooks/useFetch";
import { useLoadOnPath } from "src/hooks/useLoadOnPath";

const DEFAULT_PAGE_SIZE = 25;

type ProductFilter = "all" | "out" | "low" | "services";

const Products = () => {
  const { i18n } = useLingui();
  const { message } = App.useApp();
  const { token } = theme.useToken();
  const navigate = useNavigate();
  const location = useLocation();
  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);
  const role = useAtomValue(myOrgRoleSyncAtom);
  // Quantities use the organization's country-derived locale, not the
  // browser's — see formatQuantity above.
  const qtyLocale = numberFormatLocale(organization?.country_code) ?? i18n.locale;
  // The table itself no longer reads the shared productsAtom — it fetches
  // its own paginated page below. ProductForm still reads productsAtom (to
  // look up the product being edited, populate the BOM component picker,
  // and derive a collision-free SKU proposal), but fetches it itself, gated
  // on the drawer actually being open — see form.tsx — rather than this
  // page paying an unpaginated full-catalog fetch on every visit whether or
  // not the drawer is ever opened.
  const taxRates = useAtomValue(taxRatesAtom);
  const setTaxRates = useSetAtom(setTaxRatesAtom);

  const productFamilies = useAtomValue(productFamiliesAtom);
  // id → name, built once per family list rather than a find() per row.
  const familyNameById = useMemo(
    () => new Map(productFamilies.map((f) => [f.id, f.name])),
    [productFamilies],
  );
  const setProductFamilies = useSetAtom(setProductFamiliesAtom);

  const [products, setPageProducts] = useState<Product[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const [typeFilter, setTypeFilter] = useState<string | undefined>(undefined);
  const [categoryFilter, setCategoryFilter] = useState<string | undefined>(undefined);
  const [familyFilter, setFamilyFilter] = useState<string | undefined>(undefined);
  const [sortField, setSortField] = useState<string | undefined>(undefined);
  const [sortOrder, setSortOrder] = useState<"asc" | "desc" | undefined>(undefined);
  const [chip, setChip] = useState<ProductFilter>("all");
  // The picked product itself, not its id: the list is paginated, so the
  // picked row can leave the page while its panel stays open.
  const [selected, setSelected] = useState<Product | null>(null);

  // Summaries (stock value, chip counts, the panel) are on when the
  // organization has them switched on. They reload whenever the location
  // changes, which is how ProductForm closing after a save shows up here. A
  // failed load falls back to the plain list.
  const summariesEnabled = useSummariesEnabled("products");
  const { data: summaries, failed: summariesFailed } = useFetch<ProductSummaryList | null>(
    summariesEnabled && organizationId ? [organizationId, location.key] : null,
    () => GetProductSummaries(organizationId!),
    null,
  );
  const showSummaries = summariesEnabled && !summariesFailed;
  // With summaries on, the chips replace the type filter.
  const listType = showSummaries ? (chip === "services" ? "service" : undefined) : typeFilter;
  const listStock = showSummaries && (chip === "out" || chip === "low") ? chip : undefined;

  // requestIdRef guards against an in-flight earlier request (e.g. a slow
  // response to a stale search term) overwriting the result of a newer one.
  const requestIdRef = useRef(0);

  const debouncedSetSearch = useMemo(
    () =>
      debounce((value: string) => {
        setSearch(value);
        setPage(1);
      }, 300),
    [],
  );
  useEffect(() => () => debouncedSetSearch.cancel(), [debouncedSetSearch]);

  const fetchProducts = useCallback(() => {
    if (!organizationId) return;
    const requestId = ++requestIdRef.current;
    return GetProducts(organizationId, {
      search: search || undefined,
      type: listType,
      category: categoryFilter,
      familyId: familyFilter,
      stock: listStock,
      limit: pageSize,
      offset: (page - 1) * pageSize,
      sort: sortField,
      order: sortOrder,
    })
      .then((res) => {
        if (requestId !== requestIdRef.current) return;
        setPageProducts(res.data);
        setTotal(res.total);
      })
      .catch((error) => {
        if (requestId !== requestIdRef.current) return;
        console.error("Failed to load products:", error);
        message.error(error instanceof Error ? error.message : t`Failed to load products`);
      });
  }, [
    organizationId,
    page,
    pageSize,
    search,
    listType,
    categoryFilter,
    familyFilter,
    listStock,
    sortField,
    sortOrder,
    message,
  ]);

  // Re-runs whenever `location` changes — including when ProductForm closes
  // its drawer via navigate(), which is what refreshes this page after a
  // create/update/delete without a dedicated callback prop — and whenever the
  // page, sort or a filter changes.
  const loading = useLoadOnPath(
    "/products",
    () => {
      setTaxRates();
      setProductFamilies();
      return fetchProducts();
    },
    [
      organizationId,
      page,
      pageSize,
      search,
      listType,
      categoryFilter,
      familyFilter,
      listStock,
      sortField,
      sortOrder,
    ],
  );

  const handleTableChange: TableProps<Product>["onChange"] = (pagination, _filters, sorter) => {
    setPage(pagination.current ?? 1);
    setPageSize(pagination.pageSize ?? DEFAULT_PAGE_SIZE);
    const s = Array.isArray(sorter) ? sorter[0] : sorter;
    if (!s?.order) {
      setSortField(undefined);
      setSortOrder(undefined);
    } else {
      setSortField(String(s.columnKey ?? s.field));
      setSortOrder(s.order === "ascend" ? "asc" : "desc");
    }
  };

  if (showSummaries) {
    // The picked product as the list last loaded it (an edit refreshes the
    // page, not the copy picked earlier); the picked copy when it's off-page.
    const current = (selected && products.find((p) => p.id === selected.id)) || selected;
    const threshold = summaries?.lowStockThreshold ?? 2;
    const quantityOnly = summaries?.inventoryValuation === "quantity_only";
    // The stock value follows the Dashboard: only roles that see its stock
    // panel get the money figure, everyone else the units.
    const showsValue = !quantityOnly && dashboardWidgetsForRole(role).stock;
    const money = (cents: number) => formatOrgCents(cents, organization, i18n.locale);
    const pickChip = (next: ProductFilter) => {
      setChip(next);
      setPage(1);
    };
    const pick = (product: Product) => setSelected(product);
    const notSet = (
      <Typography.Text type="secondary" aria-label={t`Not set`}>
        —
      </Typography.Text>
    );
    const filtered = !!(search || categoryFilter || familyFilter || chip !== "all");

    const list = (
      <Table
        dataSource={products}
        pagination={{
          current: page,
          pageSize,
          total,
          showSizeChanger: true,
          hideOnSinglePage: true,
        }}
        onChange={handleTableChange}
        rowKey="id"
        loading={loading}
        size="middle"
        scroll={{ x: "max-content" }}
        rowClassName={(record: Product) =>
          record.id === selected?.id ? "master-data-selected-row" : ""
        }
        locale={{
          emptyText: filtered ? (
            <Empty description={<Trans>No products match your filters</Trans>} />
          ) : (
            <Empty description={<Trans>No products yet</Trans>}>
              <Link to="/products" state={{ productModal: true }}>
                <Button type="primary">
                  <Trans>Create your first product</Trans>
                </Button>
              </Link>
            </Empty>
          ),
        }}
        onRow={(record: Product) => ({
          onClick: () => pick(record),
          onKeyDown: (e) => {
            if (e.key === "Enter" || e.key === " ") {
              e.preventDefault();
              pick(record);
            }
          },
          style: { cursor: "pointer" },
          tabIndex: 0,
          role: "button",
          "aria-pressed": record.id === selected?.id,
        })}
      >
        <Table.Column
          title={<Trans>Product</Trans>}
          key="name"
          sorter
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
          title={<Trans>Code</Trans>}
          dataIndex="sku"
          key="sku"
          // Hidden below 1440px (src/styles/base.scss) so the margin keeps its
          // room beside the panel; the panel shows the code.
          className="master-data-wide-only"
          sorter
          render={(sku: string | null) =>
            sku ? <span style={{ whiteSpace: "nowrap" }}>{sku}</span> : notSet
          }
        />
        <Table.Column
          title={<Trans>Price excl. tax</Trans>}
          dataIndex="price"
          key="price"
          align="right"
          sorter
          render={(price: number) => (
            <span style={{ whiteSpace: "nowrap", fontVariantNumeric: "tabular-nums" }}>
              {money(price)}
            </span>
          )}
        />
        <Table.Column
          title={<Trans>Margin</Trans>}
          key="margin"
          align="right"
          // Under quantity-only valuation the organization keeps no costs, so
          // there is no margin to show — never a misleading one.
          render={(p: Product) =>
            !quantityOnly && p.unitCost != null && p.price > 0 ? (
              <span style={{ whiteSpace: "nowrap", fontVariantNumeric: "tabular-nums" }}>
                {`${(((p.price - p.unitCost) / p.price) * 100).toLocaleString(i18n.locale, {
                  maximumFractionDigits: 1,
                })} %`}
              </span>
            ) : (
              notSet
            )
          }
        />
        <Table.Column
          title={<Trans>Stock</Trans>}
          key="stock"
          align="right"
          // Pinned, so the stock stays in view when the table scrolls (1280px
          // with the panel open), like Owes on Clients.
          fixed="right"
          sorter
          render={(p: Product) =>
            p.stockEnabled ? (
              <span style={{ whiteSpace: "nowrap", fontVariantNumeric: "tabular-nums" }}>
                <StockTag quantity={p.stockQuantity ?? 0} threshold={threshold}>
                  {formatQuantity(p.stockQuantity ?? 0, qtyLocale)}
                </StockTag>
              </span>
            ) : (
              <Typography.Text type="secondary">
                {p.type === "service" ? <Trans>Service</Trans> : "—"}
              </Typography.Text>
            )
          }
        />
      </Table>
    );

    const selectedTaxRate = current?.taxRateId
      ? (taxRates.find((r: TaxRate) => r.id === current.taxRateId) ?? null)
      : null;

    return (
      <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
        <PageHeader
          icon={<AppstoreOutlined />}
          title={<Trans>Products</Trans>}
          extra={
            <>
              <Select
                allowClear
                placeholder={t`All families`}
                style={{ width: 160 }}
                value={familyFilter}
                onChange={(value) => {
                  setFamilyFilter(value);
                  setPage(1);
                }}
                options={[
                  ...productFamilies.map((f) => ({ value: f.id, label: f.name })),
                  { value: "none", label: t`No family` },
                ]}
              />
              <Select
                allowClear
                placeholder={t`All categories`}
                style={{ width: 160 }}
                value={categoryFilter}
                onChange={(value) => {
                  setCategoryFilter(value);
                  setPage(1);
                }}
                options={[
                  { value: "finished", label: t`Finished good` },
                  { value: "component", label: t`Component / intermediate` },
                  { value: "unclassified", label: t`Unclassified` },
                ]}
              />
            </>
          }
          search={{
            placeholder: t`Name or code`,
            value: searchInput,
            onChange: (value) => {
              setSearchInput(value);
              debouncedSetSearch(value);
            },
          }}
          actions={
            <Space wrap>
              {organizationId && (
                <MassDataExcelActions
                  organizationId={organizationId}
                  resource="products"
                  filenamePrefix="products"
                  onImported={fetchProducts}
                  compact
                />
              )}
              <Link to="/products" state={{ productModal: true }}>
                <Button type="primary">
                  <Trans>New product</Trans>
                </Button>
              </Link>
            </Space>
          }
        />
        {summaries && (
          <p style={{ margin: "-12px 0 0", color: token.colorTextSecondary }}>
            {t`${summaries.itemCount} items: ${summaries.stockTracked} tracked in stock and ${summaries.services} services`}
          </p>
        )}

        <div
          style={{
            display: "flex",
            flexWrap: "wrap",
            alignItems: "flex-end",
            justifyContent: "space-between",
            gap: "12px 24px",
            borderTop: `1px solid ${token.colorBorderSecondary}`,
            paddingTop: 20,
          }}
        >
          {showsValue ? (
            <HeadlineFigure
              label={<Trans>Stock value at average cost</Trans>}
              value={summaries ? money(summaries.stockValue) : "…"}
              note={summaries ? t`${formatQuantity(summaries.units, qtyLocale)} units` : undefined}
            />
          ) : (
            <HeadlineFigure
              label={<Trans>Units in stock</Trans>}
              value={summaries ? formatQuantity(summaries.units, qtyLocale) : "…"}
              note={summaries ? t`across ${summaries.stockTracked} products` : undefined}
            />
          )}
          <FilterChips<ProductFilter>
            ariaLabel={t`Filter products`}
            value={chip}
            onChange={pickChip}
            chips={[
              { key: "all", label: <Trans>All</Trans>, count: summaries?.itemCount ?? 0 },
              {
                key: "out",
                label: <Trans>Out of stock</Trans>,
                count: summaries?.outOfStock ?? 0,
              },
              {
                key: "low",
                label: t`${threshold} or less`,
                count: summaries?.lowStock ?? 0,
              },
              {
                key: "services",
                label: <Trans>Services</Trans>,
                count: summaries?.services ?? 0,
              },
            ]}
          />
        </div>

        <ListWithPanel
          list={list}
          panelOpen={!!current}
          onClosePanel={() => setSelected(null)}
          panel={
            current ? (
              <ProductSummaryPanel
                key={current.id}
                product={current}
                familyName={(current.familyId && familyNameById.get(current.familyId)) || null}
                taxRate={selectedTaxRate}
                quantityOnly={quantityOnly}
                lowStockThreshold={threshold}
                qtyLocale={qtyLocale}
                onEdit={() =>
                  navigate("/products", {
                    state: { productModal: true, productId: current.id },
                  })
                }
              />
            ) : (
              <p style={{ margin: 0, color: token.colorTextSecondary }}>
                <Trans>Select a product to see its stock, margin and latest movements.</Trans>
              </p>
            )
          }
        />
        <ProductForm />
      </div>
    );
  }

  return (
    <>
      <PageHeader
        icon={<AppstoreOutlined />}
        title={<Trans>Products</Trans>}
        extra={
          <>
            <Select
              allowClear
              placeholder={t`All types`}
              style={{ width: 140 }}
              value={typeFilter}
              onChange={(value) => {
                setTypeFilter(value);
                setPage(1);
              }}
              options={[
                { value: "product", label: t`Product` },
                { value: "service", label: t`Service` },
              ]}
            />
            <Select
              allowClear
              placeholder={t`All categories`}
              style={{ width: 160 }}
              value={categoryFilter}
              onChange={(value) => {
                setCategoryFilter(value);
                setPage(1);
              }}
              options={[
                { value: "finished", label: t`Finished good` },
                { value: "component", label: t`Component / intermediate` },
                { value: "unclassified", label: t`Unclassified` },
              ]}
            />
            <Select
              allowClear
              placeholder={t`All families`}
              style={{ width: 160 }}
              value={familyFilter}
              onChange={(value) => {
                setFamilyFilter(value);
                setPage(1);
              }}
              options={[
                ...productFamilies.map((f) => ({ value: f.id, label: f.name })),
                { value: "none", label: t`No family` },
              ]}
            />
          </>
        }
        search={{
          placeholder: t`Search`,
          value: searchInput,
          onChange: (value) => {
            setSearchInput(value);
            debouncedSetSearch(value);
          },
        }}
        actions={
          <Space wrap>
            {organizationId && (
              <MassDataExcelActions
                organizationId={organizationId}
                resource="products"
                filenamePrefix="products"
                onImported={fetchProducts}
                compact
              />
            )}
            <Link to="/products" state={{ productModal: true }}>
              <Button type="primary">
                <Trans>New product</Trans>
              </Button>
            </Link>
          </Space>
        }
      />
      <Row style={{ marginTop: 16 }}>
        <Col span={24}>
          <Table
            dataSource={products}
            pagination={{
              current: page,
              pageSize,
              total,
              showSizeChanger: true,
              hideOnSinglePage: true,
            }}
            onChange={handleTableChange}
            rowKey="id"
            loading={loading}
            locale={{
              emptyText:
                search || typeFilter || categoryFilter || familyFilter ? (
                  <Empty description={<Trans>No products match your filters</Trans>} />
                ) : (
                  <Empty description={<Trans>No products yet</Trans>}>
                    <Link to="/products" state={{ productModal: true }}>
                      <Button type="primary">
                        <Trans>Create your first product</Trans>
                      </Button>
                    </Link>
                  </Empty>
                ),
            }}
            onRow={(record: Product) => ({
              onClick: () =>
                navigate("/products", { state: { productModal: true, productId: record.id } }),
              onKeyDown: (e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  navigate("/products", { state: { productModal: true, productId: record.id } });
                }
              },
              style: { cursor: "pointer" },
              tabIndex: 0,
            })}
          >
            <Table.Column
              title={<Trans>Name</Trans>}
              key="name"
              sorter
              render={(p) => (
                <Link
                  to="/products"
                  state={{ productModal: true, productId: p.id }}
                  onClick={(e) => e.stopPropagation()}
                >
                  {p.name}
                </Link>
              )}
            />
            <Table.Column
              title={<Trans>Type</Trans>}
              dataIndex="type"
              key="type"
              sorter
              render={(type: string) =>
                type === "product" ? (
                  <Tag color="blue">
                    <Trans>Product</Trans>
                  </Tag>
                ) : (
                  <Tag color="green">
                    <Trans>Service</Trans>
                  </Tag>
                )
              }
            />
            <Table.Column
              title={<Trans>Category</Trans>}
              dataIndex="category"
              key="category"
              render={(category: string | null) =>
                category === "finished" ? (
                  <Tag color="purple">
                    <Trans>Finished good</Trans>
                  </Tag>
                ) : category === "component" ? (
                  <Tag color="gold">
                    <Trans>Component</Trans>
                  </Tag>
                ) : null
              }
            />
            <Table.Column
              title={<Trans>Family</Trans>}
              key="family"
              render={(p: Product) => (p.familyId && familyNameById.get(p.familyId)) || "—"}
            />
            <Table.Column title={<Trans>SKU</Trans>} dataIndex="sku" key="sku" sorter />
            <Table.Column
              title={<Trans>Price</Trans>}
              dataIndex="price"
              key="price"
              align="right"
              sorter
              render={(price: number, p: Product) =>
                `${formatOrgCents(price, organization, i18n.locale)}${p.unit ? ` / ${unitLabel(p.unit)}` : ""}`
              }
            />
            <Table.Column
              title={<Trans>Cost</Trans>}
              dataIndex="unitCost"
              key="unitCost"
              align="right"
              sorter
              render={(cost: number | null) =>
                cost != null ? formatOrgCents(cost, organization, i18n.locale) : "—"
              }
            />
            <Table.Column
              title={<Trans>Tax rate</Trans>}
              dataIndex="taxRateId"
              key="taxRate"
              sorter
              render={(taxRateId: string | null) => {
                if (!taxRateId) return "—";
                const tr = taxRates.find((r: TaxRate) => r.id === taxRateId);
                return tr ? `${tr.name} (${tr.percentage}%)` : "—";
              }}
            />
            <Table.Column
              title={<Trans>Stock</Trans>}
              key="stock"
              align="center"
              sorter
              render={(p: Product) => {
                if (!p.stockEnabled) return <Typography.Text type="secondary">—</Typography.Text>;
                const qty: number = p.stockQuantity ?? 0;
                const status = qty <= 0 ? "error" : qty <= 5 ? "warning" : "success";
                return (
                  <Tooltip
                    title={`${formatQuantity(qty, qtyLocale)} ${p.unit ? unitLabel(p.unit) : t`units`}`}
                  >
                    <Badge status={status} text={formatQuantity(qty, qtyLocale)} />
                  </Tooltip>
                );
              }}
            />
          </Table>
        </Col>
      </Row>
      <ProductForm />
    </>
  );
};

export default Products;
