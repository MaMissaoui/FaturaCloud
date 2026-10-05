import { useEffect, useMemo, useState } from "react";
import type { IncomingInvoice } from "src/types/models";
import { Link, useLocation, useNavigate } from "react-router";
import { Button, Col, Empty, Row, Space, Table, Tag, theme } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { plural, t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { AuditOutlined } from "@ant-design/icons";
import filter from "lodash/filter";

import { useDateFormatter } from "src/utils/date";
import { formatOrgCents, getFormattedNumber } from "src/utils/currencies";
import { centsToUnits } from "src/utils/currency";
import {
  GetIncomingInvoiceMatchSummaries,
  GetOutstandingIncomingInvoices,
  GetVendorSummaries,
  type OutstandingDocument,
  type VendorSummaryList,
} from "src/api";
import {
  incomingInvoiceStateColor,
  incomingInvoiceStateLabel,
  type IncomingInvoiceState,
} from "src/types/incoming-invoice";
import { organizationAtom } from "src/atoms/organization";
import { incomingInvoicesAtom, setIncomingInvoicesAtom } from "src/atoms/incoming-invoice";
import { vendorsAtom, setVendorsAtom } from "src/atoms/vendor";
import PageHeader from "src/components/page-header";
import DocumentFilters, { matchesDocumentFilters } from "src/components/document-filters";
import dayjs, { type Dayjs } from "dayjs";
import { useLoadOnPath } from "src/hooks/useLoadOnPath";
import { useFetch } from "src/hooks/useFetch";
import FilterChips from "src/components/master-data/filter-chips";
import HeadlineFigure from "src/components/master-data/headline-figure";
import DueDate, { daysLate } from "src/components/master-data/due-date";
import { useSummariesEnabled } from "src/components/master-data/use-summaries-enabled";
import { useWideScreen } from "src/components/master-data/use-wide-screen";

type BillChip = "all" | "draft" | "approved" | "overdue" | "paid" | "cancelled";

const IncomingInvoices = () => {
  const { i18n } = useLingui();
  const { token } = theme.useToken();
  const wide = useWideScreen();
  const location = useLocation();
  const navigate = useNavigate();
  const formatDate = useDateFormatter();
  const organization = useAtomValue(organizationAtom);
  const invoices = useAtomValue(incomingInvoicesAtom);
  const setInvoices = useSetAtom(setIncomingInvoicesAtom);
  const setVendors = useSetAtom(setVendorsAtom);
  const [search, setSearch] = useState("");
  const [chip, setChip] = useState<BillChip>("all");
  const [vendorFilter, setVendorFilter] = useState("");
  const [dateRange, setDateRange] = useState<[Dayjs, Dayjs] | null>(null);
  const vendors = useAtomValue(vendorsAtom);
  const [variances, setVariances] = useState<Record<string, boolean>>({});

  const loading = useLoadOnPath("/incoming-invoices", () => {
    setVendors();
    return setInvoices();
  });

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

  const hasFilters = !!(search || chip !== "all" || vendorFilter || dateRange);
  // The start of today, once per render, for the fallback below (see
  // src/routes/invoices/index.tsx).
  // oxlint-disable-next-line react/purity
  const today = dayjs(Date.now()).startOf("day").valueOf();

  // What the search, vendor and date filters leave; the chips split it by
  // state, so each chip's count is exactly the rows it shows.
  const matching = useMemo(
    () =>
      filter(invoices, (i: IncomingInvoice) =>
        matchesDocumentFilters({
          search,
          searchFields: [i.vendorInvoiceNumber, i.vendorName, i.reference],
          status: "",
          rowStatus: i.state,
          partyId: vendorFilter,
          rowPartyId: i.vendorId,
          dateRange,
          rowDate: i.date,
        }),
      ),
    [invoices, search, vendorFilter, dateRange],
  );
  // Which bills still have a balance and how late they are, from the payable
  // aging query — a bill paid in full before payments kept the state in step
  // (2026-10), or set by hand, can still read approved.
  const { data: outstanding, failed: outstandingFailed } = useFetch<Map<
    string,
    OutstandingDocument
  > | null>(
    organization?.id ? [organization.id, invoices.length] : null,
    () =>
      GetOutstandingIncomingInvoices(organization!.id).then(
        (rows) => new Map(rows.map((r) => [r.id, r])),
      ),
    null,
  );
  // How many days late an approved bill is; 0 when it isn't (or is settled).
  // Falls back to the due date alone when the outstanding list failed.
  const lateBy = (i: IncomingInvoice) => {
    if (i.state !== "approved") return 0;
    if (outstanding) {
      const row = outstanding.get(i.id);
      return row && row.bucket !== "current" ? row.daysOverdue : 0;
    }
    return outstandingFailed && i.dueDate ? daysLate(i.dueDate, today) : 0;
  };
  // Until the balances arrive, the chips that depend on them show no count
  // rather than a state-based one that then jumps.
  const balancesPending = !outstanding && !outstandingFailed;
  const isToPay = (i: IncomingInvoice) =>
    i.state === "approved" && (outstanding ? outstanding.has(i.id) : true);
  const inChip = (i: IncomingInvoice, key: BillChip) => {
    switch (key) {
      case "all":
        return true;
      case "approved":
        return isToPay(i);
      case "overdue":
        return isToPay(i) && lateBy(i) > 0;
      case "paid":
        // Marked paid, or approved with nothing left to pay.
        return i.state === "paid" || (i.state === "approved" && !isToPay(i));
      default:
        return i.state === key;
    }
  };
  const countOf = (key: BillChip) => matching.filter((i) => inChip(i, key)).length;
  const filtered = matching.filter((i) => inChip(i, chip));

  // What the organization owes its vendors — the payables aging total, from
  // the vendor summaries — when summaries are on and the role sees vendor
  // balances. Never a sum of the totals listed here (part-paid bills, foreign
  // currencies).
  const summariesEnabled = useSummariesEnabled("vendor-balances");
  const { data: summaries } = useFetch<VendorSummaryList | null>(
    summariesEnabled && organization?.id ? [organization.id, invoices.length] : null,
    () => GetVendorSummaries(organization!.id),
    null,
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
            dateLabel={t`Date`}
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

      <p style={{ margin: "4px 0 0", color: token.colorTextSecondary }}>
        {plural(invoices.length, { one: "# incoming invoice", other: "# incoming invoices" })}
      </p>

      <div
        style={{
          display: "flex",
          flexWrap: "wrap",
          alignItems: "flex-end",
          justifyContent: "space-between",
          gap: "12px 24px",
          borderTop: `1px solid ${token.colorBorderSecondary}`,
          marginTop: 16,
          paddingTop: 20,
        }}
      >
        {summaries && (
          <HeadlineFigure
            label={<Trans>What you owe your vendors</Trans>}
            value={formatOrgCents(summaries.totalOwed, organization, i18n.locale)}
            note={plural(summaries.owingCount, {
              one: "across # vendor",
              other: "across # vendors",
            })}
          />
        )}
        <FilterChips<BillChip>
          ariaLabel={t`Filter incoming invoices`}
          value={chip}
          onChange={setChip}
          chips={[
            {
              key: "all",
              label: <Trans context="document filter">All</Trans>,
              count: countOf("all"),
            },
            {
              key: "draft",
              label: <Trans context="document filter">Draft</Trans>,
              count: countOf("draft"),
            },
            {
              key: "approved",
              label: <Trans context="document filter">To pay</Trans>,
              count: balancesPending ? undefined : countOf("approved"),
            },
            {
              key: "overdue",
              label: <Trans context="document filter">Overdue</Trans>,
              count: balancesPending ? undefined : countOf("overdue"),
            },
            {
              key: "paid",
              label: <Trans context="document filter">Paid</Trans>,
              count: balancesPending ? undefined : countOf("paid"),
            },
            ...(countOf("cancelled") > 0 || chip === "cancelled"
              ? [
                  {
                    key: "cancelled" as const,
                    label: <Trans context="document filter">Cancelled</Trans>,
                    count: countOf("cancelled"),
                  },
                ]
              : []),
          ]}
        />
      </div>

      <Row style={{ marginTop: 16 }}>
        <Col span={24}>
          <Table
            dataSource={filtered}
            pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
            rowKey="id"
            loading={loading}
            scroll={{ x: "max-content" }}
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
            {/* Only from 1440px: below that the due date, which the
                Overdue filter is about, would sit under the pinned total. */}
            {wide && (
              <Table.Column
                title={<Trans>Purchase order</Trans>}
                dataIndex="orderNumber"
                sorter={(a: IncomingInvoice, b: IncomingInvoice) =>
                  (a.orderNumber ?? "").localeCompare(b.orderNumber ?? "")
                }
                key="orderNumber"
                render={(v: string | null) => v ?? "—"}
              />
            )}
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
              render={(v: number | null, record: IncomingInvoice) => (
                <DueDate date={v} daysLate={lateBy(record)} format={formatDate} />
              )}
            />
            <Table.Column
              title={<Trans>Total</Trans>}
              dataIndex="total"
              key="total"
              align="right"
              // Pinned: the purchase order and the days-late note make the
              // table wider than the page at 1440px and below.
              fixed="right"
              sorter={(a: IncomingInvoice, b: IncomingInvoice) => (a.total ?? 0) - (b.total ?? 0)}
              render={(total: number, record: IncomingInvoice) => {
                const left = record.state === "approved" ? outstanding?.get(record.id) : undefined;
                // What is left of a part-paid bill, in the organization's
                // currency (the aging figure); always shown for a foreign one.
                const partPaid =
                  !!left &&
                  ((!!record.currency && record.currency !== organization?.currency) ||
                    left.outstanding !== total);
                // Approved but nothing left: listed under Paid though its
                // state still reads Approved (paid before payments set it), so say why.
                const paidInFull = record.state === "approved" && !!outstanding && !left;
                return (
                  <>
                    <span style={{ whiteSpace: "nowrap" }}>
                      {getFormattedNumber(
                        centsToUnits(total),
                        record.currency,
                        i18n.locale,
                        organization,
                      )}
                    </span>
                    {partPaid && (
                      <div
                        style={{
                          fontSize: 12,
                          color: token.colorTextSecondary,
                          whiteSpace: "nowrap",
                        }}
                      >
                        {t`${formatOrgCents(left.outstanding, organization, i18n.locale)} left`}
                      </div>
                    )}
                    {paidInFull && (
                      <div
                        style={{
                          fontSize: 12,
                          color: token.colorTextSecondary,
                          whiteSpace: "nowrap",
                        }}
                      >
                        <Trans context="invoice">Paid in full</Trans>
                      </div>
                    )}
                  </>
                );
              }}
            />
          </Table>
        </Col>
      </Row>
    </>
  );
};

export default IncomingInvoices;
