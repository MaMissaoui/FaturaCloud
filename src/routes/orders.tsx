import { useEffect, useMemo, useState } from "react";
import type { Order } from "src/types/models";
import { Link, useLocation, useNavigate } from "react-router";
import { Button, Col, Empty, Row, Table, Tag } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { ShoppingOutlined } from "@ant-design/icons";
import filter from "lodash/filter";
import includes from "lodash/includes";

import { ordersAtom, setOrdersAtom } from "src/atoms/order";
import {
  ORDER_STATUSES,
  orderStatusColor,
  orderStatusLabel,
  type OrderStatus,
} from "src/types/order";
import PageHeader from "src/components/page-header";
import { useDateFormatter } from "src/utils/date";

const statusTag = (status: string) => (
  <Tag color={orderStatusColor[status as OrderStatus]}>{orderStatusLabel(status)}</Tag>
);

const Orders = () => {
  useLingui();
  const location = useLocation();
  const navigate = useNavigate();
  const orders = useAtomValue(ordersAtom);
  const setOrders = useSetAtom(setOrdersAtom);
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(false);
  const formatDate = useDateFormatter();

  useEffect(() => {
    if (location.pathname === "/orders") {
      setLoading(true);
      setOrders().finally(() => setLoading(false));
    }
  }, [location, setOrders]);

  const filtered = useMemo(
    () =>
      filter(
        orders,
        (o: Order) =>
          includes((o.orderNumber ?? "").toLowerCase(), search.toLowerCase()) ||
          includes((o.clientName ?? "").toLowerCase(), search.toLowerCase()),
      ),
    [orders, search],
  );

  return (
    <>
      <PageHeader
        icon={<ShoppingOutlined />}
        title={<Trans>Orders</Trans>}
        search={{ placeholder: t`Search`, value: search, onChange: setSearch }}
        actions={
          <Button type="primary" onClick={() => navigate("/orders/new")}>
            <Trans>New order</Trans>
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
            locale={{
              emptyText: search ? (
                <Empty description={<Trans>No orders match your search</Trans>} />
              ) : (
                <Empty description={<Trans>No orders yet</Trans>}>
                  <Link to="/orders/new">
                    <Button type="primary">
                      <Trans>Create your first order</Trans>
                    </Button>
                  </Link>
                </Empty>
              ),
            }}
            onRow={(record: Order) => ({
              onClick: () => navigate(`/orders/${record.id}`),
              onKeyDown: (e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  navigate(`/orders/${record.id}`);
                }
              },
              style: { cursor: "pointer" },
              tabIndex: 0,
              role: "link",
            })}
          >
            <Table.Column
              title={<Trans>Order #</Trans>}
              key="orderNumber"
              sorter={(a: Order, b: Order) =>
                (a.orderNumber ?? "").localeCompare(b.orderNumber ?? "")
              }
              render={(o: Order) => (
                <Link to={`/orders/${o.id}`} onClick={(e) => e.stopPropagation()}>
                  {o.orderNumber}
                </Link>
              )}
            />
            <Table.Column
              title={<Trans>Client</Trans>}
              dataIndex="clientName"
              key="clientName"
              sorter={(a: Order, b: Order) =>
                (a.clientName ?? "").localeCompare(b.clientName ?? "")
              }
              render={(v: string | null) => v ?? "—"}
            />
            <Table.Column
              title={<Trans>Status</Trans>}
              dataIndex="status"
              key="status"
              filters={ORDER_STATUSES.map((s) => ({ text: orderStatusLabel(s), value: s }))}
              onFilter={(value, record: Order) => record.status === value}
              sorter={(a: Order, b: Order) => (a.status ?? "").localeCompare(b.status ?? "")}
              render={statusTag}
            />
            <Table.Column
              title={<Trans>Order date</Trans>}
              dataIndex="orderDate"
              key="orderDate"
              sorter={(a: Order, b: Order) => (a.orderDate ?? 0) - (b.orderDate ?? 0)}
              render={(v: number) => (v ? formatDate(v) : "—")}
            />
            <Table.Column
              title={<Trans>Delivery date</Trans>}
              dataIndex="deliveryDate"
              key="deliveryDate"
              sorter={(a: Order, b: Order) => (a.deliveryDate ?? 0) - (b.deliveryDate ?? 0)}
              render={(v: number | null) => (v ? formatDate(v) : "—")}
            />
            <Table.Column
              title={<Trans>Tracking</Trans>}
              dataIndex="trackingNumber"
              key="trackingNumber"
              sorter={(a: Order, b: Order) =>
                (a.trackingNumber ?? "").localeCompare(b.trackingNumber ?? "")
              }
              render={(v: string | null) => v ?? "—"}
            />
          </Table>
        </Col>
      </Row>
    </>
  );
};

export default Orders;
