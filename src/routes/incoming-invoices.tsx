import { useEffect, useMemo, useState } from "react";
import type { IncomingInvoice } from "src/types/models";
import { Link, useLocation, useNavigate } from "react-router";
import { Button, Col, Empty, Row, Space, Table, Tag } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { AuditOutlined } from "@ant-design/icons";
import filter from "lodash/filter";

import { useDateFormatter } from "src/utils/date";
import { getFormattedNumber } from "src/utils/currencies";
import { centsToUnits } from "src/utils/currency";
import { GetIncomingInvoiceMatchSummaries } from "src/api";
import {
  INCOMING_INVOICE_STATES,
  incomingInvoiceStateColor,
  incomingInvoiceStateLabel,
  type IncomingInvoiceState,
} from "src/types/incoming-invoice";
import { organizationAtom } from "src/atoms/organization";
import { incomingInvoicesAtom, setIncomingInvoicesAtom } from "src/atoms/incoming-invoice";
import { vendorsAtom, setVendorsAtom } from "src/atoms/vendor";
import PageHeader from "src/components/page-header";
import DocumentFilters, { matchesDocumentFilters } from "src/components/document-filters";
import type { Dayjs } from "dayjs";

const IncomingInvoices = () => {
  const { i18n } = useLingui();
  const location = useLocation();
  const navigate = useNavigate();
  const formatDate = useDateFormatter();
  const organization = useAtomValue(organizationAtom);
  const invoices = useAtomValue(incomingInvoicesAtom);
  const setInvoices = useSetAtom(setIncomingInvoicesAtom);
  const setVendors = useSetAtom(setVendorsAtom);
  const [search, setSearch] = useState("");
  const [stateFilter, setStateFilter] = useState("");
  const [vendorFilter, setVendorFilter] = useState("");
  const [dateRange, setDateRange] = useState<[Dayjs, Dayjs] | null>(null);
  const vendors = useAtomValue(vendorsAtom);
  const [loading, setLoading] = useState(false);
  const [variances, setVariances] = useState<Record<string, boolean>>({});

  useEffect(() => {
    if (location.pathname === "/incoming-invoices") {
      setLoading(true);
      setVendors();
      setInvoices().finally(() => setLoading(false));
    }
  }, [location, setInvoices, setVendors]);

  useEffect(() => {
    if (location.pathname !== "/incoming-invoices" || !organization?.id) return;
    GetIncomingInvoiceMatchSummaries(organization.id)
      .then(setVariances)
      .catch(() => setVariances({}));
  }, [location, organization?.id]);

  const vendorOptions = useMemo(
    () =>
      (vendors as any[]).map((v) => ({
        value: v.id,
        label: [v.name, v.code ? `· ${v.code}` : v.phone].filter(Boolean).join(" "),
      })),
    [vendors],
  );

  const hasFilters = !!(search || stateFilter || vendorFilter || dateRange);

  const filtered = useMemo(
    () =>
      filter(invoices, (i: IncomingInvoice) =>
        matchesDocumentFilters({
          search,
          searchFields: [i.vendorInvoiceNumber, i.vendorName, i.reference],
          status: stateFilter,
          rowStatus: i.state,
          partyId: vendorFilter,
          rowPartyId: i.vendorId,
          dateRange,
          rowDate: i.date,
        }),
      ),
    [invoices, search, stateFilter, vendorFilter, dateRange],
  );

  return (
    <>
      <PageHeader
        icon={<AuditOutlined />}
        title={<Trans>Incoming Invoices</Trans>}
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
            status={stateFilter}
            onStatusChange={setStateFilter}
            statusOptions={INCOMING_INVOICE_STATES.map((s) => ({
              value: s,
              label: incomingInvoiceStateLabel(s),
            }))}
            partyOptions={vendorOptions}
            partyValue={vendorFilter}
            onPartyChange={setVendorFilter}
            partyPlaceholder={t`All vendors`}
          />
        }
        actions={
          <Button type="primary" onClick={() => navigate("/incoming-invoices/new")}>
            <Trans>New incoming invoice</Trans>
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
                <Empty description={<Trans>No incoming invoices match your filters</Trans>} />
              ) : (
                <Empty description={<Trans>No incoming invoices yet</Trans>}>
                  <Link to="/incoming-invoices/new">
                    <Button type="primary">
                      <Trans>Create your first incoming invoice</Trans>
                    </Button>
                  </Link>
                </Empty>
              ),
            }}
            onRow={(record: IncomingInvoice) => ({
              onClick: () => navigate(`/incoming-invoices/${record.id}`),
              onKeyDown: (e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  navigate(`/incoming-invoices/${record.id}`);
                }
              },
              style: { cursor: "pointer" },
              tabIndex: 0,
            })}
          >
            <Table.Column
              title={<Trans>Vendor invoice #</Trans>}
              key="vendorInvoiceNumber"
              sorter={(a: IncomingInvoice, b: IncomingInvoice) =>
                (a.vendorInvoiceNumber ?? "").localeCompare(b.vendorInvoiceNumber ?? "")
              }
              render={(i: IncomingInvoice) => (
                <Link to={`/incoming-invoices/${i.id}`} onClick={(e) => e.stopPropagation()}>
                  {i.vendorInvoiceNumber}
                </Link>
              )}
            />
            <Table.Column
              title={<Trans>Vendor</Trans>}
              dataIndex="vendorName"
              key="vendorName"
              sorter={(a: IncomingInvoice, b: IncomingInvoice) =>
                (a.vendorName ?? "").localeCompare(b.vendorName ?? "")
              }
              render={(v: string | null) => v ?? "—"}
            />
            <Table.Column
              title={<Trans>Purchase order</Trans>}
              dataIndex="orderNumber"
              sorter={(a: IncomingInvoice, b: IncomingInvoice) =>
                (a.orderNumber ?? "").localeCompare(b.orderNumber ?? "")
              }
              key="orderNumber"
              render={(v: string | null) => v ?? "—"}
            />
            <Table.Column
              title={<Trans>State</Trans>}
              dataIndex="state"
              sorter={(a: IncomingInvoice, b: IncomingInvoice) => a.state.localeCompare(b.state)}
              key="state"
              render={(state: string, record: IncomingInvoice) => (
                <Space size={4}>
                  <Tag color={incomingInvoiceStateColor[state as IncomingInvoiceState]}>
                    {incomingInvoiceStateLabel(state)}
                  </Tag>
                  {record.matchOverride === 1 && (
                    <Tag color="warning">
                      <Trans>Override</Trans>
                    </Tag>
                  )}
                  {record.matchOverride !== 1 && variances[record.id] && (
                    <Tag color="error">
                      <Trans>Variance</Trans>
                    </Tag>
                  )}
                </Space>
              )}
            />
            <Table.Column
              title={<Trans>Date</Trans>}
              dataIndex="date"
              key="date"
              sorter={(a: IncomingInvoice, b: IncomingInvoice) => (a.date ?? 0) - (b.date ?? 0)}
              render={(v: number) => (v ? formatDate(v) : "—")}
            />
            <Table.Column
              title={<Trans>Due date</Trans>}
              dataIndex="dueDate"
              key="dueDate"
              sorter={(a: IncomingInvoice, b: IncomingInvoice) =>
                (a.dueDate ?? 0) - (b.dueDate ?? 0)
              }
              render={(v: number | null) => (v ? formatDate(v) : "—")}
            />
            <Table.Column
              title={<Trans>Total</Trans>}
              dataIndex="total"
              key="total"
              align="right"
              sorter={(a: IncomingInvoice, b: IncomingInvoice) => (a.total ?? 0) - (b.total ?? 0)}
              render={(total: number, record: IncomingInvoice) =>
                getFormattedNumber(centsToUnits(total), record.currency, i18n.locale, organization)
              }
            />
          </Table>
        </Col>
      </Row>
    </>
  );
};

export default IncomingInvoices;
