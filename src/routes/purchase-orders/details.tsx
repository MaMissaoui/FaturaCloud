import { useEffect, useState } from "react";
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
  Tag,
  theme,
} from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import { loadable } from "jotai/utils";
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
import {
  purchaseOrderStatusColor,
  purchaseOrderStatusLabel,
  purchaseOrderTransitions,
  type PurchaseOrderStatus,
} from "src/types/purchase-order";
import { organizationAtom } from "src/atoms/organization";
import { productsAtom, setProductsAtom } from "src/atoms/product";
import { vendorsAtom, setVendorsAtom } from "src/atoms/vendor";
import {
  purchaseOrderIdAtom,
  purchaseOrderAtom,
  nextPurchaseOrderNumberAtom,
  updatePurchaseOrderStatusAtom,
  deletePurchaseOrderAtom,
} from "src/atoms/purchase-order";
import PurchaseOrderPDF from "src/components/purchase-orders/purchase-order-pdf";

const { TextArea } = Input;
const { Option } = Select;
const { Footer } = Layout;

// Wrapped in `loadable` so reading it never suspends. A bare async atom read
// here suspends the whole page a second time (after the order itself has
// resolved); when that late suspension unwinds, the Form remounts empty and the
// populate effect does not re-run — the order's line items and dates silently
// vanish on a direct page load. Only the number is needed, and only when
// creating, so a non-suspending read is both safer and sufficient.
const loadableNextNumberAtom = loadable(nextPurchaseOrderNumberAtom);

const PurchaseOrderDetails = () => {
  const { id } = useParams<string>();
  const navigate = useNavigate();
  const { i18n } = useLingui();
  const {
    token: { colorBgContainer },
  } = theme.useToken();
  const dateFormat = useDatePickerFormat();

  const isNew = id === "new";

  const organization = useAtomValue(organizationAtom);
  const vendors = useAtomValue(vendorsAtom);
  const setVendors = useSetAtom(setVendorsAtom);
  const products = useAtomValue(productsAtom);
  const setProducts = useSetAtom(setProductsAtom);
  const nextNumberState = useAtomValue(loadableNextNumberAtom);
  const nextNumber = nextNumberState.state === "hasData" ? nextNumberState.data : "PO-0001";

  const [orderId, setOrderId] = useAtom(purchaseOrderIdAtom);
  const [order, setOrder] = useAtom(purchaseOrderAtom);
  const updateStatus = useSetAtom(updatePurchaseOrderStatusAtom);
  const deleteOrder = useSetAtom(deletePurchaseOrderAtom);

  const [form] = Form.useForm();
  const [statusOverride, setStatusOverride] = useState<string | null>(null);

  useEffect(() => {
    setVendors();
    setProducts();
    setStatusOverride(null);
    if (!isNew) {
      setOrderId(id ?? null);
    }
    return () => {
      setOrderId(null);
    };
  }, [id, isNew, setVendors, setProducts, setOrderId]);

  // After create, navigate to the new purchase order
  useEffect(() => {
    if (isNew && orderId) {
      navigate(`/purchase-orders/${orderId}`);
    }
  }, [isNew, orderId, navigate]);

  // Populate form when the order loads. The `"then" in order` guard is because
  // the async read atom's value can transiently be a promise.
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

  const currency =
    (!isNew && order && !(order as any).then ? (order as any).currency : null) ??
    organization?.currency ??
    "EUR";

  // onFinish's `values` cannot be trusted on this page: antd only reports
  // fields it considers registered, and on an existing record it hands back an
  // empty object even though every input is populated on screen. Submitting
  // that would persist emptiness and wipe the order's line items. Read the
  // whole form store instead — the same reason handlePrintPurchaseOrder uses
  // getFieldsValue(true).
  const handleSubmit = async () => {
    await setOrder(form.getFieldsValue(true));
  };

  const handleDelete = async () => {
    if (!id || isNew) return;
    const success = await deleteOrder(id);
    if (success) navigate("/purchase-orders");
  };

  // A status change is the only thing that moves, and the PATCH response
  // already confirms it — so record it locally rather than forcing the async
  // read atom to re-run.
  //
  // The obvious idiom (setOrderId(null); setTimeout(() => setOrderId(id), 0))
  // is actively destructive here: while the id is null the atom yields null,
  // the `!order` guard below returns null, and the whole Form unmounts. It
  // remounts empty, and the next Save then persists that emptiness — silently
  // wiping the order's line items and dates.
  const handleStatusChange = async (next: string) => {
    if (!id || isNew) return;
    const ok = await updateStatus({ orderId: id, status: next });
    if (ok) setStatusOverride(next);
  };

  const handlePrintPurchaseOrder = async () => {
    // getFieldsValue(true) returns the whole form store. The no-argument form
    // returns {} here — the sales-side PDF handlers use it and silently do
    // nothing as a result — so the `true` is load-bearing, not incidental.
    const values = form.getFieldsValue(true);
    const vendorData = find(vendors, { id: values.vendorId });
    if (!vendorData) return;

    const orderData = {
      ...(!isNew && order && !(order as any).then ? order : {}),
      ...values,
      orderDate: values.orderDate?.valueOf ? values.orderDate.valueOf() : values.orderDate,
      expectedDate: values.expectedDate?.valueOf
        ? values.expectedDate.valueOf()
        : values.expectedDate,
      currency,
    };

    // The PDF's currency formatter takes cents, so pre-multiply here.
    const lineItemsForPdf = (values.lineItems ?? []).map((item: any) => ({
      ...item,
      unitPrice: Math.round((item.unitPrice ?? 0) * 100),
      sku: item.productId ? (find(products, { id: item.productId }) as any)?.sku : undefined,
    }));

    const doc = (
      <PurchaseOrderPDF
        order={orderData}
        lineItems={lineItemsForPdf}
        vendor={vendorData}
        organization={organization}
        i18n={i18n}
      />
    );

    const blob = await pdf(doc).toBlob();
    const orderNum = values.orderNumber ?? id ?? "purchase-order";
    await SaveFile(`purchase-order-${orderNum}.pdf`, blob);
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

  const currentStatus =
    statusOverride ??
    (!isNew && order && !(order as any).then ? (order as any).status : "draft");
  const transitions = isNew ? [] : purchaseOrderTransitions(currentStatus);

  if (!organization) return null;
  if (!isNew && !order) return null;

  return (
    <Form form={form} onFinish={handleSubmit} layout="vertical" initialValues={initialValues}>
      <Row gutter={24}>
        <Col span={8}>
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
          <Form.Item label={<Trans>Expected date</Trans>} name="expectedDate">
            <DatePicker style={{ width: "100%" }} format={dateFormat} />
          </Form.Item>
        </Col>
        <Col span={4}>
          <Form.Item label={<Trans>Status</Trans>}>
            <Tag color={purchaseOrderStatusColor[currentStatus as PurchaseOrderStatus]}>
              {purchaseOrderStatusLabel(currentStatus)}
            </Tag>
          </Form.Item>
        </Col>
      </Row>

      <Row gutter={24}>
        <Col span={12}>
          <Form.Item label={<Trans>Delivery address</Trans>} name="deliveryAddress">
            <TextArea rows={2} placeholder={t`Leave blank to use organization address`} />
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
                          const items = form.getFieldValue("lineItems");
                          items[field.name] = {
                            ...items[field.name],
                            description: (product as any).name,
                            unit: (product as any).unit,
                            // Purchases are priced at cost, not at the sale price.
                            unitPrice: centsToUnits((product as any).unitCost ?? 0),
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
                title={<Trans>Unit</Trans>}
                key="unit"
                width={90}
                render={(field) => (
                  <Form.Item name={[field.name, "unit"]} noStyle>
                    <Input />
                  </Form.Item>
                )}
              />
              <Table.Column
                title={<Trans>Unit cost</Trans>}
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
                }).format(subTotal)}
              </Descriptions.Item>
            </Descriptions>
          </Col>
        </Row>
      )}

      {/* Footer bar — portaled into the slot BaseLayout renders */}
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
                    title={t`Delete this purchase order?`}
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
                    <Button
                      key={transition.next}
                      type={transition.type ?? "default"}
                      onClick={() => handleStatusChange(transition.next)}
                    >
                      {transition.label}
                    </Button>
                  ))}
                  {!isNew && !["cancelled", "received"].includes(currentStatus) && (
                    <Popconfirm
                      title={t`Cancel this purchase order?`}
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
                    <Button onClick={handlePrintPurchaseOrder}>
                      <FilePdfOutlined /> <Trans>Purchase order PDF</Trans>
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

export default PurchaseOrderDetails;
