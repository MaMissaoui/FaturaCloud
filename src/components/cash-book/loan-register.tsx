import { useMemo, useState, type ReactNode } from "react";
import {
  Alert,
  Button,
  Card,
  Col,
  Empty,
  Input,
  Row,
  Segmented,
  Space,
  Table,
  theme,
  Typography,
} from "antd";
import { Trans } from "@lingui/react/macro";
import { plural, t } from "@lingui/core/macro";
import { FileExcelOutlined, FilePdfOutlined, ShoppingCartOutlined } from "@ant-design/icons";
import dayjs from "dayjs";

import type { LoanStatusRow } from "src/api";
import type { Client, Payment } from "src/types/models";
import PaymentProductsCell from "src/components/payments/payment-products-cell";
import { paymentRowMethodLabel } from "src/types/payment";
import Slip from "src/components/cash-book/slip";
import { customerIdentifiers } from "src/components/cash-book/shared";
import {
  buildLoanRegister,
  filterLoanRegister,
  STALE_AFTER_DAYS,
  type LoanRegisterFilter,
  type RegisterCustomer,
} from "src/components/cash-book/loan-register-model";
import type { CashBookState } from "src/components/cash-book/use-cash-book";

const figure = { fontVariantNumeric: "tabular-nums" as const, whiteSpace: "nowrap" as const };

const matches = (c: RegisterCustomer, client: any, needle: string) =>
  !needle ||
  [
    c.clientName,
    client?.code,
    client?.phone,
    client?.phone2,
    client?.phone3,
    client?.identity_number,
  ]
    .filter(Boolean)
    .join(" ")
    .toLowerCase()
    .includes(needle);

// One line of the customer list: name and last activity on the left, the
// balance on a small slip in the customer's tone on the right.
const CustomerRow = ({
  customer,
  selected,
  onSelect,
  money,
  dateFormat,
}: {
  customer: RegisterCustomer;
  selected: boolean;
  onSelect: () => void;
  money: (cents: number) => string;
  dateFormat: string;
}) => {
  const { token } = theme.useToken();
  const c = customer;
  const activity =
    c.tone === "settled"
      ? t`Settled on ${dayjs(c.lastPaymentDate ?? c.lastSaleDate).format(dateFormat)}`
      : c.lastPaymentDate === null
        ? plural(c.idleDays, {
            0: "No payment yet, sold today",
            one: "No payment yet, sold # day ago",
            other: "No payment yet, sold # days ago",
          })
        : c.tone === "stale"
          ? t`Nothing paid for ${c.idleDays} days`
          : plural(c.idleDays, {
              0: "Last payment today",
              one: "Last payment # day ago",
              other: "Last payment # days ago",
            });
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-current={selected ? "true" : undefined}
      style={{
        width: "100%",
        display: "grid",
        gridTemplateColumns: "minmax(0, 1fr) auto",
        gap: "2px 12px",
        alignItems: "center",
        padding: "12px 14px",
        border: 0,
        borderBottom: `1px solid ${token.colorBorderSecondary}`,
        background: selected ? token.controlItemBgActive : "transparent",
        boxShadow: selected ? `inset 3px 0 0 ${token.colorPrimary}` : undefined,
        textAlign: "left",
        font: "inherit",
        color: token.colorText,
        cursor: "pointer",
      }}
    >
      <Typography.Text strong ellipsis style={{ fontSize: 15 }}>
        {c.clientName}
      </Typography.Text>
      <Slip tone={c.tone} style={{ padding: "2px 10px", justifySelf: "end" }}>
        <span style={{ ...figure, fontWeight: 600 }}>
          {c.tone === "settled" ? <Trans>Settled</Trans> : money(c.outstanding)}
        </span>
      </Slip>
      <Typography.Text
        type={c.tone === "stale" ? "danger" : "secondary"}
        strong={c.tone === "stale"}
        style={{ fontSize: 13 }}
      >
        {activity}
      </Typography.Text>
      <Typography.Text type="secondary" style={{ fontSize: 13, textAlign: "right" }}>
        {c.amount > 0 ? t`${Math.round((c.paid / c.amount) * 100)}% paid` : ""}
      </Typography.Text>
    </button>
  );
};

// The selected customer's card: identity, totals, each loan as a slip with a
// Collect button per item still owed, and their payment history.
const CustomerDetail = ({
  customer,
  client,
  payments,
  cb,
  onServe,
}: {
  customer: RegisterCustomer;
  client: Client | undefined;
  payments: Payment[];
  cb: CashBookState;
  onServe: (client: Client) => void;
}) => {
  const { token } = theme.useToken();
  const {
    money,
    dateFormat,
    isToday,
    openPayment,
    handleExportPaymentHistory,
    downloadingPaymentsPdf,
    downloadingPaymentsExcel,
  } = cb;
  const c = customer;
  const details: any = client ?? {};
  const total = (label: ReactNode, value: string, big?: boolean) => (
    <div>
      <Typography.Text type="secondary" style={{ fontSize: 13 }}>
        {label}
      </Typography.Text>
      <div
        style={{
          ...figure,
          fontSize: big ? 30 : 18,
          fontWeight: big ? 700 : 600,
          lineHeight: 1.15,
          color: big ? token.colorPrimary : undefined,
        }}
      >
        {value}
      </div>
    </div>
  );
  return (
    <Card size="small" styles={{ body: { padding: 20 } }}>
      <div style={{ display: "flex", justifyContent: "space-between", gap: 20, flexWrap: "wrap" }}>
        <div style={{ minWidth: 0 }}>
          <Typography.Title level={3} style={{ margin: "0 0 6px" }}>
            {c.clientName}
          </Typography.Title>
          <Space wrap size={[16, 4]}>
            {customerIdentifiers(details, (text) => text)}
            {details.guarantor ? (
              <span>
                <Trans>Guarantor</Trans>: {details.guarantor}
              </span>
            ) : null}
          </Space>
        </div>
        <div style={{ display: "flex", gap: 24, alignItems: "flex-end", flexWrap: "wrap" }}>
          {total(<Trans>Sold</Trans>, money(c.amount))}
          {total(<Trans>Paid</Trans>, money(c.paid))}
          {total(<Trans>Balance owing</Trans>, money(c.outstanding), true)}
        </div>
      </div>

      {client && isToday && (
        <Button
          icon={<ShoppingCartOutlined />}
          style={{ marginTop: 14 }}
          onClick={() => onServe(client)}
        >
          <Trans>Serve at the counter</Trans>
        </Button>
      )}

      {c.tone === "stale" && (
        <Alert
          type="error"
          showIcon
          style={{ marginTop: 16 }}
          message={
            c.lastPaymentDate === null ? (
              <Trans>No payment since the sale, {c.idleDays} days ago.</Trans>
            ) : (
              <Trans>
                Last payment on {dayjs(c.lastPaymentDate).format(dateFormat)}, {c.idleDays} days
                ago.
              </Trans>
            )
          }
        />
      )}

      <div style={{ display: "flex", flexDirection: "column", gap: 14, marginTop: 18 }}>
        {c.loans.map((loan) => (
          <Slip
            key={loan.invoiceId}
            tone={loan.outstanding <= 0 ? "settled" : c.tone === "stale" ? "stale" : "open"}
          >
            <Space wrap size={12} style={{ marginBottom: 8 }}>
              <Typography.Text strong style={{ fontSize: 15, color: token.colorPrimary }}>
                {loan.invoiceNumber || "—"}
              </Typography.Text>
              <Typography.Text type="secondary">
                <Trans>sold on {dayjs(loan.date).format(dateFormat)}</Trans>
              </Typography.Text>
            </Space>
            <Table<LoanStatusRow>
              dataSource={loan.lines}
              rowKey="lineId"
              size="small"
              pagination={false}
              scroll={{ x: "max-content" }}
              style={{ background: "transparent" }}
              columns={[
                {
                  title: t`Product`,
                  key: "product",
                  render: (_, row) => (
                    <Typography.Text
                      ellipsis={{ tooltip: row.productName }}
                      style={{ maxWidth: 190 }}
                    >
                      {row.productName || "—"}
                    </Typography.Text>
                  ),
                },
                { title: t`Qty`, key: "qty", align: "right", render: (_, row) => row.quantity },
                {
                  title: t`Amount`,
                  key: "amount",
                  align: "right",
                  render: (_, row) => <span style={figure}>{money(row.amount)}</span>,
                },
                {
                  title: t`Paid`,
                  key: "paid",
                  align: "right",
                  render: (_, row) => <span style={figure}>{money(row.paid)}</span>,
                },
                {
                  title: t`Balance owing`,
                  key: "outstanding",
                  align: "right",
                  // Pinned with the action: on a narrow screen Amount and Paid
                  // scroll, but the balance and its Collect button stay.
                  fixed: "right",
                  render: (_, row) => (
                    <Typography.Text strong style={figure}>
                      {money(row.outstanding)}
                    </Typography.Text>
                  ),
                },
                {
                  key: "action",
                  align: "right",
                  fixed: "right",
                  render: (_, row) =>
                    row.outstanding > 0 ? (
                      <Button
                        size="small"
                        disabled={!isToday}
                        title={!isToday ? t`Switch to today to record a payment` : undefined}
                        onClick={() => openPayment(row)}
                        aria-label={`${t`Collect`} ${row.productName}`}
                      >
                        <Trans>Collect</Trans>
                      </Button>
                    ) : (
                      <Typography.Text type="secondary">
                        <Trans>Paid off</Trans>
                      </Typography.Text>
                    ),
                },
              ]}
            />
          </Slip>
        ))}
      </div>

      <div
        style={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          gap: 12,
          flexWrap: "wrap",
          margin: "24px 0 8px",
        }}
      >
        <Typography.Title level={5} style={{ margin: 0 }}>
          <Trans>Payment history</Trans>
        </Typography.Title>
        <Space wrap>
          <Button
            loading={downloadingPaymentsPdf}
            onClick={handleExportPaymentHistory("pdf", { clientId: c.clientId })}
          >
            <FilePdfOutlined /> PDF
          </Button>
          <Button
            loading={downloadingPaymentsExcel}
            onClick={handleExportPaymentHistory("xlsx", { clientId: c.clientId })}
          >
            <FileExcelOutlined /> <Trans>Excel</Trans>
          </Button>
        </Space>
      </div>
      <Table<Payment>
        dataSource={payments}
        rowKey="id"
        size="small"
        scroll={{ x: "max-content" }}
        pagination={{ hideOnSinglePage: true, defaultPageSize: 10 }}
        locale={{ emptyText: <Trans>No payments yet</Trans> }}
        columns={[
          {
            title: t`Date`,
            key: "date",
            render: (_, p) => <span style={figure}>{dayjs(p.date).format(dateFormat)}</span>,
          },
          {
            title: t`Invoice`,
            key: "invoices",
            render: (_, p) => (p.invoiceNumbers?.length ? p.invoiceNumbers.join(", ") : "—"),
          },
          {
            title: t`Product`,
            key: "products",
            render: (_, p) => <PaymentProductsCell payment={p} money={money} />,
          },
          { title: t`Method`, key: "method", render: (_, p) => paymentRowMethodLabel(p) },
          { title: t`Reference`, key: "reference", render: (_, p) => p.reference || "—" },
          {
            title: t`Amount`,
            key: "amount",
            align: "right",
            // Pinned like the current layout's payment history (#448): a long
            // product list must not push the amount off a narrow card.
            fixed: "right",
            render: (_, p) => <span style={figure}>{money(p.amount)}</span>,
          },
        ]}
      />
    </Card>
  );
};

// The new layout's second tab: every customer with a loan, from the same
// loan-status and payments data the counter uses. Stalled loans (no payment
// for STALE_AFTER_DAYS) are flagged and listed first.
const LoanRegister = ({
  cb,
  onServe,
}: {
  cb: CashBookState;
  onServe: (client: Client) => void;
}) => {
  const {
    loanStatusRows,
    loadingLoanStatus,
    loanStatusFailed,
    payments,
    clients,
    money,
    dateFormat,
    handleExportLoanStatus,
    downloadingLoanPdf,
    downloadingLoanExcel,
  } = cb;
  // Fixed for the visit, so "days since" doesn't shift under a re-render.
  const [now] = useState(() => Date.now());
  const [filter, setFilter] = useState<LoanRegisterFilter>("open");
  const [query, setQuery] = useState("");
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const customers = useMemo(
    () => buildLoanRegister(loanStatusRows, payments, now),
    [loanStatusRows, payments, now],
  );
  const clientById = useMemo(() => {
    const byId = new Map<string, Client>();
    for (const c of clients as Client[]) byId.set(c.id, c);
    return byId;
  }, [clients]);

  const owing = filterLoanRegister(customers, "open");
  const stale = filterLoanRegister(customers, "stale");
  const settled = filterLoanRegister(customers, "settled");
  const needle = query.trim().toLowerCase();
  const list = (filter === "stale" ? stale : filter === "settled" ? settled : owing).filter((c) =>
    matches(c, clientById.get(c.clientId), needle),
  );
  const selected = list.find((c) => c.clientId === selectedId) ?? list[0];
  const owedText = money(owing.reduce((n, c) => n + c.outstanding, 0));
  const selectedPayments = selected
    ? payments.filter((p) => p.direction === "inbound" && p.clientId === selected.clientId)
    : [];

  return (
    <>
      <Typography.Paragraph style={{ fontSize: 16, marginBottom: 14 }}>
        {plural(owing.length, {
          one: `# customer owes ${owedText} in total.`,
          other: `# customers owe ${owedText} in total.`,
        })}{" "}
        {stale.length > 0 && (
          <Typography.Text type="danger" strong>
            {plural(stale.length, {
              one: `# has paid nothing for more than ${STALE_AFTER_DAYS} days.`,
              other: `# have paid nothing for more than ${STALE_AFTER_DAYS} days.`,
            })}
          </Typography.Text>
        )}
      </Typography.Paragraph>

      {loanStatusFailed && (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 12 }}
          message={
            <Trans>Open-loan figures couldn't be loaded — any loan shown may be incomplete</Trans>
          }
        />
      )}

      <div
        style={{
          display: "flex",
          gap: 10,
          flexWrap: "wrap",
          alignItems: "center",
          marginBottom: 14,
        }}
      >
        <Input.Search
          allowClear
          style={{ flex: "1 1 240px", maxWidth: 360 }}
          placeholder={t`Name, mobile or identity number`}
          aria-label={t`Name, mobile or identity number`}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <Segmented<LoanRegisterFilter>
          aria-label={t`Filter loans`}
          // Scrolls inside itself on a phone rather than widening the page.
          style={{ maxWidth: "100%", overflowX: "auto" }}
          value={filter}
          onChange={setFilter}
          options={[
            { value: "open", label: t`Owing (${owing.length})` },
            {
              value: "stale",
              label: t`No payment for ${STALE_AFTER_DAYS}+ days (${stale.length})`,
            },
            { value: "settled", label: t`Settled (${settled.length})` },
          ]}
        />
        <Space wrap style={{ marginLeft: "auto" }}>
          <Button
            loading={downloadingLoanPdf}
            onClick={handleExportLoanStatus("pdf", {
              clientId: "",
              openOnly: filter !== "settled",
            })}
          >
            <FilePdfOutlined /> PDF
          </Button>
          <Button
            loading={downloadingLoanExcel}
            onClick={handleExportLoanStatus("xlsx", {
              clientId: "",
              openOnly: filter !== "settled",
            })}
          >
            <FileExcelOutlined /> <Trans>Excel</Trans>
          </Button>
        </Space>
      </div>

      <Row gutter={[16, 16]} align="top">
        <Col xs={24} lg={9} xxl={8} style={{ minWidth: 0 }}>
          <Card size="small" loading={loadingLoanStatus} styles={{ body: { padding: 0 } }}>
            {list.length === 0 ? (
              <Empty style={{ padding: 24 }} description={<Trans>No loan sales</Trans>} />
            ) : (
              <ul
                aria-label={t`Customers`}
                style={{
                  listStyle: "none",
                  margin: 0,
                  padding: 0,
                  maxHeight: 720,
                  overflowY: "auto",
                }}
              >
                {list.map((c) => (
                  <li key={c.clientId}>
                    <CustomerRow
                      customer={c}
                      selected={c.clientId === selected?.clientId}
                      onSelect={() => setSelectedId(c.clientId)}
                      money={money}
                      dateFormat={dateFormat}
                    />
                  </li>
                ))}
              </ul>
            )}
          </Card>
        </Col>
        <Col xs={24} lg={15} xxl={16} style={{ minWidth: 0 }}>
          {selected && (
            <CustomerDetail
              customer={selected}
              client={clientById.get(selected.clientId)}
              payments={selectedPayments}
              cb={cb}
              onServe={onServe}
            />
          )}
        </Col>
      </Row>
    </>
  );
};

export default LoanRegister;
