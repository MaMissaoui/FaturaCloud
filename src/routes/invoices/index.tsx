import React, { useMemo, useState } from "react";
import { Link, useNavigate } from "react-router";
import {
  Button,
  Empty,
  Table,
  Tag,
  Typography,
  Dropdown,
  MenuProps,
  Popconfirm,
  theme,
} from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import {
  FileTextOutlined,
  MoreOutlined,
  CopyOutlined,
  EditOutlined,
  DeleteOutlined,
} from "@ant-design/icons";
import { Trans } from "@lingui/react/macro";
import { plural, t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import dayjs from "dayjs";
import filter from "lodash/filter";

import {
  invoicesAtom,
  setInvoicesAtom,
  duplicateInvoiceAtom,
  deleteInvoiceAtom,
} from "src/atoms/invoice";
import { organizationAtom, organizationIdAtom } from "src/atoms/organization";
import { clientsAtom, setClientsAtom } from "src/atoms/client";
import { formatOrgCents, getFormattedNumber } from "src/utils/currencies";
import { unitsToCents } from "src/utils/currency";
import { useDateFormatter } from "src/utils/date";
import InvoiceStateSelect from "src/components/invoices/state-select";
import PageHeader from "src/components/page-header";
import { useFetch } from "src/hooks/useFetch";
import DocumentFilters, { matchesDocumentFilters } from "src/components/document-filters";
import type { InvoiceDisplay } from "src/types/invoice";
import type { Dayjs } from "dayjs";
import {
  GetClientSummaries,
  GetOutstandingInvoices,
  type ClientSummaryList,
  type OutstandingDocument,
} from "src/api";
import FilterChips from "src/components/master-data/filter-chips";
import HeadlineFigure from "src/components/master-data/headline-figure";
import DueDate, { daysLate } from "src/components/master-data/due-date";
import { useSummariesEnabled } from "src/components/master-data/use-summaries-enabled";

type InvoiceChip = "all" | "draft" | "sent" | "overdue" | "paid" | "cancelled";

const Invoices = () => {
  const { i18n } = useLingui();
  const { token } = theme.useToken();
  const navigate = useNavigate();
  const formatDate = useDateFormatter();

  const organization = useAtomValue(organizationAtom);
  const invoices = useAtomValue(invoicesAtom);
  const setInvoices = useSetAtom(setInvoicesAtom);
  const setClients = useSetAtom(setClientsAtom);
  const duplicateInvoice = useSetAtom(duplicateInvoiceAtom);
  const deleteInvoice = useSetAtom(deleteInvoiceAtom);
  const [search, setSearch] = useState("");
  const [chip, setChip] = useState<InvoiceChip>("all");
  const [clientFilter, setClientFilter] = useState("");
  const [dateRange, setDateRange] = useState<[Dayjs, Dayjs] | null>(null);
  const clients = useAtomValue(clientsAtom);
  const organizationId = useAtomValue(organizationIdAtom);
  // The start of today, read once per render so every row compares against
  // the same day — only for the fallback below, when the outstanding list
  // couldn't be loaded.
  // oxlint-disable-next-line react/purity
  const today = dayjs(Date.now()).startOf("day").valueOf();

  // Loaded once per visit (a constant key). Loads the client list too, so
  // the header's customer picker has options — the list pages don't
  // otherwise fetch it.
  const { loading } = useFetch(
    ["invoices"],
    () => {
      setClients();
      return setInvoices();
    },
    undefined,
  );

  const clientOptions = useMemo(
    () =>
      (clients as any[]).map((c) => ({
        value: c.id,
        label: [c.name, c.code ? `· ${c.code}` : c.phone].filter(Boolean).join(" "),
      })),
    [clients],
  );

  const hasFilters = !!(search || chip !== "all" || clientFilter || dateRange);

  // What the search, client and date filters leave; the chips split it by
  // state, so each chip's count is exactly the rows it shows.
  const matching = useMemo(
    () =>
      filter(invoices, (invoice: InvoiceDisplay) =>
        matchesDocumentFilters({
          search,
          searchFields: [invoice.clientName, invoice.number, invoice.customerNotes, invoice.total],
          status: "",
          rowStatus: invoice.state,
          partyId: clientFilter,
          rowPartyId: invoice.clientId,
          dateRange,
          rowDate: invoice.date,
        }),
      ),
    [invoices, search, clientFilter, dateRange],
  );
  // Which invoices still have a balance and how late they are, from the
  // receivable aging query. The state alone can't say: it is a manual flag,
  // and an invoice paid in full often stays "sent".
  const { data: outstanding, failed: outstandingFailed } = useFetch<Map<
    string,
    OutstandingDocument
  > | null>(
    organizationId ? [organizationId, invoices.length] : null,
    () =>
      GetOutstandingInvoices(organizationId!).then((rows) => new Map(rows.map((r) => [r.id, r]))),
    null,
  );
  // How many days late a sent invoice is; 0 when it isn't (or is settled).
  // Falls back to the due date alone when the outstanding list failed.
  const lateBy = (invoice: InvoiceDisplay) => {
    if (invoice.state !== "sent") return 0;
    if (outstanding) {
      const row = outstanding.get(invoice.id);
      return row && row.bucket !== "current" ? row.daysOverdue : 0;
    }
    return outstandingFailed && invoice.dueDate
      ? daysLate(dayjs(invoice.dueDate).valueOf(), today)
      : 0;
  };
  // Until the balances arrive, the chips that depend on them show no count
  // rather than a state-based one that then jumps.
  const balancesPending = !outstanding && !outstandingFailed;
  const isUnpaid = (invoice: InvoiceDisplay) =>
    invoice.state === "sent" && (outstanding ? outstanding.has(invoice.id) : true);
  const inChip = (invoice: InvoiceDisplay, key: InvoiceChip) => {
    switch (key) {
      case "all":
        return true;
      case "sent":
        return isUnpaid(invoice);
      case "overdue":
        return isUnpaid(invoice) && lateBy(invoice) > 0;
      case "paid":
        // Marked paid, or sent with nothing left to pay.
        return invoice.state === "paid" || (invoice.state === "sent" && !isUnpaid(invoice));
      default:
        return invoice.state === key;
    }
  };
  const countOf = (key: InvoiceChip) => matching.filter((i) => inChip(i, key)).length;
  const filtered = matching.filter((i) => inChip(i, chip));

  // What clients owe — the Dashboard's figure, from the client summaries —
  // when the organization has summaries on and the role sees client balances.
  // Never a sum of the totals listed here: a part-paid invoice owes less than
  // its total, and a foreign-currency total isn't in the organization's.
  const summariesEnabled = useSummariesEnabled("client-balances");
  const { data: summaries } = useFetch<ClientSummaryList | null>(
    summariesEnabled && organizationId ? [organizationId, invoices.length] : null,
    () => GetClientSummaries(organizationId!),
    null,
  );

  const handleDuplicateInvoice = async (invoiceId: string) => {
    const newInvoiceId = await duplicateInvoice(invoiceId);
    if (newInvoiceId) {
      navigate(`/invoices/${newInvoiceId}`);
    }
  };

  const handleDeleteInvoice = async (invoiceId: string) => {
    await deleteInvoice(invoiceId);
  };

  const getActionItems = (invoice: InvoiceDisplay): MenuProps["items"] => [
    {
      key: "edit",
      label: <Trans>Edit</Trans>,
      icon: <EditOutlined />,
      onClick: () => navigate(`/invoices/${invoice.id}`),
    },
    {
      key: "duplicate",
      label: <Trans>Duplicate</Trans>,
      icon: <CopyOutlined />,
      onClick: () => handleDuplicateInvoice(invoice.id),
    },
    ...(invoice.state !== "paid"
      ? [
          { type: "divider" as const },
          {
            key: "delete",
            label: (
              <Popconfirm
                title={t`Delete the invoice?`}
                description={t`Are you sure to delete this invoice?`}
                onConfirm={(e?: React.MouseEvent<HTMLElement>) => {
                  e?.stopPropagation();
                  handleDeleteInvoice(invoice.id);
                }}
                okText={t`Yes`}
                cancelText={t`No`}
              >
                <span>
                  <DeleteOutlined /> <Trans>Delete</Trans>
                </span>
              </Popconfirm>
            ),
            onClick: (e: any) => e.domEvent.stopPropagation(),
          },
        ]
      : []),
  ];

  return (
    <>
      <PageHeader
        icon={<FileTextOutlined />}
        title={<Trans>Invoices</Trans>}
        search={{
          placeholder: t`Search text`,
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
            partyOptions={clientOptions}
            partyValue={clientFilter}
            onPartyChange={setClientFilter}
            partyPlaceholder={t`All clients`}
          />
        }
        actions={
          <Link to="/invoices/new">
            <Button type="primary" style={{ marginBottom: 10 }}>
              <Trans>New invoice</Trans>
            </Button>
          </Link>
        }
      />

      <p style={{ margin: "4px 0 0", color: token.colorTextSecondary }}>
        {plural(invoices.length, { one: "# invoice", other: "# invoices" })}
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
            label={<Trans>What your clients owe you</Trans>}
            value={formatOrgCents(summaries.totalOwed, organization, i18n.locale)}
            note={plural(summaries.owingCount, {
              one: "across # client",
              other: "across # clients",
            })}
          />
        )}
        <FilterChips<InvoiceChip>
          ariaLabel={t`Filter invoices`}
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
              key: "sent",
              label: <Trans context="document filter">Unpaid</Trans>,
              count: balancesPending ? undefined : countOf("sent"),
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

      <Table
        style={{ marginTop: 16 }}
        dataSource={filtered}
        pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
        rowKey="id"
        loading={loading}
        scroll={{ x: "max-content" }}
        locale={{
          emptyText: hasFilters ? (
            <Empty description={<Trans>No invoices match your filters</Trans>} />
          ) : (
            <Empty description={<Trans>No invoices yet</Trans>}>
              <Link to="/invoices/new">
                <Button type="primary">
                  <Trans>Create your first invoice</Trans>
                </Button>
              </Link>
            </Empty>
          ),
        }}
        onRow={(record: InvoiceDisplay) => ({
          onClick: () => navigate(`/invoices/${record.id}`),
          onKeyDown: (e) => {
            if (e.key === "Enter" || e.key === " ") {
              e.preventDefault();
              navigate(`/invoices/${record.id}`);
            }
          },
          style: { cursor: "pointer" },
          tabIndex: 0,
        })}
      >
        <Table.Column
          title="#"
          dataIndex="number"
          sorter={(a: InvoiceDisplay, b: InvoiceDisplay) =>
            a.number < b.number ? -1 : a.number === b.number ? 0 : 1
          }
          render={(number, invoice: InvoiceDisplay) => (
            <>
              <Link to={`/invoices/${invoice.id}`} onClick={(e) => e.stopPropagation()}>
                {number}
              </Link>
              {/* A loan brought forward from the paper register (its number
                  is the register reference) — see db/opening_loan.go. */}
              {invoice.origin === "opening" && (
                <Tag
                  style={{ marginLeft: 8 }}
                  title={t`Brought forward from the paper loan register`}
                >
                  {t`Migrated`}
                </Tag>
              )}
            </>
          )}
        />
        <Table.Column
          title={<Trans>Client</Trans>}
          dataIndex="clientName"
          sorter={(a: InvoiceDisplay, b: InvoiceDisplay) =>
            (a.clientName ?? "").localeCompare(b.clientName ?? "")
          }
          // Capped like the master-data lists' names (full name on hover), so a
          // long company name doesn't push the row menu out of view at 1280px.
          render={(clientName) =>
            clientName ? (
              <Typography.Text ellipsis={{ tooltip: clientName }} style={{ maxWidth: 220 }}>
                {clientName}
              </Typography.Text>
            ) : (
              "—"
            )
          }
        />
        <Table.Column
          title={<Trans>Date</Trans>}
          dataIndex="date"
          key="date"
          sorter={(a: InvoiceDisplay, b: InvoiceDisplay) =>
            dayjs(a.date).valueOf() - dayjs(b.date).valueOf()
          }
          render={(date) => (date ? formatDate(date) : "—")}
        />
        <Table.Column
          title={<Trans>Due date</Trans>}
          dataIndex="dueDate"
          key="dueDate"
          sorter={(a: InvoiceDisplay, b: InvoiceDisplay) =>
            dayjs(a.dueDate).valueOf() - dayjs(b.dueDate).valueOf()
          }
          render={(date, invoice: InvoiceDisplay) => (
            <DueDate
              date={date ? dayjs(date).valueOf() : null}
              daysLate={lateBy(invoice)}
              format={formatDate}
            />
          )}
        />
        <Table.Column
          title={<Trans>Total</Trans>}
          dataIndex="total"
          key="total"
          align="right"
          sorter={(a: InvoiceDisplay, b: InvoiceDisplay) => a.total - b.total}
          render={(total, invoice: InvoiceDisplay) => {
            const left = invoice.state === "sent" ? outstanding?.get(invoice.id) : undefined;
            // What is left of a part-paid invoice, in the organization's
            // currency (the aging figure); always shown for a foreign one.
            const partPaid =
              !!left &&
              ((!!invoice.currency && invoice.currency !== organization?.currency) ||
                left.outstanding !== unitsToCents(total));
            // Sent but nothing left: listed under Paid though its state still
            // reads Sent (the state is manual), so say why.
            const paidInFull = invoice.state === "sent" && !!outstanding && !left;
            return (
              <>
                <span style={{ whiteSpace: "nowrap" }}>
                  {getFormattedNumber(total, invoice.currency, i18n.locale, organization)}
                </span>
                {partPaid && (
                  <div
                    style={{ fontSize: 12, color: token.colorTextSecondary, whiteSpace: "nowrap" }}
                  >
                    {t`${formatOrgCents(left.outstanding, organization, i18n.locale)} left`}
                  </div>
                )}
                {paidInFull && (
                  <div
                    style={{ fontSize: 12, color: token.colorTextSecondary, whiteSpace: "nowrap" }}
                  >
                    <Trans context="invoice">Paid in full</Trans>
                  </div>
                )}
              </>
            );
          }}
        />
        <Table.Column
          title={<Trans>State</Trans>}
          key="state"
          sorter={(a: InvoiceDisplay, b: InvoiceDisplay) =>
            (a.state ?? "").localeCompare(b.state ?? "")
          }
          render={(invoice) => (
            <span onClick={(e) => e.stopPropagation()}>
              <InvoiceStateSelect invoice={invoice} />
            </span>
          )}
        />
        <Table.Column
          key="actions"
          align="center"
          width={60}
          render={(invoice) => (
            <span onClick={(e) => e.stopPropagation()}>
              <Dropdown menu={{ items: getActionItems(invoice) }} trigger={["click"]}>
                <Button type="text" icon={<MoreOutlined />} aria-label={t`More actions`} />
              </Dropdown>
            </span>
          )}
        />
      </Table>
    </>
  );
};

export default Invoices;
