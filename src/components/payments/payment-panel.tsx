import { useCallback, useEffect, useState } from "react";
import {
  App,
  Button,
  DatePicker,
  Descriptions,
  Divider,
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
import { formatCents, unitsToCents, centsToUnits } from "src/utils/currency";
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
    } catch (error) {
      console.error("Failed to record payment:", error);
      message.error(error instanceof Error ? error.message : t`Failed to record payment`);
    } finally {
      setSubmitting(false);
    }
  };

  if (!hasPostedEntry && rows.length === 0) return null;

  return (
    <>
      <Divider>
        <Trans>Payments</Trans>
      </Divider>
      <Descriptions column={3} size="small" style={{ marginBottom: 16 }}>
        <Descriptions.Item label={<Trans>Total</Trans>}>
          {formatCents(total, currency, i18n.locale)}
        </Descriptions.Item>
        <Descriptions.Item label={<Trans>Paid</Trans>}>
          {formatCents(paidCents, currency, i18n.locale)}
        </Descriptions.Item>
        <Descriptions.Item label={<Trans>Balance due</Trans>}>
          <strong>{formatCents(balanceDue, currency, i18n.locale)}</strong>
        </Descriptions.Item>
      </Descriptions>

      <Table
        dataSource={rows}
        rowKey={(r) => r.application.id}
        pagination={false}
        size="small"
        loading={loading}
        style={{ marginBottom: 16 }}
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
          render={(row: PaymentRow) => formatCents(row.application.amount, currency, i18n.locale)}
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
    </>
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

export default PaymentPanel;
