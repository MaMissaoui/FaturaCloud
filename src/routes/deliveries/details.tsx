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
  Layout,
  Popconfirm,
  Row,
  Select,
  Space,
  Tag,
  theme,
} from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import { loadable } from "src/utils/loadable";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { DeleteOutlined, FilePdfOutlined, SaveOutlined } from "@ant-design/icons";
import { pdf } from "@react-pdf/renderer";
import dayjs from "dayjs";
import find from "lodash/find";
import { SaveFile, GetOrderLineItems, GetOrderDeliveredQuantities } from "src/api";
import { useDatePickerFormat } from "src/utils/date";
import LineItemsTable from "src/components/line-items/table";
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
import {
  deliveryStatusColor,
  deliveryStatusLabel,
  deliveryTransitions,
  type DeliveryStatus,
} from "src/types/delivery";

const { TextArea } = Input;
const { Option } = Select;
const { Footer } = Layout;

// deliveryAtom is async; reading it with plain useAtom throws to the app's
// single top-level Suspense boundary whenever deliveryIdAtom changes after
// mount, which unmounts this whole route (the effect below's cleanup resets
// deliveryIdAtom to null) and remounts it once the fetch resolves, setting
// the id back — an infinite loop. loadable() resolves synchronously instead
// of suspending, same fix as src/routes/invoices/details.tsx (#31/#32).
const loadableDeliveryAtom = loadable(deliveryAtom);

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
  const deliveryLoadable = useAtomValue(loadableDeliveryAtom);
  const setDelivery = useSetAtom(deliveryAtom);
  const delivery = deliveryLoadable.state === "hasData" ? deliveryLoadable.data : undefined;
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
    return () => {
      setDeliveryId(null);
    };
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
    return () => {
      cancelled = true;
    };
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
    const success = await deleteDelivery(id);
    if (success) navigate("/deliveries");
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
      deliveryDate: values.deliveryDate?.valueOf
        ? values.deliveryDate.valueOf()
        : values.deliveryDate,
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
    const num = deliveryData.deliveryNumber ?? id ?? "delivery";
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

  const currentStatus =
    !isNew && delivery && !(delivery as any).then ? ((delivery as any).status ?? "draft") : "draft";

  // Mirrors the server-side guard in db/delivery.go: line items are frozen
  // once a delivery is shipped/delivered. Header-only fields (tracking
  // number, notes) stay editable — this only gates the line-item table.
  const isEditable = isNew || !["shipped", "delivered"].includes(currentStatus);

  const transitions = deliveryTransitions(currentStatus);

  if (!organization) return null;
  if (!isNew && !delivery) return null;

  return (
    <Form form={form} onFinish={handleSubmit} layout="vertical" initialValues={initialValues}>
      <Row gutter={24}>
        <Col xs={24} md={12} xl={6}>
          <Form.Item label={<Trans>Linked order</Trans>} name="orderId">
            <Select allowClear showSearch optionFilterProp="children">
              {(orders as any[]).map((o: any) => (
                <Option key={o.id} value={o.id}>
                  {o.orderNumber}
                </Option>
              ))}
            </Select>
          </Form.Item>
        </Col>
        <Col xs={24} md={12} xl={4}>
          <Form.Item
            label={<Trans>Delivery number</Trans>}
            name="deliveryNumber"
            rules={[{ required: true, message: t`Required` }]}
          >
            <Input />
          </Form.Item>
        </Col>
        <Col xs={24} md={12} xl={4}>
          <Form.Item
            label={<Trans>Delivery date</Trans>}
            name="deliveryDate"
            rules={[{ required: true, message: t`Delivery date is required` }]}
          >
            <DatePicker style={{ width: "100%" }} format={dateFormat} />
          </Form.Item>
        </Col>
        <Col xs={24} md={12} xl={4}>
          <Form.Item label={<Trans>Tracking number</Trans>} name="trackingNumber">
            <Input />
          </Form.Item>
        </Col>
        <Col xs={24} md={12} xl={4}>
          <Form.Item label={<Trans>Status</Trans>}>
            <Tag
              color={deliveryStatusColor[currentStatus as DeliveryStatus]}
              style={{ fontSize: 13, padding: "4px 10px", marginTop: 4 }}
            >
              {deliveryStatusLabel(currentStatus)}
            </Tag>
          </Form.Item>
        </Col>
      </Row>

      <Row gutter={24}>
        <Col xs={24} md={12}>
          <Form.Item label={<Trans>Shipping address</Trans>} name="shippingAddress">
            <TextArea rows={2} />
          </Form.Item>
        </Col>
        <Col xs={24} md={12}>
          <Form.Item label={<Trans>Notes</Trans>} name="notes">
            <TextArea rows={2} />
          </Form.Item>
        </Col>
      </Row>

      <Divider style={{ marginTop: 0 }} />

      {/* Line items — no prices */}
      <LineItemsTable
        disabled={!isEditable}
        columns={[
          { kind: "index" },
          {
            kind: "product",
            products,
            required: true,
            onSelect: (productId, fieldName, formInstance) => {
              const lineItems = formInstance.getFieldValue("lineItems");
              const product = productId ? find(products, { id: productId }) : null;
              lineItems[fieldName] = {
                ...lineItems[fieldName],
                description: (product as any)?.name ?? lineItems[fieldName]?.description,
                unit: (product as any)?.unit,
                stockEnabled: (product as any)?.stockEnabled,
                availableStock: (product as any)?.stockQuantity,
              };
              formInstance.setFieldValue("lineItems", [...lineItems]);
            },
          },
          { kind: "description", required: true },
          { kind: "quantity", width: 110 },
          {
            kind: "custom",
            key: "availableStock",
            title: <Trans>Available stock</Trans>,
            width: 120,
            render: (field) => (
              <Form.Item shouldUpdate noStyle>
                {() => {
                  const stockEnabled = form.getFieldValue([
                    "lineItems",
                    field.name,
                    "stockEnabled",
                  ]);
                  if (!stockEnabled) return null;
                  const available =
                    form.getFieldValue(["lineItems", field.name, "availableStock"]) ?? 0;
                  const requested = form.getFieldValue(["lineItems", field.name, "quantity"]) ?? 0;
                  return <Tag color={requested > available ? "error" : "default"}>{available}</Tag>;
                }}
              </Form.Item>
            ),
          },
          { kind: "unit" },
        ]}
      />

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
                  {!isNew &&
                    transitions.map((tr) => (
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
