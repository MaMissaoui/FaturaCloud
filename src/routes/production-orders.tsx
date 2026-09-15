import { useEffect, useMemo, useState } from "react";
import type { ProductionOrder } from "src/types/models";
import { Link, useLocation, useNavigate } from "react-router";
import { Button, Col, Row, Table, Tag } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { DeploymentUnitOutlined } from "@ant-design/icons";

import { useDateFormatter } from "src/utils/date";
import {
  PRODUCTION_ORDER_STATUSES,
  productionOrderStatusColor,
  productionOrderStatusLabel,
  type ProductionOrderStatus,
} from "src/types/production-order";
import { productionOrdersAtom, setProductionOrdersAtom } from "src/atoms/production-order";
import PageHeader from "src/components/page-header";

const ProductionOrders = () => {
  useLingui();
  const location = useLocation();
  const navigate = useNavigate();
  const formatDate = useDateFormatter();
  const orders = useAtomValue(productionOrdersAtom);
  const setOrders = useSetAtom(setProductionOrdersAtom);
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (location.pathname === "/production-orders") {
      setLoading(true);
      setOrders().finally(() => setLoading(false));
    }
  }, [location, setOrders]);

  // useState + useMemo, matching bill-of-materials.tsx rather than the older
  // module-level searchAtom + unmemoized filter this used to share with the
  // settings pages: that combination wrote a global atom and rebuilt
  // dataSource on every keystroke, re-rendering every visible row (audit
  // 2026-09-14 F91).
  const filtered = useMemo(() => {
    const term = search.trim().toLowerCase();
    if (!term) return orders;
    return orders.filter(
      (o: ProductionOrder) =>
        (o.orderNumber ?? "").toLowerCase().includes(term) ||
        (o.finishedProductName ?? "").toLowerCase().includes(term),
    );
  }, [orders, search]);

  return (
    <>
      <PageHeader
        icon={<DeploymentUnitOutlined />}
        title={<Trans>Production Orders</Trans>}
        search={{ placeholder: t`Search`, onChange: setSearch }}
        actions={
          <Button type="primary" onClick={() => navigate("/production-orders/new")}>
            <Trans>New production order</Trans>
          </Button>
        }
      />

      <Row style={{ marginTop: 16 }}>
        <Col span={24}>
          <Table
            dataSource={filtered}
            pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
            rowKey="id"
            loading={loading}
            onRow={(record: ProductionOrder) => ({
              onClick: () => navigate(`/production-orders/${record.id}`),
              style: { cursor: "pointer" },
            })}
          >
            <Table.Column
              title={<Trans>Order #</Trans>}
              key="orderNumber"
              sorter={(a: ProductionOrder, b: ProductionOrder) =>
                (a.orderNumber ?? "").localeCompare(b.orderNumber ?? "")
              }
              render={(o: ProductionOrder) => (
                <Link to={`/production-orders/${o.id}`} onClick={(e) => e.stopPropagation()}>
                  {o.orderNumber}
                </Link>
              )}
            />
            <Table.Column
              title={<Trans>Finished product</Trans>}
              dataIndex="finishedProductName"
              key="finishedProductName"
              sorter={(a: ProductionOrder, b: ProductionOrder) =>
                (a.finishedProductName ?? "").localeCompare(b.finishedProductName ?? "")
              }
            />
            <Table.Column
              title={<Trans>Quantity</Trans>}
              dataIndex="quantity"
              key="quantity"
              align="right"
              sorter={(a: ProductionOrder, b: ProductionOrder) => a.quantity - b.quantity}
            />
            <Table.Column
              title={<Trans>Import</Trans>}
              dataIndex="importNumber"
              key="importNumber"
              render={(v: string | null) => v ?? "—"}
            />
            <Table.Column
              title={<Trans>Status</Trans>}
              dataIndex="status"
              key="status"
              filters={PRODUCTION_ORDER_STATUSES.map((s) => ({
                text: productionOrderStatusLabel(s),
                value: s,
              }))}
              onFilter={(value, record: ProductionOrder) => record.status === value}
              sorter={(a: ProductionOrder, b: ProductionOrder) =>
                (a.status ?? "").localeCompare(b.status ?? "")
              }
              render={(status: string) => (
                <Tag color={productionOrderStatusColor[status as ProductionOrderStatus]}>
                  {productionOrderStatusLabel(status)}
                </Tag>
              )}
            />
            <Table.Column
              title={<Trans>Date</Trans>}
              dataIndex="date"
              key="date"
              sorter={(a: ProductionOrder, b: ProductionOrder) => (a.date ?? 0) - (b.date ?? 0)}
              render={(v: number) => (v ? formatDate(v) : "—")}
            />
          </Table>
        </Col>
      </Row>
    </>
  );
};

export default ProductionOrders;
