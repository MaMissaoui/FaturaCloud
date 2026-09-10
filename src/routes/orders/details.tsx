import { useEffect, useState, useMemo } from "react";
import { createPortal } from "react-dom";
import { useNavigate, useParams } from "react-router";
import {
  Button,
  Col,
  DatePicker,
  Descriptions,
  Divider,
  Form,
  Input,
  Layout,
  message,
  Popconfirm,
  Row,
  Select,
  Space,
  Tag,
  theme,
  Tooltip,
} from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import { loadable } from "src/utils/loadable";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import {
  DeleteOutlined,
  FileExcelOutlined,
  FilePdfOutlined,
  PlusOutlined,
  SaveOutlined,
  ShoppingOutlined,
  UserAddOutlined,
} from "@ant-design/icons";
import dayjs from "dayjs";
import find from "lodash/find";
import get from "lodash/get";
import includes from "lodash/includes";
import isString from "lodash/isString";
import lowerCase from "lodash/lowerCase";
import map from "lodash/map";
import sum from "lodash/sum";
import { ExportOrderDocument, GetOrderDeliveredQuantities } from "src/api";
import PageHeader from "src/components/page-header";
import { useDatePickerFormat } from "src/utils/date";
import { centsToUnits } from "src/utils/currency";
import ExchangeRateFields, {
  CurrencySelect,
  prefillExchangeRate,
} from "src/components/currency/currency-fields";
import { clientsAtom, setClientsAtom } from "src/atoms/client";
import { organizationAtom } from "src/atoms/organization";
import { productsAtom, setProductsAtom } from "src/atoms/product";
import {
  orderIdAtom,
  orderAtom,
  nextOrderNumberAtom,
  updateOrderStatusAtom,
  deleteOrderAtom,
} from "src/atoms/order";
import LineItemsTable from "src/components/line-items/table";
import StatusFlow from "src/components/status-flow";
import {
  ORDER_STATUSES,
  orderStatusColor,
  orderStatusLabel,
  orderStatusTransitionMatrix,
  orderTransitions,
  type OrderStatus,
} from "src/types/order";

const { TextArea } = Input;
const { Option } = Select;
const { Footer } = Layout;

// Module-level so it's referentially stable across renders — StatusFlow is
// memoized and an inline arrow here would defeat that on every keystroke.
const getOrderStatusColor = (s: OrderStatus) => orderStatusColor[s];

// orderAtom is async; reading it with plain useAtom throws to the app's
// single top-level Suspense boundary whenever orderIdAtom changes after
// mount, which unmounts this whole route (the effect below's cleanup resets
// orderIdAtom to null) and remounts it once the fetch resolves, setting the
// id back — an infinite loop. loadable() resolves synchronously instead of
// suspending, same fix as src/routes/invoices/details.tsx (#31/#32).
const loadableOrderAtom = loadable(orderAtom);

const OrderDetails = () => {
  const { id } = useParams<string>();
  const navigate = useNavigate();
  const { i18n } = useLingui();
  const {
    token: { colorBgContainer },
  } = theme.useToken();
  const dateFormat = useDatePickerFormat();

  const isNew = id === "new";

  const organization = useAtomValue(organizationAtom);
  const clients = useAtomValue(clientsAtom);
  const setClients = useSetAtom(setClientsAtom);
  const products = useAtomValue(productsAtom);
  // A component/intermediate isn't sellable — exclude it from the picker.
  // Unclassified products (category null) stay eligible everywhere.
  const sellableProducts = useMemo(
    () => products.filter((p: any) => p.category !== "component"),
    [products],
  );
  const setProducts = useSetAtom(setProductsAtom);
  const nextNumber = useAtomValue(nextOrderNumberAtom);

  const [orderId, setOrderId] = useAtom(orderIdAtom);
  const orderLoadable = useAtomValue(loadableOrderAtom);
  const setOrder = useSetAtom(orderAtom);
  const order = orderLoadable.state === "hasData" ? orderLoadable.data : undefined;
  const updateStatus = useSetAtom(updateOrderStatusAtom);
  const deleteOrder = useSetAtom(deleteOrderAtom);

  const [form] = Form.useForm();
  const [deliveredQuantities, setDeliveredQuantities] = useState<Record<string, number>>({});
  const [isDirty, setIsDirty] = useState(false);
  const [downloadingPdf, setDownloadingPdf] = useState(false);
  const [downloadingExcel, setDownloadingExcel] = useState(false);

  useEffect(() => {
    setClients();
    setProducts();
    if (!isNew) {
      setOrderId(id ?? null);
    }
    return () => {
      setOrderId(null);
    };
  }, [id, isNew, setClients, setProducts, setOrderId]);

  // Track how much of each line item has already been delivered, so partial
  // fulfillment is visible without opening every delivery for this order.
  useEffect(() => {
    if (isNew || !id) {
      setDeliveredQuantities({});
      return;
    }
    GetOrderDeliveredQuantities(id)
      .then(setDeliveredQuantities)
      .catch(() => setDeliveredQuantities({}));
  }, [id, isNew]);

  // After create, navigate to the new order
  useEffect(() => {
    if (isNew && orderId) {
      navigate(`/orders/${orderId}`);
    }
  }, [isNew, orderId, navigate]);

  // Populate form when order loads
  useEffect(() => {
    if (!isNew && order && typeof order === "object" && !("then" in order)) {
      form.resetFields();
      form.setFieldsValue(order);
    }
  }, [order, isNew, form]);

  const lineItems = Form.useWatch("lineItems", form) ?? [];
  const subTotal = sum(
    lineItems.map((item: any) => {
      const qty = parseFloat(item?.quantity ?? 0) || 0;
      const price = parseFloat(item?.unitPrice ?? 0) || 0;
      return qty * price;
    }),
  );

  const handleSubmit = async (values: any) => {
    await setOrder(values);
    setIsDirty(false);
  };

  const handleDelete = async () => {
    if (!id || isNew) return;
    const success = await deleteOrder(id);
    if (success) navigate("/orders");
  };

  const handleStatusChange = async (next: string) => {
    if (!id || isNew) return;
    await updateStatus({ orderId: id, status: next });
    // Reload order
    setOrderId(null);
    setTimeout(() => setOrderId(id), 0);
  };

  // Server fill-and-convert path (db/xlsx_export_order.go /
  // db/pdf_convert.go) — the same mechanism invoices/purchase orders use, so
  // a PDF and an Excel export of the same order are always the same
  // document. Reads persisted line items by order id, so both are gated on
  // isDirty.
  const handleServerExport = (format: "pdf" | "xlsx") => async () => {
    if (!id) return;
    const setDownloading = format === "xlsx" ? setDownloadingExcel : setDownloadingPdf;
    setDownloading(true);
    try {
      await ExportOrderDocument(id, format);
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Export failed`);
    } finally {
      setDownloading(false);
    }
  };

  const initialValues = isNew
    ? {
        orderNumber: nextNumber,
        orderDate: dayjs(),
        status: "draft",
        currency: organization?.currency ?? "EUR",
        lineItems: [{ quantity: 1 }],
      }
    : undefined;

  const watchedCurrency = Form.useWatch("currency", form);
  const orgCurrency = organization?.currency ?? "EUR";
  const currency =
    watchedCurrency ??
    (!isNew && order && !(order as any).then ? (order as any).currency : null) ??
    orgCurrency;

  const currentStatus = !isNew && order && !(order as any).then ? (order as any).status : "draft";

  const transitions = orderTransitions(currentStatus);

  if (!organization) return null;
  if (!isNew && !order) return null;

  return (
    <>
      <PageHeader
        icon={<ShoppingOutlined />}
        title={<Trans>Order</Trans>}
        style={{ marginBottom: 24 }}
      />
      <Form
        form={form}
        onFinish={handleSubmit}
        layout="vertical"
        initialValues={initialValues}
        onValuesChange={() => setIsDirty(true)}
      >
        <Row gutter={24}>
          <Col xs={24} md={12} xl={7}>
            <Form.Item
              label={<Trans>Client</Trans>}
              name="clientId"
              rules={[{ required: true, message: t`Client is required` }]}
            >
              <Select
                showSearch
                allowClear
                optionFilterProp="children"
                filterOption={(input, option) => {
                  const name = get(option, ["props", "children"]);
                  return isString(name) ? includes(lowerCase(name), lowerCase(input)) : true;
                }}
                onChange={(clientId) => {
                  // Only cascade the default on a new order — resetting the
                  // currency of an already-saved order just because its client
                  // changed would silently disturb an existing document.
                  if (!isNew) return;
                  const client = find(clients, { id: clientId }) as any;
                  if (client?.defaultCurrency) {
                    form.setFieldValue("currency", client.defaultCurrency);
                    prefillExchangeRate(
                      form,
                      organization?.id,
                      client.defaultCurrency,
                      orgCurrency,
                    );
                  }
                }}
                popupRender={(menu) => (
                  <>
                    {menu}
                    <Divider style={{ margin: "8px 0" }} />
                    <Button
                      type="text"
                      block
                      icon={<UserAddOutlined />}
                      onClick={(e) => {
                        e.preventDefault();
                        navigate("/clients");
                      }}
                      style={{ textAlign: "left", paddingLeft: 11 }}
                    >
                      <Trans>Manage clients</Trans>
                    </Button>
                  </>
                )}
              >
                {map(clients, (c: any) => (
                  <Option key={c.id} value={c.id}>
                    {c.name}
                  </Option>
                ))}
              </Select>
            </Form.Item>
          </Col>
          <Col xs={24} md={12} xl={3}>
            <Form.Item
              label={<Trans>Order number</Trans>}
              name="orderNumber"
              rules={[{ required: true, message: t`Order number is required` }]}
            >
              <Input />
            </Form.Item>
          </Col>
          <Col xs={24} md={12} xl={3}>
            <Form.Item
              label={<Trans>Order date</Trans>}
              name="orderDate"
              rules={[{ required: true, message: t`Order date is required` }]}
            >
              <DatePicker style={{ width: "100%" }} format={dateFormat} />
            </Form.Item>
          </Col>
          <Col xs={24} md={12} xl={3}>
            <Form.Item label={<Trans>Delivery date</Trans>} name="deliveryDate">
              <DatePicker style={{ width: "100%" }} format={dateFormat} />
            </Form.Item>
          </Col>
          <Col xs={24} md={12} xl={3}>
            <Form.Item label={<Trans>Tracking number</Trans>} name="trackingNumber">
              <Input placeholder="e.g. FX1234567890" />
            </Form.Item>
          </Col>
          <Col xs={24} md={12} xl={4}>
            <Form.Item label={<Trans>Status</Trans>}>
              <Tag color={orderStatusColor[currentStatus as OrderStatus]}>
                {orderStatusLabel(currentStatus)}
              </Tag>
              <StatusFlow
                current={currentStatus as OrderStatus}
                statuses={ORDER_STATUSES}
                transitions={orderStatusTransitionMatrix}
                getLabel={orderStatusLabel}
                getColor={getOrderStatusColor}
              />
            </Form.Item>
          </Col>
          <CurrencySelect form={form} organizationId={organization?.id} orgCurrency={orgCurrency} />
          <ExchangeRateFields currency={watchedCurrency} orgCurrency={orgCurrency} />
        </Row>

        <Row gutter={24}>
          <Col xs={24} md={12}>
            <Form.Item label={<Trans>Shipping address</Trans>} name="shippingAddress">
              <TextArea rows={2} placeholder={t`Leave blank to use client address`} />
            </Form.Item>
          </Col>
          <Col xs={24} md={12}>
            <Form.Item label={<Trans>Notes</Trans>} name="notes">
              <TextArea rows={2} />
            </Form.Item>
          </Col>
        </Row>

        {/* Line items */}
        <LineItemsTable
          columns={[
            { kind: "index" },
            {
              kind: "product",
              products: sellableProducts,
              required: true,
              onSelect: (productId, fieldName, formInstance) => {
                const product = find(products, { id: productId });
                if (product) {
                  const lineItems = formInstance.getFieldValue("lineItems");
                  lineItems[fieldName] = {
                    ...lineItems[fieldName],
                    description: (product as any).name,
                    unitPrice: centsToUnits((product as any).price ?? 0),
                  };
                  formInstance.setFieldValue("lineItems", [...lineItems]);
                }
              },
            },
            { kind: "description", required: true },
            { kind: "quantity" },
            { kind: "unitPrice" },
            ...(!isNew
              ? [
                  {
                    kind: "custom" as const,
                    key: "delivered",
                    title: <Trans>Delivered</Trans>,
                    width: 90,
                    render: (field: { name: number }) => (
                      <Form.Item shouldUpdate noStyle>
                        {() => {
                          const itemId = form.getFieldValue(["lineItems", field.name, "id"]);
                          const quantity =
                            form.getFieldValue(["lineItems", field.name, "quantity"]) ?? 0;
                          if (!itemId) return null;
                          const delivered = deliveredQuantities[itemId] ?? 0;
                          return (
                            <Tag
                              color={
                                delivered >= quantity
                                  ? "success"
                                  : delivered > 0
                                    ? "processing"
                                    : "default"
                              }
                            >
                              {delivered} / {quantity}
                            </Tag>
                          );
                        }}
                      </Form.Item>
                    ),
                  },
                ]
              : []),
          ]}
        />

        {/* Totals */}
        {subTotal > 0 && (
          <Row justify="end" style={{ marginTop: 16 }}>
            <Col>
              <Descriptions
                column={1}
                styles={{
                  content: { textAlign: "right", minWidth: 100, fontSize: 14 },
                  label: { textAlign: "right", fontWeight: 500, fontSize: 14 },
                }}
              >
                <Descriptions.Item label={<Trans>Subtotal</Trans>}>
                  {Intl.NumberFormat(i18n.locale, {
                    style: "currency",
                    currency,
                    minimumFractionDigits: organization.minimum_fraction_digits ?? undefined,
                  }).format(subTotal)}
                </Descriptions.Item>
              </Descriptions>
            </Col>
          </Row>
        )}

        {/* Footer bar */}
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
                  {!isNew && !["shipped", "delivered"].includes(currentStatus) && (
                    <Popconfirm
                      title={t`Delete this order?`}
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
                    {!isNew &&
                      transitions.map((t2) => (
                        <Button
                          key={t2.next}
                          type={t2.type ?? "default"}
                          onClick={() => handleStatusChange(t2.next)}
                        >
                          {t2.label}
                        </Button>
                      ))}
                    {!isNew &&
                      orderStatusTransitionMatrix[currentStatus as OrderStatus]?.includes(
                        "cancelled",
                      ) && (
                        <Popconfirm
                          title={t`Cancel this order?`}
                          onConfirm={() => handleStatusChange("cancelled")}
                          okText={t`Yes`}
                          cancelText={t`No`}
                        >
                          <Button type="dashed" danger>
                            <Trans>Cancel order</Trans>
                          </Button>
                        </Popconfirm>
                      )}
                    {!isNew && (
                      <Tooltip title={isDirty ? t`Save your changes before exporting` : undefined}>
                        <Button
                          disabled={isDirty}
                          loading={downloadingPdf}
                          onClick={handleServerExport("pdf")}
                        >
                          <FilePdfOutlined /> PDF
                        </Button>
                      </Tooltip>
                    )}
                    {!isNew && (
                      // Always the server fill-and-convert path
                      // (db/xlsx_export_order.go) — every order has an
                      // embedded fallback template (resolveTemplateBytes) to
                      // fill even with no org override, so both buttons
                      // always work.
                      <Tooltip title={isDirty ? t`Save your changes before exporting` : undefined}>
                        <Button
                          disabled={isDirty}
                          loading={downloadingExcel}
                          onClick={handleServerExport("xlsx")}
                        >
                          <FileExcelOutlined /> <Trans>Excel</Trans>
                        </Button>
                      </Tooltip>
                    )}
                    {!isNew && (
                      <Button onClick={() => navigate(`/deliveries/new?orderId=${id}`)}>
                        <PlusOutlined /> <Trans>New delivery</Trans>
                      </Button>
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
    </>
  );
};

export default OrderDetails;
