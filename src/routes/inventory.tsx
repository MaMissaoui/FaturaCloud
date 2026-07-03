import { useEffect } from "react";
import { Link, useLocation } from "react-router";
import { Button, Col, Popconfirm, Row, Select, Space, Table, Tag, theme, Tooltip, Typography } from "antd";
import { atom, useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { InboxOutlined, DeleteOutlined } from "@ant-design/icons";
import filter from "lodash/filter";
import find from "lodash/find";

import { productsAtom, setProductsAtom } from "src/atoms/product";
import { stockMovementsAtom, setStockMovementsAtom, deleteStockMovementAtom } from "src/atoms/stock";
import MovementForm from "src/components/stock/movement-form";

const { Title } = Typography;

const productFilterAtom = atom<string | null>(null);

const movementTypeTag = (type: string) => {
  if (type === "in") return <Tag color="green">↑ <Trans>In</Trans></Tag>;
  if (type === "out") return <Tag color="red">↓ <Trans>Out</Trans></Tag>;
  return <Tag color="blue">⇆ <Trans>Adjustment</Trans></Tag>;
};

const formatQty = (qty: number) =>
  (qty >= 0 ? "+" : "") + (qty % 1 === 0 ? String(qty) : qty.toFixed(2));

const Inventory = () => {
  useLingui();
  const { token } = theme.useToken();
  const location = useLocation();
  const products = useAtomValue(productsAtom);
  const setProducts = useSetAtom(setProductsAtom);
  const movements = useAtomValue(stockMovementsAtom);
  const setMovements = useSetAtom(setStockMovementsAtom);
  const deleteMovement = useSetAtom(deleteStockMovementAtom);
  const [productFilter, setProductFilter] = useAtom(productFilterAtom);

  useEffect(() => {
    if (location.pathname === "/inventory") {
      setProducts();
      setMovements();
    }
  }, [location, setProducts, setMovements]);

  const trackedProducts = filter(products, (p: any) => p.stockEnabled);

  const filtered = productFilter
    ? filter(movements, (m: any) => m.productId === productFilter)
    : movements;

  return (
    <>
      <Row>
        <Col span={12}>
          <Title level={3} style={{ marginTop: 0, marginBottom: 0 }}>
            <InboxOutlined style={{ marginRight: 8 }} />
            <Trans>Inventory</Trans>
          </Title>
        </Col>
        <Col span={12} style={{ display: "flex", justifyContent: "flex-end" }}>
          <Space style={{ alignItems: "start" }}>
            <Select
              allowClear
              placeholder={t`Filter by product`}
              style={{ width: 200 }}
              onChange={(val) => setProductFilter(val ?? null)}
              value={productFilter}
            >
              {trackedProducts.map((p: any) => (
                <Select.Option key={p.id} value={p.id}>{p.name}{p.sku ? ` (${p.sku})` : ""}</Select.Option>
              ))}
            </Select>
            <Link to="/inventory" state={{ movementModal: true }}>
              <Button type="primary">
                <Trans>Record movement</Trans>
              </Button>
            </Link>
          </Space>
        </Col>
      </Row>

      {/* Stock levels summary */}
      {trackedProducts.length > 0 && (
        <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
          {trackedProducts.map((p: any) => {
            const qty: number = p.stockQuantity ?? 0;
            const color = qty <= 0 ? token.colorError : qty <= 5 ? token.colorWarning : token.colorSuccess;
            return (
              <Col key={p.id} xs={12} sm={8} md={6} lg={4}>
                <div
                  style={{
                    padding: "12px 16px",
                    border: `1px solid ${token.colorBorderSecondary}`,
                    borderRadius: 6,
                    borderLeft: `4px solid ${color}`,
                  }}
                >
                  <div style={{ fontSize: 12, color: token.colorTextSecondary, marginBottom: 4, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                    {p.name}
                  </div>
                  <div style={{ fontSize: 20, fontWeight: 600, color }}>
                    {qty % 1 === 0 ? qty : qty.toFixed(2)}
                    {p.unit && <span style={{ fontSize: 12, fontWeight: 400, marginLeft: 4 }}>{p.unit}</span>}
                  </div>
                </div>
              </Col>
            );
          })}
        </Row>
      )}

      <Row style={{ marginTop: 16 }}>
        <Col span={24}>
          <Table dataSource={filtered} pagination={{ pageSize: 50 }} rowKey="id">
            <Table.Column
              title={<Trans>Date</Trans>}
              dataIndex="createdAt"
              key="createdAt"
              render={(v: string) => v ? new Date(v).toLocaleString() : "—"}
            />
            <Table.Column
              title={<Trans>Product</Trans>}
              dataIndex="productId"
              key="productId"
              render={(productId: string) => {
                const p = find(products, { id: productId });
                return p ? (
                  <Link to="/products" state={{ productModal: true, productId }}>
                    {(p as any).name}{(p as any).sku ? ` (${(p as any).sku})` : ""}
                  </Link>
                ) : productId;
              }}
            />
            <Table.Column
              title={<Trans>Type</Trans>}
              key="type"
              render={(m: any) => movementTypeTag(m.type)}
            />
            <Table.Column
              title={<Trans>Quantity</Trans>}
              dataIndex="quantity"
              key="quantity"
              align="right"
              render={(qty: number) => (
                <span style={{ color: qty >= 0 ? "#52c41a" : "#ff4d4f", fontWeight: 600 }}>
                  {formatQty(qty)}
                </span>
              )}
            />
            <Table.Column
              title={<Trans>Reference</Trans>}
              dataIndex="reference"
              key="reference"
              render={(v: string | null) => v ?? "—"}
            />
            <Table.Column
              title={<Trans>Note</Trans>}
              dataIndex="note"
              key="note"
              render={(v: string | null) => v ?? "—"}
            />
            <Table.Column
              key="actions"
              align="center"
              width={60}
              render={(m: any) => (
                <Tooltip title={t`Delete movement`}>
                  <Popconfirm
                    title={<Trans>Delete this stock movement? The stock level will be recalculated.</Trans>}
                    onConfirm={() => deleteMovement(m.id)}
                    okText={<Trans>Yes</Trans>}
                    cancelText={<Trans>No</Trans>}
                    placement="left"
                  >
                    <Button type="text" danger icon={<DeleteOutlined />} size="small" />
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
