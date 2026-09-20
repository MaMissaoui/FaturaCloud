import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { Product, StockMovement } from "src/types/models";
import { Link, useLocation } from "react-router";
import {
  Alert,
  App,
  Button,
  Col,
  Input,
  Popconfirm,
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
import { InboxOutlined, DeleteOutlined } from "@ant-design/icons";
import find from "lodash/find";
import debounce from "lodash/debounce";

import { organizationIdAtom } from "src/atoms/organization";
import { productsAtom, setProductsAtom } from "src/atoms/product";
import { deleteStockMovementAtom } from "src/atoms/stock";
import { GetStockMovements } from "src/api";
import MovementForm from "src/components/stock/movement-form";
import PageHeader from "src/components/page-header";
import { unitLabel } from "src/utils/units";

const movementTypeTag = (type: string) => {
  if (type === "in")
    return (
      <Tag color="green">
        <span aria-hidden="true">↑</span> <Trans>In</Trans>
      </Tag>
    );
  if (type === "out")
    return (
      <Tag color="red">
        <span aria-hidden="true">↓</span> <Trans>Out</Trans>
      </Tag>
    );
  if (type === "count_addition")
    return (
      <Tag color="cyan">
        <span aria-hidden="true">↑</span> <Trans>Stock count (surplus)</Trans>
      </Tag>
    );
  if (type === "count_subtraction")
    return (
      <Tag color="orange">
        <span aria-hidden="true">↓</span> <Trans>Stock count (shortage)</Trans>
      </Tag>
    );
  return (
    <Tag color="blue">
      <span aria-hidden="true">⇆</span> <Trans>Adjustment</Trans>
    </Tag>
  );
};

const formatQty = (qty: number) =>
  (qty >= 0 ? "+" : "") + (qty % 1 === 0 ? String(qty) : qty.toFixed(2));

const DEFAULT_PAGE_SIZE = 50;

const Inventory = () => {
  useLingui();
  const { token } = theme.useToken();
  const { message } = App.useApp();
  const location = useLocation();
  const organizationId = useAtomValue(organizationIdAtom);
  const products = useAtomValue(productsAtom);
  const setProducts = useSetAtom(setProductsAtom);
  const deleteMovement = useSetAtom(deleteStockMovementAtom);

  const [movements, setMovements] = useState<StockMovement[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  // Starts true — productsAtom is [] until the fetch below resolves, and
  // without tracking this separately the "No products are tracking stock
  // yet" empty state flashed falsely during that window on every cold
  // load/navigation, for organizations that do have tracked products.
  const [productsLoading, setProductsLoading] = useState(true);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [productFilter, setProductFilter] = useState<string | null>(null);
  const [categoryFilter, setCategoryFilter] = useState<string | null>(null);
  const [movementTypeFilter, setMovementTypeFilter] = useState<string | null>(null);
  // referenceInput updates on every keystroke (what the box shows);
  // referenceFilter is debounced and is what actually drives the server
  // query — same split as products.tsx's search/searchInput pair, so a
  // fast typist doesn't fire a request per character.
  const [referenceInput, setReferenceInput] = useState("");
  const [referenceFilter, setReferenceFilter] = useState("");
  const [stockSearch, setStockSearch] = useState("");
  // Stock levels' own pagination — kept as state rather than left to the
  // Table's default uncontrolled pagination, which doesn't reset itself when
  // the filtered dataSource shrinks: browsing to page 2+ of "Finished good"
  // and then switching to "Component" (a smaller filtered set) left antd
  // pinned on a page number the new set doesn't have, rendering an empty
  // table even though matching rows existed on page 1.
  const [stockPage, setStockPage] = useState(1);
  const [stockPageSize, setStockPageSize] = useState(25);
  const [sortField, setSortField] = useState<string | undefined>(undefined);
  const [sortOrder, setSortOrder] = useState<"asc" | "desc" | undefined>(undefined);

  // Guards against an in-flight earlier request overwriting a newer one.
  const requestIdRef = useRef(0);

  const debouncedSetReferenceFilter = useMemo(
    () =>
      debounce((value: string) => {
        setReferenceFilter(value);
        setPage(1);
      }, 300),
    [],
  );
  useEffect(() => () => debouncedSetReferenceFilter.cancel(), [debouncedSetReferenceFilter]);

  // Memoized — this page has two filter rows on the same component, and an
  // unmemoized derivation here got rebuilt (and both consuming tables'
  // dataSource/option lists with it) on every keystroke anywhere on the
  // page, including in Recent movements' own, unrelated filter boxes. Same
  // bug class already found and fixed on the Production Orders list.
  const trackedProducts = useMemo(() => products.filter((p) => p.stockEnabled), [products]);
  // Drives both the Stock levels table below and — via categoryFilter,
  // shared with fetchMovements' `category` param — the Recent movements
  // table too, so the two sections can't disagree about what "filtered by
  // product type" means. The Recent movements product picker also draws
  // its options from this list rather than the unfiltered trackedProducts,
  // so it can never offer a product the current category/search filter
  // excludes — see the reconciling effect right below for what happens
  // when a change here would otherwise strand a stale selection there.
  const filteredTrackedProducts = useMemo(
    () =>
      trackedProducts.filter((p) => {
        if (categoryFilter && p.category !== categoryFilter) return false;
        if (!stockSearch) return true;
        const needle = stockSearch.toLowerCase();
        return (
          p.name.toLowerCase().includes(needle) || (p.sku ?? "").toLowerCase().includes(needle)
        );
      }),
    [trackedProducts, categoryFilter, stockSearch],
  );

  // Keeps the Recent movements product picker's selection consistent with
  // whichever product it's now drawing its options from: if a category or
  // search change (in Stock levels) excludes the currently-selected
  // product, clear the selection instead of leaving a query that combines
  // a stale productId with the new category/search and silently returns
  // nothing. Surfaced with a toast — silently emptying a filter someone
  // just set is exactly the "looked broken" trap this page's filters have
  // hit before, just from the other direction.
  useEffect(() => {
    if (productFilter && !filteredTrackedProducts.some((p) => p.id === productFilter)) {
      setProductFilter(null);
      setPage(1);
      message.info(t`Product filter cleared — no longer matches the Stock levels filter above.`);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [categoryFilter, stockSearch]);

  useEffect(() => {
    setStockPage(1);
  }, [categoryFilter, stockSearch]);

  const fetchMovements = useCallback(() => {
    if (!organizationId) return;
    const requestId = ++requestIdRef.current;
    setLoading(true);
    GetStockMovements(organizationId, {
      productId: productFilter ?? undefined,
      category: categoryFilter ?? undefined,
      type: movementTypeFilter ?? undefined,
      reference: referenceFilter || undefined,
      limit: pageSize,
      offset: (page - 1) * pageSize,
      sort: sortField,
      order: sortOrder,
    })
      .then((res) => {
        if (requestId !== requestIdRef.current) return;
        setMovements(res.data);
        setTotal(res.total);
      })
      .catch((error) => {
        if (requestId !== requestIdRef.current) return;
        console.error("Failed to load stock movements:", error);
        message.error(error instanceof Error ? error.message : t`Failed to load stock movements`);
      })
      .finally(() => {
        if (requestId === requestIdRef.current) setLoading(false);
      });
  }, [
    organizationId,
    page,
    pageSize,
    productFilter,
    categoryFilter,
    movementTypeFilter,
    referenceFilter,
    sortField,
    sortOrder,
    message,
  ]);

  useEffect(() => {
    if (location.pathname === "/inventory") {
      // Re-runs whenever `location` changes — including when MovementForm
      // closes its drawer via navigate(), which is what refreshes this page
      // after recording a movement without a dedicated callback prop.
      setProductsLoading(true);
      setProducts().finally(() => setProductsLoading(false));
      fetchMovements();
    }
  }, [location, fetchMovements, setProducts]);

  const handleDelete = async (movement: StockMovement) => {
    const success = await deleteMovement({
      id: movement.id,
      productId: movement.productId,
      quantity: movement.quantity,
    });
    if (success) fetchMovements();
  };

  const handleTableChange: TableProps<StockMovement>["onChange"] = (
    pagination,
    _filters,
    sorter,
  ) => {
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

  return (
    <>
      <PageHeader
        icon={<InboxOutlined />}
        title={<Trans>Inventory</Trans>}
        actions={
          <Link to="/inventory" state={{ movementModal: true }}>
            <Button type="primary">
              <Trans>Record movement</Trans>
            </Button>
          </Link>
        }
      />

      {/* Stock levels — a Table rather than one card per product (the
          previous shape) once this app's own demo catalog reached ~300
          stock-tracked products: an unpaginated, unsearchable, unsortable
          grid of cards is fine at a handful of products but becomes
          unnavigable at that scale, and gives no way to answer "what's
          actually low or out of stock" without scanning every card. Search
          is client-side over the already-loaded productsAtom (the same data
          the old card grid already relied on) — no new endpoint needed. */}
      {!productsLoading && trackedProducts.length === 0 && (
        <Alert
          style={{ marginTop: 24 }}
          type="info"
          showIcon
          message={<Trans>No products are tracking stock yet</Trans>}
          description={
            <Trans>
              Enable "Track inventory" on a product (Master Data → Products) to see it here.
            </Trans>
          }
        />
      )}
      {trackedProducts.length > 0 && (
        <>
          <Row style={{ marginTop: 24 }} align="middle" justify="space-between">
            <Col>
              <Typography.Title level={5} style={{ margin: 0 }}>
                <Trans>Stock levels</Trans>
              </Typography.Title>
            </Col>
            <Col>
              <Space wrap>
                <Select
                  allowClear
                  placeholder={t`Filter by product type`}
                  style={{ width: 180 }}
                  onChange={(val) => {
                    setCategoryFilter(val ?? null);
                    setPage(1);
                  }}
                  value={categoryFilter}
                >
                  <Select.Option value="finished">
                    <Trans>Finished good</Trans>
                  </Select.Option>
                  <Select.Option value="component">
                    <Trans>Component</Trans>
                  </Select.Option>
                </Select>
                <Input.Search
                  allowClear
                  placeholder={t`Search products`}
                  style={{ width: 260 }}
                  value={stockSearch}
                  onChange={(e) => setStockSearch(e.target.value)}
                />
              </Space>
            </Col>
          </Row>
          <Row style={{ marginTop: 12 }}>
            <Col span={24}>
              <Table
                dataSource={filteredTrackedProducts}
                rowKey="id"
                loading={productsLoading}
                pagination={{
                  current: stockPage,
                  pageSize: stockPageSize,
                  showSizeChanger: true,
                  hideOnSinglePage: true,
                }}
                onChange={(pagination) => {
                  setStockPage(pagination.current ?? 1);
                  setStockPageSize(pagination.pageSize ?? 25);
                }}
                locale={{ emptyText: t`No products match your filters` }}
              >
                <Table.Column
                  title={<Trans>Product</Trans>}
                  dataIndex="name"
                  key="name"
                  sorter={(a: Product, b: Product) => a.name.localeCompare(b.name)}
                  defaultSortOrder="ascend"
                  render={(name: string, p: Product) => (
                    <Link to="/products" state={{ productModal: true, productId: p.id }}>
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
                  title={<Trans>On hand</Trans>}
                  dataIndex="stockQuantity"
                  key="stockQuantity"
                  align="right"
                  sorter={(a: Product, b: Product) =>
                    (a.stockQuantity ?? 0) - (b.stockQuantity ?? 0)
                  }
                  render={(qty: number, p: Product) => {
                    const q = qty ?? 0;
                    // Only "at or below zero" is unambiguous across a whole
                    // catalog — there's no per-product low-stock threshold
                    // field, so a fixed mid-tier number (e.g. "5") applied
                    // to every product regardless of unit/scale produced
                    // both false alarms (5 units of something ordered in
                    // bulk) and false confidence (5 of something scarce).
                    // colorErrorText/colorSuccessText, not the base
                    // colorError/colorSuccess — the base 6-shade tokens are
                    // under the WCAG AA 4.5:1 contrast floor for text at
                    // this weight (~2.3:1/~3.3:1 measured against white);
                    // the *Text variants are the darker 9-shade tokens antd
                    // itself reserves for exactly this use.
                    const color = q <= 0 ? token.colorErrorText : token.colorSuccessText;
                    return (
                      <span style={{ color, fontWeight: 600 }}>
                        {q % 1 === 0 ? q : q.toFixed(2)}
                        {p.unit && (
                          <span style={{ fontWeight: 400, marginLeft: 4 }}>
                            {unitLabel(p.unit)}
                          </span>
                        )}
                      </span>
                    );
                  }}
                />
              </Table>
            </Col>
          </Row>
        </>
      )}

      <Row style={{ marginTop: 24 }} align="middle" justify="space-between">
        <Col>
          <Space align="center">
            <Typography.Title level={5} style={{ margin: 0 }}>
              <Trans>Recent movements</Trans>
            </Typography.Title>
            {/* categoryFilter isn't set from a control in this section (see
                the comment on the filter row below) — without this, a
                category picked in Stock levels above silently narrows this
                table too, which this page's own filters have hit before as
                a "looks broken" trap. */}
            {categoryFilter && (
              <Tooltip title={<Trans>Filtered by product type, set above</Trans>}>
                <Tag color="blue">
                  {categoryFilter === "finished" ? (
                    <Trans>Finished good</Trans>
                  ) : (
                    <Trans>Component</Trans>
                  )}
                </Tag>
              </Tooltip>
            )}
          </Space>
        </Col>
        <Col>
          {/* Product/movement-type/reference filter this table only. Product
              type is deliberately not here — it lives in the Stock levels
              header above and reaches this table through the shared
              categoryFilter state (passed to GetStockMovements as
              `category`), so narrowing by product type visibly narrows both
              tables from one control instead of two that could disagree.
              The product picker below draws from filteredTrackedProducts
              (already scoped to that shared filter), not the raw
              trackedProducts list, so it can't offer a product the current
              product-type filter would exclude — the exact "selecting a
              filter had no visible effect and looked broken" trap this
              page's filters have hit before, just from the other
              direction. */}
          <Space wrap>
            <Select
              allowClear
              placeholder={t`Filter by product`}
              style={{ width: 220 }}
              showSearch
              optionFilterProp="label"
              onChange={(val) => {
                setProductFilter(val ?? null);
                setPage(1);
              }}
              value={productFilter}
            >
              {filteredTrackedProducts.map((p: Product) => (
                <Select.Option key={p.id} value={p.id} label={p.name}>
                  {p.name}
                  {p.sku ? ` (${p.sku})` : ""}
                </Select.Option>
              ))}
            </Select>
            <Select
              allowClear
              placeholder={t`Filter by movement type`}
              style={{ width: 200 }}
              onChange={(val) => {
                setMovementTypeFilter(val ?? null);
                setPage(1);
              }}
              value={movementTypeFilter}
            >
              <Select.Option value="in">{movementTypeTag("in")}</Select.Option>
              <Select.Option value="out">{movementTypeTag("out")}</Select.Option>
              <Select.Option value="count_addition">
                {movementTypeTag("count_addition")}
              </Select.Option>
              <Select.Option value="count_subtraction">
                {movementTypeTag("count_subtraction")}
              </Select.Option>
              <Select.Option value="adjustment">{movementTypeTag("adjustment")}</Select.Option>
            </Select>
            <Input.Search
              allowClear
              placeholder={t`Filter by reference`}
              style={{ width: 200 }}
              value={referenceInput}
              onChange={(e) => {
                setReferenceInput(e.target.value);
                debouncedSetReferenceFilter(e.target.value);
              }}
            />
          </Space>
        </Col>
      </Row>
      <Row style={{ marginTop: 12 }}>
        <Col span={24}>
          <Table
            dataSource={movements}
            pagination={{ current: page, pageSize, total, showSizeChanger: true }}
            onChange={handleTableChange}
            loading={loading}
            rowKey="id"
          >
            <Table.Column
              title={<Trans>Date</Trans>}
              dataIndex="createdAt"
              key="date"
              sorter
              render={(v: string) => (v ? new Date(v).toLocaleString() : "—")}
            />
            <Table.Column
              title={<Trans>Product</Trans>}
              dataIndex="productId"
              key="product"
              sorter
              render={(productId: string) => {
                const p = find(products, { id: productId });
                return p ? (
                  <Link to="/products" state={{ productModal: true, productId }}>
                    {p.name}
                    {p.sku ? ` (${p.sku})` : ""}
                  </Link>
                ) : (
                  productId
                );
              }}
            />
            <Table.Column
              title={<Trans>Type</Trans>}
              key="type"
              sorter
              render={(m: StockMovement) => movementTypeTag(m.type)}
            />
            <Table.Column
              title={<Trans>Quantity</Trans>}
              dataIndex="quantity"
              key="quantity"
              align="right"
              sorter
              render={(qty: number) => (
                <span
                  style={{
                    color: qty >= 0 ? token.colorSuccessText : token.colorErrorText,
                    fontWeight: 600,
                  }}
                >
                  {formatQty(qty)}
                </span>
              )}
            />
            <Table.Column
              title={<Trans>Reference</Trans>}
              dataIndex="reference"
              key="reference"
              sorter
              render={(v: string | null) => v ?? "—"}
            />
            <Table.Column
              title={<Trans>Note</Trans>}
              dataIndex="note"
              key="note"
              sorter
              render={(v: string | null) => v ?? "—"}
            />
            <Table.Column
              key="actions"
              align="center"
              width={60}
              render={(m: StockMovement) => (
                <Tooltip title={t`Delete movement`}>
                  <Popconfirm
                    title={
                      <Trans>
                        Delete this stock movement? The stock level will be recalculated.
                      </Trans>
                    }
                    onConfirm={() => handleDelete(m)}
                    okText={<Trans>Yes</Trans>}
                    cancelText={<Trans>No</Trans>}
                    placement="left"
                  >
                    <Button
                      type="text"
                      danger
                      icon={<DeleteOutlined />}
                      size="small"
                      aria-label={t`Delete movement`}
                    />
                  </Popconfirm>
                </Tooltip>
              )}
            />
          </Table>
        </Col>
      </Row>

      <MovementForm />
    </>
  );
};

export default Inventory;
