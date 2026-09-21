import React, { useEffect, useMemo, useState } from "react";
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
  Tooltip,
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
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import dayjs from "dayjs";
import filter from "lodash/filter";

import {
  invoicesAtom,
  setInvoicesAtom,
  duplicateInvoiceAtom,
  deleteInvoiceAtom,
} from "src/atoms/invoice";
import { organizationAtom } from "src/atoms/organization";
import { clientsAtom, setClientsAtom } from "src/atoms/client";
import { getFormattedNumber } from "src/utils/currencies";
import { useDateFormatter } from "src/utils/date";
import InvoiceStateSelect from "src/components/invoices/state-select";
import PageHeader from "src/components/page-header";
import DocumentFilters, { matchesDocumentFilters } from "src/components/document-filters";
import { INVOICE_STATES, invoiceStateLabel } from "src/types/invoice";
import type { InvoiceDisplay } from "src/types/invoice";
import type { Dayjs } from "dayjs";

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
  const [stateFilterValue, setStateFilterValue] = useState("");
  const [clientFilter, setClientFilter] = useState("");
  const [dateRange, setDateRange] = useState<[Dayjs, Dayjs] | null>(null);
  const [loading, setLoading] = useState(false);
  const clients = useAtomValue(clientsAtom);
  // Computed once per component render rather than inside the Due date
  // column's per-row render callback, so every row's overdue comparison
  // uses the same instant instead of each potentially reading a slightly
  // different one. Reading the clock during render is exactly what a
  // display-only "is this already overdue" comparison needs — there's no
  // prop/state to derive it from instead, and the row naturally reflects a
  // fresher value on the next re-render (e.g. a refetch) with no staleness
  // risk worth guarding against here.
  // oxlint-disable-next-line react/purity
  const now = Date.now();

  useEffect(() => {
    setLoading(true);
    // Load the client list so the header's customer picker has options —
    // the list pages don't otherwise fetch it.
    setClients();
    setInvoices().finally(() => setLoading(false));
  }, [setInvoices, setClients]);

  const clientOptions = useMemo(
    () =>
      (clients as any[]).map((c) => ({
        value: c.id,
        label: [c.name, c.code ? `· ${c.code}` : c.phone].filter(Boolean).join(" "),
      })),
    [clients],
  );

  const hasFilters = !!(search || stateFilterValue || clientFilter || dateRange);

  const filtered = useMemo(
    () =>
      filter(invoices, (invoice: InvoiceDisplay) =>
        matchesDocumentFilters({
          search,
          searchFields: [invoice.clientName, invoice.number, invoice.customerNotes, invoice.total],
          status: stateFilterValue,
          rowStatus: invoice.state,
          partyId: clientFilter,
          rowPartyId: invoice.clientId,
          dateRange,
          rowDate: invoice.date,
        }),
      ),
    [invoices, search, stateFilterValue, clientFilter, dateRange],
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
            status={stateFilterValue}
            onStatusChange={setStateFilterValue}
            statusOptions={INVOICE_STATES.map((s) => ({ value: s, label: invoiceStateLabel(s) }))}
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
            <Link to={`/invoices/${invoice.id}`} onClick={(e) => e.stopPropagation()}>
              {number}
            </Link>
          )}
        />
        <Table.Column
          title={<Trans>Client</Trans>}
          dataIndex="clientName"
          sorter={(a: InvoiceDisplay, b: InvoiceDisplay) =>
            (a.clientName ?? "").localeCompare(b.clientName ?? "")
          }
          render={(clientName) => (clientName ? clientName : "—")}
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
          render={(date, invoice: InvoiceDisplay) => {
            if (!date) return "—";
            // A sent (unpaid) invoice past its due date is overdue — flag it.
            const overdue = invoice.state === "sent" && dayjs(date).valueOf() < now;
            if (!overdue) return formatDate(date);
            return (
              <Tooltip title={t`Overdue`}>
                <Typography.Text style={{ color: token.colorErrorText }}>
                  {formatDate(date)}
                </Typography.Text>{" "}
                <Tag color="red">{t`Overdue`}</Tag>
              </Tooltip>
            );
          }}
        />
        <Table.Column
          title={<Trans>Total</Trans>}
          dataIndex="total"
          key="total"
          align="right"
          sorter={(a: InvoiceDisplay, b: InvoiceDisplay) => a.total - b.total}
          render={(total, invoice: InvoiceDisplay) =>
            getFormattedNumber(total, invoice.currency, i18n.locale, organization)
          }
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
