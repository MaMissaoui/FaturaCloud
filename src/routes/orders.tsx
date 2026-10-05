import { useMemo, useState } from "react";
import type { Order } from "src/types/models";
import { Link, useNavigate } from "react-router";
import { Button, Col, Empty, Row, Table, Tag, theme } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { plural, t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { ShoppingOutlined } from "@ant-design/icons";
import filter from "lodash/filter";

import { ordersAtom, setOrdersAtom } from "src/atoms/order";
import { clientsAtom, setClientsAtom } from "src/atoms/client";
import { orderStatusColor, orderStatusLabel, type OrderStatus } from "src/types/order";
import PageHeader from "src/components/page-header";
import DocumentFilters, { matchesDocumentFilters } from "src/components/document-filters";
import { useStatusChips } from "src/components/master-data/use-status-chips";
import { useDateFormatter } from "src/utils/date";
import type { Dayjs } from "dayjs";
import { useLoadOnPath } from "src/hooks/useLoadOnPath";

const statusTag = (status: string) => (
  <Tag color={orderStatusColor[status as OrderStatus]}>{orderStatusLabel(status)}</Tag>
);

const Orders = () => {
  useLingui();
  const { token } = theme.useToken();
  const navigate = useNavigate();
  const orders = useAtomValue(ordersAtom);
  const setOrders = useSetAtom(setOrdersAtom);
  const setClients = useSetAtom(setClientsAtom);
  const clients = useAtomValue(clientsAtom);
  const [search, setSearch] = useState("");
  const [clientFilter, setClientFilter] = useState("");
  const [dateRange, setDateRange] = useState<[Dayjs, Dayjs] | null>(null);
  const formatDate = useDateFormatter();

  const loading = useLoadOnPath("/orders", () => {
    setClients();
    return setOrders();
  });

  const clientOptions = useMemo(
    () =>
      (clients as any[]).map((c) => ({
        value: c.id,
        label: [c.name, c.code ? `· ${c.code}` : c.phone].filter(Boolean).join(" "),
      })),
    [clients],
  );

  // What the search, party and date filters leave; the status chips split it.
  const matching = useMemo(
    () =>
      filter(orders, (o: Order) =>
        matchesDocumentFilters({
          search,
          searchFields: [o.orderNumber, o.clientName, o.trackingNumber],
          status: "",
          rowStatus: o.status,
          partyId: clientFilter,
          rowPartyId: o.clientId,
          dateRange,
          rowDate: o.orderDate,
        }),
      ),
    [orders, search, clientFilter, dateRange],
  );
  const { shown, picked, bar } = useStatusChips(
    matching,
    (row) => row.status ?? "",
    [
      { key: "draft", label: <Trans context="document filter">Draft</Trans> },
      { key: "confirmed", label: <Trans context="document filter">Confirmed</Trans> },
      { key: "shipped", label: <Trans context="document filter">Shipped</Trans> },
      { key: "delivered", label: <Trans context="document filter">Delivered</Trans> },
      {
        key: "cancelled",
        label: <Trans context="document filter">Cancelled</Trans>,
        hideWhenEmpty: true,
      },
    ],
    t`Filter orders`,
  );
  const hasFilters = !!(search || picked || clientFilter || dateRange);

  return (
    <>
      <PageHeader
        icon={<ShoppingOutlined />}
        title={<Trans>Orders</Trans>}
        search={{
          placeholder: t`Search`,
          value: search,
          onChange: setSearch,
          allowClear: true,
          onClear: () => setSearch(""),
        }}
        filters={
          <DocumentFilters
            dateRange={dateRange}
            onDateRangeChange={setDateRange}
            dateLabel={t`Order date`}
            partyOptions={clientOptions}
            partyValue={clientFilter}
            onPartyChange={setClientFilter}
            partyPlaceholder={t`All clients`}
          />
        }
        actions={
          <Button type="primary" onClick={() => navigate("/orders/new")}>
            <Trans>New order</Trans>
          </Button>
        }
      />

      <p style={{ margin: "4px 0 0", color: token.colorTextSecondary }}>
        {plural(orders.length, { one: "# order", other: "# orders" })}
      </p>
      {bar}

      <Row style={{ marginTop: 16 }}>
        <Col span={24}>
          <Table
            dataSource={shown}
            pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
            rowKey="id"
            loading={loading}
            locale={{
              emptyText: hasFilters ? (
                <Empty description={<Trans>No orders match your filters</Trans>} />
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
