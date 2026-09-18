import { useEffect, useState } from "react";
import { Button, Card, Col, Form, Input, InputNumber, Layout, Row, Typography, theme } from "antd";
import { useAtomValue } from "jotai";
import { createPortal } from "react-dom";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { OrderedListOutlined, SaveOutlined } from "@ant-design/icons";
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
    </Card>
  );
}

function SettingsDocumentNumbering() {
  const [form] = Form.useForm<FormValues>();
  const { token } = theme.useToken();
  const organization = useAtomValue(organizationAtom);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);

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
        settings.forEach((setting) => {
          values[setting.documentType] = { format: setting.format, counter: setting.counter };
        });
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
        documentNumberTypes.map((documentType) =>
          UpdateDocumentNumberSetting(
            organization.id,
            documentType,
            values[documentType].format,
            values[documentType].counter,
          ),
        ),
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
