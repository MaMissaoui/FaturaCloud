import { useCallback, useEffect, useMemo, useState } from "react";
import { createPortal } from "react-dom";
import { useNavigate, useParams, useSearchParams } from "react-router";
import {
  Alert,
  Button,
  Checkbox,
  Col,
  DatePicker,
  Descriptions,
  Divider,
  Form,
  Input,
  InputNumber,
  Layout,
  Popconfirm,
  Row,
  Select,
  Space,
  Table,
  Tag,
  theme,
} from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import { loadable } from "jotai/utils";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { DeleteOutlined, PlusOutlined, SaveOutlined } from "@ant-design/icons";
import dayjs from "dayjs";
import find from "lodash/find";
import map from "lodash/map";
import sum from "lodash/sum";

import { GetIncomingInvoiceMatch, GetPurchaseOrderLineItems } from "src/api";
import { useDatePickerFormat } from "src/utils/date";
import { getFormattedNumber } from "src/utils/currencies";
import { addDecimal, calculateTax, centsToUnits, multiplyDecimal } from "src/utils/currency";
import {
  INCOMING_INVOICE_STATES,
  incomingInvoiceStateColor,
  incomingInvoiceStateLabel,
  matchStatusColor,
  matchStatusLabel,
  matchStatusDetail,
  hasBlockingVariance,
  type IncomingInvoiceState,
  type MatchLine,
} from "src/types/incoming-invoice";
import { organizationAtom } from "src/atoms/organization";
import { vendorsAtom, setVendorsAtom } from "src/atoms/vendor";
import { taxRatesAtom, setTaxRatesAtom } from "src/atoms/tax-rate";
import { purchaseOrdersAtom, setPurchaseOrdersAtom } from "src/atoms/purchase-order";
import {
  incomingInvoiceIdAtom,
  incomingInvoiceAtom,
  updateIncomingInvoiceStateAtom,
  deleteIncomingInvoiceAtom,
} from "src/atoms/incoming-invoice";

const { TextArea } = Input;
const { Option } = Select;
const { Footer } = Layout;

// incomingInvoiceAtom is async; reading it with plain useAtom throws to the
// app's single top-level Suspense boundary whenever incomingInvoiceIdAtom
// changes after mount, which unmounts this whole route (the effect below's
// cleanup resets incomingInvoiceIdAtom to null) and remounts it once the
// fetch resolves, setting the id back — an infinite loop. loadable()
// resolves synchronously instead of suspending, same fix as
// src/components/tax-rates/form.tsx and src/routes/invoices/details.tsx.
const loadableIncomingInvoiceAtom = loadable(incomingInvoiceAtom);

const IncomingInvoiceDetails = () => {
  const { id } = useParams<string>();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const { i18n } = useLingui();
  const {
    token: { colorBgContainer },
  } = theme.useToken();
  const dateFormat = useDatePickerFormat();

  const isNew = id === "new";
  const prefillOrderId = searchParams.get("purchaseOrderId");

  const organization = useAtomValue(organizationAtom);
  const vendors = useAtomValue(vendorsAtom);
  const setVendors = useSetAtom(setVendorsAtom);
  const taxRates = useAtomValue(taxRatesAtom);
  const setTaxRates = useSetAtom(setTaxRatesAtom);
  const purchaseOrders = useAtomValue(purchaseOrdersAtom);
  const setPurchaseOrders = useSetAtom(setPurchaseOrdersAtom);

  const [invoiceId, setInvoiceId] = useAtom(incomingInvoiceIdAtom);
  const invoiceLoadable = useAtomValue(loadableIncomingInvoiceAtom);
  const setInvoice = useSetAtom(incomingInvoiceAtom);
  const invoice = invoiceLoadable.state === "hasData" ? invoiceLoadable.data : undefined;
  const updateState = useSetAtom(updateIncomingInvoiceStateAtom);
  const deleteInvoice = useSetAtom(deleteIncomingInvoiceAtom);

  const [form] = Form.useForm();
  const [stateOverride, setStateOverride] = useState<string | null>(null);
  const [matchLines, setMatchLines] = useState<MatchLine[]>([]);

  useEffect(() => {
    setVendors();
    setTaxRates();
    setPurchaseOrders();
    setStateOverride(null);
    if (!isNew) {
      setInvoiceId(id ?? null);
    }
    return () => {
      setInvoiceId(null);
    };
  }, [id, isNew, setVendors, setTaxRates, setPurchaseOrders, setInvoiceId]);

  const refreshMatch = useCallback(() => {
    if (isNew || !id) {
      setMatchLines([]);
      return;
    }
    GetIncomingInvoiceMatch(id)
      .then(setMatchLines)
      .catch(() => setMatchLines([]));
  }, [id, isNew]);

  useEffect(refreshMatch, [refreshMatch]);

  // Prefill the lines from a purchase order so they arrive already linked —
  // an unlinked line has nothing to match against.
  useEffect(() => {
    if (!isNew || !prefillOrderId) return;
    let cancelled = false;

    (async () => {
      try {
        const lineItems = await GetPurchaseOrderLineItems(prefillOrderId);
        if (cancelled) return;
        const order: any = find(purchaseOrders, { id: prefillOrderId });
        form.setFieldsValue({
          purchaseOrderId: prefillOrderId,
          vendorId: order?.vendorId,
          currency: order?.currency ?? organization?.currency ?? "EUR",
          lineItems: (lineItems || []).map((item: any) => ({
            purchaseOrderLineItemId: item.id,
            productId: item.productId,
            description: item.description,
            quantity: item.quantity,
            unitPrice: centsToUnits(item.unitPrice ?? 0),
            taxRate: item.taxRate,
          })),
        });
      } catch {
        // Leave the blank line the form starts with.
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [isNew, prefillOrderId, purchaseOrders, organization, form]);

  useEffect(() => {
    if (isNew && invoiceId) {
      navigate(`/incoming-invoices/${invoiceId}`);
    }
  }, [isNew, invoiceId, navigate]);

  useEffect(() => {
    if (!isNew && invoice && typeof invoice === "object" && !("then" in invoice)) {
      form.resetFields();
      form.setFieldsValue(invoice);
    }
  }, [invoice, isNew, form]);

  // Watched raw so the useMemo below has a stable dependency — `?? []` here
  // would allocate a new array on every render and defeat the memo.
  const watchedLineItems = Form.useWatch("lineItems", form);

  // Totals must agree with db/invoice_totals.go or every save 409s: line totals
  // summed, then tax rounded once per tax-rate group — not per line.
  const totals = useMemo(() => {
    const lines = ((watchedLineItems as any[]) ?? []).filter(Boolean);
    const subTotal = sum(
      lines.map((item) => multiplyDecimal(item?.quantity ?? 0, item?.unitPrice ?? 0)),
    );

    const groups: Record<string, number> = {};
    for (const item of lines) {
      if (!item?.taxRate) continue;
      const lineTotal = multiplyDecimal(item?.quantity ?? 0, item?.unitPrice ?? 0);
      groups[item.taxRate] = addDecimal(groups[item.taxRate] ?? 0, lineTotal);
    }

    let taxTotal = 0;
    for (const [rateId, groupSubtotal] of Object.entries(groups)) {
      const rate: any = find(taxRates, { id: rateId });
      if (!rate) continue;
      taxTotal = addDecimal(taxTotal, calculateTax(groupSubtotal, rate.percentage));
    }

    return { subTotal, taxTotal, total: addDecimal(subTotal, taxTotal) };
  }, [watchedLineItems, taxRates]);

  const currentState =
    stateOverride ??
    (!isNew && invoice && !(invoice as any).then ? (invoice as any).state : "draft");
  const currency =
    (!isNew && invoice && !(invoice as any).then ? (invoice as any).currency : null) ??
    organization?.currency ??
    "EUR";
  const blocked = hasBlockingVariance(matchLines);
  const overrideActive = Form.useWatch("matchOverride", form) === true;

  // Read the whole form store, for the same StrictMode reason as the other
  // purchasing detail pages.
  const handleSubmit = async () => {
    const values = form.getFieldsValue(true);
    const ok = await setInvoice({
      ...values,
      matchOverride: values.matchOverride ? 1 : 0,
      ...totals,
    });
    if (ok) refreshMatch();
  };

  const handleDelete = async () => {
    if (!id || isNew) return;
    const success = await deleteInvoice(id);
    if (success) navigate("/incoming-invoices");
  };

  const handleStateChange = async (next: string) => {
    if (!id || isNew) return;
    const ok = await updateState({ invoiceId: id, state: next });
    if (ok) {
      setStateOverride(next);
      refreshMatch();
    }
  };

  const initialValues = isNew
    ? {
        date: dayjs(),
        state: "draft",
        currency: organization?.currency ?? "EUR",
        purchaseOrderId: prefillOrderId ?? undefined,
        matchOverride: false,
        lineItems: [{ quantity: 1 }],
      }
    : undefined;

  if (!organization) return null;
  if (!isNew && !invoice) return null;

  const money = (units: number) => getFormattedNumber(units, currency, i18n.locale, organization);

  return (
    <Form form={form} onFinish={handleSubmit} layout="vertical" initialValues={initialValues}>
      <Row gutter={24}>
        <Col span={6}>
          <Form.Item
            label={<Trans>Vendor</Trans>}
            name="vendorId"
            rules={[{ required: true, message: t`Vendor is required` }]}
          >
            <Select showSearch allowClear optionFilterProp="children">
              {map(vendors, (v: any) => (
                <Option key={v.id} value={v.id}>
                  {v.name}
                </Option>
              ))}
            </Select>
          </Form.Item>
        </Col>
        <Col span={5}>
          <Form.Item
            label={<Trans>Vendor invoice #</Trans>}
            name="vendorInvoiceNumber"
            rules={[{ required: true, message: t`Vendor invoice number is required` }]}
          >
            <Input placeholder={t`The number on the vendor's invoice`} />
          </Form.Item>
        </Col>
        <Col span={5}>
          <Form.Item label={<Trans>Purchase order</Trans>} name="purchaseOrderId">
            <Select showSearch allowClear optionFilterProp="children" placeholder={t`Optional`}>
              {map(purchaseOrders, (o: any) => (
                <Option key={o.id} value={o.id}>
                  {o.orderNumber}
                </Option>
              ))}
            </Select>
          </Form.Item>
        </Col>
        <Col span={4}>
          <Form.Item
            label={<Trans>Invoice date</Trans>}
            name="date"
            rules={[{ required: true, message: t`Invoice date is required` }]}
          >
            <DatePicker style={{ width: "100%" }} format={dateFormat} />
          </Form.Item>
        </Col>
        <Col span={4}>
          <Form.Item label={<Trans>Due date</Trans>} name="dueDate">
            <DatePicker style={{ width: "100%" }} format={dateFormat} />
          </Form.Item>
        </Col>
      </Row>

      <Row gutter={24}>
        <Col span={4}>
          <Form.Item
            label={<Trans>Currency</Trans>}
            name="currency"
            rules={[{ required: true, message: t`This field is required!` }]}
          >
            <Input />
          </Form.Item>
        </Col>
        <Col span={5}>
          <Form.Item label={<Trans>Our reference</Trans>} name="reference">
            <Input />
          </Form.Item>
        </Col>
        <Col span={5}>
          <Form.Item label={<Trans>State</Trans>}>
            <Space>
              <Tag color={incomingInvoiceStateColor[currentState as IncomingInvoiceState]}>
                {incomingInvoiceStateLabel(currentState)}
              </Tag>
            </Space>
          </Form.Item>
        </Col>
        <Col span={10}>
          <Form.Item label={<Trans>Notes</Trans>} name="notes">
            <TextArea rows={1} autoSize />
          </Form.Item>
        </Col>
      </Row>

      <Form.List name="lineItems">
        {(fields, { add, remove }) => (
          <>
            <Table
              dataSource={fields.map((field, index) => ({ ...field, index }))}
              pagination={false}
              size="middle"
              locale={{ emptyText: t`No line items` }}
              rowKey={(r) => r.index.toString()}
              style={{ marginTop: 8 }}
            >
              <Table.Column
                title={<Trans>Description</Trans>}
                key="description"
                render={(field) => (
                  <Form.Item
                    name={[field.name, "description"]}
                    noStyle
                    rules={[{ required: true, message: t`Description required` }]}
                  >
                    <TextArea rows={1} autoSize />
                  </Form.Item>
                )}
              />
              <Table.Column
                title={<Trans>Qty</Trans>}
                key="quantity"
                width={90}
                render={(field) => (
                  <Form.Item
                    name={[field.name, "quantity"]}
                    noStyle
                    rules={[{ required: true, message: t`Required` }]}
                  >
                    <InputNumber style={{ width: "100%" }} min={0} precision={2} />
                  </Form.Item>
                )}
              />
              <Table.Column
                title={<Trans>Unit price</Trans>}
                key="unitPrice"
                width={110}
                render={(field) => (
                  <Form.Item name={[field.name, "unitPrice"]} noStyle>
                    <InputNumber style={{ width: "100%" }} min={0} precision={2} step={0.01} />
                  </Form.Item>
                )}
              />
              <Table.Column
                title={<Trans>Tax</Trans>}
                key="taxRate"
                width={130}
                render={(field) => (
                  <Form.Item name={[field.name, "taxRate"]} noStyle>
                    <Select allowClear placeholder={t`None`} style={{ width: "100%" }}>
                      {map(taxRates, (r: any) => (
                        <Option key={r.id} value={r.id}>
                          {r.name}
                        </Option>
                      ))}
                    </Select>
                  </Form.Item>
                )}
              />
              <Table.Column
                title={<Trans>Match</Trans>}
                key="match"
                width={140}
                render={(field) => (
                  <Form.Item shouldUpdate noStyle>
                    {() => {
                      const lineId = form.getFieldValue(["lineItems", field.name, "id"]);
                      const line = matchLines.find((l) => l.lineItemId === lineId);
                      if (!line) return null;
                      return (
                        <Tag color={matchStatusColor[line.status]} title={matchStatusDetail(line)}>
                          {matchStatusLabel(line.status)}
                        </Tag>
                      );
                    }}
                  </Form.Item>
                )}
              />
              <Table.Column
                key="remove"
                width={40}
                render={(field) => (
                  <Button
                    type="text"
                    danger
                    size="small"
                    icon={<DeleteOutlined />}
                    onClick={() => remove(field.name)}
                    aria-label={t`Remove line item`}
                  />
                )}
              />
            </Table>

            <Button
              type="default"
              size="small"
              icon={<PlusOutlined />}
              onClick={() => add({ quantity: 1 })}
              style={{ marginTop: 12 }}
            >
              <Trans>Add line item</Trans>
            </Button>
          </>
        )}
      </Form.List>

      <Row justify="end" style={{ marginTop: 16 }}>
        <Col>
          <Descriptions
            column={1}
            styles={{
              content: { textAlign: "right", minWidth: 120, fontSize: 14 },
              label: { textAlign: "right", fontWeight: 500, fontSize: 14 },
            }}
          >
            <Descriptions.Item label={<Trans>Subtotal</Trans>}>
              {money(totals.subTotal)}
            </Descriptions.Item>
            <Descriptions.Item label={<Trans>Tax</Trans>}>{money(totals.taxTotal)}</Descriptions.Item>
            <Descriptions.Item label={<Trans>Total</Trans>}>{money(totals.total)}</Descriptions.Item>
          </Descriptions>
        </Col>
      </Row>

      {/* 3-way match panel — only meaningful once the invoice exists. */}
      {!isNew && matchLines.length > 0 && (
        <>
          <Divider>
            <Trans>3-way match</Trans>
          </Divider>
          <Table
            dataSource={matchLines}
            rowKey="lineItemId"
            pagination={false}
            size="small"
            style={{ marginBottom: 16 }}
          >
            <Table.Column title={<Trans>Description</Trans>} dataIndex="description" key="description" />
            <Table.Column
              title={<Trans>Ordered</Trans>}
              key="ordered"
              align="right"
              render={(line: MatchLine) => line.orderedQuantity ?? "—"}
            />
            <Table.Column
              title={<Trans>Received</Trans>}
              key="received"
              align="right"
              render={(line: MatchLine) => line.receivedQuantity ?? "—"}
            />
            <Table.Column
              title={<Trans>Already invoiced</Trans>}
              dataIndex="previouslyInvoicedQuantity"
              key="previouslyInvoicedQuantity"
              align="right"
            />
            <Table.Column
              title={<Trans>On this invoice</Trans>}
              dataIndex="invoicedQuantity"
              key="invoicedQuantity"
              align="right"
            />
            <Table.Column
              title={<Trans>Status</Trans>}
              key="status"
              render={(line: MatchLine) => (
                <Space direction="vertical" size={0}>
                  <Tag color={matchStatusColor[line.status]}>{matchStatusLabel(line.status)}</Tag>
                  {matchStatusDetail(line) && (
                    <span style={{ fontSize: 12, opacity: 0.65 }}>{matchStatusDetail(line)}</span>
                  )}
                </Space>
              )}
            />
          </Table>

          {blocked && (
            <Alert
              type="warning"
              showIcon
              style={{ marginBottom: 16 }}
              message={<Trans>This invoice does not match the purchase order and goods received</Trans>}
              description={
                <Trans>
                  Approval is blocked until the variance is resolved, or recorded as an override
                  with a reason.
                </Trans>
              }
            />
          )}

          {blocked && (
            <Row gutter={24}>
              <Col span={6}>
                <Form.Item name="matchOverride" valuePropName="checked">
                  <Checkbox>
                    <Trans>Approve despite the variance</Trans>
                  </Checkbox>
                </Form.Item>
              </Col>
              <Col span={18}>
                <Form.Item
                  name="matchOverrideReason"
                  label={<Trans>Override reason</Trans>}
                  rules={
                    overrideActive
                      ? [{ required: true, message: t`A reason is required to override` }]
                      : []
                  }
                >
                  <Input placeholder={t`Why is this variance acceptable?`} disabled={!overrideActive} />
                </Form.Item>
              </Col>
            </Row>
          )}
        </>
      )}

      {document.getElementById("footer") &&
        createPortal(
          <Footer
            style={{
              position: "sticky",
              bottom: 0,
              zIndex: 1,
              padding: "0 16px",
              background: colorBgContainer,
            }}
          >
            <Row align="middle" justify="space-between" style={{ height: 64 }}>
              <Col>
                {!isNew && (
                  <Popconfirm
                    title={t`Delete this incoming invoice?`}
                    onConfirm={handleDelete}
                    okText={t`Yes`}
                    cancelText={t`No`}
                  >
                    <Button type="dashed" danger>
                      <DeleteOutlined /> <Trans>Delete</Trans>
                    </Button>
                  </Popconfirm>
                )}
              </Col>
              <Col>
                <Space>
                  {!isNew && (
                    <Select
                      value={currentState}
                      style={{ width: 150 }}
                      onChange={handleStateChange}
                      options={INCOMING_INVOICE_STATES.map((s) => ({
                        value: s,
                        label: incomingInvoiceStateLabel(s),
                      }))}
                    />
                  )}
                  <Button type="primary" onClick={() => form.submit()}>
                    <SaveOutlined /> <Trans>Save</Trans>
                  </Button>
                </Space>
              </Col>
            </Row>
          </Footer>,
          document.getElementById("footer") as HTMLElement,
        )}
    </Form>
  );
};

export default IncomingInvoiceDetails;
