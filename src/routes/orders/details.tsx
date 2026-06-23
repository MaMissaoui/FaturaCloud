import { useEffect } from "react";
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
  InputNumber,
  Layout,
  Popconfirm,
  Row,
  Select,
  Space,
  Table,
  theme,
} from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import {
  DeleteOutlined,
  FilePdfOutlined,
  PlusOutlined,
  SaveOutlined,
  UserAddOutlined,
} from "@ant-design/icons";
import { pdf } from "@react-pdf/renderer";
import dayjs from "dayjs";
import find from "lodash/find";
import get from "lodash/get";
import includes from "lodash/includes";
import isString from "lodash/isString";
import lowerCase from "lodash/lowerCase";
import map from "lodash/map";
import sum from "lodash/sum";
import { SaveFile } from "src/api";
import { useDatePickerFormat } from "src/utils/date";
import { centsToUnits } from "src/utils/currency";
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
import OrderConfirmationPDF from "src/components/orders/order-confirmation-pdf";

const { TextArea } = Input;
const { Option } = Select;
const { Footer } = Layout;

const STATUS_TRANSITIONS: Record<string, { label: string; next: string; type?: "primary" | "default" | "dashed" }[]> = {
  draft: [{ label: "Confirm order", next: "confirmed", type: "primary" }],
  confirmed: [{ label: "Mark as shipped", next: "shipped", type: "primary" }],
  shipped: [{ label: "Mark as delivered", next: "delivered", type: "primary" }],
  delivered: [],
  cancelled: [],
};

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
  const setProducts = useSetAtom(setProductsAtom);
  const nextNumber = useAtomValue(nextOrderNumberAtom);

  const [orderId, setOrderId] = useAtom(orderIdAtom);
  const [order, setOrder] = useAtom(orderAtom);
  const updateStatus = useSetAtom(updateOrderStatusAtom);
  const deleteOrder = useSetAtom(deleteOrderAtom);

  const [form] = Form.useForm();

  useEffect(() => {
    setClients();
    setProducts();
    if (!isNew) {
      setOrderId(id ?? null);
    }
    return () => { setOrderId(null); };
  }, [id, isNew, setClients, setProducts, setOrderId]);

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
  };

  const handleDelete = async () => {
    if (!id || isNew) return;
    await deleteOrder(id);
    navigate("/orders");
  };

  const handleStatusChange = async (next: string) => {
    if (!id || isNew) return;
    await updateStatus({ orderId: id, status: next });
    // Reload order
    setOrderId(null);
    setTimeout(() => setOrderId(id), 0);
  };

  const handlePrintOrderConfirmation = async () => {
    const values = form.getFieldsValue();
    const clientData = find(clients, { id: values.clientId });
    if (!clientData) return;

    const orderData = {
      ...(!isNew && order && !(order as any).then ? order : {}),
      ...values,
      orderDate: values.orderDate?.valueOf ? values.orderDate.valueOf() : values.orderDate,
      deliveryDate: values.deliveryDate?.valueOf ? values.deliveryDate.valueOf() : values.deliveryDate,
    };

    const lineItemsForPdf = (values.lineItems ?? []).map((item: any) => ({
      ...item,
      unitPrice: Math.round((item.unitPrice ?? 0) * 100),
    }));

    const doc = (
      <OrderConfirmationPDF
        order={orderData}
        lineItems={lineItemsForPdf}
        client={clientData}
        organization={organization}
        locale={i18n.locale}
      />
    );

    const blob = await pdf(doc).toBlob();
    const orderNum = values.orderNumber ?? (id ?? "order");
    await SaveFile(`order-confirmation-${orderNum}.pdf`, blob);
  };

  const initialValues = isNew
    ? {
        orderNumber: nextNumber,
        orderDate: dayjs(),
        status: "draft",
        lineItems: [{ quantity: 1 }],
      }
    : undefined;

  const currentStatus = !isNew && order && !(order as any).then
    ? (order as any).status
    : "draft";

  const transitions = STATUS_TRANSITIONS[currentStatus] ?? [];

  if (!organization) return null;
  if (!isNew && !order) return null;

  return (
    <Form
      form={form}
      onFinish={handleSubmit}
      layout="vertical"
      initialValues={initialValues}
    >
      <Row gutter={24}>
        <Col span={8}>
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
              popupRender={(menu) => (
                <>
                  {menu}
                  <Divider style={{ margin: "8px 0" }} />
                  <Button
                    type="text"
                    block
                    icon={<UserAddOutlined />}
                    onClick={(e) => { e.preventDefault(); navigate("/clients"); }}
                    style={{ textAlign: "left", paddingLeft: 11 }}
                  >
                    <Trans>Manage clients</Trans>
                  </Button>
                </>
              )}
            >
              {map(clients, (c: any) => (
                <Option key={c.id} value={c.id}>{c.name}</Option>
              ))}
            </Select>
          </Form.Item>
        </Col>
        <Col span={4}>
          <Form.Item
            label={<Trans>Order number</Trans>}
            name="orderNumber"
            rules={[{ required: true, message: t`Order number is required` }]}
          >
            <Input />
          </Form.Item>
        </Col>
        <Col span={4}>
          <Form.Item
            label={<Trans>Order date</Trans>}
            name="orderDate"
            rules={[{ required: true, message: t`Order date is required` }]}
          >
            <DatePicker style={{ width: "100%" }} format={dateFormat} />
          </Form.Item>
        </Col>
        <Col span={4}>
          <Form.Item label={<Trans>Expected delivery</Trans>} name="deliveryDate">
            <DatePicker style={{ width: "100%" }} format={dateFormat} />
          </Form.Item>
        </Col>
        <Col span={4}>
          <Form.Item label={<Trans>Tracking number</Trans>} name="trackingNumber">
            <Input placeholder="e.g. FX1234567890" />
          </Form.Item>
        </Col>
      </Row>

      <Row gutter={24}>
        <Col span={12}>
          <Form.Item label={<Trans>Shipping address</Trans>} name="shippingAddress">
            <TextArea rows={2} placeholder={t`Leave blank to use client address`} />
          </Form.Item>
        </Col>
        <Col span={12}>
          <Form.Item label={<Trans>Notes</Trans>} name="notes">
            <TextArea rows={2} />
          </Form.Item>
        </Col>
      </Row>

      {/* Line items */}
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
                  <Form.Item name={[field.name, "productId"]} noStyle>
                    <Select
                      allowClear
                      showSearch
                      style={{ width: "100%" }}
                      placeholder={t`Optional`}
                      optionFilterProp="children"
                      onChange={(productId) => {
                        const product = find(products, { id: productId });
                        if (product) {
                          const lineItems = form.getFieldValue("lineItems");
                          lineItems[field.name] = {
                            ...lineItems[field.name],
                            description: (product as any).name,
                            unitPrice: centsToUnits((product as any).price ?? 0),
                          };
                          form.setFieldValue("lineItems", [...lineItems]);
                        }
                      }}
                    >
                      {map(products, (p: any) => (
                        <Option key={p.id} value={p.id}>{p.name}</Option>
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
                key="remove"
                width={40}
                render={(field) => (
                  <DeleteOutlined
                    style={{ color: "#ff4d4f", cursor: "pointer" }}
                    onClick={() => remove(field.name)}
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
                  currency: organization.currency ?? "EUR",
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
                {!isNew && (
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
                  {!isNew && transitions.map((t2) => (
                    <Button
                      key={t2.next}
                      type={t2.type ?? "default"}
                      onClick={() => handleStatusChange(t2.next)}
                    >
                      {t2.label}
                    </Button>
                  ))}
                  {!isNew && currentStatus !== "cancelled" && (
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
                    <Button onClick={handlePrintOrderConfirmation}>
                      <FilePdfOutlined /> <Trans>Order confirmation</Trans>
                    </Button>
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
  );
};

export default OrderDetails;
