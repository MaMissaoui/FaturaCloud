import React, { useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router";
import { Button, Empty, Table, Typography, Dropdown, MenuProps, Popconfirm, Tooltip } from "antd";
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
import get from "lodash/get";
import includes from "lodash/includes";
import some from "lodash/some";
import toString from "lodash/toString";

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
import DocumentFilters from "src/components/document-filters";
import { INVOICE_STATES, invoiceStateLabel } from "src/types/invoice";
import type { InvoiceDisplay } from "src/types/invoice";
import type { Dayjs } from "dayjs";

const Invoices = () => {
  // Built inside the component (not at module scope) so the filter labels
  // follow the active locale rather than freezing at import-time locale.
  const stateFilter = INVOICE_STATES.map((value) => ({
    text: invoiceStateLabel(value),
    value,
  }));

  const { i18n } = useLingui();
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

  const filtered = useMemo(() => {
    const fromMs = dateRange?.[0] ? dateRange[0].startOf("day").valueOf() : null;
    const toMs = dateRange?.[1] ? dateRange[1].endOf("day").valueOf() : null;
    return filter(invoices, (invoice: InvoiceDisplay) => {
      if (
        search &&
        !some(["clientName", "number", "customerNotes", "total"], (field) =>
          includes(toString(get(invoice, field)).toLowerCase(), search.toLowerCase()),
        )
      ) {
        return false;
      }
      if (stateFilterValue && invoice.state !== stateFilterValue) return false;
      if (clientFilter && invoice.clientId !== clientFilter) return false;
      if (fromMs !== null && (invoice.date ?? 0) < fromMs) return false;
      if (toMs !== null && (invoice.date ?? 0) > toMs) return false;
      return true;
    });
  }, [invoices, search, stateFilterValue, clientFilter, dateRange]);

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
        search={{ placeholder: t`Search text`, value: search, onChange: setSearch }}
        extra={
          <DocumentFilters
            dateRange={dateRange}
            onDateRangeChange={setDateRange}
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
        dataSource={filtered}
        pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
        rowKey="id"
        loading={loading}
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
          role: "link",
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
          render={(clientName) => (clientName ? clientName : "-")}
        />
        <Table.Column
          title={<Trans>Date</Trans>}
          dataIndex="date"
          key="date"
          sorter={(a: InvoiceDisplay, b: InvoiceDisplay) =>
            dayjs(a.date).valueOf() - dayjs(b.date).valueOf()
          }
          render={(date) => (date ? formatDate(date) : "-")}
        />
        <Table.Column
          title={<Trans>Due date</Trans>}
          dataIndex="dueDate"
          key="dueDate"
          sorter={(a: InvoiceDisplay, b: InvoiceDisplay) =>
            dayjs(a.dueDate).valueOf() - dayjs(b.dueDate).valueOf()
          }
          render={(date, invoice: InvoiceDisplay) => {
            if (!date) return "-";
            // A sent (unpaid) invoice past its due date is overdue — flag it.
            const overdue = invoice.state === "sent" && dayjs(date).valueOf() < now;
            if (!overdue) return formatDate(date);
            return (
              <Tooltip title={t`Overdue`}>
                <Typography.Text type="danger">{formatDate(date)}</Typography.Text>
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
          filters={stateFilter}
          onFilter={(value, record: InvoiceDisplay) => record.state === String(value)}
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
