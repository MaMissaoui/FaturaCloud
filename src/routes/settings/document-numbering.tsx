import { useEffect, useRef, useState } from "react";
import {
  Button,
  Card,
  Col,
  Form,
  Input,
  InputNumber,
  Layout,
  Row,
  Space,
  Typography,
  theme,
} from "antd";
import { useAtomValue } from "jotai";
import { createPortal } from "react-dom";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import {
  CaretDownOutlined,
  CaretRightOutlined,
  OrderedListOutlined,
  SaveOutlined,
} from "@ant-design/icons";
import isEmpty from "lodash/isEmpty";

import { organizationAtom } from "src/atoms/organization";
import { GetDocumentNumberSetting, UpdateDocumentNumberSetting } from "src/api";
import { message } from "src/utils/message";
import { generateDocumentNumber, validateDocumentNumberFormat } from "src/utils/document-number";

const { Title, Text } = Typography;
const { Footer } = Layout;

// The five document types with a document_number_settings row — invoices
// keep their own numbering fields on Settings -> Invoice (see
// db/document_number.go for why they weren't folded into this table).
const documentNumberTypes = [
  "order",
  "purchase_order",
  "delivery",
  "inbound_delivery",
  "production_order",
] as const;

// A function, not a module-level map, so the label re-evaluates against the
// active locale at render time rather than freezing whatever locale was
// active when this module first loaded (lingui's t macro reads i18n.locale
// live, but only if called at call time, not at import time).
function documentTypeLabel(documentType: string): string {
  switch (documentType) {
    case "order":
      return t`Order`;
    case "purchase_order":
      return t`Purchase Order`;
    case "delivery":
      return t`Outbound Delivery`;
    case "inbound_delivery":
      return t`Inbound Delivery`;
    case "production_order":
      return t`Production Order`;
    default:
      return documentType;
  }
}

type FormValues = Record<string, { format: string; counter: number }>;

function DocumentNumberingCard({ documentType, label }: { documentType: string; label: string }) {
  const form = Form.useFormInstance<FormValues>();
  const { token } = theme.useToken();
  const [showVariables, setShowVariables] = useState(false);
  const format = Form.useWatch([documentType, "format"], form);
  const counter = Form.useWatch([documentType, "counter"], form);
  const preview =
    format && validateDocumentNumberFormat(format).isValid
      ? generateDocumentNumber(format, (counter || 0) + 1, new Date(), "AB")
      : "";

  return (
    <Card size="small" title={label} style={{ marginBottom: 16 }}>
      <Row gutter={[16, 0]}>
        <Col xs={24} md={12}>
          <Form.Item
            label={t`Number format`}
            name={[documentType, "format"]}
            rules={[
              { required: true, message: t`This field is required!` },
              {
                validator: (_, value) => {
                  if (!value) return Promise.resolve();
                  const validation = validateDocumentNumberFormat(value);
                  return validation.isValid
                    ? Promise.resolve()
                    : Promise.reject(new Error(validation.error));
                },
              },
            ]}
          >
            <Input />
          </Form.Item>
        </Col>
        <Col xs={24} md={12}>
          <Form.Item
            label={t`Counter`}
            name={[documentType, "counter"]}
            help={t`Next document will use this number + 1`}
            rules={[{ type: "number", min: 0, message: t`Counter must be 0 or greater` }]}
          >
            <InputNumber min={0} style={{ width: "100%" }} />
          </Form.Item>
        </Col>
        <Col xs={24}>
          <Form.Item label={t`Preview`} style={{ marginBottom: 0 }}>
            <Text code style={{ fontSize: 14 }}>
              {preview || t`Enter a format to see a preview`}
            </Text>
          </Form.Item>
        </Col>
      </Row>

      <Button
        type="link"
        size="small"
        onClick={() => setShowVariables(!showVariables)}
        style={{ padding: 0, height: "auto", gap: 4 }}
      >
        {showVariables ? <CaretDownOutlined /> : <CaretRightOutlined />}
        <Trans>Available variables</Trans>
      </Button>
      {showVariables && (
        <div
          style={{
            padding: "12px 16px",
            backgroundColor: token.colorFillAlter,
            borderRadius: 4,
            marginTop: 8,
          }}
        >
          <Space direction="vertical" size={4} style={{ width: "100%" }}>
            {[
              ["{number}", <Trans key="n">Sequential number</Trans>],
              [
                "{number:N}",
                <Trans key="np">{`Zero-padded sequential number (e.g. {number:4} → 0007)`}</Trans>,
              ],
              ["{year}", <Trans key="y">{`4-digit year (${new Date().getFullYear()})`}</Trans>],
              [
                "{y}",
                <Trans key="y2">{`2-digit year (${String(new Date().getFullYear() % 100).padStart(2, "0")})`}</Trans>,
              ],
              [
                "{month}",
                <Trans key="mo">{`2-digit month (${String(new Date().getMonth() + 1).padStart(2, "0")})`}</Trans>,
              ],
              [
                "{m}",
                <Trans key="m">{`Month name (${new Date().toLocaleString("en", { month: "short" })})`}</Trans>,
              ],
              [
                "{day}",
                <Trans key="d">{`Day of month (${String(new Date().getDate()).padStart(2, "0")})`}</Trans>,
              ],
              ["{clientCode}", <Trans key="cc">Client code (e.g. AP, MS)</Trans>],
            ].map(([code, desc]) => (
              <div key={String(code)}>
                <Text code>{code}</Text> — {desc}
              </div>
            ))}
          </Space>
        </div>
      )}
    </Card>
  );
}

function SettingsDocumentNumbering() {
  const [form] = Form.useForm<FormValues>();
  const { token } = theme.useToken();
  const organization = useAtomValue(organizationAtom);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  // What the server reported the counter was when this page loaded. The
  // counter is only sent back on save if the user actually changed it —
  // otherwise an admin who opened this page while someone was creating a
  // document would rewind the counter on a format-only edit (see
  // db/document_number.go for the server-side half of this fix).
  const loadedCounters = useRef<Record<string, number>>({});

  useEffect(() => {
    if (!organization?.id) return;
    setLoading(true);
    Promise.all(
      documentNumberTypes.map((documentType) =>
        GetDocumentNumberSetting(organization.id, documentType),
      ),
    )
      .then((settings) => {
        const values: FormValues = {};
        const counters: Record<string, number> = {};
        settings.forEach((setting) => {
          values[setting.documentType] = { format: setting.format, counter: setting.counter };
          counters[setting.documentType] = setting.counter;
        });
        loadedCounters.current = counters;
        form.setFieldsValue(values);
      })
      .catch((error) => {
        message.error(
          error instanceof Error ? error.message : t`Failed to load numbering settings`,
        );
      })
      .finally(() => setLoading(false));
    // eslint-disable-next-line react-hooks/exhaustive-deps -- form is stable
  }, [organization?.id]);

  if (isEmpty(organization)) return null;

  const onSubmit = async (values: FormValues) => {
    setSubmitting(true);
    try {
      await Promise.all(
        documentNumberTypes.map((documentType) => {
          const counter = values[documentType].counter;
          // Only send the counter when the user actually changed it. An
          // untouched counter is omitted so the server leaves it alone —
          // otherwise saving a format edit would rewind a counter that
          // advanced after this page loaded.
          const counterChanged = counter !== loadedCounters.current[documentType];
          return UpdateDocumentNumberSetting(
            organization.id,
            documentType,
            values[documentType].format,
            counterChanged ? counter : undefined,
          );
        }),
      );
      message.success(t`Numbering settings saved`);
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Failed to save numbering settings`);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div style={{ maxWidth: 1100 }}>
      <Title level={3} style={{ marginTop: 0, marginBottom: 12 }}>
        <OrderedListOutlined style={{ marginRight: 8 }} />
        <Trans>Document Numbering</Trans>
      </Title>
      <Text type="secondary" style={{ display: "block", marginBottom: 16 }}>
        <Trans>
          The number format and next counter for each document type. Invoice numbering is configured
          separately under Invoice settings.
        </Trans>
      </Text>

      {!loading && (
        <Form form={form} layout="vertical" onFinish={onSubmit}>
          <Row gutter={[16, 0]}>
            {documentNumberTypes.map((documentType) => (
              <Col xs={24} xl={12} key={documentType}>
                <DocumentNumberingCard
                  documentType={documentType}
                  label={documentTypeLabel(documentType)}
                />
              </Col>
            ))}
          </Row>
        </Form>
      )}

      {document.getElementById("footer") &&
        createPortal(
          <Footer
            style={{
              position: "sticky",
              bottom: 0,
              zIndex: 1,
              padding: "0 16px",
              background: token.colorBgContainer,
            }}
          >
            <Row align="middle" justify="end" style={{ height: 64 }}>
              <Col>
                <Button
                  type="primary"
                  icon={<SaveOutlined />}
                  loading={submitting}
                  onClick={() => form.submit()}
                >
                  <Trans>Save</Trans>
                </Button>
              </Col>
            </Row>
          </Footer>,
          document.getElementById("footer") as HTMLElement,
        )}
    </div>
  );
}

export default SettingsDocumentNumbering;
