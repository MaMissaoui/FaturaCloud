import { useEffect, useState } from "react";
import { Link, useLocation, useNavigate } from "react-router";
import { Badge, Button, Col, Input, Row, Space, Table, Tag, Tooltip, Typography } from "antd";
import { atom, useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { AppstoreOutlined } from "@ant-design/icons";
import filter from "lodash/filter";
import get from "lodash/get";
import includes from "lodash/includes";
import some from "lodash/some";
import toString from "lodash/toString";

import { productsAtom, setProductsAtom } from "src/atoms/product";
import { taxRatesAtom, setTaxRatesAtom } from "src/atoms/tax-rate";
import ProductForm from "src/components/products/form";

const { Title } = Typography;

const searchAtom = atom<string>("");

const formatPrice = (cents: number) =>
  (cents / 100).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 });

const Products = () => {
  useLingui();
  const location = useLocation();
  const navigate = useNavigate();
  const products = useAtomValue(productsAtom);
  const setProducts = useSetAtom(setProductsAtom);
  const taxRates = useAtomValue(taxRatesAtom);
  const setTaxRates = useSetAtom(setTaxRatesAtom);
  const [search, setSearch] = useAtom(searchAtom);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (location.pathname === "/products") {
      setLoading(true);
      Promise.all([setProducts(), setTaxRates()]).finally(() => setLoading(false));
    }
  }, [location, setProducts, setTaxRates]);

  const filtered = search
    ? filter(products, (p: any) =>
        some(["name", "sku", "description", "unit", "type"], (field) =>
          includes(toString(get(p, field)).toLowerCase(), search.toLowerCase()),
        ),
      )
    : products;

  return (
    <>
      <Row>
        <Col span={12}>
          <Title level={3} style={{ marginTop: 0, marginBottom: 0 }}>
            <AppstoreOutlined style={{ marginRight: 8 }} />
            <Trans>Products</Trans>
          </Title>
        </Col>
        <Col span={12} style={{ display: "flex", justifyContent: "flex-end" }}>
          <Space style={{ alignItems: "start" }}>
            <Input.Search
              placeholder={t`Search`}
              onChange={(e) => setSearch(e.target.value)}
            />
            <Link to="/products" state={{ productModal: true }}>
              <Button type="primary">
                <Trans>New product</Trans>
              </Button>
            </Link>
          </Space>
        </Col>
      </Row>
      <Row style={{ marginTop: 16 }}>
        <Col span={24}>
          <Table
            dataSource={filtered}
            pagination={false}
            rowKey="id"
            loading={loading}
            onRow={(record: any) => ({
              onClick: () => navigate("/products", { state: { productModal: true, productId: record.id } }),
              style: { cursor: "pointer" },
            })}
          >
            <Table.Column
              title={<Trans>Name</Trans>}
              key="name"
              sorter={(a: any, b: any) => a.name.localeCompare(b.name)}
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
              sorter={(a: any, b: any) => (a.type ?? "").localeCompare(b.type ?? "")}
              render={(type: string) =>
                type === "product" ? (
                  <Tag color="blue"><Trans>Product</Trans></Tag>
                ) : (
                  <Tag color="green"><Trans>Service</Trans></Tag>
                )
              }
            />
            <Table.Column
              title={<Trans>SKU</Trans>}
              dataIndex="sku"
              key="sku"
              sorter={(a: any, b: any) => (a.sku ?? "").localeCompare(b.sku ?? "")}
            />
            <Table.Column
              title={<Trans>Price</Trans>}
              dataIndex="price"
              key="price"
              align="right"
              sorter={(a: any, b: any) => a.price - b.price}
              render={(price: number, p: any) =>
                `${formatPrice(price)}${p.unit ? ` / ${p.unit}` : ""}`
              }
            />
            <Table.Column
              title={<Trans>Cost</Trans>}
              dataIndex="unitCost"
              key="unitCost"
              align="right"
              sorter={(a: any, b: any) => (a.unitCost ?? 0) - (b.unitCost ?? 0)}
              render={(cost: number | null) => (cost != null ? formatPrice(cost) : "—")}
            />
            <Table.Column
              title={<Trans>Tax rate</Trans>}
              dataIndex="taxRateId"
              key="taxRateId"
              sorter={(a: any, b: any) => {
                const trA = taxRates.find((r: any) => r.id === a.taxRateId);
                const trB = taxRates.find((r: any) => r.id === b.taxRateId);
                return (trA?.name ?? "").localeCompare(trB?.name ?? "");
              }}
              render={(taxRateId: string | null) => {
                if (!taxRateId) return "—";
                const tr = taxRates.find((r: any) => r.id === taxRateId);
                return tr ? `${tr.name} (${tr.percentage}%)` : "—";
              }}
            />
            <Table.Column
              title={<Trans>Stock</Trans>}
              key="stock"
              align="center"
              sorter={(a: any, b: any) => (a.stockQuantity ?? 0) - (b.stockQuantity ?? 0)}
              render={(p: any) => {
                if (!p.stockEnabled) return null;
                const qty: number = p.stockQuantity ?? 0;
                const status = qty <= 0 ? "error" : qty <= 5 ? "warning" : "success";
                return (
                  <Tooltip title={`${qty} ${p.unit ?? "units"}`}>
                    <Badge
                      status={status}
                      text={qty % 1 === 0 ? String(qty) : qty.toFixed(2)}
                    />
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
