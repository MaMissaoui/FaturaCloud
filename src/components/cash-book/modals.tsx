import { Col, Form, Input, InputNumber, Modal, Row, Select, Typography } from "antd";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import toNumber from "lodash/toNumber";
import { unitsToCents } from "src/utils/currency";
import type { CashBookState } from "src/components/cash-book/use-cash-book";

const { Option } = Select;
const { TextArea } = Input;

// The Cash Book's three modals (new customer, withdrawal, per-line
// payment), shared by every layout of the screen.
const CashBookModals = ({ cb }: { cb: CashBookState }) => {
  const {
    organization,
    newClientModalOpen,
    setNewClientModalOpen,
    newClientForm,
    withdrawModalOpen,
    setWithdrawModalOpen,
    withdrawSubmitting,
    withdrawForm,
    payingLine,
    setPayingLine,
    payingSubmitting,
    payLineForm,
    bankAccounts,
    expenseAccounts,
    withdrawDestination,
    handleWithdrawSubmit,
    handleNewClientSubmit,
    currency,
    money,
    handlePayLineSubmit,
  } = cb;

  return (
    <>
      <Modal
        title={<Trans>New customer</Trans>}
        open={newClientModalOpen}
        onCancel={() => setNewClientModalOpen(false)}
        onOk={() => newClientForm.submit()}
        okText={t`Continue`}
        cancelText={t`Cancel`}
        destroyOnHidden
      >
        <Form
          form={newClientForm}
          layout="vertical"
          onFinish={handleNewClientSubmit}
          scrollToFirstError
        >
          <Form.Item
            label={t`Name`}
            name="name"
            rules={[{ required: true, message: t`This field is required!` }]}
          >
            <Input autoFocus />
          </Form.Item>
          <Form.Item
            label={t`Mobile number`}
            name="phone"
            rules={[{ required: true, message: t`This field is required!` }]}
          >
            <Input />
          </Form.Item>
          <Form.Item label={t`Phone 2`} name="phone2">
            <Input />
          </Form.Item>
          <Form.Item label={t`Phone 3`} name="phone3">
            <Input />
          </Form.Item>
          <Form.Item label={t`Address`} name="address">
            <Input />
          </Form.Item>
          <Form.Item label={t`Identity number`} name="identity_number">
            <Input />
          </Form.Item>
          <Form.Item label={t`IBAN`} name="iban">
            <Input />
          </Form.Item>
          <Form.Item label={t`Guarantor`} name="guarantor">
            <Input />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={<Trans>Withdraw from cash register</Trans>}
        open={withdrawModalOpen}
        onCancel={() => setWithdrawModalOpen(false)}
        onOk={() => withdrawForm.submit()}
        confirmLoading={withdrawSubmitting}
        okText={t`Record`}
        cancelText={t`Cancel`}
        destroyOnHidden
      >
        <Form form={withdrawForm} layout="vertical" onFinish={handleWithdrawSubmit}>
          <Form.Item
            label={t`Amount (${currency})`}
            name="amount"
            rules={[{ required: true, message: t`This field is required!` }]}
          >
            <InputNumber style={{ width: "100%" }} min={0.01} precision={2} autoFocus />
          </Form.Item>
          <Form.Item
            label={t`Destination`}
            name="counterAccountType"
            rules={[{ required: true, message: t`This field is required!` }]}
          >
            <Select
              onChange={(value) =>
                // Prefill the receiving account from the organization's own
                // defaults: Default cash account (Bank, 1020) for a deposit,
                // Default expense account (5100) for an expense.
                withdrawForm.setFieldValue(
                  "counterAccountId",
                  (value === "expense"
                    ? organization?.defaultExpenseAccountId
                    : organization?.defaultCashAccountId) ?? undefined,
                )
              }
            >
              <Option value="bank">
                <Trans>Deposit to the bank</Trans>
              </Option>
              <Option value="expense">
                <Trans>Spend on an expense (no vendor bill)</Trans>
              </Option>
            </Select>
          </Form.Item>
          <Form.Item
            label={
              withdrawDestination === "expense" ? (
                <Trans>Expense account</Trans>
              ) : (
                <Trans>Bank account</Trans>
              )
            }
            name="counterAccountId"
            rules={[{ required: true, message: t`This field is required!` }]}
          >
            <Select showSearch optionFilterProp="children">
              {(withdrawDestination === "expense" ? expenseAccounts : bankAccounts).map(
                (a: any) => (
                  <Option key={a.id} value={a.id}>
                    {a.code} — {a.name}
                  </Option>
                ),
              )}
            </Select>
          </Form.Item>
          <Form.Item label={t`Note`} name="note">
            <TextArea rows={2} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={<Trans>Record payment</Trans>}
        open={!!payingLine}
        onCancel={() => setPayingLine(null)}
        onOk={() => payLineForm.submit()}
        confirmLoading={payingSubmitting}
        okText={t`Record`}
        cancelText={t`Cancel`}
        destroyOnHidden
      >
        {payingLine && (
          <>
            <Typography.Paragraph>
              <Typography.Text strong>{payingLine.clientName}</Typography.Text>
              {payingLine.invoiceNumber && (
                <Typography.Text type="secondary">
                  {" · "}
                  <Trans>Invoice {payingLine.invoiceNumber}</Trans>
                </Typography.Text>
              )}
              <br />
              {payingLine.productName}
              {payingLine.sku ? ` (${payingLine.sku})` : ""} × {payingLine.quantity}
            </Typography.Paragraph>
            <Row gutter={16} style={{ marginBottom: 16 }}>
              <Col span={8}>
                <Typography.Text type="secondary">
                  <Trans>Amount</Trans>
                </Typography.Text>
                <div style={{ whiteSpace: "nowrap" }}>{money(payingLine.amount)}</div>
              </Col>
              <Col span={8}>
                <Typography.Text type="secondary">
                  <Trans>Paid</Trans>
                </Typography.Text>
                <div style={{ whiteSpace: "nowrap" }}>{money(payingLine.paid)}</div>
              </Col>
              <Col span={8}>
                <Typography.Text type="secondary">
                  <Trans>Outstanding</Trans>
                </Typography.Text>
                <div style={{ whiteSpace: "nowrap" }}>
                  <Typography.Text strong>{money(payingLine.outstanding)}</Typography.Text>
                </div>
              </Col>
            </Row>
          </>
        )}
        <Form form={payLineForm} layout="vertical" onFinish={handlePayLineSubmit}>
          <Form.Item
            label={t`Amount received (${currency})`}
            name="amount"
            rules={[
              { required: true, message: t`This field is required!` },
              {
                validator: (_, value) =>
                  payingLine && unitsToCents(toNumber(value) || 0) > payingLine.outstanding
                    ? Promise.reject(
                        new Error(t`Cannot exceed the outstanding balance of this item`),
                      )
                    : Promise.resolve(),
              },
            ]}
          >
            <InputNumber style={{ width: "100%" }} min={0.01} precision={2} autoFocus />
          </Form.Item>
          <Form.Item label={t`Reference`} name="reference">
            <Input />
          </Form.Item>
        </Form>
      </Modal>
    </>
  );
};

export default CashBookModals;
