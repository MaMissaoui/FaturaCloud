import { useEffect, useState } from "react";
import type { Delivery } from "src/types/models";
import { Link, useLocation, useNavigate } from "react-router";
import { Button, Table, Tag } from "antd";
import { atom, useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { SendOutlined } from "@ant-design/icons";
import dayjs from "dayjs";
import filter from "lodash/filter";
import includes from "lodash/includes";

import { deliveriesAtom, setDeliveriesAtom } from "src/atoms/delivery";
import { deliveryStatusColor, deliveryStatusLabel, type DeliveryStatus } from "src/types/delivery";
import PageHeader from "src/components/page-header";

const searchAtom = atom<string>("");

const statusTag = (status: string) => (
  <Tag color={deliveryStatusColor[status as DeliveryStatus]}>{deliveryStatusLabel(status)}</Tag>
);

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
    ? filter(deliveries, (d: Delivery) =>
        includes((d.deliveryNumber ?? "").toLowerCase(), search.toLowerCase()) ||
        includes((d.clientName ?? "").toLowerCase(), search.toLowerCase()) ||
        includes((d.orderNumber ?? "").toLowerCase(), search.toLowerCase()),
      )
    : deliveries;

  return (
    <>
      <PageHeader
        icon={<SendOutlined />}
        title={<Trans>Outbound Deliveries</Trans>}
        search={{ placeholder: t`Search`, onChange: setSearch }}
        actions={
          <Link to="/deliveries/new">
            <Button type="primary" style={{ marginBottom: 10 }}>
              <Trans>New delivery</Trans>
            </Button>
          </Link>
        }
      />
      <Table
        dataSource={filtered}
        pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
        rowKey="id"
        loading={loading}
        onRow={(record: Delivery) => ({
          onClick: () => navigate(`/deliveries/${record.id}`),
          style: { cursor: "pointer" },
        })}
      >
        <Table.Column
          title={<Trans>Number</Trans>}
          key="deliveryNumber"
          render={(d: Delivery) => (
            <Link to={`/deliveries/${d.id}`} onClick={(e) => e.stopPropagation()}>
              {d.deliveryNumber}
            </Link>
          )}
          sorter={(a: Delivery, b: Delivery) => a.deliveryNumber.localeCompare(b.deliveryNumber)}
        />
        <Table.Column
          title={<Trans>Order</Trans>}
          dataIndex="orderNumber"
          key="orderNumber"
          sorter={(a: Delivery, b: Delivery) => (a.orderNumber ?? "").localeCompare(b.orderNumber ?? "")}
        />
        <Table.Column
          title={<Trans>Client</Trans>}
          dataIndex="clientName"
          key="clientName"
          sorter={(a: Delivery, b: Delivery) => (a.clientName ?? "").localeCompare(b.clientName ?? "")}
        />
        <Table.Column
          title={<Trans>Delivery date</Trans>}
          dataIndex="deliveryDate"
          key="deliveryDate"
          render={(v: number) => (v ? dayjs(v).format("L") : "—")}
          sorter={(a: Delivery, b: Delivery) => (a.deliveryDate ?? 0) - (b.deliveryDate ?? 0)}
        />
        <Table.Column
          title={<Trans>Status</Trans>}
          dataIndex="status"
          key="status"
          sorter={(a: Delivery, b: Delivery) => (a.status ?? "").localeCompare(b.status ?? "")}
          render={statusTag}
        />
      </Table>
    </>
  );
};

export default Deliveries;
