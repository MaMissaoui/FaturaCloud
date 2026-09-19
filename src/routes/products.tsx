import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { Product, TaxRate } from "src/types/models";
import { Link, useLocation, useNavigate } from "react-router";
import { Badge, Button, Col, Row, Select, Space, Table, Tag, Tooltip, Typography } from "antd";
import type { TableProps } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { AppstoreOutlined } from "@ant-design/icons";
import debounce from "lodash/debounce";

import { organizationAtom, organizationIdAtom } from "src/atoms/organization";
import { taxRatesAtom, setTaxRatesAtom } from "src/atoms/tax-rate";
import { GetProducts } from "src/api";
import ProductForm from "src/components/products/form";
import MassDataExcelActions from "src/components/mass-data/mass-data-excel-actions";
import PageHeader from "src/components/page-header";
import { unitLabel } from "src/utils/units";

// Decimals are a display concern only — storage stays cents regardless (see
// db/exchange_rate.go's decimals note) — so this takes the organization's
// configured precision rather than hardcoding 2, matching every other money
// formatter in the app (getFormattedNumber, invoice/PDF totals, …).
//
// `locale` must be the app's own selected locale (i18n.locale), not
// `undefined` — passing `undefined` to toLocaleString/Intl.NumberFormat
// resolves to the *browser's* locale, which varies per viewer's OS/browser
// settings independently of the language the app is actually showing (this
// page previously did exactly that, producing "8.409,53"-style separators
// on an English-language screen for anyone with a European system locale).
// This also switches to currency style so the organization's currency code
// shows here the same way it does on every other money display in the app.
// The narrow no-break space (U+202F) some locales use as a grouping
// separator renders with zero visible width in some contexts in this app
// (a confirmed browser rendering bug, not a data bug) — see
// src/utils/currencies.tsx's formatMoneyUnits for the full explanation.
// Normalized here too since this formatter isn't a formatMoneyUnits caller
// (it needs maximumFractionDigits, which that shared helper doesn't take).
const formatPrice = (cents: number, currency: string, locale: string, fractionDigits: number) =>
  new Intl.NumberFormat(locale, {
    style: "currency",
    currency,
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: fractionDigits,
  })
    .format(cents / 100)
    .replace(/[  ]/g, " ");

const DEFAULT_PAGE_SIZE = 25;

const Products = () => {
  const { i18n } = useLingui();
  const location = useLocation();
  const navigate = useNavigate();
  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);
  const fractionDigits = organization?.minimum_fraction_digits ?? 2;
  const currency = organization?.currency ?? "EUR";
  // The table itself no longer reads the shared productsAtom — it fetches
  // its own paginated page below. ProductForm still reads productsAtom (to
  // look up the product being edited, populate the BOM component picker,
  // and derive a collision-free SKU proposal), but fetches it itself, gated
  // on the drawer actually being open — see form.tsx — rather than this
  // page paying an unpaginated full-catalog fetch on every visit whether or
  // not the drawer is ever opened.
  const taxRates = useAtomValue(taxRatesAtom);
  const setTaxRates = useSetAtom(setTaxRatesAtom);

  const [products, setPageProducts] = useState<Product[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const [typeFilter, setTypeFilter] = useState<string | undefined>(undefined);
  const [categoryFilter, setCategoryFilter] = useState<string | undefined>(undefined);
  const [sortField, setSortField] = useState<string | undefined>(undefined);
  const [sortOrder, setSortOrder] = useState<"asc" | "desc" | undefined>(undefined);

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
    setLoading(true);
    GetProducts(organizationId, {
      search: search || undefined,
      type: typeFilter,
      category: categoryFilter,
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
      .finally(() => {
        if (requestId === requestIdRef.current) setLoading(false);
      });
  }, [organizationId, page, pageSize, search, typeFilter, categoryFilter, sortField, sortOrder]);

  useEffect(() => {
    if (location.pathname === "/products") {
      // Re-runs whenever `location` changes — including when ProductForm
      // closes its drawer via navigate(), which is what refreshes this page
      // after a create/update/delete without a dedicated callback prop.
      setTaxRates();
      fetchProducts();
    }
  }, [location, fetchProducts, setTaxRates]);

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
            onRow={(record: Product) => ({
              onClick: () =>
                navigate("/products", { state: { productModal: true, productId: record.id } }),
              style: { cursor: "pointer" },
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
            <Table.Column title={<Trans>SKU</Trans>} dataIndex="sku" key="sku" sorter />
            <Table.Column
              title={<Trans>Price</Trans>}
              dataIndex="price"
              key="price"
              align="right"
              sorter
              render={(price: number, p: Product) =>
                `${formatPrice(price, currency, i18n.locale, fractionDigits)}${p.unit ? ` / ${unitLabel(p.unit)}` : ""}`
              }
            />
            <Table.Column
              title={<Trans>Cost</Trans>}
              dataIndex="unitCost"
              key="unitCost"
              align="right"
              sorter
              render={(cost: number | null) =>
                cost != null ? formatPrice(cost, currency, i18n.locale, fractionDigits) : "—"
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
                  <Tooltip title={`${qty} ${p.unit ? unitLabel(p.unit) : t`units`}`}>
                    <Badge status={status} text={qty % 1 === 0 ? String(qty) : qty.toFixed(2)} />
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
