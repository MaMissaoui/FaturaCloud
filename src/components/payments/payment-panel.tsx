import { memo, useCallback, useEffect, useState } from "react";
import {
  App,
  Button,
  Card,
  DatePicker,
  Descriptions,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Table,
  Tag,
} from "antd";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import dayjs from "dayjs";
import map from "lodash/map";

import {
  GetAccounts,
  GetIncomingInvoicePayments,
  GetInvoicePayments,
  GetPayment,
  CreatePayment,
  VoidPayment,
} from "src/api";
import type { Account, Payment, PaymentApplication } from "src/types/models";
import {
  PAYMENT_METHODS,
  paymentMethodLabel,
  paymentStatusColor,
  paymentStatusLabel,
} from "src/types/payment";
import { useDatePickerFormat } from "src/utils/date";
import { unitsToCents, centsToUnits } from "src/utils/currency";
import { showExchangeRateFields } from "src/components/currency/currency-fields";

const { Option } = Select;
const { TextArea } = Input;

interface PaymentRow {
  application: PaymentApplication;
  payment: Payment;
}

interface PaymentPanelProps {
  organizationId: string;
  documentType: "invoice" | "incoming_invoice";
  documentId: string;
  direction: "inbound" | "outbound";
  clientId?: string | null;
  vendorId?: string | null;
  currency: string;
  orgCurrency: string;
  total: number; // cents
  hasPostedEntry: boolean;
  // Cash Book reuse (src/routes/cash-book.tsx): hides the payment-history
  // table, keeping only the balance summary and the record-payment action —
  // that screen shows one open invoice at a time and has no use for its
  // full history. Defaults to false so every existing embedding (invoice/
  // incoming-invoice detail pages) is unaffected.
  hideHistory?: boolean;
  // Cash Book reuse: called after a payment is recorded that brings the
  // balance to exactly zero, so the caller can follow up by moving the
  // invoice to "paid" — see db/cash_sale.go's CreateCashSale doc comment for
  // why that auto-progression is scoped to the Cash Book screen rather than
  // built into this shared component's own behavior.
  onSettled?: () => void;
  // Organization's configured "Decimal places" (Settings → Invoice), the
  // same value every other money display in the app (getFormattedNumber,
  // invoice/PO/order totals, the accounting reports) formats with. Without
  // it, Intl.NumberFormat falls back to the currency's ISO 4217 minor unit —
  // 3 for TND — which is correct by the standard but disagrees with every
  // other TND amount on the page, which is 2dp by the org's own setting.
  minimumFractionDigits?: number;
}

const PaymentPanel: React.FC<PaymentPanelProps> = ({
  organizationId,
  documentType,
  documentId,
  direction,
  clientId,
  vendorId,
  currency,
  orgCurrency,
  total,
  hasPostedEntry,
  hideHistory = false,
  onSettled,
  minimumFractionDigits,
}) => {
  const { i18n } = useLingui();
  const { message } = App.useApp();
  const dateFormat = useDatePickerFormat();
  const [rows, setRows] = useState<PaymentRow[]>([]);
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [form] = Form.useForm();

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const applications =
        documentType === "invoice"
          ? await GetInvoicePayments(documentId)
          : await GetIncomingInvoicePayments(documentId);
      const uniquePaymentIds = [...new Set(map(applications, "paymentId"))];
      const payments = await Promise.all(uniquePaymentIds.map((id) => GetPayment(id)));
      const paymentsById = new Map(payments.map((p) => [p.id, p]));
      setRows(
        applications
          .map((application) => ({
            application,
            payment: paymentsById.get(application.paymentId)!,
          }))
          .filter((row) => row.payment)
          .sort((a, b) => b.payment.date - a.payment.date),
      );
    } catch (error) {
      console.error("Failed to fetch payments:", error);
      message.error(t`Failed to fetch payments`);
    } finally {
      setLoading(false);
    }
  }, [documentType, documentId, message]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  // Same blank/invalid-currency guard as formatCents (src/utils/currency.ts),
  // plus the organization's own decimal-places setting — see the prop's
  // comment for why the two disagree for a 3-decimal currency like TND.
  const money = useCallback(
    (cents: number) => {
      const units = centsToUnits(cents);
      try {
        return new Intl.NumberFormat(i18n.locale, {
          style: "currency",
          currency,
          minimumFractionDigits,
        }).format(units);
      } catch {
        return new Intl.NumberFormat(i18n.locale, { minimumFractionDigits }).format(units);
      }
    },
    [currency, i18n.locale, minimumFractionDigits],
  );

  useEffect(() => {
    GetAccounts(organizationId)
      .then((accts) => setAccounts(accts.filter((a) => !a.isGroup)))
      .catch((error) => console.error("Failed to fetch accounts:", error));
  }, [organizationId]);

  const paidCents = rows
    .filter((r) => r.payment.status !== "voided")
    .reduce((sum, r) => sum + r.application.amount, 0);
  const balanceDue = total - paidCents;

  const openModal = () => {
    form.resetFields();
    form.setFieldsValue({
      date: dayjs(),
      method: "bank_transfer",
      amount: centsToUnits(balanceDue),
    });
    setModalOpen(true);
  };

  const handleVoid = async (paymentId: string) => {
    try {
      await VoidPayment(paymentId);
      message.success(t`Payment voided`);
      await refresh();
    } catch (error) {
      console.error("Failed to void payment:", error);
      message.error(error instanceof Error ? error.message : t`Failed to void payment`);
    }
  };

  const handleSubmit = async (values: any) => {
    setSubmitting(true);
    try {
      await CreatePayment({
        organizationId,
        direction,
        clientId: direction === "inbound" ? clientId : undefined,
        vendorId: direction === "outbound" ? vendorId : undefined,
        bankAccountId: values.bankAccountId,
        amount: unitsToCents(values.amount),
        currency,
        exchangeRate: values.exchangeRate,
        exchangeRateDate: values.exchangeRateDate ? values.exchangeRateDate.valueOf() : null,
        date: values.date.valueOf(),
        method: values.method,
        reference: values.reference || null,
        notes: values.notes || null,
        applications: [{ documentType, documentId, amount: unitsToCents(values.amount) }],
      });
      message.success(t`Payment recorded`);
      setModalOpen(false);
      await refresh();
      if (balanceDue - unitsToCents(values.amount) <= 0) {
        onSettled?.();
      }
    } catch (error) {
      console.error("Failed to record payment:", error);
      message.error(error instanceof Error ? error.message : t`Failed to record payment`);
    } finally {
      setSubmitting(false);
    }
  };

  if (!hasPostedEntry && rows.length === 0) return null;

  return (
    <Card size="small" title={<Trans>Payments</Trans>} style={{ marginTop: 24 }}>
      <Descriptions column={3} size="small" style={{ marginBottom: 8 }}>
        <Descriptions.Item label={<Trans>Total</Trans>}>{money(total)}</Descriptions.Item>
        <Descriptions.Item label={<Trans>Paid</Trans>}>{money(paidCents)}</Descriptions.Item>
        <Descriptions.Item label={<Trans>Balance due</Trans>}>
          <strong>{money(balanceDue)}</strong>
        </Descriptions.Item>
      </Descriptions>

      {!hideHistory && (
        <Table
          dataSource={rows}
          rowKey={(r) => r.application.id}
          pagination={false}
          size="small"
          loading={loading}
          style={{ marginBottom: 8 }}
          locale={{ emptyText: <Trans>No payments recorded yet</Trans> }}
        >
          <Table.Column
            title={<Trans>Date</Trans>}
            key="date"
            render={(row: PaymentRow) => dayjs(row.payment.date).format(dateFormat)}
          />
          <Table.Column
            title={<Trans>Method</Trans>}
            key="method"
            render={(row: PaymentRow) => paymentMethodLabel(row.payment.method)}
          />
          <Table.Column
            title={<Trans>Amount</Trans>}
            key="amount"
            align="right"
            render={(row: PaymentRow) => money(row.application.amount)}
          />
          <Table.Column
            title={<Trans>Reference</Trans>}
            key="reference"
            render={(row: PaymentRow) => row.payment.reference || "—"}
          />
          <Table.Column
            title={<Trans>Status</Trans>}
            key="status"
            render={(row: PaymentRow) => (
              <Tag color={paymentStatusColor[row.payment.status]}>
                {paymentStatusLabel(row.payment.status)}
              </Tag>
            )}
          />
          <Table.Column
            key="actions"
            render={(row: PaymentRow) =>
              row.payment.status === "posted" ? (
                <Popconfirm
                  title={t`Void this payment?`}
                  description={t`This reverses its journal entry and restores the balance due.`}
                  onConfirm={() => handleVoid(row.payment.id)}
                  okText={t`Yes`}
                  cancelText={t`No`}
                >
                  <Button type="link" danger size="small">
                    <Trans>Void</Trans>
                  </Button>
                </Popconfirm>
              ) : null
            }
          />
        </Table>
      )}

      {hasPostedEntry && balanceDue > 0 && (
        <Button onClick={openModal} style={{ marginBottom: 16 }}>
          <Trans>Record payment</Trans>
        </Button>
      )}

      <Modal
        title={<Trans>Record payment</Trans>}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={submitting}
        okText={t`Record`}
        cancelText={t`Cancel`}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" onFinish={handleSubmit}>
          <Form.Item
            label={t`Date`}
            name="date"
            rules={[{ required: true, message: t`This field is required!` }]}
          >
            <DatePicker style={{ width: "100%" }} format={dateFormat} />
          </Form.Item>
          <Form.Item
            label={t`Method`}
            name="method"
            rules={[{ required: true, message: t`This field is required!` }]}
          >
            <Select>
              {PAYMENT_METHODS.map((method) => (
                <Option key={method} value={method}>
                  {paymentMethodLabel(method)}
                </Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item
            label={t`Bank / cash account`}
            name="bankAccountId"
            rules={[{ required: true, message: t`This field is required!` }]}
          >
            <Select showSearch optionFilterProp="children">
              {accounts.map((a) => (
                <Option key={a.id} value={a.id}>
                  {a.code} — {a.name}
                </Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item
            label={t`Amount (${currency})`}
            name="amount"
            rules={[{ required: true, message: t`This field is required!` }]}
          >
            <InputNumber
              style={{ width: "100%" }}
              min={0.01}
              max={centsToUnits(balanceDue)}
              precision={2}
            />
          </Form.Item>
          {showExchangeRateFields(currency, orgCurrency) && (
            <ExchangeRateFieldsStack currency={currency} orgCurrency={orgCurrency} />
          )}
          <Form.Item label={t`Reference`} name="reference">
            <Input />
          </Form.Item>
          <Form.Item label={t`Notes`} name="notes">
            <TextArea rows={2} />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
};

// ExchangeRateFields renders a pair of <Col> — valid only inside a <Row>.
// Modal forms here have no Row, so stack them as plain Form.Items instead.
const ExchangeRateFieldsStack = ({
  currency,
  orgCurrency,
}: {
  currency: string;
  orgCurrency: string;
}) => (
  <div style={{ display: "flex", gap: 16 }}>
    <div style={{ flex: 1 }}>
      <Form.Item
        label={<Trans>Exchange rate</Trans>}
        name="exchangeRate"
        tooltip={t`1 ${currency} = this many ${orgCurrency}`}
        rules={[{ required: true, message: t`This field is required!` }]}
      >
        <InputNumber min={0} step={0.0001} precision={6} style={{ width: "100%" }} />
      </Form.Item>
    </div>
    <div style={{ flex: 1 }}>
      <Form.Item
        label={<Trans>Rate date</Trans>}
        name="exchangeRateDate"
        rules={[{ required: true, message: t`This field is required!` }]}
      >
        <DatePicker style={{ width: "100%" }} />
      </Form.Item>
    </div>
  </div>
);

// All props are primitives (no callback/object props), so a shallow-equal
// memo is a real win here: this panel does its own fetching/state and
// re-renders on every unrelated keystroke in the surrounding invoice form.
export default memo(PaymentPanel);
