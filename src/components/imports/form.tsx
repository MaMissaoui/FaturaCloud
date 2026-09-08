import { useEffect, useMemo, useState } from "react";
import { useLocation, useNavigate } from "react-router";
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
  Space,
  Statistic,
} from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { DeleteOutlined } from "@ant-design/icons";
import dayjs from "dayjs";
import get from "lodash/get";

import { GetNextImportNumber, GetImportSummary } from "src/api";
import type { ImportSummary } from "src/types/models";
import { importIdAtom, importAtom, importsAtom, deleteImportAtom } from "src/atoms/import";
import { organizationAtom, organizationIdAtom } from "src/atoms/organization";
import { centsToUnits, unitsToCents, formatCents } from "src/utils/currency";
import { useDatePickerFormat } from "src/utils/date";
import ExchangeRateFields, {
  CurrencySelect,
  showExchangeRateFields,
} from "src/components/currency/currency-fields";
import ScrollShadow from "src/components/scroll-shadow";

const { TextArea } = Input;

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
  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);
  const orgCurrency = organization?.currency ?? "EUR";

  const [submitting, setSubmitting] = useState(false);
  const [summary, setSummary] = useState<ImportSummary | null>(null);

  const isVisible = get(location.state, "importModal", false);

  const importRecord = useMemo(() => {
    if (!importId) return null;
    return imports.find((x: any) => x.id === importId) ?? null;
  }, [imports, importId]);

  const watchedCurrency = Form.useWatch("currency", form);

  const handleClose = () => {
    setImportId(null);
    form.resetFields();
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
    } else if (!importId && isVisible) {
      form.resetFields();
      form.setFieldsValue({ date: dayjs() });
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
  }, [importId]);

  const canDelete = summary === null || summary.purchaseOrderCount === 0;

  return (
    <Drawer
      title={importId ? <Trans>Edit import</Trans> : <Trans>New import</Trans>}
      open={isVisible}
      placement="right"
      size={560}
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
        <Form form={form} layout="vertical" onFinish={handleSubmit}>
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
                <ExchangeRateFields currency={watchedCurrency} orgCurrency={orgCurrency} />
              )}
              <Col xs={24}>
                <Trans>
                  Prefill only — every purchase order linked to this import still stores and freezes
                  its own currency and rate when saved.
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

          {importId && summary && (
            <Card size="small" title={<Trans>Allocation</Trans>}>
              <Row gutter={[16, 16]}>
                <Col xs={12}>
                  <Statistic
                    title={<Trans>Linked purchase orders</Trans>}
                    value={summary.purchaseOrderCount}
                  />
                </Col>
                <Col xs={12}>
                  <Statistic
                    title={<Trans>Committed value</Trans>}
                    value={formatCents(summary.totalCommittedValue, orgCurrency, i18n.locale)}
                  />
                </Col>
                <Col xs={24}>
                  <Statistic
                    title={<Trans>Landed cost rate</Trans>}
                    value={`${(summary.landedCostRate * 100).toFixed(1)}%`}
                    valueStyle={{ fontSize: 20 }}
                  />
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
