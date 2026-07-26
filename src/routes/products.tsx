import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { Product, TaxRate } from "src/types/models";
import { Link, useLocation, useNavigate } from "react-router";
import { Badge, Button, Col, Row, Table, Tag, Tooltip } from "antd";
import type { TableProps } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { AppstoreOutlined } from "@ant-design/icons";
import debounce from "lodash/debounce";

import { organizationIdAtom } from "src/atoms/organization";
import { setProductsAtom } from "src/atoms/product";
import { taxRatesAtom, setTaxRatesAtom } from "src/atoms/tax-rate";
import { GetProducts } from "src/api";
import ProductForm from "src/components/products/form";
import PageHeader from "src/components/page-header";

const formatPrice = (cents: number) =>
  (cents / 100).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 });

const DEFAULT_PAGE_SIZE = 25;

const Products = () => {
  useLingui();
  const location = useLocation();
  const navigate = useNavigate();
  const organizationId = useAtomValue(organizationIdAtom);
  // The table itself no longer reads the shared productsAtom — it fetches
  // its own paginated page below — but ProductForm still does, both to look
  // up the product being edited and to derive a collision-free SKU proposal
  // for a new one. Keep populating it here so that drawer keeps working.
  const setProducts = useSetAtom(setProductsAtom);
  const taxRates = useAtomValue(taxRatesAtom);
  const setTaxRates = useSetAtom(setTaxRatesAtom);

  const [products, setPageProducts] = useState<Product[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
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
  }, [organizationId, page, pageSize, search, sortField, sortOrder]);

  useEffect(() => {
    if (location.pathname === "/products") {
      // Re-runs whenever `location` changes — including when ProductForm
      // closes its drawer via navigate(), which is what refreshes this page
      // after a create/update/delete without a dedicated callback prop.
      setProducts();
      setTaxRates();
      fetchProducts();
    }
  }, [location, fetchProducts, setProducts, setTaxRates]);

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
        search={{
          placeholder: t`Search`,
          value: searchInput,
          onChange: (value) => {
            setSearchInput(value);
            debouncedSetSearch(value);
          },
        }}
        actions={
          <Link to="/products" state={{ productModal: true }}>
            <Button type="primary">
              <Trans>New product</Trans>
            </Button>
          </Link>
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
            <Table.Column title={<Trans>SKU</Trans>} dataIndex="sku" key="sku" sorter />
            <Table.Column
              title={<Trans>Price</Trans>}
              dataIndex="price"
              key="price"
              align="right"
              sorter
              render={(price: number, p: Product) =>
                `${formatPrice(price)}${p.unit ? ` / ${p.unit}` : ""}`
              }
            />
            <Table.Column
              title={<Trans>Cost</Trans>}
              dataIndex="unitCost"
              key="unitCost"
              align="right"
              sorter
              render={(cost: number | null) => (cost != null ? formatPrice(cost) : "—")}
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
                if (!p.stockEnabled) return null;
                const qty: number = p.stockQuantity ?? 0;
                const status = qty <= 0 ? "error" : qty <= 5 ? "warning" : "success";
                return (
                  <Tooltip title={`${qty} ${p.unit ?? "units"}`}>
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
