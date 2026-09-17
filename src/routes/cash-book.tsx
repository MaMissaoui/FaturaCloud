import { useEffect, useMemo, useState } from "react";
import {
  App,
  Button,
  Card,
  Col,
  DatePicker,
  Divider,
  Form,
  Input,
  InputNumber,
  List,
  Modal,
  Row,
  Select,
  Space,
  Table,
  Tag,
  Typography,
} from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { ArrowLeftOutlined, UserAddOutlined, WalletOutlined } from "@ant-design/icons";
import dayjs from "dayjs";
import get from "lodash/get";
import find from "lodash/find";
import map from "lodash/map";
import sum from "lodash/sum";
import toNumber from "lodash/toNumber";

import { organizationAtom, organizationIdAtom } from "src/atoms/organization";
import { clientsAtom, setClientsAtom } from "src/atoms/client";
import { productsAtom, setProductsAtom } from "src/atoms/product";
import { taxRatesAtom, setTaxRatesAtom } from "src/atoms/tax-rate";
import {
  CreateCashMovement,
  CreateCashSale,
  GetAccountBalance,
  GetAccounts,
  GetClientOpenInvoices,
  GetInvoice,
  UpdateInvoiceState,
} from "src/api";
import type { CreateCashSaleRequest, OutstandingInvoiceSummary } from "src/api";
import type { Account, Client, Invoice } from "src/types/models";
import LineItemsTable from "src/components/line-items/table";
import PaymentPanel from "src/components/payments/payment-panel";
import { useDatePickerFormat } from "src/utils/date";
import {
  addDecimal,
  calculateTax,
  centsToUnits,
  formatCents,
  multiplyDecimal,
  unitsToCents,
} from "src/utils/currency";
import { PAYMENT_METHODS, paymentMethodLabel } from "src/types/payment";

const { Option } = Select;
const { TextArea } = Input;

// A new customer picked in the "New customer" modal below — kept as local
// draft state, not created via a separate API call, until the whole sale is
// submitted. This is what makes CreateCashSale's client+invoice+payment
// creation genuinely atomic (see db/cash_sale.go's CreateCashSale doc
// comment) rather than a client-creation call followed by a separate sale.
interface NewClientDraft {
  name: string;
  phone?: string;
  identity_number?: string;
  iban?: string;
}

// Cash Book: a single fast-entry screen for a walk-in retail counter —
// search for a customer by name/mobile/IBAN/identity number, then either
// pay off one of their open (loan sale) invoices or record a new sale.
// "Cash sale" and "loan sale" aren't separate modes here: a sale is a cash
// sale exactly when the amount received equals the total, and a loan sale
// otherwise (including a zero-upfront deposit) — see the "Amount received"
// field below.
const CashBook = () => {
  const { i18n } = useLingui();
  const { message, modal } = App.useApp();
  const dateFormat = useDatePickerFormat();

  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);
  const clients = useAtomValue(clientsAtom);
  const setClients = useSetAtom(setClientsAtom);
  const products = useAtomValue(productsAtom);
  const setProducts = useSetAtom(setProductsAtom);
  // A component/intermediate isn't sellable — same filter invoices/orders use.
  const sellableProducts = useMemo(
    () => (products as any[]).filter((p) => p.category !== "component"),
    [products],
  );
  const taxRates = useAtomValue(taxRatesAtom);
  const setTaxRates = useSetAtom(setTaxRatesAtom);

  const [accounts, setAccounts] = useState<Account[]>([]);
  const [search, setSearch] = useState("");
  const [selectedClient, setSelectedClient] = useState<Client | null>(null);
  const [newClientDraft, setNewClientDraft] = useState<NewClientDraft | null>(null);
  const [newClientModalOpen, setNewClientModalOpen] = useState(false);
  const [newClientForm] = Form.useForm();
  const [openInvoices, setOpenInvoices] = useState<OutstandingInvoiceSummary[]>([]);
  const [loadingOpenInvoices, setLoadingOpenInvoices] = useState(false);
  const [payingInvoice, setPayingInvoice] = useState<Invoice | null>(null);
  const [amountReceivedTouched, setAmountReceivedTouched] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [form] = Form.useForm();

  // Register balance widget + withdrawal modal — see db/gl_reports.go's
  // GetAccountBalance and db/cash_movement.go's CreateCashMovement doc
  // comments. This is a GL running balance ("what the books say"), not a
  // physically-counted till figure — labeled accordingly below rather than
  // as "cash in the drawer", the same scoped-out-till-session boundary this
  // screen has always had.
  const registerAccountId = organization?.defaultCashRegisterAccountId;
  const [registerBalance, setRegisterBalance] = useState<number | null>(null);
  const [loadingBalance, setLoadingBalance] = useState(false);
  const [withdrawModalOpen, setWithdrawModalOpen] = useState(false);
  const [withdrawSubmitting, setWithdrawSubmitting] = useState(false);
  const [withdrawForm] = Form.useForm();

  useEffect(() => {
    setClients();
    setProducts();
    setTaxRates();
  }, [setClients, setProducts, setTaxRates]);

  useEffect(() => {
    if (!organizationId) return;
    GetAccounts(organizationId)
      .then((accts) => setAccounts((accts as any[]).filter((a) => !a.isGroup)))
      .catch((error) => console.error("Failed to fetch accounts:", error));
  }, [organizationId]);

  const refreshBalance = async () => {
    if (!organizationId || !registerAccountId) return;
    setLoadingBalance(true);
    try {
      const { balance } = await GetAccountBalance(organizationId, registerAccountId);
      setRegisterBalance(balance);
    } catch (error) {
      console.error("Failed to fetch the register balance:", error);
      message.error(t`Failed to load the cash register balance`);
    } finally {
      setLoadingBalance(false);
    }
  };

  useEffect(() => {
    refreshBalance();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [organizationId, registerAccountId]);

  const bankAccounts = useMemo(() => accounts.filter((a: any) => a.type === "asset"), [accounts]);
  const expenseAccounts = useMemo(
    () => accounts.filter((a: any) => a.type === "expense"),
    [accounts],
  );
  const withdrawDestination = Form.useWatch("counterAccountType", withdrawForm);

  const openWithdrawModal = () => {
    withdrawForm.resetFields();
    withdrawForm.setFieldsValue({ counterAccountType: "bank" });
    setWithdrawModalOpen(true);
  };

  const handleWithdrawSubmit = async (values: any) => {
    if (!organizationId || !registerAccountId) return;
    setWithdrawSubmitting(true);
    try {
      await CreateCashMovement({
        organizationId,
        accountId: registerAccountId,
        date: Date.now(),
        counterAccountType: values.counterAccountType,
        counterAccountId: values.counterAccountId,
        amount: unitsToCents(toNumber(values.amount) || 0),
        note: values.note || undefined,
      });
      message.success(t`Cash movement recorded`);
      setWithdrawModalOpen(false);
      await refreshBalance();
    } catch (error) {
      console.error("Failed to record cash movement:", error);
      message.error(error instanceof Error ? error.message : t`Failed to record cash movement`);
    } finally {
      setWithdrawSubmitting(false);
    }
  };

  const inSale = !!selectedClient || !!newClientDraft;

  const resetSaleForm = () => {
    form.resetFields();
    form.setFieldsValue({
      date: dayjs(),
      paymentMethod: "cash",
      // defaultCashAccountId is wired to Bank in every chart-of-accounts
      // template (see CLAUDE.md's cash register account note) — a Cash
      // Book sale must default to the actual till, or its payment silently
      // credits Bank instead and the register balance/report never move.
      bankAccountId:
        organization?.defaultCashRegisterAccountId ||
        organization?.defaultCashAccountId ||
        undefined,
      lineItems: [{ quantity: 1, taxRate: get(find(taxRates, { isDefault: 1 }), "id") }],
    });
    setAmountReceivedTouched(false);
  };

  const refreshOpenInvoices = async (clientId: string) => {
    setLoadingOpenInvoices(true);
    try {
      setOpenInvoices(await GetClientOpenInvoices(clientId));
    } catch (error) {
      console.error("Failed to fetch open invoices:", error);
      message.error(t`Failed to load this customer's open invoices`);
      setOpenInvoices([]);
    } finally {
      setLoadingOpenInvoices(false);
    }
  };

  const selectClient = async (client: Client) => {
    setSelectedClient(client);
    setNewClientDraft(null);
    setSearch("");
    resetSaleForm();
    await refreshOpenInvoices(client.id);
  };

  const backToSearch = () => {
    setSelectedClient(null);
    setNewClientDraft(null);
    setOpenInvoices([]);
    setSearch("");
  };

  const needle = search.trim().toLowerCase();
  const searchResults = useMemo(() => {
    if (!needle) return [];
    return (clients as any[]).filter((c) =>
      [c.name, c.phone, c.identity_number, c.iban].some(
        (field) => field && String(field).toLowerCase().includes(needle),
      ),
    );
  }, [clients, needle]);

  const handleNewClientSubmit = (values: any) => {
    const phone = values.phone?.trim();
    // Client-side duplicate-phone check — the primary UX path. The server
    // repeats this check inside CreateCashSale as a backstop (see its doc
    // comment); this is what avoids splitting a returning walk-in
    // customer's history across two client records.
    const existingByPhone = phone
      ? (clients as any[]).find((c) => c.phone && c.phone === phone)
      : null;
    if (existingByPhone) {
      modal.confirm({
        title: t`A customer with this phone number already exists`,
        content: `${existingByPhone.name || ""} — ${phone}`,
        okText: t`Use this customer`,
        cancelText: t`Cancel`,
        onOk: () => {
          setNewClientModalOpen(false);
          selectClient(existingByPhone);
        },
      });
      return;
    }
    setNewClientModalOpen(false);
    setSelectedClient(null);
    setOpenInvoices([]);
    setNewClientDraft({
      name: values.name,
      phone: phone || undefined,
      identity_number: values.identity_number || undefined,
      iban: values.iban || undefined,
    });
    resetSaleForm();
  };

  // ---- New sale totals (units, not cents — same convention as the invoice
  // form; converted to cents only when building the CreateCashSale payload) ----
  const lineItems = Form.useWatch("lineItems", form);
  const subTotal = useMemo(
    () =>
      sum(
        map(lineItems || [], (item: any) =>
          multiplyDecimal(toNumber(item?.quantity) || 0, toNumber(item?.unitPrice) || 0),
        ),
      ),
    [lineItems],
  );
  const taxGroups = useMemo(() => {
    const groups: Record<string, { taxRate: any; subtotal: number; tax: number }> = {};
    ((lineItems || []) as any[]).forEach((item) => {
      const taxRateId = item?.taxRate;
      if (!taxRateId) return;
      const lineTotal = multiplyDecimal(
        toNumber(item?.quantity) || 0,
        toNumber(item?.unitPrice) || 0,
      );
      if (!groups[taxRateId]) {
        groups[taxRateId] = { taxRate: find(taxRates, { id: taxRateId }), subtotal: 0, tax: 0 };
      }
      groups[taxRateId].subtotal = addDecimal(groups[taxRateId].subtotal, lineTotal);
    });
    Object.values(groups).forEach((g) => {
      g.tax = g.taxRate?.percentage ? calculateTax(g.subtotal, g.taxRate.percentage) : 0;
    });
    return Object.values(groups);
  }, [lineItems, taxRates]);
  const taxTotal = sum(map(taxGroups, "tax"));
  const total = addDecimal(subTotal, taxTotal);
  const amountReceivedWatched = toNumber(Form.useWatch("amountReceived", form));

  // Keeps "Amount received" synced to the running total (a cash sale, the
  // common case) until the user edits it themselves — at which point it's
  // treated as a loan sale's deposit and left alone.
  useEffect(() => {
    if (!amountReceivedTouched) {
      form.setFieldValue("amountReceived", total);
    }
  }, [total, amountReceivedTouched, form]);

  const currency = organization?.currency || "EUR";
  const money = (cents: number) => formatCents(cents, currency, i18n.locale);

  const handleSubmitSale = async (values: any) => {
    if (!organizationId) return;
    if (!selectedClient && !newClientDraft) {
      message.error(t`Pick or create a customer first`);
      return;
    }
    const totalCents = unitsToCents(total);
    const amountReceivedCents = Math.min(
      unitsToCents(toNumber(values.amountReceived) || 0),
      totalCents,
    );

    setSubmitting(true);
    try {
      const req: CreateCashSaleRequest = {
        organizationId,
        date: values.date.valueOf(),
        currency,
        lineItems: (values.lineItems || []).map((item: any) => ({
          description: item.description || null,
          quantity: item.quantity,
          unitPrice: unitsToCents(toNumber(item.unitPrice) || 0),
          taxRate: item.taxRate || null,
          productId: item.productId || null,
        })),
        subTotal: unitsToCents(subTotal),
        taxTotal: unitsToCents(taxTotal),
        total: totalCents,
        amountReceived: amountReceivedCents,
        paymentMethod: values.paymentMethod || "cash",
        bankAccountId: values.bankAccountId || undefined,
        reference: values.reference || undefined,
        notes: values.notes || undefined,
        ...(selectedClient ? { clientId: selectedClient.id } : { newClient: newClientDraft! }),
      };

      const result = await CreateCashSale(req);
      message.success(
        amountReceivedCents >= totalCents ? t`Cash sale recorded` : t`Loan sale recorded`,
      );
      setSelectedClient(result.client);
      setNewClientDraft(null);
      await setClients();
      await refreshOpenInvoices(result.client.id);
      resetSaleForm();
    } catch (error) {
      console.error("Failed to record sale:", error);
      message.error(error instanceof Error ? error.message : t`Failed to record sale`);
    } finally {
      setSubmitting(false);
    }
  };

  const openPayment = async (invoiceId: string) => {
    try {
      setPayingInvoice(await GetInvoice(invoiceId));
    } catch (error) {
      console.error("Failed to load invoice:", error);
      message.error(t`Failed to load invoice`);
    }
  };

  const closePayment = async () => {
    setPayingInvoice(null);
    if (selectedClient) await refreshOpenInvoices(selectedClient.id);
  };

  // Auto-progresses the invoice to "paid" once its balance clears — see
  // db/cash_sale.go's CreateCashSale doc comment for why this Cash-Book-only
  // follow-up call is the right scope for this behavior rather than a change
  // to PaymentPanel's shared, otherwise-manual-state convention.
  const handleSettled = async () => {
    if (payingInvoice) {
      try {
        await UpdateInvoiceState(payingInvoice.id, "paid");
      } catch (error) {
        console.error("Failed to mark invoice paid:", error);
        message.error(
          t`Payment recorded, but the invoice status couldn't be updated to Paid — update it manually from the invoice page`,
        );
      }
    }
    await closePayment();
  };

  const clientName = selectedClient?.name || newClientDraft?.name || "";

  return (
    <div style={{ padding: 24, maxWidth: 960, margin: "0 auto" }}>
      <Typography.Title level={3}>
        <WalletOutlined style={{ marginRight: 8 }} />
        <Trans>Cash Book</Trans>
      </Typography.Title>

      <Card size="small" style={{ marginBottom: 16 }} loading={loadingBalance}>
        {registerAccountId ? (
          <Row justify="space-between" align="middle">
            <Col>
              <Typography.Text type="secondary">
                <Trans>Cash account balance (per books)</Trans>
              </Typography.Text>
              <div>
                <Typography.Title level={4} style={{ margin: 0 }}>
                  {registerBalance != null ? money(registerBalance) : "—"}
                </Typography.Title>
              </div>
            </Col>
            <Col>
              <Button onClick={openWithdrawModal}>
                <Trans>Withdraw</Trans>
              </Button>
            </Col>
          </Row>
        ) : (
          <Typography.Text type="secondary">
            <Trans>
              No cash register account configured — set one in Organization settings to see the
              balance here.
            </Trans>
          </Typography.Text>
        )}
      </Card>

      {!inSale && (
        <Card size="small">
          <Input.Search
            placeholder={t`Search by name, mobile number, IBAN, or identity number`}
            allowClear
            autoFocus
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onSearch={() => {
              // Enter is the natural motion after typing a phone number at a
              // counter — auto-select when the search has narrowed to one
              // customer instead of making the cashier reach for the mouse.
              if (searchResults.length === 1) selectClient(searchResults[0]);
            }}
            style={{ marginBottom: 16 }}
          />
          {needle && (
            <List
              dataSource={searchResults}
              locale={{ emptyText: <Trans>No matching customers</Trans> }}
              renderItem={(client: any) => (
                <List.Item
                  actions={[
                    <Button type="link" onClick={() => selectClient(client)} key="select">
                      <Trans>Select</Trans>
                    </Button>,
                  ]}
                >
                  <List.Item.Meta
                    title={client.name}
                    description={[client.phone, client.identity_number, client.iban]
                      .filter(Boolean)
                      .join(" · ")}
                  />
                </List.Item>
              )}
            />
          )}
          <Button
            type="dashed"
            block
            icon={<UserAddOutlined />}
            style={{ marginTop: 16 }}
            onClick={() => {
              newClientForm.resetFields();
              if (needle && searchResults.length === 0) {
                newClientForm.setFieldValue("name", search);
              }
              setNewClientModalOpen(true);
            }}
          >
            <Trans>New customer</Trans>
          </Button>
        </Card>
      )}

      {inSale && (
        <>
          <Space style={{ marginBottom: 16 }}>
            <Button icon={<ArrowLeftOutlined />} onClick={backToSearch}>
              <Trans>Back to search</Trans>
            </Button>
            <Typography.Text strong>
              {clientName}
              {newClientDraft && (
                <Tag color="blue" style={{ marginLeft: 8 }}>
                  <Trans>New customer</Trans>
                </Tag>
              )}
            </Typography.Text>
          </Space>

          {selectedClient && (
            <Card
              size="small"
              title={<Trans>Open invoices</Trans>}
              style={{ marginBottom: 16 }}
              loading={loadingOpenInvoices}
            >
              <Table
                dataSource={openInvoices}
                rowKey="id"
                pagination={false}
                size="small"
                locale={{ emptyText: <Trans>No open invoices for this customer</Trans> }}
              >
                <Table.Column title={<Trans>Invoice</Trans>} dataIndex="number" key="number" />
                <Table.Column
                  title={<Trans>Balance due</Trans>}
                  key="total"
                  align="right"
                  render={(inv: OutstandingInvoiceSummary) => money(inv.total)}
                />
                <Table.Column
                  key="actions"
                  render={(inv: OutstandingInvoiceSummary) => (
                    <Button size="small" onClick={() => openPayment(inv.id)}>
                      <Trans>Pay</Trans>
                    </Button>
                  )}
                />
              </Table>
            </Card>
          )}

          <Card size="small" title={<Trans>New sale</Trans>}>
            <Form form={form} layout="vertical" onFinish={handleSubmitSale}>
              <Row gutter={16}>
                <Col xs={24} md={8}>
                  <Form.Item
                    label={<Trans>Date</Trans>}
                    name="date"
                    rules={[{ required: true, message: t`This field is required!` }]}
                  >
                    <DatePicker style={{ width: "100%" }} format={dateFormat} />
                  </Form.Item>
                </Col>
              </Row>

              <LineItemsTable
                defaultNewRow={{
                  quantity: 1,
                  taxRate: get(find(taxRates, { isDefault: 1 }), "id"),
                }}
                columns={[
                  { kind: "index" },
                  {
                    kind: "product",
                    products: sellableProducts,
                    onSelect: (productId, fieldName, formInstance) => {
                      const product = find(products, { id: productId }) as any;
                      if (product) {
                        const items = formInstance.getFieldValue("lineItems");
                        items[fieldName] = {
                          ...items[fieldName],
                          description: product.name,
                          unitPrice: centsToUnits(product.price ?? 0),
                          ...(product.taxRateId ? { taxRate: product.taxRateId } : {}),
                        };
                        formInstance.setFieldValue("lineItems", [...items]);
                      }
                    },
                  },
                  { kind: "description", required: true },
                  { kind: "quantity" },
                  { kind: "unitPrice" },
                  { kind: "taxRate", taxRates },
                ]}
              />

              <Row justify="end" style={{ marginTop: 8, marginBottom: 16 }}>
                <Col>
                  <Space direction="vertical" align="end" size={0}>
                    <Typography.Text>
                      <Trans>Subtotal</Trans>: {money(unitsToCents(subTotal))}
                    </Typography.Text>
                    <Typography.Text>
                      <Trans>Tax</Trans>: {money(unitsToCents(taxTotal))}
                    </Typography.Text>
                    <Typography.Text strong>
                      <Trans>Total</Trans>: {money(unitsToCents(total))}
                    </Typography.Text>
                  </Space>
                </Col>
              </Row>

              <Divider />

              <Row gutter={16}>
                <Col xs={24} md={8}>
                  <Form.Item
                    label={t`Amount received (${currency})`}
                    name="amountReceived"
                    tooltip={t`Full amount = cash sale. Less than the total (or zero) = loan sale. Entering more than the total just records the change given back — the sale itself is still only ever recorded up to the total.`}
                    extra={
                      amountReceivedWatched > total ? (
                        <Typography.Text type="success">
                          <Trans>
                            Change due: {money(unitsToCents(amountReceivedWatched - total))}
                          </Trans>
                        </Typography.Text>
                      ) : amountReceivedWatched < total ? (
                        <Typography.Text type="warning">
                          <Trans>
                            Balance to collect later:{" "}
                            {money(unitsToCents(total - amountReceivedWatched))}
                          </Trans>
                        </Typography.Text>
                      ) : undefined
                    }
                  >
                    {/* No `max`: a cashier routinely receives more than the
                    total (e.g. a round note) and needs change calculated —
                    handleSubmitSale still clamps what's actually recorded as
                    paid on the invoice to the total. */}
                    <InputNumber
                      style={{ width: "100%" }}
                      min={0}
                      precision={2}
                      onChange={() => setAmountReceivedTouched(true)}
                    />
                  </Form.Item>
                </Col>
                <Col xs={24} md={8}>
                  <Form.Item label={t`Payment method`} name="paymentMethod">
                    <Select>
                      {PAYMENT_METHODS.map((m) => (
                        <Option key={m} value={m}>
                          {paymentMethodLabel(m)}
                        </Option>
                      ))}
                    </Select>
                  </Form.Item>
                </Col>
                <Col xs={24} md={8}>
                  <Form.Item label={t`Cash / bank account`} name="bankAccountId">
                    <Select
                      showSearch
                      optionFilterProp="children"
                      allowClear
                      placeholder={t`Organization default`}
                    >
                      {accounts.map((a: any) => (
                        <Option key={a.id} value={a.id}>
                          {a.code} — {a.name}
                        </Option>
                      ))}
                    </Select>
                  </Form.Item>
                </Col>
              </Row>
              <Row gutter={16}>
                <Col xs={24} md={12}>
                  <Form.Item label={t`Reference`} name="reference">
                    <Input />
                  </Form.Item>
                </Col>
                <Col xs={24} md={12}>
                  <Form.Item label={t`Notes`} name="notes">
                    <TextArea rows={1} autoSize />
                  </Form.Item>
                </Col>
              </Row>

              {/* Color, not just label text, distinguishes the two outcomes —
              a misread button label is exactly the mistake a fast-moving
              counter screen should make hard to make: green (matches this
              app's "paid" state Tag) for a fully-settled cash sale, gold
              (matches "sent", the state an unsettled loan sale lands in) for
              anything left owing. */}
              {amountReceivedWatched >= total ? (
                <Button
                  color="green"
                  variant="solid"
                  htmlType="submit"
                  loading={submitting}
                  size="large"
                >
                  <Trans>Record cash sale</Trans>
                </Button>
              ) : (
                <Button
                  color="gold"
                  variant="solid"
                  htmlType="submit"
                  loading={submitting}
                  size="large"
                >
                  <Trans>Record loan sale</Trans>
                </Button>
              )}
            </Form>
          </Card>
        </>
      )}

      <Modal
        title={<Trans>New customer</Trans>}
        open={newClientModalOpen}
        onCancel={() => setNewClientModalOpen(false)}
        onOk={() => newClientForm.submit()}
        okText={t`Continue`}
        cancelText={t`Cancel`}
        destroyOnHidden
      >
        <Form form={newClientForm} layout="vertical" onFinish={handleNewClientSubmit}>
          <Form.Item
            label={t`Name`}
            name="name"
            rules={[{ required: true, message: t`This field is required!` }]}
          >
            <Input autoFocus />
          </Form.Item>
          <Form.Item label={t`Mobile number`} name="phone">
            <Input />
          </Form.Item>
          <Form.Item label={t`Identity number`} name="identity_number">
            <Input />
          </Form.Item>
          <Form.Item label={t`IBAN`} name="iban">
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
            <Select onChange={() => withdrawForm.setFieldValue("counterAccountId", undefined)}>
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

      {payingInvoice && organizationId && selectedClient && (
        <PaymentPanel
          organizationId={organizationId}
          documentType="invoice"
          documentId={payingInvoice.id}
          direction="inbound"
          clientId={selectedClient.id}
          currency={payingInvoice.currency}
          orgCurrency={organization?.currency || "EUR"}
          total={payingInvoice.total}
          hasPostedEntry
          embedded
          onClose={closePayment}
          onSettled={handleSettled}
          defaultMethod="cash"
          defaultBankAccountId={
            organization?.defaultCashRegisterAccountId ??
            organization?.defaultCashAccountId ??
            undefined
          }
          minimumFractionDigits={organization?.minimum_fraction_digits ?? undefined}
        />
      )}
    </div>
  );
};

export default CashBook;
