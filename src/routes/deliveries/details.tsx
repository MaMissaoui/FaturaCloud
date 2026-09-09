import { useEffect, useState } from "react";
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
  Tooltip,
  message,
} from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import { loadable } from "src/utils/loadable";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import {
  DeleteOutlined,
  FileExcelOutlined,
  FilePdfOutlined,
  SaveOutlined,
} from "@ant-design/icons";
import dayjs from "dayjs";
import find from "lodash/find";
import { ExportDeliveryDocument, GetOrderLineItems, GetOrderDeliveredQuantities } from "src/api";
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
import SerialCaptureModal from "src/components/stock/serial-capture-modal";
import StatusFlow from "src/components/status-flow";
import {
  DELIVERY_STATUSES,
  deliveryStatusColor,
  deliveryStatusLabel,
  deliveryStatusTransitionMatrix,
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
  // A component/intermediate isn't sellable — exclude it from the picker.
  // Unclassified products (category null) stay eligible everywhere.
  const sellableProducts = products.filter((p: any) => p.category !== "component");
  const setProducts = useSetAtom(setProductsAtom);
  const nextNumber = useAtomValue(nextDeliveryNumberAtom);

  const [deliveryId, setDeliveryId] = useAtom(deliveryIdAtom);
  const deliveryLoadable = useAtomValue(loadableDeliveryAtom);
  const setDelivery = useSetAtom(deliveryAtom);
  const delivery = deliveryLoadable.state === "hasData" ? deliveryLoadable.data : undefined;
  const updateStatus = useSetAtom(updateDeliveryStatusAtom);
  const deleteDelivery = useSetAtom(deleteDeliveryAtom);

  const [form] = Form.useForm();
  const [serialCapture, setSerialCapture] = useState<{
    open: boolean;
    pendingStatus: string | null;
  }>({
    open: false,
    pendingStatus: null,
  });
  const [isDirty, setIsDirty] = useState(false);
  const [downloadingPdf, setDownloadingPdf] = useState(false);
  const [downloadingExcel, setDownloadingExcel] = useState(false);

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

  // Populate form when delivery loads. The "clientId" form field always
  // represents this delivery's *own* directly-recorded client (ownClientId)
  // — never delivery.clientId, which is the order-derived effective value
  // and would otherwise leak into the field once an order is cleared.
  useEffect(() => {
    if (!isNew && delivery && typeof delivery === "object" && !("then" in delivery)) {
      form.resetFields();
      form.setFieldsValue({ ...delivery, clientId: (delivery as any).ownClientId });
    }
  }, [delivery, isNew, form]);

  const handleSubmit = async (values: any) => {
    await setDelivery(values);
    setIsDirty(false);
  };

  const handleDelete = async () => {
    if (!id || isNew) return;
    const success = await deleteDelivery(id);
    if (success) navigate("/deliveries");
  };

  // Serialized lines this delivery would ship, resolved from the persisted
  // line items (not form state) — an outbound delivery's productId can be
  // resolved server-side from a linked order line, so it isn't reliably
  // present client-side before that save round-trips.
  const serializedShipLines = (
    delivery && !(delivery as any).then ? (delivery as any).lineItems : []
  )
    .filter((l: any) => l.serialized && l.stockEnabled && l.productId)
    .map((l: any) => ({
      lineItemId: l.id,
      productId: l.productId,
      productName: l.description,
      quantity: l.quantity,
    }));

  const applyStatusChange = async (next: string, serialNumbers?: Record<string, string[]>) => {
    if (!id || isNew) return;
    const ok = await updateStatus({ deliveryId: id, status: next, serialNumbers });
    if (ok) {
      setDeliveryId(null);
      setTimeout(() => setDeliveryId(id), 0);
    }
  };

  const handleStatusChange = async (next: string) => {
    if (!id || isNew) return;
    if (next === "shipped" && serializedShipLines.length > 0) {
      setSerialCapture({ open: true, pendingStatus: next });
      return;
    }
    await applyStatusChange(next);
  };

  const handleSerialCaptureConfirm = async (serialNumbers: Record<string, string[]>) => {
    if (!serialCapture.pendingStatus) return;
    await applyStatusChange(serialCapture.pendingStatus, serialNumbers);
    setSerialCapture({ open: false, pendingStatus: null });
  };

  // Server fill-and-convert path (db/xlsx_export_delivery.go /
  // db/pdf_convert.go) — the same mechanism invoices/purchase orders/orders/
  // incoming invoices use. Reads persisted line items by delivery id, so
  // both formats are gated on isDirty. Replaces the old client-side-only
  // DeliveryNotePDF/@react-pdf/renderer button.
  const handleServerExport = (format: "pdf" | "xlsx") => async () => {
    if (!id || isNew) return;
    const setDownloading = format === "xlsx" ? setDownloadingExcel : setDownloadingPdf;
    setDownloading(true);
    try {
      await ExportDeliveryDocument(id, format);
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Export failed`);
    } finally {
      setDownloading(false);
    }
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
  const watchedOrderId = Form.useWatch("orderId", form);

  if (!organization) return null;
  if (!isNew && !delivery) return null;

  return (
    <Form
      form={form}
      onFinish={handleSubmit}
      layout="vertical"
      initialValues={initialValues}
      onValuesChange={() => setIsDirty(true)}
    >
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
        {!watchedOrderId && (
          <Col xs={24} md={12} xl={6}>
            <Form.Item label={<Trans>Client</Trans>} name="clientId">
              <Select
                allowClear
                showSearch
                optionFilterProp="children"
                placeholder={t`Walk-in / no client`}
              >
                {(clients as any[]).map((c: any) => (
                  <Option key={c.id} value={c.id}>
                    {c.name}
                  </Option>
                ))}
              </Select>
            </Form.Item>
          </Col>
        )}
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
            <StatusFlow
              current={currentStatus as DeliveryStatus}
              statuses={DELIVERY_STATUSES}
              transitions={deliveryStatusTransitionMatrix}
              getLabel={deliveryStatusLabel}
              getColor={(s) => deliveryStatusColor[s]}
            />
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
            products: sellableProducts,
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
                serialized: (product as any)?.serialized,
              };
              formInstance.setFieldValue("lineItems", [...lineItems]);
            },
          },
          { kind: "description", required: true },
          {
            kind: "quantity",
            width: 110,
            precision: (fieldName, formInstance) =>
              formInstance.getFieldValue(["lineItems", fieldName, "serialized"]) ? 0 : 2,
          },
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
                    // (db/xlsx_export_delivery.go) — every delivery has an
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
                  <Button type="primary" onClick={() => form.submit()}>
                    <SaveOutlined /> <Trans>Save</Trans>
                  </Button>
                </Space>
              </Col>
            </Row>
          </Footer>,
          document.getElementById("footer") as HTMLElement,
        )}
      <SerialCaptureModal
        open={serialCapture.open}
        mode="ship"
        lines={serializedShipLines}
        onCancel={() => setSerialCapture({ open: false, pendingStatus: null })}
        onConfirm={handleSerialCaptureConfirm}
      />
    </Form>
  );
};

export default DeliveryDetails;
