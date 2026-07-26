import { useCallback, useEffect, useRef, useState } from "react";
import type { Product, StockMovement } from "src/types/models";
import { Link, useLocation } from "react-router";
import { Button, Col, Popconfirm, Row, Select, Table, Tag, theme, Tooltip } from "antd";
import type { TableProps } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { InboxOutlined, DeleteOutlined } from "@ant-design/icons";
import filter from "lodash/filter";
import find from "lodash/find";

import { organizationIdAtom } from "src/atoms/organization";
import { productsAtom, setProductsAtom } from "src/atoms/product";
import { deleteStockMovementAtom } from "src/atoms/stock";
import { GetStockMovements } from "src/api";
import MovementForm from "src/components/stock/movement-form";
import PageHeader from "src/components/page-header";

const movementTypeTag = (type: string) => {
  if (type === "in")
    return (
      <Tag color="green">
        ↑ <Trans>In</Trans>
      </Tag>
    );
  if (type === "out")
    return (
      <Tag color="red">
        ↓ <Trans>Out</Trans>
      </Tag>
    );
  if (type === "count_addition")
    return (
      <Tag color="cyan">
        ↑ <Trans>Stock count (surplus)</Trans>
      </Tag>
    );
  if (type === "count_subtraction")
    return (
      <Tag color="orange">
        ↓ <Trans>Stock count (shortage)</Trans>
      </Tag>
    );
  return (
    <Tag color="blue">
      ⇆ <Trans>Adjustment</Trans>
    </Tag>
  );
};

const formatQty = (qty: number) =>
  (qty >= 0 ? "+" : "") + (qty % 1 === 0 ? String(qty) : qty.toFixed(2));

const DEFAULT_PAGE_SIZE = 50;

const Inventory = () => {
  useLingui();
  const { token } = theme.useToken();
  const location = useLocation();
  const organizationId = useAtomValue(organizationIdAtom);
  const products = useAtomValue(productsAtom);
  const setProducts = useSetAtom(setProductsAtom);
  const deleteMovement = useSetAtom(deleteStockMovementAtom);

  const [movements, setMovements] = useState<StockMovement[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [productFilter, setProductFilter] = useState<string | null>(null);
  const [sortField, setSortField] = useState<string | undefined>(undefined);
  const [sortOrder, setSortOrder] = useState<"asc" | "desc" | undefined>(undefined);

  // Guards against an in-flight earlier request overwriting a newer one.
  const requestIdRef = useRef(0);

  const fetchMovements = useCallback(() => {
    if (!organizationId) return;
    const requestId = ++requestIdRef.current;
    setLoading(true);
    GetStockMovements(organizationId, {
      productId: productFilter ?? undefined,
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
      .finally(() => {
        if (requestId === requestIdRef.current) setLoading(false);
      });
  }, [organizationId, page, pageSize, productFilter, sortField, sortOrder]);

  useEffect(() => {
    if (location.pathname === "/inventory") {
      // Re-runs whenever `location` changes — including when MovementForm
      // closes its drawer via navigate(), which is what refreshes this page
      // after recording a movement without a dedicated callback prop.
      setProducts();
      fetchMovements();
    }
  }, [location, fetchMovements, setProducts]);

  const trackedProducts = filter(products, (p: Product) => p.stockEnabled);

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
        extra={
          <Select
            allowClear
            placeholder={t`Filter by product`}
            style={{ width: 200 }}
            onChange={(val) => {
              setProductFilter(val ?? null);
              setPage(1);
            }}
            value={productFilter}
          >
            {trackedProducts.map((p: Product) => (
              <Select.Option key={p.id} value={p.id}>
                {p.name}
                {p.sku ? ` (${p.sku})` : ""}
              </Select.Option>
            ))}
          </Select>
        }
        actions={
          <Link to="/inventory" state={{ movementModal: true }}>
            <Button type="primary">
              <Trans>Record movement</Trans>
            </Button>
          </Link>
        }
      />

      {/* Stock levels summary */}
      {trackedProducts.length > 0 && (
        <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
          {trackedProducts.map((p: Product) => {
            const qty: number = p.stockQuantity ?? 0;
            const color =
              qty <= 0 ? token.colorError : qty <= 5 ? token.colorWarning : token.colorSuccess;
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
                  <div
                    style={{
                      fontSize: 12,
                      color: token.colorTextSecondary,
                      marginBottom: 4,
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                      whiteSpace: "nowrap",
                    }}
                  >
                    {p.name}
                  </div>
                  <div style={{ fontSize: 20, fontWeight: 600, color }}>
                    {qty % 1 === 0 ? qty : qty.toFixed(2)}
                    {p.unit && (
                      <span style={{ fontSize: 12, fontWeight: 400, marginLeft: 4 }}>{p.unit}</span>
                    )}
                  </div>
                </div>
              </Col>
            );
          })}
        </Row>
      )}

      <Row style={{ marginTop: 16 }}>
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
                <span style={{ color: qty >= 0 ? "#52c41a" : "#ff4d4f", fontWeight: 600 }}>
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
