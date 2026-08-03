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

import { SaveFile, GetPurchaseOrderReceivedQuantities } from "src/api";
import { useDatePickerFormat } from "src/utils/date";
import { centsToUnits } from "src/utils/currency";
import ExchangeRateFields, {
  CurrencySelect,
  prefillExchangeRate,
  showExchangeRateFields,
} from "src/components/currency/currency-fields";
import LineItemsTable from "src/components/line-items/table";
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

// purchaseOrderAtom is async; reading it with plain useAtom throws to the
// app's single top-level Suspense boundary whenever purchaseOrderIdAtom
// changes after mount, which unmounts this whole route (the effect below's
// cleanup resets purchaseOrderIdAtom to null) and remounts it once the fetch
// resolves, setting the id back — an infinite loop. loadable() resolves
// synchronously instead of suspending, same fix as
// src/components/tax-rates/form.tsx and src/routes/invoices/details.tsx.
const loadableOrderAtom = loadable(purchaseOrderAtom);

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
  // Read the async atom directly so the component suspends until the real
  // number arrives. A non-suspending read would let the Form mount with a
  // placeholder, and antd applies initialValues only on first mount — freezing
  // the field at that placeholder and proposing an already-used number, which
  // is exactly what NextPurchaseOrderNumber's MAX-based query exists to avoid.
  const nextNumber = useAtomValue(nextPurchaseOrderNumberAtom);

  const [orderId, setOrderId] = useAtom(purchaseOrderIdAtom);
  const orderLoadable = useAtomValue(loadableOrderAtom);
  const setOrder = useSetAtom(purchaseOrderAtom);
  const order = orderLoadable.state === "hasData" ? orderLoadable.data : undefined;
  const updateStatus = useSetAtom(updatePurchaseOrderStatusAtom);
  const deleteOrder = useSetAtom(deletePurchaseOrderAtom);

  const [form] = Form.useForm();
  const [statusOverride, setStatusOverride] = useState<string | null>(null);
  const [receivedQuantities, setReceivedQuantities] = useState<Record<string, number>>({});

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

  // Per-line fulfilment, so partial receipts are visible without opening every
  // goods receipt for this order.
  useEffect(() => {
    if (isNew || !id) {
      setReceivedQuantities({});
      return;
    }
    GetPurchaseOrderReceivedQuantities(id)
      .then(setReceivedQuantities)
      .catch(() => setReceivedQuantities({}));
  }, [id, isNew]);

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

  const watchedCurrency = Form.useWatch("currency", form);
  const orgCurrency = organization?.currency ?? "EUR";
  const currency =
    watchedCurrency ??
    (!isNew && order && !(order as any).then ? (order as any).currency : null) ??
    orgCurrency;

  // Read the form store rather than onFinish's `values`.
  //
  // Under React.StrictMode (dev only) the mount effect's cleanup transiently
  // sets the id atom to null, `order` reads as null, the guard below unmounts
  // the Form, and it remounts with no registered fields — so onFinish hands
  // back an empty object even though every input is populated on screen, and
  // saving it would wipe the order's line items. Production builds don't
  // double-invoke effects and are unaffected, but dev is where this page gets
  // edited, so read the store, which is correct in both.
  //
  // Validation is unaffected: form.submit() still runs validateFields first and
  // only reaches this on success.
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
    // getFieldsValue(true) reads the whole store, for the same StrictMode
    // reason handleSubmit does — the no-argument form returns {} in dev, which
    // makes the PDF button silently do nothing.
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
    statusOverride ?? (!isNew && order && !(order as any).then ? (order as any).status : "draft");
  const transitions = isNew ? [] : purchaseOrderTransitions(currentStatus);

  if (!organization) return null;
  if (!isNew && !order) return null;

  return (
    <Form form={form} onFinish={handleSubmit} layout="vertical" initialValues={initialValues}>
      <Row gutter={24}>
        <Col xs={24} md={12} xl={7}>
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
              onChange={(vendorId) => {
                // Only cascade on a new order — see the identical guard on
                // src/routes/orders/details.tsx's clientId.
                if (!isNew) return;
                const vendor = find(vendors, { id: vendorId }) as any;
                if (vendor?.defaultCurrency) {
                  form.setFieldValue("currency", vendor.defaultCurrency);
                  prefillExchangeRate(form, organization?.id, vendor.defaultCurrency, orgCurrency);
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
          <Form.Item label={<Trans>Expected date</Trans>} name="expectedDate">
            <DatePicker style={{ width: "100%" }} format={dateFormat} />
          </Form.Item>
        </Col>
        <Col xs={24} md={12} xl={4}>
          <Form.Item label={<Trans>Status</Trans>}>
            <Tag color={purchaseOrderStatusColor[currentStatus as PurchaseOrderStatus]}>
              {purchaseOrderStatusLabel(currentStatus)}
            </Tag>
          </Form.Item>
        </Col>
        <CurrencySelect form={form} organizationId={organization?.id} orgCurrency={orgCurrency} />
      </Row>

      {showExchangeRateFields(watchedCurrency, orgCurrency) && (
        <Row gutter={24}>
          <ExchangeRateFields currency={watchedCurrency} orgCurrency={orgCurrency} />
        </Row>
      )}

      <Row gutter={24}>
        <Col xs={24} md={12}>
          <Form.Item label={<Trans>Delivery address</Trans>} name="deliveryAddress">
            <TextArea rows={2} placeholder={t`Leave blank to use organization address`} />
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
            products,
            required: true,
            onSelect: (productId, fieldName, formInstance) => {
              const product = find(products, { id: productId });
              if (product) {
                const items = formInstance.getFieldValue("lineItems");
                items[fieldName] = {
                  ...items[fieldName],
                  description: (product as any).name,
                  unit: (product as any).unit,
                  // Purchases are priced at cost, not at the sale price.
                  unitPrice: centsToUnits((product as any).unitCost ?? 0),
                };
                formInstance.setFieldValue("lineItems", [...items]);
              }
            },
          },
          { kind: "description", required: true },
          { kind: "quantity", width: 90 },
          { kind: "unit", width: 90 },
          { kind: "unitPrice", label: <Trans>Unit cost</Trans> },
          ...(!isNew
            ? [
                {
                  kind: "custom" as const,
                  key: "received",
                  title: <Trans>Received</Trans>,
                  width: 100,
                  render: (field: { name: number }) => (
                    <Form.Item shouldUpdate noStyle>
                      {() => {
                        const itemId = form.getFieldValue(["lineItems", field.name, "id"]);
                        const quantity =
                          form.getFieldValue(["lineItems", field.name, "quantity"]) ?? 0;
                        if (!itemId) return null;
                        const received = receivedQuantities[itemId] ?? 0;
                        return (
                          <Tag
                            color={
                              received >= quantity
                                ? "success"
                                : received > 0
                                  ? "processing"
                                  : "default"
                            }
                          >
                            {received} / {quantity}
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
                  {!isNew && !["draft", "cancelled"].includes(currentStatus) && (
                    <Button
                      onClick={() => navigate(`/inbound-deliveries/new?purchaseOrderId=${id}`)}
                    >
                      <PlusOutlined /> <Trans>New goods receipt</Trans>
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
