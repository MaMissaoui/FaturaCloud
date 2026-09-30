import { useState } from "react";
import { Alert, Popover, Space, Spin, Table, Tag, Typography } from "antd";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";

import { GetPaymentInvoiceLines } from "src/api";
import type { Payment, PaymentInvoiceDetail, PaymentInvoiceLine } from "src/types/models";

const { Link, Text } = Typography;

// The Cash Book Payment history's Product cell text: the products the
// payment paid for (every product of an invoice it covered as a whole, or
// the lines a Cash Book line payment settled — see Payment.products), "Whole
// invoice" only for an invoice with no lines to name, and "" for a payment
// with no invoice. Mirrors paymentProductsLabel in db/report_export.go,
// which the exported report uses.
export const paymentProductsLabel = (p: Payment): string => {
  if (p.products && p.products.length > 0) return p.products.join(", ");
  if (p.invoiceNumbers && p.invoiceNumbers.length > 0) return t`Whole invoice`;
  return "";
};

interface Props {
  payment: Payment;
  money: (cents: number) => string;
}

// The Product cell: the names, a "whole invoice" tag when the payment
// covered an invoice as a whole, and — on click, so it works on a touch
// screen at the counter — a panel with each invoice's lines (quantity, unit
// and line amounts, tax included), the invoice total and what this payment
// covered. The lines are fetched when the panel first opens, not with the
// payments list. It's a panel here rather than a link to the invoice
// because the cashbook role can't open the Invoices section.
const PaymentProductsCell = ({ payment, money }: Props) => {
  const [details, setDetails] = useState<PaymentInvoiceDetail[] | null>(null);
  const [failed, setFailed] = useState(false);
  const [loading, setLoading] = useState(false);

  const label = paymentProductsLabel(payment);
  if (!label) return <>—</>;

  const load = (open: boolean) => {
    if (!open || details || loading) return;
    setLoading(true);
    setFailed(false);
    GetPaymentInvoiceLines(payment.id)
      .then(setDetails)
      .catch(() => setFailed(true))
      .finally(() => setLoading(false));
  };

  const content = (
    <div style={{ maxWidth: "min(560px, calc(100vw - 48px))" }}>
      {loading && <Spin size="small" />}
      {failed && <Alert type="error" showIcon message={t`Couldn't load the invoice lines`} />}
      {details?.map((invoice) => (
        <div key={invoice.invoiceId} style={{ marginBottom: 12 }}>
          <Space wrap size={[8, 0]} style={{ marginBottom: 6 }}>
            <Text strong>{invoice.invoiceNumber || t`Invoice`}</Text>
            <Text type="secondary">
              <Trans>Invoice total</Trans> {money(invoice.invoiceTotal)}
            </Text>
            <Text type="secondary">
              <Trans>This payment</Trans> {money(invoice.applied)}
            </Text>
          </Space>
          <Table<PaymentInvoiceLine>
            size="small"
            pagination={false}
            rowKey="lineId"
            dataSource={invoice.lines}
            scroll={{ x: "max-content" }}
            columns={[
              {
                title: t`Product`,
                key: "product",
                render: (_, l) => (
                  <Space size={4} wrap>
                    <span>{l.productName || "—"}</span>
                    {l.sku && <Text type="secondary">({l.sku})</Text>}
                    {l.paidByThisPayment > 0 && (
                      <Tag color="blue">
                        <Trans>Paid here</Trans> {money(l.paidByThisPayment)}
                      </Tag>
                    )}
                  </Space>
                ),
              },
              { title: t`Qty`, key: "qty", align: "right", render: (_, l) => l.quantity },
              {
                title: t`Unit price`,
                key: "unit",
                align: "right",
                render: (_, l) =>
                  l.quantity > 0 ? (
                    <span style={{ whiteSpace: "nowrap" }}>
                      {money(Math.round(l.amount / l.quantity))}
                    </span>
                  ) : (
                    "—"
                  ),
              },
              {
                title: t`Line total`,
                key: "amount",
                align: "right",
                render: (_, l) => <span style={{ whiteSpace: "nowrap" }}>{money(l.amount)}</span>,
              },
            ]}
          />
        </div>
      ))}
      {details && details.length > 0 && (
        <Text type="secondary" style={{ fontSize: 12 }}>
          <Trans>Amounts include tax and any discount.</Trans>
        </Text>
      )}
    </div>
  );

  // The label is capped and truncated in the cell, like the loan report's
  // Product column: uncapped, a whole-invoice payment's full product list
  // widened the table (scroll.x "max-content") until Method, Reference and
  // Amount fell past the card edge. The full list stays in the hover title
  // and the click panel.
  return (
    <Popover trigger="click" placement="bottomLeft" content={content} onOpenChange={load}>
      <span style={{ display: "inline-flex", alignItems: "center", gap: 4, whiteSpace: "nowrap" }}>
        <Link
          title={label}
          style={{
            display: "inline-block",
            maxWidth: 280,
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
            verticalAlign: "bottom",
          }}
        >
          {label}
        </Link>
        {payment.wholeInvoice && (
          <Tag style={{ marginInlineEnd: 0 }}>
            <Trans>whole invoice</Trans>
          </Tag>
        )}
      </span>
    </Popover>
  );
};

export default PaymentProductsCell;
