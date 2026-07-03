import { useEffect } from "react";
import { createPortal } from "react-dom";
import { useNavigate, useParams, useSearchParams } from "react-router";
import {
  Button,
  Col,
  DatePicker,
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
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { DeleteOutlined, FilePdfOutlined, PlusOutlined, SaveOutlined } from "@ant-design/icons";
import { pdf } from "@react-pdf/renderer";
import dayjs from "dayjs";
import find from "lodash/find";
import map from "lodash/map";
import { SaveFile, GetOrderLineItems, GetOrderDeliveredQuantities } from "src/api";
import { useDatePickerFormat } from "src/utils/date";
import { organizationAtom } from "src/atoms/organization";
import { ordersAtom, setOrdersAtom } from "src/atoms/order";
import { clientsAtom, setClientsAtom } from "src/atoms/client";
import { productsAtom, setProductsAtom } from "src/atoms/product";
import {
  deliveryIdAtom,
  deliveryAtom,
  nextDeliveryNumberAtom,
  updateDeliveryStatusAtom,
  deleteDeliveryAtom,
} from "src/atoms/delivery";
import DeliveryNotePDF from "src/components/deliveries/delivery-note-pdf";

const { TextArea } = Input;
const { Option } = Select;
const { Footer } = Layout;

const STATUS_TRANSITIONS: Record<string, { label: string; next: string; color: string; type?: "primary" | "default" }[]> = {
  draft: [{ label: "Mark as shipped", next: "shipped", color: "orange", type: "primary" }],
  shipped: [{ label: "Mark as delivered", next: "delivered", color: "green", type: "primary" }],
  delivered: [],
};

const STATUS_COLORS: Record<string, string> = {
  draft: "default",
  shipped: "orange",
  delivered: "green",
};

const DeliveryDetails = () => {
  const { id } = useParams<string>();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const { i18n } = useLingui();
  const {
    token: { colorBgContainer },
  } = theme.useToken();
  const dateFormat = useDatePickerFormat();

  const isNew = id === "new";
  const prefillOrderId = searchParams.get("orderId") ?? undefined;

  const organization = useAtomValue(organizationAtom);
  const orders = useAtomValue(ordersAtom);
  const setOrders = useSetAtom(setOrdersAtom);
  const clients = useAtomValue(clientsAtom);
  const setClients = useSetAtom(setClientsAtom);
  const products = useAtomValue(productsAtom);
  const setProducts = useSetAtom(setProductsAtom);
  const nextNumber = useAtomValue(nextDeliveryNumberAtom);

  const [deliveryId, setDeliveryId] = useAtom(deliveryIdAtom);
  const [delivery, setDelivery] = useAtom(deliveryAtom);
  const updateStatus = useSetAtom(updateDeliveryStatusAtom);
  const deleteDelivery = useSetAtom(deleteDeliveryAtom);

  const [form] = Form.useForm();

  useEffect(() => {
    setClients();
    setOrders();
    setProducts();
    if (!isNew) {
      setDeliveryId(id ?? null);
    }
    return () => { setDeliveryId(null); };
  }, [id, isNew, setClients, setOrders, setProducts, setDeliveryId]);

  // When creating a delivery from an order, prefill line items with the
  // quantity still outstanding (order quantity minus what's already been
  // delivered by other non-cancelled deliveries) so full or partial
  // fulfillment is just a matter of adjusting/removing lines.
  useEffect(() => {
    if (!isNew || !prefillOrderId) return;
    let cancelled = false;
    (async () => {
      const [orderLineItems, delivered] = await Promise.all([
        GetOrderLineItems(prefillOrderId),
        GetOrderDeliveredQuantities(prefillOrderId),
      ]);
      if (cancelled) return;

      const lineItems = (orderLineItems as any[])
        .map((item) => ({
          item,
          remaining: item.quantity - (delivered[item.id] ?? 0),
        }))
        .filter(({ remaining }) => remaining > 0)
        .map(({ item, remaining }) => {
          const product = item.productId ? find(products, { id: item.productId }) : null;
          return {
            orderLineItemId: item.id,
            description: item.description,
            quantity: remaining,
            unit: (product as any)?.unit,
            productId: item.productId,
            stockEnabled: (product as any)?.stockEnabled,
            availableStock: (product as any)?.stockQuantity,
          };
        });

      if (lineItems.length > 0) {
        form.setFieldsValue({ lineItems });
      }
    })();
    return () => { cancelled = true; };
  }, [isNew, prefillOrderId, products, form]);

  // After create, navigate to the new delivery
  useEffect(() => {
    if (isNew && deliveryId) {
      navigate(`/deliveries/${deliveryId}`);
    }
  }, [isNew, deliveryId, navigate]);

  // Populate form when delivery loads
  useEffect(() => {
    if (!isNew && delivery && typeof delivery === "object" && !("then" in delivery)) {
      form.resetFields();
      form.setFieldsValue(delivery);
    }
  }, [delivery, isNew, form]);

  const handleSubmit = async (values: any) => {
    await setDelivery(values);
  };

  const handleDelete = async () => {
    if (!id || isNew) return;
    await deleteDelivery(id);
    navigate("/deliveries");
  };

  const handleStatusChange = async (next: string) => {
    if (!id || isNew) return;
    await updateStatus({ deliveryId: id, status: next });
    setDeliveryId(null);
    setTimeout(() => setDeliveryId(id), 0);
  };

  const handlePrintDeliveryNote = async () => {
    const values = form.getFieldsValue();
    const deliveryData = {
      ...(!isNew && delivery && !(delivery as any).then ? delivery : {}),
      ...values,
      deliveryDate: values.deliveryDate?.valueOf ? values.deliveryDate.valueOf() : values.deliveryDate,
    };

    const orderId = values.orderId;
    const orderData = orderId ? find(orders, { id: orderId }) : null;
    if (orderData) {
      deliveryData.orderNumber = (orderData as any).orderNumber;
    }

    const clientId = orderData ? (orderData as any).clientId : null;
    const clientData = clientId ? find(clients, { id: clientId }) : null;

    const lineItemsForPdf = (values.lineItems ?? []).map((item: any) => ({
      ...item,
      sku: item.productId ? (find(products, { id: item.productId }) as any)?.sku : undefined,
    }));

    const doc = (
      <DeliveryNotePDF
        delivery={deliveryData}
        lineItems={lineItemsForPdf}
        client={clientData}
        organization={organization}
        locale={i18n.locale}
      />
    );

    const blob = await pdf(doc).toBlob();
    const num = deliveryData.deliveryNumber ?? (id ?? "delivery");
    await SaveFile(`delivery-note-${num}.pdf`, blob);
  };

  const initialValues = isNew
    ? {
        deliveryNumber: nextNumber,
        deliveryDate: dayjs(),
        status: "draft",
        orderId: prefillOrderId,
        lineItems: [{ quantity: 1 }],
      }
    : undefined;

  const currentStatus = !isNew && delivery && !(delivery as any).then
    ? (delivery as any).status ?? "draft"
    : "draft";

  const transitions = STATUS_TRANSITIONS[currentStatus] ?? [];

  if (!organization) return null;
  if (!isNew && !delivery) return null;

  return (
    <Form
      form={form}
      onFinish={handleSubmit}
      layout="vertical"
      initialValues={initialValues}
    >
      <Row gutter={24}>
        <Col span={6}>
          <Form.Item label={<Trans>Linked order</Trans>} name="orderId">
            <Select allowClear showSearch optionFilterProp="children">
              {(orders as any[]).map((o: any) => (
                <Option key={o.id} value={o.id}>{o.orderNumber}</Option>
              ))}
            </Select>
          </Form.Item>
        </Col>
        <Col span={4}>
          <Form.Item
            label={<Trans>Delivery number</Trans>}
            name="deliveryNumber"
            rules={[{ required: true, message: t`Required` }]}
          >
            <Input />
          </Form.Item>
        </Col>
        <Col span={4}>
          <Form.Item
            label={<Trans>Delivery date</Trans>}
            name="deliveryDate"
            rules={[{ required: true, message: t`Delivery date is required` }]}
          >
            <DatePicker style={{ width: "100%" }} format={dateFormat} />
          </Form.Item>
        </Col>
        <Col span={4}>
          <Form.Item label={<Trans>Tracking number</Trans>} name="trackingNumber">
            <Input />
          </Form.Item>
        </Col>
        <Col span={4}>
          <Form.Item label={<Trans>Status</Trans>}>
            <Tag color={STATUS_COLORS[currentStatus] ?? "default"} style={{ fontSize: 13, padding: "4px 10px", marginTop: 4 }}>
              {currentStatus}
            </Tag>
          </Form.Item>
        </Col>
      </Row>

      <Row gutter={24}>
        <Col span={12}>
          <Form.Item label={<Trans>Shipping address</Trans>} name="shippingAddress">
            <TextArea rows={2} />
          </Form.Item>
        </Col>
        <Col span={12}>
          <Form.Item label={<Trans>Notes</Trans>} name="notes">
            <TextArea rows={2} />
          </Form.Item>
        </Col>
      </Row>

      <Divider style={{ marginTop: 0 }} />

      {/* Line items — no prices */}
      <Form.List name="lineItems">
        {(fields, { add, remove }) => (
          <>
            <Table
              dataSource={fields.map((field, index) => ({ ...field, index }))}
              pagination={false}
              size="middle"
              locale={{ emptyText: t`No line items` }}
              rowKey={(r) => r.index.toString()}
            >
              <Table.Column
                title={<Trans>Product</Trans>}
                key="productId"
                width={180}
                render={(field) => (
                  <Form.Item name={[field.name, "productId"]} noStyle>
                    <Select
                      allowClear
                      showSearch
                      style={{ width: "100%" }}
                      placeholder={t`Optional`}
                      optionFilterProp="children"
                      onChange={(productId) => {
                        const lineItems = form.getFieldValue("lineItems");
                        const product = productId ? find(products, { id: productId }) : null;
                        lineItems[field.name] = {
                          ...lineItems[field.name],
                          description: (product as any)?.name ?? lineItems[field.name]?.description,
                          unit: (product as any)?.unit,
                          stockEnabled: (product as any)?.stockEnabled,
                          availableStock: (product as any)?.stockQuantity,
                        };
                        form.setFieldValue("lineItems", [...lineItems]);
                      }}
                    >
                      {map(products, (p: any) => (
                        <Option key={p.id} value={p.id}>{p.name}{p.sku ? ` (${p.sku})` : ""}</Option>
                      ))}
                    </Select>
                  </Form.Item>
                )}
              />
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
                width={110}
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
                title={<Trans>Available stock</Trans>}
                key="availableStock"
                width={120}
                render={(field) => (
                  <Form.Item shouldUpdate noStyle>
                    {() => {
                      const stockEnabled = form.getFieldValue(["lineItems", field.name, "stockEnabled"]);
                      if (!stockEnabled) return null;
                      const available = form.getFieldValue(["lineItems", field.name, "availableStock"]) ?? 0;
                      const requested = form.getFieldValue(["lineItems", field.name, "quantity"]) ?? 0;
                      return (
                        <Tag color={requested > available ? "error" : "default"}>
                          {available}
                        </Tag>
                      );
                    }}
                  </Form.Item>
                )}
              />
              <Table.Column
                title={<Trans>Unit</Trans>}
                key="unit"
                width={110}
                render={(field) => (
                  <Form.Item name={[field.name, "unit"]} noStyle>
                    <Input placeholder={t`pcs, kg, m…`} />
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
                    title={t`Delete this delivery?`}
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
                  {!isNew && transitions.map((tr) => (
                    <Button
                      key={tr.next}
                      type={tr.type ?? "default"}
                      onClick={() => handleStatusChange(tr.next)}
                    >
                      {tr.label}
                    </Button>
                  ))}
                  {!isNew && !["cancelled", "delivered"].includes(currentStatus) && (
                    <Popconfirm
                      title={t`Cancel this delivery?`}
                      onConfirm={() => handleStatusChange("cancelled")}
                      okText={t`Yes`}
                      cancelText={t`No`}
                    >
                      <Button type="dashed" danger>
                        <Trans>Cancel delivery</Trans>
                      </Button>
                    </Popconfirm>
                  )}
                  {!isNew && (
                    <Button onClick={handlePrintDeliveryNote}>
                      <FilePdfOutlined /> <Trans>Delivery note</Trans>
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
  );
};

export default DeliveryDetails;
