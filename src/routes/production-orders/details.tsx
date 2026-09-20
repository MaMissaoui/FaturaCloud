import { useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router";
import {
  Alert,
  Button,
  Col,
  DatePicker,
  Descriptions,
  Form,
  Input,
  InputNumber,
  Popconfirm,
  Row,
  Select,
  Skeleton,
  Space,
  Table,
  Tag,
  theme,
  Tooltip,
  Typography,
} from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import { loadable } from "src/utils/loadable";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { DeleteOutlined, DeploymentUnitOutlined, SaveOutlined } from "@ant-design/icons";
import dayjs from "dayjs";
import type { Dayjs } from "dayjs";
import find from "lodash/find";
import PageHeader from "src/components/page-header";
import ResponsiveFooter from "src/components/responsive-footer";
import { message } from "src/utils/message";

import { GetProductBOM } from "src/api";
import type {
  BillOfMaterialsLine,
  Import,
  Product,
  ProductionOrder,
  ProductionOrderComponentLine,
} from "src/types/models";
import { useDatePickerFormat, useDateFormatter } from "src/utils/date";
import SerialCaptureModal from "src/components/stock/serial-capture-modal";
import StatusFlow from "src/components/status-flow";
import {
  PRODUCTION_ORDER_STATUSES,
  productionOrderStatusColor,
  productionOrderStatusLabel,
  productionOrderStatusTransitionMatrix,
  productionOrderTransitions,
  type ProductionOrderStatus,
} from "src/types/production-order";
import { productsAtom, setProductsAtom } from "src/atoms/product";
import { importsAtom, setImportsAtom } from "src/atoms/import";
import { organizationAtom } from "src/atoms/organization";
import { numberFormatLocale } from "src/utils/currencies";
import {
  productionOrderIdAtom,
  productionOrderAtom,
  nextProductionOrderNumberAtom,
  createProductionOrderAtom,
  updateProductionOrderStatusAtom,
  deleteProductionOrderAtom,
} from "src/atoms/production-order";

const { TextArea } = Input;
const { Option } = Select;

// Quantities in the BOM preview are always "for this order's quantity" —
// rounded the same way the BOM editor's own display-side rounding is
// (Math.round(q * 10000) / 10000), not db/product_bom.go's roundBOMQuantity
// (the stored-value authority), just to avoid ugly floats on screen.
const roundDisplay = (q: number) => Math.round(q * 10000) / 10000;

// Quantities displayed in the BOM preview and the component table are a
// display concern only — the org's country-derived locale (falling back to
// the viewer's UI language) renders "2,5" not a hardcoded "."; whole values
// stay clean and fractions are capped at 2 decimals.
const formatQty = (q: number, locale: string) =>
  new Intl.NumberFormat(locale, {
    minimumFractionDigits: 0,
    maximumFractionDigits: 2,
  }).format(q);

// Module-level so it's referentially stable across renders — StatusFlow is
// memoized and an inline arrow here would defeat that on every keystroke.
const getProductionOrderStatusColor = (s: ProductionOrderStatus) => productionOrderStatusColor[s];

// productionOrderAtom is async; reading it with plain useAtom would suspend
// to the app's top-level Suspense boundary whenever productionOrderIdAtom
// changes after mount, unmounting/remounting this route — same fix as
// src/routes/inbound-deliveries/details.tsx.
const loadableOrderAtom = loadable(productionOrderAtom);

// Extracted so Form.useForm()/Form.useWatch() only ever mount while
// actually creating an order — every other document type's detail page
// keeps one Form mounted in both modes, but a production order has no PUT
// (see db/production_order.go's comment: nothing is left to edit on a
// draft besides its status), so the view path below renders no Form at
// all. Keeping the hook at the parent's top level regardless of isNew
// would create a form instance that's never attached to any <Form>.
const CreateProductionOrderForm = ({
  finishedProducts,
  products,
  imports,
  nextNumber,
  createOrder,
  dateFormat,
  qtyLocale,
}: {
  finishedProducts: Product[];
  products: Product[];
  imports: Import[];
  nextNumber: string;
  createOrder: (values: Partial<ProductionOrder>) => Promise<unknown>;
  dateFormat: string;
  qtyLocale: string;
}) => {
  const { token } = theme.useToken();
  const [form] = Form.useForm();
  const [submitting, setSubmitting] = useState(false);
  const [bomLines, setBomLines] = useState<BillOfMaterialsLine[]>([]);
  const [bomLoading, setBomLoading] = useState(false);
  // A failed fetch used to be swallowed into an empty list, which the Alert
  // below then reported as "this product has no Bill of Materials" — telling
  // the user to fix data that is already correct, with Create disabled and
  // no way to retry (F85).
  const [bomLoadFailed, setBomLoadFailed] = useState(false);
  const [bomReloadToken, setBomReloadToken] = useState(0);

  const watchedProductId = Form.useWatch("finishedProductId", form);
  const watchedQuantity = Form.useWatch("quantity", form) ?? 1;
  const selectedProduct = find(finishedProducts, { id: watchedProductId });

  // Preview the recipe scaled by the entered quantity, before creating —
  // GetBillOfMaterials' quantityPerUnit is always per one finished unit
  // (see CLAUDE.md's Bill of Materials note), the same value
  // db.CreateProductionOrder multiplies by req.Quantity server-side.
  useEffect(() => {
    if (!watchedProductId) {
      setBomLines([]);
      return;
    }
    let cancelled = false;
    setBomLoading(true);
    setBomLoadFailed(false);
    GetProductBOM(watchedProductId)
      .then((lines) => {
        if (!cancelled) setBomLines(lines ?? []);
      })
      .catch((error) => {
        if (cancelled) return;
        setBomLines([]);
        setBomLoadFailed(true);
        message.error(error instanceof Error ? error.message : t`Failed to load bill of materials`);
      })
      .finally(() => {
        if (!cancelled) setBomLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [watchedProductId, bomReloadToken]);

  // The create form's raw values: date is a Dayjs until submitted, and the
  // two optional fields arrive as "" from an untouched Select rather than
  // null, which is why they're normalized below.
  type CreateFormValues = {
    orderNumber: string;
    finishedProductId: string;
    quantity: number;
    date: Dayjs;
    importId?: string | null;
    notes?: string | null;
  };

  const handleCreate = async (values: CreateFormValues) => {
    setSubmitting(true);
    try {
      await createOrder({
        orderNumber: values.orderNumber,
        finishedProductId: values.finishedProductId,
        quantity: values.quantity,
        date: values.date.valueOf(),
        importId: values.importId || null,
        notes: values.notes || null,
      });
    } catch {
      // createProductionOrderAtom already toasted the error — keep the form.
    } finally {
      setSubmitting(false);
    }
  };

  const initialValues = {
    orderNumber: nextNumber,
    date: dayjs(),
    quantity: 1,
  };

  return (
    <Form form={form} onFinish={handleCreate} layout="vertical" initialValues={initialValues}>
      <Row gutter={24}>
        <Col xs={24} md={12} xl={6}>
          <Form.Item
            label={<Trans>Finished product</Trans>}
            name="finishedProductId"
            rules={[{ required: true, message: t`Finished product is required` }]}
          >
            <Select showSearch allowClear optionFilterProp="children" placeholder={t`Select…`}>
              {finishedProducts.map((p) => (
                <Option key={p.id} value={p.id}>
                  {p.name}
                </Option>
              ))}
            </Select>
          </Form.Item>
        </Col>
        <Col xs={24} md={12} xl={4}>
          <Form.Item
            label={<Trans>Quantity</Trans>}
            name="quantity"
            rules={[{ required: true, message: t`Quantity is required` }]}
          >
            <InputNumber
              min={0.0001}
              precision={selectedProduct?.serialized ? 0 : 2}
              style={{ width: "100%" }}
            />
          </Form.Item>
        </Col>
        <Col xs={24} md={12} xl={4}>
          <Form.Item
            label={<Trans>Order number</Trans>}
            name="orderNumber"
            rules={[{ required: true, message: t`Order number is required` }]}
          >
            <Input />
          </Form.Item>
        </Col>
        <Col xs={24} md={12} xl={4}>
          <Form.Item
            label={<Trans>Date</Trans>}
            name="date"
            rules={[{ required: true, message: t`Date is required` }]}
          >
            <DatePicker style={{ width: "100%" }} format={dateFormat} />
          </Form.Item>
        </Col>
        <Col xs={24} md={12} xl={6}>
          <Form.Item
            label={<Trans>Import</Trans>}
            name="importId"
            tooltip={t`Optional — links the produced units to a shipment's reserved serial-number range.`}
          >
            <Select allowClear showSearch optionFilterProp="children" placeholder={t`None`}>
              {imports.map((imp) => (
                <Option key={imp.id} value={imp.id}>
                  {imp.importNumber}
                </Option>
              ))}
            </Select>
          </Form.Item>
        </Col>
      </Row>
      <Row gutter={24}>
        <Col xs={24}>
          <Form.Item label={<Trans>Notes</Trans>} name="notes">
            <TextArea rows={1} autoSize />
          </Form.Item>
        </Col>
      </Row>

      {watchedProductId && !bomLoading && bomLoadFailed && (
        <Alert
          type="error"
          showIcon
          style={{ marginBottom: 16 }}
          message={<Trans>Couldn't load this product's Bill of Materials</Trans>}
          action={
            <Button size="small" onClick={() => setBomReloadToken((n) => n + 1)}>
              <Trans>Retry</Trans>
            </Button>
          }
        />
      )}

      {watchedProductId && !bomLoading && !bomLoadFailed && bomLines.length === 0 && (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
          message={
            <Trans>
              This product has no Bill of Materials — define one before creating a production order
              for it.
            </Trans>
          }
        />
      )}

      <Table
        dataSource={bomLines}
        loading={bomLoading}
        pagination={false}
        rowKey="componentProductId"
        size="small"
        locale={{ emptyText: <Trans>Select a finished product to preview its recipe</Trans> }}
      >
        <Table.Column
          title={<Trans>Component</Trans>}
          key="componentName"
          render={(l: BillOfMaterialsLine) =>
            l.componentSku ? `${l.componentName} (${l.componentSku})` : l.componentName
          }
        />
        <Table.Column
          title={<Trans>Qty per unit</Trans>}
          dataIndex="quantityPerUnit"
          key="quantityPerUnit"
          align="right"
          render={(v: number) => roundDisplay(v)}
        />
        <Table.Column
          title={<Trans>Total quantity</Trans>}
          key="totalQuantity"
          align="right"
          render={(l: BillOfMaterialsLine) => roundDisplay(l.quantityPerUnit * watchedQuantity)}
        />
        <Table.Column title={<Trans>Unit</Trans>} dataIndex="componentUnit" key="componentUnit" />
        <Table.Column
          title={<Trans>On hand</Trans>}
          key="onHand"
          align="right"
          // stockQuantity comes from the already-loaded productsAtom — no
          // new endpoint. Completion is what actually consumes stock (see
          // the transitions comment on ProductionOrderStatus), so before
          // this column existed the only shortfall signal was a 409 on
          // "Mark as completed," after the order was already created.
          render={(l: BillOfMaterialsLine) => {
            const component = find(products, { id: l.componentProductId });
            const onHand = component?.stockQuantity ?? 0;
            const required = roundDisplay(l.quantityPerUnit * watchedQuantity);
            const short = onHand < required;
            return (
              <span style={{ color: short ? token.colorError : undefined, fontWeight: 600 }}>
                {formatQty(onHand, qtyLocale)}
                {short && (
                  <Typography.Text type="danger" style={{ marginLeft: 4, fontSize: 12 }}>
                    (−{formatQty(required - onHand, qtyLocale)})
                  </Typography.Text>
                )}
              </span>
            );
          }}
        />
      </Table>

      <ResponsiveFooter>
        <Row align="middle" justify="end" style={{ height: 64 }}>
          <Col>
            <Tooltip
              title={
                !watchedProductId
                  ? t`Select a finished product first`
                  : bomLines.length === 0
                    ? t`This product has no Bill of Materials`
                    : undefined
              }
            >
              <Button
                type="primary"
                loading={submitting}
                disabled={bomLines.length === 0}
                onClick={() => form.submit()}
              >
                <SaveOutlined /> <Trans>Create</Trans>
              </Button>
            </Tooltip>
          </Col>
        </Row>
      </ResponsiveFooter>
    </Form>
  );
};

const ProductionOrderDetails = () => {
  const { id } = useParams<string>();
  const navigate = useNavigate();
  const { i18n } = useLingui();
  const { token } = theme.useToken();
  const dateFormat = useDatePickerFormat();
  const formatDate = useDateFormatter();

  const organization = useAtomValue(organizationAtom);
  const qtyLocale = numberFormatLocale(organization?.country_code) ?? i18n.locale;

  const isNew = id === "new";

  const products = useAtomValue(productsAtom);
  const setProducts = useSetAtom(setProductsAtom);
  // Only a "finished" good has a BOM to consume — component/unclassified
  // products aren't produced by a production order.
  const finishedProducts = useMemo(
    () => products.filter((p) => p.category === "finished"),
    [products],
  );
  const imports = useAtomValue(importsAtom);
  const setImports = useSetAtom(setImportsAtom);
  const nextNumber = useAtomValue(nextProductionOrderNumberAtom);

  const [orderId, setOrderId] = useAtom(productionOrderIdAtom);
  const orderLoadable = useAtomValue(loadableOrderAtom);
  const order = orderLoadable.state === "hasData" ? orderLoadable.data : undefined;
  const createOrder = useSetAtom(createProductionOrderAtom);
  const updateStatus = useSetAtom(updateProductionOrderStatusAtom);
  const deleteOrder = useSetAtom(deleteProductionOrderAtom);

  const [statusOverride, setStatusOverride] = useState<string | null>(null);
  const [serialCapture, setSerialCapture] = useState(false);
  // Without this the modal's OK stayed enabled through the await below, so a
  // second click fired a second PATCH — and since draft -> completed has
  // already consumed stock by then, the retry 409s and toasts an error on
  // top of the success (F86).
  const [confirmingSerials, setConfirmingSerials] = useState(false);

  useEffect(() => {
    setProducts();
    setImports();
    setStatusOverride(null);
    if (!isNew) setOrderId(id ?? null);
    return () => setOrderId(null);
  }, [id, isNew, setProducts, setImports, setOrderId]);

  // After create, navigate to the new order.
  useEffect(() => {
    if (isNew && orderId) {
      navigate(`/production-orders/${orderId}`);
    }
  }, [isNew, orderId, navigate]);

  const handleDelete = async () => {
    if (!id || isNew) return;
    const success = await deleteOrder(id);
    if (success) navigate("/production-orders");
  };

  const applyStatusChange = async (next: string, serialNumbers?: string[]) => {
    if (!id || isNew) return;
    const ok = await updateStatus({ orderId: id, status: next, serialNumbers });
    if (ok) setStatusOverride(next);
  };

  const isSerialized = order?.serialized === 1;

  const handleStatusChange = async (next: string) => {
    if (!id || isNew) return;
    if (next === "completed" && isSerialized) {
      setSerialCapture(true);
      return;
    }
    await applyStatusChange(next);
  };

  const handleSerialCaptureConfirm = async (serialNumbers: Record<string, string[]>) => {
    if (confirmingSerials) return;
    setConfirmingSerials(true);
    try {
      await applyStatusChange("completed", serialNumbers.finished ?? []);
      setSerialCapture(false);
    } finally {
      setConfirmingSerials(false);
    }
  };

  // No `.then` guard here any more: loadable() already unwraps the promise,
  // so those checks were vestigial copy-paste from the pre-loadable sibling
  // and could never be true (audit 2026-09-14 F90).
  const currentOrder = order ?? undefined;
  const currentStatus = statusOverride ?? currentOrder?.status ?? "draft";
  const transitions = isNew ? [] : productionOrderTransitions(currentStatus);

  const linkedImport = currentOrder?.importId
    ? (find(imports, { id: currentOrder.importId }) ?? null)
    : null;
  const importRange =
    linkedImport &&
    linkedImport.serialNumberRangeStart != null &&
    linkedImport.serialNumberRangeEnd != null
      ? {
          prefix: linkedImport.serialNumberPrefix ?? "",
          start: linkedImport.serialNumberRangeStart,
          end: linkedImport.serialNumberRangeEnd,
          importNumber: linkedImport.importNumber,
        }
      : null;

  // Loading and failure used to share `return null`, so both rendered a
  // completely empty page — no header, no retry (F89).
  if (!isNew && !order) {
    return (
      <>
        <PageHeader title={<Trans>Production Order</Trans>} icon={<DeploymentUnitOutlined />} />
        <div style={{ padding: 24 }}>
          {orderLoadable.state === "loading" ? (
            <Skeleton active paragraph={{ rows: 6 }} />
          ) : (
            <Alert
              type="error"
              showIcon
              message={<Trans>Couldn't load this production order</Trans>}
              action={
                <Button size="small" onClick={() => navigate("/production-orders")}>
                  <Trans>Back to list</Trans>
                </Button>
              }
            />
          )}
        </div>
      </>
    );
  }

  return (
    <>
      <PageHeader
        icon={<DeploymentUnitOutlined />}
        title={<Trans>Production Order</Trans>}
        style={{ marginBottom: 24 }}
      />

      {isNew ? (
        <CreateProductionOrderForm
          finishedProducts={finishedProducts}
          products={products}
          imports={imports}
          nextNumber={nextNumber}
          createOrder={createOrder}
          dateFormat={dateFormat}
          qtyLocale={qtyLocale}
        />
      ) : (
        currentOrder && (
          <>
            <Descriptions column={2} size="small" style={{ marginBottom: 16 }}>
              <Descriptions.Item label={<Trans>Order #</Trans>}>
                {currentOrder.orderNumber}
              </Descriptions.Item>
              <Descriptions.Item label={<Trans>Status</Trans>}>
                <Space>
                  <Tag color={productionOrderStatusColor[currentStatus as ProductionOrderStatus]}>
                    {productionOrderStatusLabel(currentStatus)}
                  </Tag>
                  <StatusFlow
                    current={currentStatus as ProductionOrderStatus}
                    statuses={PRODUCTION_ORDER_STATUSES}
                    transitions={productionOrderStatusTransitionMatrix}
                    getLabel={productionOrderStatusLabel}
                    getColor={getProductionOrderStatusColor}
                  />
                </Space>
              </Descriptions.Item>
              <Descriptions.Item label={<Trans>Finished product</Trans>}>
                {currentOrder.finishedProductName}
              </Descriptions.Item>
              <Descriptions.Item label={<Trans>Quantity</Trans>}>
                {currentOrder.quantity}
              </Descriptions.Item>
              <Descriptions.Item label={<Trans>Date</Trans>}>
                {formatDate(currentOrder.date)}
              </Descriptions.Item>
              <Descriptions.Item label={<Trans>Import</Trans>}>
                {currentOrder.importNumber ?? "—"}
              </Descriptions.Item>
              <Descriptions.Item label={<Trans>Notes</Trans>} span={2}>
                {currentOrder.notes ?? "—"}
              </Descriptions.Item>
            </Descriptions>

            <Table
              dataSource={currentOrder.componentLines}
              pagination={false}
              rowKey="id"
              size="small"
            >
              <Table.Column
                title={<Trans>Component</Trans>}
                key="componentName"
                render={(_: unknown, l: ProductionOrderComponentLine) =>
                  l.componentSku ? `${l.componentName} (${l.componentSku})` : l.componentName
                }
              />
              <Table.Column
                title={<Trans>Qty per unit</Trans>}
                dataIndex="quantityPerUnit"
                key="quantityPerUnit"
                align="right"
              />
              <Table.Column
                title={<Trans>Total quantity</Trans>}
                dataIndex="totalQuantity"
                key="totalQuantity"
                align="right"
              />
              <Table.Column
                title={<Trans>Unit</Trans>}
                dataIndex="componentUnit"
                key="componentUnit"
              />
              <Table.Column
                title={<Trans>On hand</Trans>}
                key="onHand"
                align="right"
                render={(_: unknown, l: ProductionOrderComponentLine) => {
                  if (!l.componentProductId) return "—";
                  const component = find(products, { id: l.componentProductId });
                  const onHand = component?.stockQuantity ?? 0;
                  const short = onHand < l.totalQuantity;
                  return (
                    <span style={{ color: short ? token.colorError : undefined, fontWeight: 600 }}>
                      {formatQty(onHand, qtyLocale)}
                    </span>
                  );
                }}
              />
            </Table>

            <ResponsiveFooter>
              <Row align="middle" justify="space-between" style={{ height: 64 }}>
                <Col>
                  {currentStatus === "draft" && (
                    <Popconfirm
                      title={t`Delete this production order?`}
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
                        title={t`This will consume the recipe's components and produce the finished units. Continue?`}
                        onConfirm={() => handleStatusChange(transition.next)}
                        okText={t`Yes`}
                        cancelText={t`No`}
                        placement="topRight"
                      >
                        <Button type={transition.type ?? "default"}>{transition.label}</Button>
                      </Popconfirm>
                    ))}
                    {currentStatus !== "cancelled" && (
                      <Popconfirm
                        title={
                          currentStatus === "completed"
                            ? t`This will reverse the stock this order consumed and produced. Continue?`
                            : t`Cancel this production order?`
                        }
                        onConfirm={() => handleStatusChange("cancelled")}
                        okText={t`Yes`}
                        cancelText={t`No`}
                        placement="topRight"
                      >
                        <Button type="dashed" danger>
                          <Trans>Cancel order</Trans>
                        </Button>
                      </Popconfirm>
                    )}
                  </Space>
                </Col>
              </Row>
            </ResponsiveFooter>

            {/* finishedProductId is nullable (ON DELETE SET NULL), which the
                removed `any` was hiding. A deleted finished product has no
                serial registry to capture into, and completing such an order
                409s server-side anyway, so there is nothing to show. */}
            <SerialCaptureModal
              open={serialCapture && !!currentOrder.finishedProductId}
              mode="produce"
              lines={[
                {
                  lineItemId: "finished",
                  productId: currentOrder.finishedProductId ?? "",
                  productName: currentOrder.finishedProductName,
                  quantity: currentOrder.quantity,
                },
              ]}
              importRange={importRange}
              confirming={confirmingSerials}
              onCancel={() => setSerialCapture(false)}
              onConfirm={handleSerialCaptureConfirm}
            />
          </>
        )
      )}
    </>
  );
};

export default ProductionOrderDetails;
