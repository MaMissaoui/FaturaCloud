import {
  Button,
  Card,
  Col,
  Divider,
  Form,
  Input,
  InputNumber,
  Layout,
  Row,
  Select,
  Space,
  Typography,
  theme,
} from "antd";
import { atom, useAtom, useSetAtom } from "jotai";
import { CaretDownOutlined, CaretRightOutlined, FileTextOutlined, SaveOutlined } from "@ant-design/icons";
import { useState } from "react";
import { createPortal } from "react-dom";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import map from "lodash/map";
import isEmpty from "lodash/isEmpty";

import { organizationAtom, setOrganizationsAtom } from "src/atoms/organization";
import { currencies, getCurrencySymbol } from "src/utils/currencies";
import { validateInvoiceFormat, generateInvoiceNumber } from "src/utils/invoice";

const { Title, Text } = Typography;
const { TextArea } = Input;
const { Option } = Select;
const { Footer } = Layout;

const submittingAtom = atom(false);

function SettingsInvoice() {
  const [form] = Form.useForm();
  const { i18n } = useLingui();
  const { token } = theme.useToken();

  const setOrganizations = useSetAtom(setOrganizationsAtom);
  const [organization, setOrganization] = useAtom(organizationAtom);
  const [submitting, setSubmitting] = useAtom(submittingAtom);
  const [showVariables, setShowVariables] = useState(false);

  const invoiceFormat = Form.useWatch("invoiceNumberFormat", form);
  const getPreview = (format: string | undefined) => {
    const template = format || organization?.invoiceNumberFormat;
    if (!template) return "";
    const counter = (organization?.invoiceNumberCounter || 0) + 1;
    return generateInvoiceNumber(template, counter, new Date(), "AB");
  };
  const invoiceNumberPreview = getPreview(invoiceFormat);

  const onSubmit = async (values: object) => {
    setSubmitting(true);
    await setOrganization(values);
    await setOrganizations();
    setSubmitting(false);
  };

  if (isEmpty(organization)) return null;

  return (
    <div style={{ maxWidth: 1100 }}>
      <Title level={3} style={{ marginTop: 0, marginBottom: 12 }}>
        <FileTextOutlined style={{ marginRight: 8 }} />
        <Trans>Invoice settings</Trans>
      </Title>

      <Form form={form} layout="vertical" onFinish={onSubmit} initialValues={organization}>
        <Row gutter={[16, 0]}>
          <Col xs={24} xl={12}>
            <Card size="small" title={<Trans>Defaults</Trans>} style={{ marginBottom: 16 }}>
              <Row gutter={[16, 0]}>
                <Col xs={24} md={12}>
                  <Form.Item
                    label={t`Currency`}
                    name="currency"
                    rules={[{ required: true, message: t`This field is required!` }]}
                  >
                    <Select showSearch>
                      {map(currencies, (currency) => {
                        const symbol = getCurrencySymbol(i18n.locale, currency);
                        return (
                          <Option value={currency} key={currency}>
                            {`${currency}${currency !== symbol ? ` ${symbol}` : ""}`}
                          </Option>
                        );
                      })}
                    </Select>
                  </Form.Item>
                </Col>
                <Col xs={24} md={12}>
                  <Form.Item label={t`Decimal places`} name="minimum_fraction_digits">
                    <InputNumber min={0} max={10} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
                <Col xs={24} md={12}>
                  <Form.Item label={t`Due days`} name="due_days">
                    <InputNumber min={0} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
                <Col xs={24} md={12}>
                  <Form.Item
                    label={t`Overdue charge`}
                    help={<Trans>% per day</Trans>}
                    name="overdueCharge"
                  >
                    <InputNumber
                      min={0}
                      step={0.01}
                      style={{ width: "100%" }}
                      formatter={(value) => `${value} %`}
                      parser={(value) => value?.replace("%", "") as any}
                      placeholder="0%"
                    />
                  </Form.Item>
                </Col>
                <Col xs={24}>
                  <Form.Item label={t`Notes`} name="customerNotes">
                    <TextArea rows={2} />
                  </Form.Item>
                </Col>
              </Row>
            </Card>
          </Col>

          <Col xs={24} xl={12}>
            <Card size="small" title={<Trans>Numbering</Trans>} style={{ marginBottom: 16 }}>
              <Row gutter={[16, 0]}>
                <Col xs={24}>
                  <Form.Item
                    label={t`Invoice number format`}
                    name="invoiceNumberFormat"
                    rules={[
                      { required: true, message: t`This field is required!` },
                      {
                        validator: (_, value) => {
                          if (!value) return Promise.resolve();
                          const validation = validateInvoiceFormat(value);
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
                <Col xs={24}>
                  <Form.Item label={t`Preview`} style={{ marginBottom: 0 }}>
                    <Text code style={{ fontSize: 14 }}>
                      {invoiceNumberPreview || t`Enter format to see preview`}
                    </Text>
                  </Form.Item>
                </Col>
              </Row>

              <Button
                type="link"
                size="small"
                onClick={() => setShowVariables(!showVariables)}
                style={{ padding: 0, height: "auto", gap: 4, marginBottom: 8 }}
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
                    marginBottom: 16,
                  }}
                >
                  <Space direction="vertical" size={4} style={{ width: "100%" }}>
                    {[
                      ["{number}", <Trans key="n">Sequential number</Trans>],
                      [
                        "{year}",
                        <Trans key="y">{`4-digit year (${new Date().getFullYear()})`}</Trans>,
                      ],
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

              <Divider style={{ marginTop: 0 }} />

              <Row gutter={[16, 0]}>
                <Col xs={24} md={12}>
                  <Form.Item
                    label={t`Invoice number counter`}
                    name="invoiceNumberCounter"
                    help={t`Next invoice will use this number + 1`}
                    rules={[
                      { required: true, message: t`This field is required!` },
                      { type: "number", min: 0, message: t`Counter must be 0 or greater` },
                    ]}
                  >
                    <InputNumber min={0} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
              </Row>
            </Card>
          </Col>
        </Row>

        {/* 3-way matching policy for incoming (vendor) invoices. Zero means any
            variance is flagged and blocks approval until it's resolved or
            explicitly overridden with a reason. */}
        <Card size="small" title={<Trans>Vendor invoice matching</Trans>} style={{ marginBottom: 16 }}>
          <Row gutter={[16, 0]}>
            <Col xs={24} md={12}>
              <Form.Item
                name="match_price_tolerance_percent"
                label={<Trans>Price tolerance (%)</Trans>}
                extra={
                  <Trans>How far a vendor's unit price may differ from the purchase order.</Trans>
                }
              >
                <InputNumber min={0} max={100} precision={2} style={{ width: "100%" }} />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item
                name="match_quantity_tolerance_percent"
                label={<Trans>Quantity tolerance (%)</Trans>}
                extra={
                  <Trans>How far a billed quantity may exceed what was ordered and received.</Trans>
                }
              >
                <InputNumber min={0} max={100} precision={2} style={{ width: "100%" }} />
              </Form.Item>
            </Col>
          </Row>
        </Card>
      </Form>

      {/* Footer bar — portaled into the slot BaseLayout renders, so Save
          stays reachable regardless of viewport height or scroll position,
          matching the document detail pages (e.g. purchase-orders/details.tsx)
          rather than scrolling out of view as a trailing in-flow button. */}
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

export default SettingsInvoice;
