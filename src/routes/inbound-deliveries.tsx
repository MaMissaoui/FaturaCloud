import { useMemo, useState } from "react";
import type { InboundDelivery } from "src/types/models";
import { Link, useNavigate } from "react-router";
import { Button, Col, Empty, Row, Table, Tag, theme } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { plural, t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { ImportOutlined } from "@ant-design/icons";
import filter from "lodash/filter";

import { useDateFormatter } from "src/utils/date";
import {
  inboundDeliveryStatusColor,
  inboundDeliveryStatusLabel,
  type InboundDeliveryStatus,
} from "src/types/inbound-delivery";
import { inboundDeliveriesAtom, setInboundDeliveriesAtom } from "src/atoms/inbound-delivery";
import { vendorsAtom, setVendorsAtom } from "src/atoms/vendor";
import PageHeader from "src/components/page-header";
import DocumentFilters, { matchesDocumentFilters } from "src/components/document-filters";
import { useStatusChips } from "src/components/master-data/use-status-chips";
import type { Dayjs } from "dayjs";
import { useLoadOnPath } from "src/hooks/useLoadOnPath";

const InboundDeliveries = () => {
  useLingui();
  const { token } = theme.useToken();
  const navigate = useNavigate();
  const formatDate = useDateFormatter();
  const deliveries = useAtomValue(inboundDeliveriesAtom);
  const setDeliveries = useSetAtom(setInboundDeliveriesAtom);
  const setVendors = useSetAtom(setVendorsAtom);
  const [search, setSearch] = useState("");
  const [vendorFilter, setVendorFilter] = useState("");
  const [dateRange, setDateRange] = useState<[Dayjs, Dayjs] | null>(null);
  const vendors = useAtomValue(vendorsAtom);

  const loading = useLoadOnPath("/inbound-deliveries", () => {
    setVendors();
    return setDeliveries();
  });

  const vendorOptions = useMemo(
    () =>
      (vendors as any[]).map((v) => ({
        value: v.id,
        label: [v.name, v.code ? `· ${v.code}` : v.phone].filter(Boolean).join(" "),
      })),
    [vendors],
  );

  // What the search, party and date filters leave; the status chips split it.
  const matching = useMemo(
    () =>
      filter(deliveries, (d: InboundDelivery) =>
        matchesDocumentFilters({
          search,
          searchFields: [d.deliveryNumber, d.vendorName, d.orderNumber],
          status: "",
          rowStatus: d.status,
          partyId: vendorFilter,
          rowPartyId: d.vendorId,
          dateRange,
          rowDate: d.deliveryDate,
        }),
      ),
    [deliveries, search, vendorFilter, dateRange],
  );
  const { shown, picked, bar } = useStatusChips(
    matching,
    (row) => row.status ?? "",
    [
      { key: "draft", label: <Trans context="document filter">Draft</Trans> },
      { key: "received", label: <Trans context="document filter">Received</Trans> },
      {
        key: "cancelled",
        label: <Trans context="document filter">Cancelled</Trans>,
        hideWhenEmpty: true,
      },
    ],
    t`Filter goods receipts`,
  );
  const hasFilters = !!(search || picked || vendorFilter || dateRange);

  return (
    <>
      <PageHeader
        icon={<ImportOutlined />}
        title={<Trans>Goods Receipts</Trans>}
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
            partyOptions={vendorOptions}
            partyValue={vendorFilter}
            onPartyChange={setVendorFilter}
            partyPlaceholder={t`All vendors`}
          />
        }
        actions={
          <Button type="primary" onClick={() => navigate("/inbound-deliveries/new")}>
            <Trans>New goods receipt</Trans>
          </Button>
        }
      />

      <p style={{ margin: "4px 0 0", color: token.colorTextSecondary }}>
        {plural(deliveries.length, { one: "# goods receipt", other: "# goods receipts" })}
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
                <Empty description={<Trans>No goods receipts match your filters</Trans>} />
              ) : (
                <Empty description={<Trans>No goods receipts yet</Trans>}>
                  <Link to="/inbound-deliveries/new">
                    <Button type="primary">
                      <Trans>Create your first goods receipt</Trans>
                    </Button>
                  </Link>
                </Empty>
              ),
            }}
            onRow={(record: InboundDelivery) => ({
              onClick: () => navigate(`/inbound-deliveries/${record.id}`),
              onKeyDown: (e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  navigate(`/inbound-deliveries/${record.id}`);
                }
              },
              style: { cursor: "pointer" },
              tabIndex: 0,
            })}
          >
            <Table.Column
              title={<Trans>Receipt #</Trans>}
              key="deliveryNumber"
              sorter={(a: InboundDelivery, b: InboundDelivery) =>
                (a.deliveryNumber ?? "").localeCompare(b.deliveryNumber ?? "")
              }
              render={(d: InboundDelivery) => (
                <Link to={`/inbound-deliveries/${d.id}`} onClick={(e) => e.stopPropagation()}>
                  {d.deliveryNumber}
                </Link>
              )}
            />
            <Table.Column
              title={<Trans>Vendor</Trans>}
              dataIndex="vendorName"
              key="vendorName"
              sorter={(a: InboundDelivery, b: InboundDelivery) =>
                (a.vendorName ?? "").localeCompare(b.vendorName ?? "")
              }
              render={(v: string | null) => v ?? "—"}
            />
            <Table.Column
              title={<Trans>Purchase order</Trans>}
              dataIndex="orderNumber"
              key="orderNumber"
              sorter={(a: InboundDelivery, b: InboundDelivery) =>
                (a.orderNumber ?? "").localeCompare(b.orderNumber ?? "")
              }
              render={(v: string | null) => v ?? "—"}
            />
            <Table.Column
              title={<Trans>Status</Trans>}
              dataIndex="status"
              key="status"
              sorter={(a: InboundDelivery, b: InboundDelivery) =>
                (a.status ?? "").localeCompare(b.status ?? "")
              }
              render={(status: string) => (
                <Tag color={inboundDeliveryStatusColor[status as InboundDeliveryStatus]}>
                  {inboundDeliveryStatusLabel(status)}
                </Tag>
              )}
            />
            <Table.Column
              title={<Trans>Receipt date</Trans>}
              dataIndex="deliveryDate"
              key="deliveryDate"
              sorter={(a: InboundDelivery, b: InboundDelivery) =>
                (a.deliveryDate ?? 0) - (b.deliveryDate ?? 0)
              }
              render={(v: number) => (v ? formatDate(v) : "—")}
            />
          </Table>
        </Col>
      </Row>
    </>
  );
};

export default InboundDeliveries;
