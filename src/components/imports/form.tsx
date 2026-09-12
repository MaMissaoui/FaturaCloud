import { useEffect, useMemo, useState } from "react";
import { Link, useLocation, useNavigate } from "react-router";
import {
  Button,
  Card,
  Col,
  DatePicker,
  Drawer,
  Form,
  Input,
  InputNumber,
  Popconfirm,
  Row,
  Select,
  Space,
  Statistic,
  Table,
  Tag,
} from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { DeleteOutlined, DisconnectOutlined } from "@ant-design/icons";
import dayjs from "dayjs";
import get from "lodash/get";
import map from "lodash/map";

import { GetNextImportNumber, GetImportSummary } from "src/api";
import type { ImportSummary } from "src/types/models";
import { importIdAtom, importAtom, importsAtom, deleteImportAtom } from "src/atoms/import";
import { purchaseOrdersAtom, setPurchaseOrderImportAtom } from "src/atoms/purchase-order";
import {
  purchaseOrderStatusColor,
  purchaseOrderStatusLabel,
  type PurchaseOrderStatus,
} from "src/types/purchase-order";
import { organizationAtom, organizationIdAtom } from "src/atoms/organization";
import { centsToUnits, unitsToCents, formatCents } from "src/utils/currency";
import { useDatePickerFormat } from "src/utils/date";
import ExchangeRateFields, {
  CurrencySelect,
  showExchangeRateFields,
} from "src/components/currency/currency-fields";
import ScrollShadow from "src/components/scroll-shadow";

const { TextArea } = Input;
const { Option } = Select;

const ImportForm = () => {
  const { i18n } = useLingui();
  const location = useLocation();
  const navigate = useNavigate();
  const [form] = Form.useForm();
  const dateFormat = useDatePickerFormat();

  const [importId, setImportId] = useAtom(importIdAtom);
  const imports = useAtomValue(importsAtom);
  const setImportRecord = useSetAtom(importAtom);
  const deleteImport = useSetAtom(deleteImportAtom);
  const orders = useAtomValue(purchaseOrdersAtom);
  const setPurchaseOrderImport = useSetAtom(setPurchaseOrderImportAtom);
  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);
  const orgCurrency = organization?.currency ?? "EUR";

  const [submitting, setSubmitting] = useState(false);
  const [summary, setSummary] = useState<ImportSummary | null>(null);
  // Bumped after a link/unlink so the Allocation card's committed-value/rate
  // re-fetches — GetImportSummary is otherwise only keyed on importId.
  const [summaryTick, setSummaryTick] = useState(0);
  // Tracks edits to the import's own fields (not the linked-PO actions
  // below), so "New purchase order" can warn before navigating away with
  // unsaved changes — same isDirty idiom as src/routes/invoices/details.tsx.
  const [isDirty, setIsDirty] = useState(false);
  const [linkOrderId, setLinkOrderId] = useState<string | null>(null);
  const [linking, setLinking] = useState(false);

  const isVisible = get(location.state, "importModal", false);

  const linkedOrders = useMemo(
    () => orders.filter((o: any) => o.importId === importId),
    [orders, importId],
  );
  // Only a PO that isn't already someone else's shipment and hasn't moved
  // past "confirmed" is a sensible attach target — matches the statuses
  // purchaseOrderTransitions still allows moving out of.
  const candidateOrders = useMemo(
    () =>
      orders.filter((o: any) => !o.importId && (o.status === "draft" || o.status === "confirmed")),
    [orders],
  );

  const importRecord = useMemo(() => {
    if (!importId) return null;
    return imports.find((x: any) => x.id === importId) ?? null;
  }, [imports, importId]);

  const watchedCurrency = Form.useWatch("currency", form);

  const handleClose = () => {
    setImportId(null);
    form.resetFields();
    setIsDirty(false);
    navigate(location.pathname, { state: { importModal: false } });
  };

  const handleSubmit = async (values: any) => {
    setSubmitting(true);
    try {
      await setImportRecord({
        ...values,
        date: values.date?.valueOf ? values.date.valueOf() : values.date,
        exchangeRateDate: values.exchangeRateDate?.valueOf
          ? values.exchangeRateDate.valueOf()
          : values.exchangeRateDate,
        freightCost: unitsToCents(values.freightCost ?? 0),
        customsCost: unitsToCents(values.customsCost ?? 0),
      });
      handleClose();
    } catch {
      // setImportRecord already toasted the error — keep the drawer open
      // with the user's input intact rather than closing on a failed save.
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async () => {
    if (importId) {
      setSubmitting(true);
      const deleted = await deleteImport(importId);
      // Deletion is refused while a purchase order still links to this
      // import; keep the drawer open so the message stays in context.
      if (deleted) handleClose();
      setSubmitting(false);
    }
  };

  useEffect(() => {
    const navImportId = get(location.state, "importId");
    if (isVisible && navImportId) {
      setImportId(navImportId);
    } else if (!isVisible) {
      setImportId(null);
      form.resetFields();
    }
  }, [isVisible, location.state, setImportId, form]);

  useEffect(() => {
    if (importRecord) {
      form.setFieldsValue({
        ...importRecord,
        date: dayjs(importRecord.date),
        exchangeRateDate: importRecord.exchangeRateDate
          ? dayjs(importRecord.exchangeRateDate)
          : undefined,
        freightCost: centsToUnits(importRecord.freightCost),
        customsCost: centsToUnits(importRecord.customsCost),
      });
      setIsDirty(false);
    } else if (!importId && isVisible) {
      form.resetFields();
      form.setFieldsValue({ date: dayjs() });
      setIsDirty(false);
      if (organizationId) {
        GetNextImportNumber(organizationId).then((number) =>
          form.setFieldValue("importNumber", number),
        );
      }
    }
  }, [importRecord, importId, isVisible, organizationId, form]);

  useEffect(() => {
    if (importId) {
      GetImportSummary(importId)
        .then(setSummary)
        .catch(() => setSummary(null));
    } else {
      setSummary(null);
    }
    // summaryTick: re-fetch after a linked purchase order is attached/detached
    // from the "Purchase orders" card below, since that never touches the
    // import record itself.
  }, [importId, summaryTick]);

  // Server guard (GetImportPurchaseOrderCount) counts every linked purchase
  // order regardless of status or line items — matching that exactly here
  // avoids offering a Delete button that then 409s.
  const canDelete = linkedOrders.length === 0;

  const handleLinkOrder = async () => {
    if (!importId || !linkOrderId) return;
    setLinking(true);
    const ok = await setPurchaseOrderImport({ orderId: linkOrderId, importId });
    if (ok) {
      setLinkOrderId(null);
      setSummaryTick((tick) => tick + 1);
    }
    setLinking(false);
  };

  const handleUnlinkOrder = async (orderId: string) => {
    if (!importId) return;
    const ok = await setPurchaseOrderImport({ orderId, importId: null });
    if (ok) setSummaryTick((tick) => tick + 1);
  };

  const handleNewPurchaseOrder = () => {
    if (!importId) return;
    navigate("/purchase-orders/new", { state: { importId } });
  };

  return (
    <Drawer
      title={importId ? <Trans>Edit import</Trans> : <Trans>New import</Trans>}
      open={isVisible}
      placement="right"
      size={640}
      onClose={handleClose}
      footer={
        <div style={{ display: "flex", justifyContent: "space-between" }}>
          <div>
            {importId && canDelete && (
              <Popconfirm
                title={<Trans>Are you sure you want to delete this import?</Trans>}
                onConfirm={handleDelete}
                okText={<Trans>Yes</Trans>}
                cancelText={<Trans>No</Trans>}
                placement="topRight"
              >
                <Button danger icon={<DeleteOutlined />} loading={submitting}>
                  <Trans>Delete</Trans>
                </Button>
              </Popconfirm>
            )}
          </div>
          <Space>
            <Button onClick={handleClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="primary" loading={submitting} onClick={() => form.submit()}>
              <Trans>Save</Trans>
            </Button>
          </Space>
        </div>
      }
    >
      <ScrollShadow>
        <Form
          form={form}
          layout="vertical"
          onFinish={handleSubmit}
          onValuesChange={() => setIsDirty(true)}
        >
          <Card size="small" title={<Trans>Shipment</Trans>} style={{ marginBottom: 12 }}>
            <Row gutter={[16, 0]}>
              <Col xs={24} md={12}>
                <Form.Item
                  name="importNumber"
                  label={<Trans>Import number</Trans>}
                  rules={[{ required: true, message: t`Please input a number!` }]}
                >
                  <Input placeholder={t`Import number`} />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item
                  name="date"
                  label={<Trans>Date</Trans>}
                  rules={[{ required: true, message: t`Date is required` }]}
                >
                  <DatePicker style={{ width: "100%" }} format={dateFormat} />
                </Form.Item>
              </Col>
              <CurrencySelect
                form={form}
                organizationId={organizationId ?? undefined}
                orgCurrency={orgCurrency}
                xl={12}
              />
              {showExchangeRateFields(watchedCurrency, orgCurrency) && (
                <ExchangeRateFields currency={watchedCurrency} orgCurrency={orgCurrency} xl={12} />
              )}
              <Col xs={24}>
                <Trans>
                  Prefill only — every purchase order linked to this import still stores and freezes
                  its own currency and rate when saved.
                </Trans>
              </Col>
            </Row>
          </Card>

          <Card
            size="small"
            title={<Trans>Serial Numbers</Trans>}
            style={{ marginBottom: 12 }}
            extra={<Trans>Optional</Trans>}
          >
            <Row gutter={[16, 0]}>
              <Col xs={24} md={8}>
                <Form.Item name="serialNumberPrefix" label={<Trans>Prefix</Trans>}>
                  <Input placeholder="SN-" />
                </Form.Item>
              </Col>
              <Col xs={12} md={8}>
                <Form.Item name="serialNumberRangeStart" label={<Trans>Range start</Trans>}>
                  <InputNumber min={0} style={{ width: "100%" }} placeholder="1001" />
                </Form.Item>
              </Col>
              <Col xs={12} md={8}>
                <Form.Item
                  name="serialNumberRangeEnd"
                  label={<Trans>Range end</Trans>}
                  style={{ marginBottom: 0 }}
                >
                  <InputNumber min={0} style={{ width: "100%" }} placeholder="1050" />
                </Form.Item>
              </Col>
              <Col xs={24}>
                <Trans>
                  Reserves a serial-number range for whatever gets produced from this shipment's
                  components — a Production Order linked to this import can only register serials
                  inside it.
                </Trans>
              </Col>
            </Row>
          </Card>

          <Card size="small" title={<Trans>Landed cost</Trans>} style={{ marginBottom: 12 }}>
            <Row gutter={[16, 0]}>
              <Col xs={24} md={12}>
                <Form.Item name="freightCost" label={<Trans>Freight cost</Trans>}>
                  <InputNumber
                    min={0}
                    precision={2}
                    style={{ width: "100%" }}
                    addonAfter={orgCurrency}
                  />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="customsCost" label={<Trans>Customs cost</Trans>}>
                  <InputNumber
                    min={0}
                    precision={2}
                    style={{ width: "100%" }}
                    addonAfter={orgCurrency}
                  />
                </Form.Item>
              </Col>
              <Col xs={24}>
                <Form.Item name="notes" label={<Trans>Notes</Trans>} style={{ marginBottom: 0 }}>
                  <TextArea rows={2} placeholder={t`Notes`} />
                </Form.Item>
              </Col>
            </Row>
          </Card>

          {importId && (
            <Card
              size="small"
              title={<Trans>Purchase orders</Trans>}
              style={{ marginBottom: 12 }}
              extra={
                isDirty ? (
                  <Popconfirm
                    title={<Trans>Discard unsaved changes to this import?</Trans>}
                    onConfirm={handleNewPurchaseOrder}
                    okText={<Trans>Yes</Trans>}
                    cancelText={<Trans>No</Trans>}
                  >
                    <Button type="link" size="small">
                      <Trans>New purchase order</Trans>
                    </Button>
                  </Popconfirm>
                ) : (
                  <Button type="link" size="small" onClick={handleNewPurchaseOrder}>
                    <Trans>New purchase order</Trans>
                  </Button>
                )
              }
            >
              <Space.Compact style={{ width: "100%", marginBottom: 12 }}>
                <Select
                  showSearch
                  optionFilterProp="children"
                  placeholder={t`Link an existing purchase order`}
                  style={{ flex: 1 }}
                  value={linkOrderId ?? undefined}
                  onChange={setLinkOrderId}
                  notFoundContent={<Trans>No unlinked draft or confirmed purchase orders</Trans>}
                >
                  {map(candidateOrders, (o: any) => (
                    <Option key={o.id} value={o.id}>
                      {o.orderNumber} — {o.vendorName ?? t`No vendor`}
                    </Option>
                  ))}
                </Select>
                <Button
                  type="primary"
                  disabled={!linkOrderId}
                  loading={linking}
                  onClick={handleLinkOrder}
                >
                  <Trans>Link</Trans>
                </Button>
              </Space.Compact>

              <Table
                size="small"
                pagination={false}
                dataSource={linkedOrders}
                rowKey="id"
                locale={{
                  emptyText: <Trans>No purchase orders linked to this shipment yet.</Trans>,
                }}
              >
                <Table.Column
                  title={<Trans>Order #</Trans>}
                  key="orderNumber"
                  render={(o: any) => <Link to={`/purchase-orders/${o.id}`}>{o.orderNumber}</Link>}
                />
                <Table.Column
                  title={<Trans>Vendor</Trans>}
                  key="vendorName"
                  render={(o: any) => o.vendorName ?? "—"}
                />
                <Table.Column
                  title={<Trans>Status</Trans>}
                  key="status"
                  render={(o: any) => (
                    <Tag color={purchaseOrderStatusColor[o.status as PurchaseOrderStatus]}>
                      {purchaseOrderStatusLabel(o.status)}
                    </Tag>
                  )}
                />
                <Table.Column
                  key="actions"
                  width={40}
                  align="center"
                  render={(o: any) => (
                    <Popconfirm
                      title={<Trans>Unlink this purchase order from the shipment?</Trans>}
                      onConfirm={() => handleUnlinkOrder(o.id)}
                      okText={<Trans>Yes</Trans>}
                      cancelText={<Trans>No</Trans>}
                    >
                      <Button
                        type="text"
                        danger
                        size="small"
                        icon={<DisconnectOutlined />}
                        aria-label={t`Unlink purchase order`}
                      />
                    </Popconfirm>
                  )}
                />
              </Table>
            </Card>
          )}

          {importId && summary && (
            <Card size="small" title={<Trans>Allocation</Trans>}>
              <Row gutter={[16, 16]}>
                <Col xs={12}>
                  <Statistic
                    title={<Trans>Committed value</Trans>}
                    value={formatCents(summary.totalCommittedValue, orgCurrency, i18n.locale)}
                  />
                </Col>
                <Col xs={12}>
                  <Statistic
                    title={<Trans>Landed cost rate</Trans>}
                    value={`${(summary.landedCostRate * 100).toFixed(1)}%`}
                  />
                </Col>
                <Col xs={24}>
                  <Trans>
                    Applied to each received line's own value once its purchase order's goods are
                    received — not force-balanced against freight+customs across partial or uncosted
                    receiving.
                  </Trans>
                </Col>
              </Row>
            </Card>
          )}
        </Form>
      </ScrollShadow>
    </Drawer>
  );
};

export default ImportForm;
