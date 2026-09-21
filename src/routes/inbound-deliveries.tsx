import { useEffect, useMemo, useState } from "react";
import type { InboundDelivery } from "src/types/models";
import { Link, useLocation, useNavigate } from "react-router";
import { Button, Col, Empty, Row, Table, Tag } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { ImportOutlined } from "@ant-design/icons";
import filter from "lodash/filter";

import { useDateFormatter } from "src/utils/date";
import {
  INBOUND_DELIVERY_STATUSES,
  inboundDeliveryStatusColor,
  inboundDeliveryStatusLabel,
  type InboundDeliveryStatus,
} from "src/types/inbound-delivery";
import { inboundDeliveriesAtom, setInboundDeliveriesAtom } from "src/atoms/inbound-delivery";
import { vendorsAtom, setVendorsAtom } from "src/atoms/vendor";
import PageHeader from "src/components/page-header";
import DocumentFilters, { matchesDocumentFilters } from "src/components/document-filters";
import type { Dayjs } from "dayjs";

const InboundDeliveries = () => {
  useLingui();
  const location = useLocation();
  const navigate = useNavigate();
  const formatDate = useDateFormatter();
  const deliveries = useAtomValue(inboundDeliveriesAtom);
  const setDeliveries = useSetAtom(setInboundDeliveriesAtom);
  const setVendors = useSetAtom(setVendorsAtom);
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [vendorFilter, setVendorFilter] = useState("");
  const [dateRange, setDateRange] = useState<[Dayjs, Dayjs] | null>(null);
  const vendors = useAtomValue(vendorsAtom);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (location.pathname === "/inbound-deliveries") {
      setLoading(true);
      setVendors();
      setDeliveries().finally(() => setLoading(false));
    }
  }, [location, setDeliveries, setVendors]);

  const vendorOptions = useMemo(
    () =>
      (vendors as any[]).map((v) => ({
        value: v.id,
        label: [v.name, v.code ? `· ${v.code}` : v.phone].filter(Boolean).join(" "),
      })),
    [vendors],
  );

  const hasFilters = !!(search || statusFilter || vendorFilter || dateRange);

  const filtered = useMemo(
    () =>
      filter(deliveries, (d: InboundDelivery) =>
        matchesDocumentFilters({
          search,
          searchFields: [d.deliveryNumber, d.vendorName, d.orderNumber],
          status: statusFilter,
          rowStatus: d.status,
          partyId: vendorFilter,
          rowPartyId: d.vendorId,
          dateRange,
          rowDate: d.deliveryDate,
        }),
      ),
    [deliveries, search, statusFilter, vendorFilter, dateRange],
  );

  return (
    <>
      <PageHeader
        icon={<ImportOutlined />}
        title={<Trans>Goods Receipts</Trans>}
        search={{ placeholder: t`Search`, value: search, onChange: setSearch }}
        extra={
          <DocumentFilters
            dateRange={dateRange}
            onDateRangeChange={setDateRange}
            status={statusFilter}
            onStatusChange={setStatusFilter}
            statusPlaceholder={t`All statuses`}
            statusAriaLabel={t`Filter by status`}
            statusOptions={INBOUND_DELIVERY_STATUSES.map((s) => ({
              value: s,
              label: inboundDeliveryStatusLabel(s),
            }))}
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

      <Row style={{ marginTop: 16 }}>
        <Col span={24}>
          <Table
            dataSource={filtered}
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
              role: "link",
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
              filters={INBOUND_DELIVERY_STATUSES.map((s) => ({
                text: inboundDeliveryStatusLabel(s),
                value: s,
              }))}
              onFilter={(value, record: InboundDelivery) => record.status === value}
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
