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
import { loadable } from "src/utils/loadable";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { DeleteOutlined, PlusOutlined, SaveOutlined, UserAddOutlined } from "@ant-design/icons";
import dayjs from "dayjs";
import find from "lodash/find";
import get from "lodash/get";
import includes from "lodash/includes";
import isString from "lodash/isString";
import lowerCase from "lodash/lowerCase";
import map from "lodash/map";

import { GetPurchaseOrderLineItems, GetPurchaseOrderReceivedQuantities } from "src/api";
import { useDatePickerFormat } from "src/utils/date";
import { centsToUnits } from "src/utils/currency";
import { requiredForNewLineItem } from "src/utils/line-items";
import {
  inboundDeliveryStatusColor,
  inboundDeliveryStatusLabel,
  inboundDeliveryTransitions,
  type InboundDeliveryStatus,
} from "src/types/inbound-delivery";
import { organizationAtom } from "src/atoms/organization";
import { productsAtom, setProductsAtom } from "src/atoms/product";
import { vendorsAtom, setVendorsAtom } from "src/atoms/vendor";
import { purchaseOrdersAtom, setPurchaseOrdersAtom } from "src/atoms/purchase-order";
import {
  inboundDeliveryIdAtom,
  inboundDeliveryAtom,
  nextInboundDeliveryNumberAtom,
  updateInboundDeliveryStatusAtom,
  deleteInboundDeliveryAtom,
} from "src/atoms/inbound-delivery";

const { TextArea } = Input;
const { Option } = Select;
const { Footer } = Layout;

// inboundDeliveryAtom is async; reading it with plain useAtom throws to the
// app's single top-level Suspense boundary whenever inboundDeliveryIdAtom
// changes after mount, which unmounts this whole route (the effect below's
// cleanup resets inboundDeliveryIdAtom to null) and remounts it once the
// fetch resolves, setting the id back — an infinite loop. loadable()
// resolves synchronously instead of suspending, same fix as
// src/components/tax-rates/form.tsx and src/routes/invoices/details.tsx.
const loadableDeliveryAtom = loadable(inboundDeliveryAtom);

const InboundDeliveryDetails = () => {
  const { id } = useParams<string>();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const {
    token: { colorBgContainer },
  } = theme.useToken();
  const dateFormat = useDatePickerFormat();

  const isNew = id === "new";
  const prefillOrderId = searchParams.get("purchaseOrderId");

  const organization = useAtomValue(organizationAtom);
  const vendors = useAtomValue(vendorsAtom);
  const setVendors = useSetAtom(setVendorsAtom);
  const products = useAtomValue(productsAtom);
  const setProducts = useSetAtom(setProductsAtom);
  const purchaseOrders = useAtomValue(purchaseOrdersAtom);
  const setPurchaseOrders = useSetAtom(setPurchaseOrdersAtom);
  const nextNumber = useAtomValue(nextInboundDeliveryNumberAtom);

  const [deliveryId, setDeliveryId] = useAtom(inboundDeliveryIdAtom);
  const deliveryLoadable = useAtomValue(loadableDeliveryAtom);
  const setDelivery = useSetAtom(inboundDeliveryAtom);
  const delivery = deliveryLoadable.state === "hasData" ? deliveryLoadable.data : undefined;
  const updateStatus = useSetAtom(updateInboundDeliveryStatusAtom);
  const deleteDelivery = useSetAtom(deleteInboundDeliveryAtom);

  const [form] = Form.useForm();
  const [statusOverride, setStatusOverride] = useState<string | null>(null);

  useEffect(() => {
    setVendors();
    setProducts();
    setPurchaseOrders();
    setStatusOverride(null);
    if (!isNew) {
      setDeliveryId(id ?? null);
    }
    return () => {
      setDeliveryId(null);
    };
  }, [id, isNew, setVendors, setProducts, setPurchaseOrders, setDeliveryId]);

  // Prefill from a purchase order with only the quantity still outstanding on
  // each line, so a partial receipt is the default rather than a correction.
  useEffect(() => {
    if (!isNew || !prefillOrderId) return;
    let cancelled = false;

    (async () => {
      try {
        const [lineItems, received] = await Promise.all([
          GetPurchaseOrderLineItems(prefillOrderId),
          GetPurchaseOrderReceivedQuantities(prefillOrderId),
        ]);
        if (cancelled) return;

        const outstanding = (lineItems || [])
          .map((item: any) => ({
            purchaseOrderLineItemId: item.id,
            productId: item.productId,
            description: item.description,
            quantity: item.quantity - (received[item.id] ?? 0),
            unit: item.unit,
            unitCost: centsToUnits(item.unitPrice ?? 0),
          }))
          .filter((item: any) => item.quantity > 0);

        const order: any = find(purchaseOrders, { id: prefillOrderId });
        form.setFieldsValue({
          purchaseOrderId: prefillOrderId,
          vendorId: order?.vendorId,
          lineItems: outstanding.length > 0 ? outstanding : [{ quantity: 1 }],
        });
      } catch {
        // Leave the blank line the form starts with.
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [isNew, prefillOrderId, purchaseOrders, form]);

  // After create, navigate to the new receipt.
  useEffect(() => {
    if (isNew && deliveryId) {
      navigate(`/inbound-deliveries/${deliveryId}`);
    }
  }, [isNew, deliveryId, navigate]);

  useEffect(() => {
    if (!isNew && delivery && typeof delivery === "object" && !("then" in delivery)) {
      form.resetFields();
      form.setFieldsValue(delivery);
    }
  }, [delivery, isNew, form]);

  // Read the whole form store rather than onFinish's values — see the same
  // note on the purchase order page: under StrictMode the Form can remount
  // with no registered fields, and onFinish then reports nothing.
  const handleSubmit = async () => {
    await setDelivery(form.getFieldsValue(true));
  };

  const handleDelete = async () => {
    if (!id || isNew) return;
    const success = await deleteDelivery(id);
    if (success) navigate("/inbound-deliveries");
  };

  const handleStatusChange = async (next: string) => {
    if (!id || isNew) return;
    const ok = await updateStatus({ deliveryId: id, status: next });
    // Only reflect the new status if the server accepted it — cancelling a
    // receipt whose goods are gone is rejected.
    if (ok) setStatusOverride(next);
  };

  const initialValues = isNew
    ? {
        deliveryNumber: nextNumber,
        deliveryDate: dayjs(),
        status: "draft",
        purchaseOrderId: prefillOrderId ?? undefined,
        lineItems: [{ quantity: 1 }],
      }
    : undefined;

  const currentStatus =
    statusOverride ??
    (!isNew && delivery && !(delivery as any).then ? (delivery as any).status : "draft");
  const transitions = isNew ? [] : inboundDeliveryTransitions(currentStatus);
  const isEditable = isNew || currentStatus === "draft";

  if (!organization) return null;
  if (!isNew && !delivery) return null;

  return (
    <Form form={form} onFinish={handleSubmit} layout="vertical" initialValues={initialValues}>
      <Row gutter={24}>
        <Col span={6}>
          <Form.Item
            label={<Trans>Vendor</Trans>}
            name="vendorId"
            rules={[{ required: true, message: t`Vendor is required` }]}
          >
            <Select
              showSearch
              allowClear
              optionFilterProp="children"
              filterOption={(input, option) => {
                const name = get(option, ["props", "children"]);
                return isString(name) ? includes(lowerCase(name), lowerCase(input)) : true;
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
                      navigate("/vendors");
                    }}
                    style={{ textAlign: "left", paddingLeft: 11 }}
                  >
                    <Trans>Manage vendors</Trans>
                  </Button>
                </>
              )}
            >
              {map(vendors, (v: any) => (
                <Option key={v.id} value={v.id}>
                  {v.name}
                </Option>
              ))}
            </Select>
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
            label={<Trans>Receipt number</Trans>}
            name="deliveryNumber"
            rules={[{ required: true, message: t`Receipt number is required` }]}
          >
            <Input />
          </Form.Item>
        </Col>
        <Col span={4}>
          <Form.Item
            label={<Trans>Receipt date</Trans>}
            name="deliveryDate"
            rules={[{ required: true, message: t`Receipt date is required` }]}
          >
            <DatePicker style={{ width: "100%" }} format={dateFormat} />
          </Form.Item>
        </Col>
        <Col span={5}>
          <Form.Item label={<Trans>Status</Trans>}>
            <Tag color={inboundDeliveryStatusColor[currentStatus as InboundDeliveryStatus]}>
              {inboundDeliveryStatusLabel(currentStatus)}
            </Tag>
          </Form.Item>
        </Col>
      </Row>

      <Row gutter={24}>
        <Col span={8}>
          <Form.Item label={<Trans>Vendor delivery note</Trans>} name="vendorDeliveryNote">
            <Input placeholder={t`Number on the vendor's paperwork`} />
          </Form.Item>
        </Col>
        <Col span={8}>
          <Form.Item label={<Trans>Tracking number</Trans>} name="trackingNumber">
            <Input />
          </Form.Item>
        </Col>
        <Col span={8}>
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
                title={<Trans>Product</Trans>}
                key="productId"
                width={180}
                render={(field) => (
                  <Form.Item
                    name={[field.name, "productId"]}
                    rules={[requiredForNewLineItem(form, field.name, t`This field is required!`)]}
                    noStyle
                  >
                    <Select
                      showSearch
                      disabled={!isEditable}
                      style={{ width: "100%" }}
                      placeholder={t`Select product`}
                      optionFilterProp="children"
                      onChange={(productId) => {
                        const product = find(products, { id: productId });
                        if (product) {
                          const items = form.getFieldValue("lineItems");
                          items[field.name] = {
                            ...items[field.name],
                            description: (product as any).name,
                            unit: (product as any).unit,
                            unitCost: centsToUnits((product as any).unitCost ?? 0),
                          };
                          form.setFieldValue("lineItems", [...items]);
                        }
                      }}
                    >
                      {map(products, (p: any) => (
                        <Option key={p.id} value={p.id}>
                          {p.name}
                          {p.sku ? ` (${p.sku})` : ""}
                        </Option>
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
                    <TextArea rows={1} autoSize disabled={!isEditable} />
                  </Form.Item>
                )}
              />
              <Table.Column
                title={<Trans>Qty received</Trans>}
                key="quantity"
                width={110}
                render={(field) => (
                  <Form.Item
                    name={[field.name, "quantity"]}
                    noStyle
                    rules={[{ required: true, message: t`Required` }]}
                  >
                    <InputNumber
                      style={{ width: "100%" }}
                      min={0}
                      precision={2}
                      disabled={!isEditable}
                    />
                  </Form.Item>
                )}
              />
              <Table.Column
                title={<Trans>Unit</Trans>}
                key="unit"
                width={80}
                render={(field) => (
                  <Form.Item name={[field.name, "unit"]} noStyle>
                    <Input disabled={!isEditable} />
                  </Form.Item>
                )}
              />
              <Table.Column
                title={<Trans>Unit cost</Trans>}
                key="unitCost"
                width={110}
                render={(field) => (
                  <Form.Item name={[field.name, "unitCost"]} noStyle>
                    <InputNumber
                      style={{ width: "100%" }}
                      min={0}
                      precision={2}
                      step={0.01}
                      disabled={!isEditable}
                    />
                  </Form.Item>
                )}
              />
              <Table.Column
                title={<Trans>In stock</Trans>}
                key="currentStock"
                width={90}
                render={(field) => (
                  <Form.Item shouldUpdate noStyle>
                    {() => {
                      const productId = form.getFieldValue(["lineItems", field.name, "productId"]);
                      if (!productId) return null;
                      const product: any = find(products, { id: productId });
                      if (!product || !product.stockEnabled) return null;
                      return <Tag>{product.stockQuantity}</Tag>;
                    }}
                  </Form.Item>
                )}
              />
              {isEditable && (
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
              )}
            </Table>

            {isEditable && (
              <Button
                type="default"
                size="small"
                icon={<PlusOutlined />}
                onClick={() => add({ quantity: 1 })}
                style={{ marginTop: 12 }}
              >
                <Trans>Add line item</Trans>
              </Button>
            )}
          </>
        )}
      </Form.List>

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
                {!isNew && currentStatus !== "received" && (
                  <Popconfirm
                    title={t`Delete this goods receipt?`}
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
                  {transitions.map((transition) => (
                    <Popconfirm
                      key={transition.next}
                      title={t`This will add the received quantities to stock. Continue?`}
                      onConfirm={() => handleStatusChange(transition.next)}
                      okText={t`Yes`}
                      cancelText={t`No`}
                    >
                      <Button type={transition.type ?? "default"}>{transition.label}</Button>
                    </Popconfirm>
                  ))}
                  {!isNew && currentStatus !== "cancelled" && (
                    <Popconfirm
                      title={
                        currentStatus === "received"
                          ? t`This will reverse the stock this receipt added. Continue?`
                          : t`Cancel this goods receipt?`
                      }
                      onConfirm={() => handleStatusChange("cancelled")}
                      okText={t`Yes`}
                      cancelText={t`No`}
                    >
                      <Button type="dashed" danger>
                        <Trans>Cancel receipt</Trans>
                      </Button>
                    </Popconfirm>
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

export default InboundDeliveryDetails;
