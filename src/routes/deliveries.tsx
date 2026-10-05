import { useMemo, useState } from "react";
import type { Delivery } from "src/types/models";
import { Link, useNavigate } from "react-router";
import { Button, Col, Empty, Row, Table, Tag, theme } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { plural, t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { SendOutlined } from "@ant-design/icons";
import filter from "lodash/filter";

import { deliveriesAtom, setDeliveriesAtom } from "src/atoms/delivery";
import { deliveryStatusColor, deliveryStatusLabel, type DeliveryStatus } from "src/types/delivery";
import PageHeader from "src/components/page-header";
import DocumentFilters, { matchesDocumentFilters } from "src/components/document-filters";
import { useStatusChips } from "src/components/master-data/use-status-chips";
import { clientsAtom, setClientsAtom } from "src/atoms/client";
import { useDateFormatter } from "src/utils/date";
import type { Dayjs } from "dayjs";
import { useLoadOnPath } from "src/hooks/useLoadOnPath";

const statusTag = (status: string) => (
  <Tag color={deliveryStatusColor[status as DeliveryStatus]}>{deliveryStatusLabel(status)}</Tag>
);

const Deliveries = () => {
  useLingui();
  const { token } = theme.useToken();
  const navigate = useNavigate();
  const deliveries = useAtomValue(deliveriesAtom);
  const setDeliveries = useSetAtom(setDeliveriesAtom);
  const setClients = useSetAtom(setClientsAtom);
  const [search, setSearch] = useState("");
  const [clientFilter, setClientFilter] = useState("");
  const [dateRange, setDateRange] = useState<[Dayjs, Dayjs] | null>(null);
  const clients = useAtomValue(clientsAtom);
  const formatDate = useDateFormatter();

  const loading = useLoadOnPath("/deliveries", () => {
    setClients();
    return setDeliveries();
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
      filter(deliveries, (d: Delivery) =>
        matchesDocumentFilters({
          search,
          searchFields: [d.deliveryNumber, d.clientName, d.orderNumber],
          status: "",
          rowStatus: d.status,
          partyId: clientFilter,
          rowPartyId: d.clientId,
          dateRange,
          rowDate: d.deliveryDate,
        }),
      ),
    [deliveries, search, clientFilter, dateRange],
  );
  const { shown, picked, bar } = useStatusChips(
    matching,
    (row) => row.status ?? "",
    [
      { key: "draft", label: <Trans context="document filter">Draft</Trans> },
      { key: "shipped", label: <Trans context="document filter">Shipped</Trans> },
      { key: "delivered", label: <Trans context="document filter">Delivered</Trans> },
      {
        key: "cancelled",
        label: <Trans context="document filter">Cancelled</Trans>,
        hideWhenEmpty: true,
      },
    ],
    t`Filter deliveries`,
  );
  const hasFilters = !!(search || picked || clientFilter || dateRange);

  return (
    <>
      <PageHeader
        icon={<SendOutlined />}
        title={<Trans>Outbound Deliveries</Trans>}
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
            dateLabel={t`Delivery date`}
            partyOptions={clientOptions}
            partyValue={clientFilter}
            onPartyChange={setClientFilter}
            partyPlaceholder={t`All clients`}
          />
        }
        actions={
          <Button type="primary" onClick={() => navigate("/deliveries/new")}>
            <Trans>New delivery</Trans>
          </Button>
        }
      />
      <p style={{ margin: "4px 0 0", color: token.colorTextSecondary }}>
        {plural(deliveries.length, { one: "# delivery", other: "# deliveries" })}
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
                <Empty description={<Trans>No deliveries match your filters</Trans>} />
              ) : (
                <Empty description={<Trans>No deliveries yet</Trans>}>
                  <Link to="/deliveries/new">
                    <Button type="primary">
                      <Trans>Create your first delivery</Trans>
                    </Button>
                  </Link>
                </Empty>
              ),
            }}
            onRow={(record: Delivery) => ({
              onClick: () => navigate(`/deliveries/${record.id}`),
              onKeyDown: (e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  navigate(`/deliveries/${record.id}`);
                }
              },
              style: { cursor: "pointer" },
              tabIndex: 0,
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
              sorter={(a: Delivery, b: Delivery) =>
                a.deliveryNumber.localeCompare(b.deliveryNumber)
              }
            />
            <Table.Column
              title={<Trans>Order</Trans>}
              key="orderNumber"
              sorter={(a: Delivery, b: Delivery) =>
                (a.orderNumber ?? "").localeCompare(b.orderNumber ?? "")
              }
              render={(d: Delivery) =>
                d.orderId ? (
                  <Link to={`/orders/${d.orderId}`} onClick={(e) => e.stopPropagation()}>
                    {d.orderNumber}
                  </Link>
                ) : (
                  (d.orderNumber ?? "—")
                )
              }
            />
            <Table.Column
              title={<Trans>Client</Trans>}
              dataIndex="clientName"
              key="clientName"
              sorter={(a: Delivery, b: Delivery) =>
                (a.clientName ?? "").localeCompare(b.clientName ?? "")
              }
              render={(v: string | null) => v ?? "—"}
            />
            <Table.Column
              title={<Trans>Delivery date</Trans>}
              dataIndex="deliveryDate"
              key="deliveryDate"
              render={(v: number) => (v ? formatDate(v) : "—")}
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
        </Col>
      </Row>
    </>
  );
};

export default Deliveries;
