import { useEffect, useState, useMemo } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router";
import {
  Alert,
  Button,
  Card,
  Col,
  DatePicker,
  Descriptions,
  Divider,
  Form,
  Input,
  message,
  Popconfirm,
  Row,
  Select,
  Skeleton,
  Space,
  Table,
  Tag,
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
  ShoppingCartOutlined,
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

import { ExportPurchaseOrderDocument, GetPurchaseOrderReceivedQuantities } from "src/api";
import PageHeader from "src/components/page-header";
import ResponsiveFooter from "src/components/responsive-footer";
import useSaveShortcut from "src/hooks/useSaveShortcut";
import useUnsavedChangesWarning from "src/hooks/useUnsavedChangesWarning";
import { useDatePickerFormat, useDateFormatter } from "src/utils/date";
import { centsToUnits } from "src/utils/currency";
import { formatMoneyUnits, numberFormatLocale } from "src/utils/currencies";
import ExchangeRateFields, {
  CurrencySelect,
  prefillExchangeRate,
  showExchangeRateFields,
} from "src/components/currency/currency-fields";
import LineItemsTable from "src/components/line-items/table";
import StatusFlow from "src/components/status-flow";
import {
  PURCHASE_ORDER_STATUSES,
  purchaseOrderStatusColor,
  purchaseOrderStatusLabel,
  purchaseOrderStatusTransitionMatrix,
  purchaseOrderTransitions,
  type PurchaseOrderStatus,
} from "src/types/purchase-order";
import {
  inboundDeliveryStatusColor,
  inboundDeliveryStatusLabel,
  type InboundDeliveryStatus,
} from "src/types/inbound-delivery";
import {
  incomingInvoiceStateColor,
  incomingInvoiceStateLabel,
  type IncomingInvoiceState,
} from "src/types/incoming-invoice";
import { organizationAtom } from "src/atoms/organization";
import { productsAtom, setProductsAtom } from "src/atoms/product";
import { vendorsAtom, setVendorsAtom } from "src/atoms/vendor";
import { importsAtom, setImportsAtom } from "src/atoms/import";
import { inboundDeliveriesAtom, setInboundDeliveriesAtom } from "src/atoms/inbound-delivery";
import { incomingInvoicesAtom, setIncomingInvoicesAtom } from "src/atoms/incoming-invoice";
import {
  purchaseOrderIdAtom,
  purchaseOrderAtom,
  nextPurchaseOrderNumberAtom,
  updatePurchaseOrderStatusAtom,
  deletePurchaseOrderAtom,
} from "src/atoms/purchase-order";

const { TextArea } = Input;
const { Option } = Select;

// Module-level so it's referentially stable across renders — StatusFlow is
// memoized and an inline arrow here would defeat that on every keystroke.
const getPurchaseOrderStatusColor = (s: PurchaseOrderStatus) => purchaseOrderStatusColor[s];

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
  const location = useLocation();
  const navigate = useNavigate();
  const { i18n } = useLingui();
  const dateFormat = useDatePickerFormat();
  const formatDate = useDateFormatter();

  const isNew = id === "new";
  // Set when navigating here from the Imports drawer's "New purchase order"
  // action (src/components/imports/form.tsx) — pre-links the order the same
  // way picking it from the Import Select below would.
  const prefillImportId = isNew ? ((location.state as any)?.importId ?? null) : null;

  const organization = useAtomValue(organizationAtom);
  const vendors = useAtomValue(vendorsAtom);
  const setVendors = useSetAtom(setVendorsAtom);
  const imports = useAtomValue(importsAtom);
  const setImports = useSetAtom(setImportsAtom);
  const products = useAtomValue(productsAtom);
  // A finished good isn't purchasable from a vendor — exclude it from the
  // picker. Unclassified products (category null) stay eligible everywhere.
  const purchasableProducts = useMemo(
    () => products.filter((p: any) => p.category !== "finished"),
    [products],
  );
  const setProducts = useSetAtom(setProductsAtom);
  // Client-side filter of the already-fetched org-wide lists — same
  // "no extra request" pattern as orders/details.tsx's linkedDeliveries —
  // rather than a dedicated by-purchase-order endpoint.
  const inboundDeliveries = useAtomValue(inboundDeliveriesAtom);
  const setInboundDeliveries = useSetAtom(setInboundDeliveriesAtom);
  const linkedReceipts = useMemo(
    () => inboundDeliveries.filter((rc: any) => rc.purchaseOrderId === id),
    [inboundDeliveries, id],
  );
  const incomingInvoices = useAtomValue(incomingInvoicesAtom);
  const setIncomingInvoices = useSetAtom(setIncomingInvoicesAtom);
  const linkedIncomingInvoices = useMemo(
    () => incomingInvoices.filter((inv: any) => inv.purchaseOrderId === id),
    [incomingInvoices, id],
  );
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
  const [isDirty, setIsDirty] = useState(false);
  const [downloadingPdf, setDownloadingPdf] = useState(false);
  const [downloadingExcel, setDownloadingExcel] = useState(false);

  useSaveShortcut(form);
  useUnsavedChangesWarning(isDirty);

  useEffect(() => {
    setVendors();
    setProducts();
    setImports();
    setInboundDeliveries();
    setIncomingInvoices();
    setStatusOverride(null);
    if (!isNew) {
      setOrderId(id ?? null);
    }
    return () => {
      setOrderId(null);
    };
  }, [
    id,
    isNew,
    setVendors,
    setProducts,
    setImports,
    setInboundDeliveries,
    setIncomingInvoices,
    setOrderId,
  ]);

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

  // Mirrors the server-side guard in db/purchase_order_freeze.go: once goods
  // have actually been received against this order, its line items are the
  // basis of a posted GRNI accrual and of 3-way matching, so they're frozen.
  // Header fields stay editable.
  //
  // This is a deliberately conservative approximation of the server's rule.
  // GetPurchaseOrderReceivedQuantities only counts receipts linked to this
  // order by header *and* carrying per-line links, while the server also
  // freezes on a line-level link alone — so this can under-freeze, never
  // over-freeze, and the server is the authority either way (it answers a
  // changed payload with a 409 naming the receipt).
  const lineItemsFrozen = !isNew && Object.keys(receivedQuantities).length > 0;

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
      // A domestic (non-import-linked) purchase order's currency is null at
      // the DB layer — the normal case, not bad data (see cmd/seed-demo's
      // createPurchaseOrder vs createImportLinkedPurchaseOrder). Left as
      // null here, it renders the Currency Select completely blank instead
      // of falling back to the organization's own currency the way this
      // page's other currency fallbacks already do.
      form.setFieldsValue({
        ...order,
        currency: (order as any).currency ?? organization?.currency ?? "EUR",
      });
    }
  }, [order, isNew, form, organization]);

  // Same cascade as the Import Select's onChange below, run once for a
  // pre-linked new order — `imports` may still be loading on first render
  // (fetched by the effect above), so this can't rely on initialValues alone.
  useEffect(() => {
    if (!isNew || !prefillImportId) return;
    const imp = find(imports, { id: prefillImportId }) as any;
    if (imp?.currency) {
      form.setFieldsValue({
        currency: imp.currency,
        exchangeRate: imp.exchangeRate ?? undefined,
        exchangeRateDate: imp.exchangeRateDate ? dayjs(imp.exchangeRateDate) : undefined,
      });
    }
  }, [isNew, prefillImportId, imports, form]);

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
    setIsDirty(false);
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

  // Server fill-and-convert path (db/xlsx_export_purchase_order.go /
  // db/pdf_convert.go) — the same mechanism invoices use, so a PDF and an
  // Excel export of the same purchase order are always the same document.
  // Reads persisted line items by order id, so both are gated on isDirty.
  const handleServerExport = (format: "pdf" | "xlsx") => async () => {
    if (!id) return;
    const setDownloading = format === "xlsx" ? setDownloadingExcel : setDownloadingPdf;
    setDownloading(true);
    try {
      await ExportPurchaseOrderDocument(id, format);
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
        importId: prefillImportId ?? undefined,
        lineItems: [{ quantity: 1 }],
      }
    : undefined;

  const currentStatus =
    statusOverride ?? (!isNew && order && !(order as any).then ? (order as any).status : "draft");
  const orderNumber =
    !isNew && order && !(order as any).then ? (order as any).orderNumber : undefined;
  const transitions = isNew ? [] : purchaseOrderTransitions(currentStatus);

  // Re-runs the async detail read after a failed fetch. The null-then-restore
  // idiom is the one that actually invalidates the atom's cached read (jotai
  // skips the notification when the id is unchanged). handleStatusChange
  // above warns this idiom is destructive for a *loaded* order because the
  // `!order` guard unmounts the Form mid-edit — but this only runs from that
  // very `!order` branch, where there is no loaded order (and no form state)
  // to lose.
  const retryLoad = () => {
    if (!id) return;
    setOrderId(null);
    setTimeout(() => setOrderId(id), 0);
  };

  if (!organization) return null;
  if (!isNew && !order) {
    return (
      <>
        <PageHeader
          icon={<ShoppingCartOutlined />}
          title={<Trans>Purchase Order</Trans>}
          style={{ marginBottom: 24 }}
        />
        {orderLoadable.state === "loading" ? (
          <Skeleton active paragraph={{ rows: 12 }} />
        ) : (
          <Alert
            type="error"
            showIcon
            message={<Trans>Couldn't load this purchase order</Trans>}
            action={
              <Button size="small" onClick={retryLoad}>
                <Trans>Retry</Trans>
              </Button>
            }
          />
        )}
      </>
    );
  }

  return (
    <>
      <PageHeader
        icon={<ShoppingCartOutlined />}
        title={
          isNew ? <Trans>New purchase order</Trans> : <Trans>Purchase order {orderNumber}</Trans>
        }
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
                    prefillExchangeRate(
                      form,
                      organization?.id,
                      vendor.defaultCurrency,
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
          <Col xs={24} md={12} xl={4}>
            <Form.Item
              label={<Trans>Import</Trans>}
              name="importId"
              tooltip={t`The shipment this order's goods travel in — drives landed cost (freight/customs) allocation once received.`}
            >
              <Select
                allowClear
                showSearch
                optionFilterProp="children"
                placeholder={t`None`}
                onChange={(newImportId) => {
                  // Only cascade on a new order — same guard as the vendor
                  // cascade above. currency/exchangeRate are a *prefill*
                  // (db/migrations/0066's comment): the order still stores and
                  // freezes its own values once saved.
                  if (!isNew || !newImportId) return;
                  const imp = find(imports, { id: newImportId }) as any;
                  if (imp?.currency) {
                    form.setFieldsValue({
                      currency: imp.currency,
                      exchangeRate: imp.exchangeRate ?? undefined,
                      exchangeRateDate: imp.exchangeRateDate
                        ? dayjs(imp.exchangeRateDate)
                        : undefined,
                    });
                  }
                }}
              >
                {map(imports, (imp: any) => (
                  <Option key={imp.id} value={imp.id}>
                    {imp.importNumber}
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
              <StatusFlow
                current={currentStatus as PurchaseOrderStatus}
                statuses={PURCHASE_ORDER_STATUSES}
                transitions={purchaseOrderStatusTransitionMatrix}
                getLabel={purchaseOrderStatusLabel}
                getColor={getPurchaseOrderStatusColor}
              />
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
        {lineItemsFrozen && (
          <Alert
            type="info"
            showIcon
            style={{ marginBottom: 16 }}
            message={
              <Trans>
                Line items are locked because goods have already been received against this order.
                Cancel the goods receipt to change them — header fields can still be edited.
              </Trans>
            }
          />
        )}
        <LineItemsTable
          disabled={lineItemsFrozen}
          columns={[
            { kind: "index" },
            {
              kind: "product",
              products: purchasableProducts,
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
                    align: "right" as const,
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
                  {formatMoneyUnits(
                    subTotal,
                    currency,
                    numberFormatLocale(organization.country_code) ?? i18n.locale,
                    organization.minimum_fraction_digits ?? undefined,
                  )}
                </Descriptions.Item>
              </Descriptions>
            </Col>
          </Row>
        )}

        {/* Documents already created against this order — no visibility into
            these otherwise short of navigating to Goods Receipts/Incoming
            Invoices and searching for this order number. */}
        {!isNew && linkedReceipts.length > 0 && (
          <Card size="small" title={<Trans>Goods receipts</Trans>} style={{ marginTop: 16 }}>
            <Table dataSource={linkedReceipts} rowKey="id" size="small" pagination={false}>
              <Table.Column
                title={<Trans>Number</Trans>}
                key="deliveryNumber"
                render={(receipt: any) => (
                  <Link to={`/inbound-deliveries/${receipt.id}`}>{receipt.deliveryNumber}</Link>
                )}
              />
              <Table.Column
                title={<Trans>Date</Trans>}
                key="deliveryDate"
                render={(receipt: any) => formatDate(receipt.deliveryDate)}
              />
              <Table.Column
                title={<Trans>Status</Trans>}
                key="status"
                render={(receipt: any) => (
                  <Tag color={inboundDeliveryStatusColor[receipt.status as InboundDeliveryStatus]}>
                    {inboundDeliveryStatusLabel(receipt.status)}
                  </Tag>
                )}
              />
            </Table>
          </Card>
        )}
        {!isNew && linkedIncomingInvoices.length > 0 && (
          <Card size="small" title={<Trans>Incoming invoices</Trans>} style={{ marginTop: 16 }}>
            <Table dataSource={linkedIncomingInvoices} rowKey="id" size="small" pagination={false}>
              <Table.Column
                title={<Trans>Vendor invoice #</Trans>}
                key="vendorInvoiceNumber"
                render={(invoice: any) => (
                  <Link to={`/incoming-invoices/${invoice.id}`}>{invoice.vendorInvoiceNumber}</Link>
                )}
              />
              <Table.Column
                title={<Trans>Date</Trans>}
                key="date"
                render={(invoice: any) => formatDate(invoice.date)}
              />
              <Table.Column
                title={<Trans>State</Trans>}
                key="state"
                render={(invoice: any) => (
                  <Tag color={incomingInvoiceStateColor[invoice.state as IncomingInvoiceState]}>
                    {incomingInvoiceStateLabel(invoice.state)}
                  </Tag>
                )}
              />
            </Table>
          </Card>
        )}

        {/* Footer bar — portaled into the slot BaseLayout renders */}
        <ResponsiveFooter>
          <Row align="middle" justify="space-between" style={{ height: 64 }}>
            <Col>
              {!isNew && currentStatus !== "received" && !lineItemsFrozen && (
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
                {!isNew &&
                  purchaseOrderStatusTransitionMatrix[
                    currentStatus as PurchaseOrderStatus
                  ]?.includes("cancelled") && (
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
                  // (db/xlsx_export_purchase_order.go) — every purchase
                  // order has an embedded fallback template
                  // (resolveTemplateBytes) to fill even with no org
                  // override, so both buttons always work.
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
                {!isNew && !["draft", "cancelled"].includes(currentStatus) && (
                  <Button onClick={() => navigate(`/inbound-deliveries/new?purchaseOrderId=${id}`)}>
                    <PlusOutlined /> <Trans>New goods receipt</Trans>
                  </Button>
                )}
                {!isNew && !["draft", "cancelled"].includes(currentStatus) && (
                  <Button onClick={() => navigate(`/incoming-invoices/new?purchaseOrderId=${id}`)}>
                    <PlusOutlined /> <Trans>New incoming invoice</Trans>
                  </Button>
                )}
                <Button type="primary" onClick={() => form.submit()}>
                  <SaveOutlined /> <Trans>Save</Trans>
                </Button>
              </Space>
            </Col>
          </Row>
        </ResponsiveFooter>
      </Form>
    </>
  );
};

export default PurchaseOrderDetails;
