import { useEffect } from "react";
import { Link, useLocation, useNavigate } from "react-router";
import { Button, Col, Row, Space, Table, Tag, Typography } from "antd";
import { atom, useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { ShoppingOutlined } from "@ant-design/icons";
import filter from "lodash/filter";
import includes from "lodash/includes";
import { Input } from "antd";

import { ordersAtom, setOrdersAtom } from "src/atoms/order";

const { Title } = Typography;
const searchAtom = atom<string>("");

const statusTag = (status: string) => {
  const map: Record<string, { color: string; label: string }> = {
    draft: { color: "default", label: "Draft" },
    confirmed: { color: "blue", label: "Confirmed" },
    shipped: { color: "orange", label: "Shipped" },
    delivered: { color: "green", label: "Delivered" },
    cancelled: { color: "red", label: "Cancelled" },
  };
  const s = map[status] ?? { color: "default", label: status };
  return <Tag color={s.color}>{s.label}</Tag>;
};

const Orders = () => {
  useLingui();
  const location = useLocation();
  const navigate = useNavigate();
  const orders = useAtomValue(ordersAtom);
  const setOrders = useSetAtom(setOrdersAtom);
  const [search, setSearch] = useAtom(searchAtom);

  useEffect(() => {
    if (location.pathname === "/orders") {
      setOrders();
    }
  }, [location, setOrders]);

  const filtered = search
    ? filter(orders, (o: any) =>
        includes((o.orderNumber ?? "").toLowerCase(), search.toLowerCase()) ||
        includes((o.clientName ?? "").toLowerCase(), search.toLowerCase()),
      )
    : orders;

  return (
    <>
      <Row>
        <Col span={12}>
          <Title level={3} style={{ marginTop: 0, marginBottom: 0 }}>
            <ShoppingOutlined style={{ marginRight: 8 }} />
            <Trans>Orders</Trans>
          </Title>
        </Col>
        <Col span={12} style={{ display: "flex", justifyContent: "flex-end" }}>
          <Space style={{ alignItems: "start" }}>
            <Input.Search
              placeholder={t`Search`}
              onChange={(e) => setSearch(e.target.value)}
            />
            <Button type="primary" onClick={() => navigate("/orders/new")}>
              <Trans>New order</Trans>
            </Button>
          </Space>
        </Col>
      </Row>

      <Row style={{ marginTop: 16 }}>
        <Col span={24}>
          <Table dataSource={filtered} pagination={false} rowKey="id">
            <Table.Column
              title={<Trans>Order #</Trans>}
              key="orderNumber"
              render={(o: any) => (
                <Link to={`/orders/${o.id}`}>{o.orderNumber}</Link>
              )}
            />
            <Table.Column
              title={<Trans>Client</Trans>}
              dataIndex="clientName"
              key="clientName"
              render={(v: string | null) => v ?? "—"}
            />
            <Table.Column
              title={<Trans>Status</Trans>}
              dataIndex="status"
              key="status"
              render={statusTag}
            />
            <Table.Column
              title={<Trans>Order date</Trans>}
              dataIndex="orderDate"
              key="orderDate"
              render={(v: number) => v ? new Date(v).toLocaleDateString() : "—"}
            />
            <Table.Column
              title={<Trans>Delivery date</Trans>}
              dataIndex="deliveryDate"
              key="deliveryDate"
              render={(v: number | null) => v ? new Date(v).toLocaleDateString() : "—"}
            />
            <Table.Column
              title={<Trans>Tracking</Trans>}
              dataIndex="trackingNumber"
              key="trackingNumber"
              render={(v: string | null) => v ?? "—"}
            />
          </Table>
        </Col>
      </Row>
    </>
  );
};

export default Orders;
