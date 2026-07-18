import { useEffect, useState } from "react";
import { Link, useLocation, useNavigate } from "react-router";
import { Button, Col, Input, Row, Space, Table, Tag, Typography } from "antd";
import { atom, useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { SendOutlined } from "@ant-design/icons";
import dayjs from "dayjs";
import filter from "lodash/filter";
import includes from "lodash/includes";

import { deliveriesAtom, setDeliveriesAtom } from "src/atoms/delivery";

const { Title } = Typography;
const searchAtom = atom<string>("");

const statusTag = (status: string) => {
  const map: Record<string, { color: string; label: string }> = {
    draft: { color: "default", label: "Draft" },
    shipped: { color: "orange", label: "Shipped" },
    delivered: { color: "green", label: "Delivered" },
  };
  const s = map[status] ?? { color: "default", label: status };
  return <Tag color={s.color}>{s.label}</Tag>;
};

const Deliveries = () => {
  useLingui();
  const location = useLocation();
  const navigate = useNavigate();
  const deliveries = useAtomValue(deliveriesAtom);
  const setDeliveries = useSetAtom(setDeliveriesAtom);
  const [search, setSearch] = useAtom(searchAtom);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (location.pathname === "/deliveries") {
      setLoading(true);
      setDeliveries().finally(() => setLoading(false));
    }
  }, [location, setDeliveries]);

  const filtered = search
    ? filter(deliveries, (d: any) =>
        includes((d.deliveryNumber ?? "").toLowerCase(), search.toLowerCase()) ||
        includes((d.clientName ?? "").toLowerCase(), search.toLowerCase()) ||
        includes((d.orderNumber ?? "").toLowerCase(), search.toLowerCase()),
      )
    : deliveries;

  return (
    <>
      <Row>
        <Col span={12}>
          <Title level={3} style={{ marginTop: 0, marginBottom: 0 }}>
            <SendOutlined style={{ marginRight: 8 }} />
            <Trans>Outbound Deliveries</Trans>
          </Title>
        </Col>
        <Col span={12} style={{ display: "flex", justifyContent: "flex-end" }}>
          <Space style={{ alignItems: "start" }}>
            <Input.Search
              placeholder={t`Search`}
              onChange={(e) => setSearch(e.target.value)}
            />
            <Link to="/deliveries/new">
              <Button type="primary" style={{ marginBottom: 10 }}>
                <Trans>New delivery</Trans>
              </Button>
            </Link>
          </Space>
        </Col>
      </Row>
      <Table
        dataSource={filtered}
        pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
        rowKey="id"
        loading={loading}
        onRow={(record: any) => ({
          onClick: () => navigate(`/deliveries/${record.id}`),
          style: { cursor: "pointer" },
        })}
      >
        <Table.Column
          title={<Trans>Number</Trans>}
          key="deliveryNumber"
          render={(d: any) => (
            <Link to={`/deliveries/${d.id}`} onClick={(e) => e.stopPropagation()}>
              {d.deliveryNumber}
            </Link>
          )}
          sorter={(a: any, b: any) => a.deliveryNumber.localeCompare(b.deliveryNumber)}
        />
        <Table.Column
          title={<Trans>Order</Trans>}
          dataIndex="orderNumber"
          key="orderNumber"
          sorter={(a: any, b: any) => (a.orderNumber ?? "").localeCompare(b.orderNumber ?? "")}
        />
        <Table.Column
          title={<Trans>Client</Trans>}
          dataIndex="clientName"
          key="clientName"
          sorter={(a: any, b: any) => (a.clientName ?? "").localeCompare(b.clientName ?? "")}
        />
        <Table.Column
          title={<Trans>Delivery date</Trans>}
          dataIndex="deliveryDate"
          key="deliveryDate"
          render={(v: number) => (v ? dayjs(v).format("L") : "—")}
          sorter={(a: any, b: any) => (a.deliveryDate ?? 0) - (b.deliveryDate ?? 0)}
        />
        <Table.Column
          title={<Trans>Status</Trans>}
          dataIndex="status"
          key="status"
          sorter={(a: any, b: any) => (a.status ?? "").localeCompare(b.status ?? "")}
          render={statusTag}
        />
      </Table>
    </>
  );
};

export default Deliveries;
