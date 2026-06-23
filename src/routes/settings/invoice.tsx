import {
  Button,
  Card,
  Col,
  Divider,
  Form,
  Input,
  InputNumber,
  Row,
  Select,
  Space,
  Typography,
  Upload,
  message,
} from "antd";
import { atom, useAtom, useSetAtom } from "jotai";
import {
  CaretDownOutlined,
  CaretRightOutlined,
  FileTextOutlined,
  SaveOutlined,
  UploadOutlined,
} from "@ant-design/icons";
import { useState } from "react";
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

const submittingAtom = atom(false);

function SettingsInvoice() {
  const [form] = Form.useForm();
  const { i18n } = useLingui();

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

  const onLogoUpload = (data: any) => {
    const file = data.file;
    const validTypes = ["image/png", "image/jpeg", "image/jpg"];
    if (!validTypes.includes(file.type)) {
      message.error(t`Please upload a PNG or JPEG image`);
      return;
    }
    const reader = new FileReader();
    reader.onload = function () {
      setOrganization({ ...organization, logo: reader.result });
    };
    reader.readAsDataURL(file);
  };

  if (isEmpty(organization)) return null;

  return (
    <div style={{ maxWidth: 720 }}>
      <Title level={4} style={{ marginTop: 0, marginBottom: 20 }}>
        <FileTextOutlined style={{ marginRight: 8 }} />
        <Trans>Invoice settings</Trans>
      </Title>

      <Form form={form} layout="vertical" onFinish={onSubmit} initialValues={organization}>
        <Card title={<Trans>Defaults</Trans>} style={{ marginBottom: 16 }}>
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
                <TextArea rows={3} />
              </Form.Item>
            </Col>
          </Row>
        </Card>

        <Card title={<Trans>Numbering</Trans>} style={{ marginBottom: 16 }}>
          <Row gutter={[16, 0]}>
            <Col xs={24} md={14}>
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
            <Col xs={24} md={10}>
              <Form.Item label={t`Preview`}>
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
            <div style={{ padding: "12px 16px", backgroundColor: "#f5f5f5", borderRadius: 4, marginBottom: 16 }}>
              <Space direction="vertical" size={4} style={{ width: "100%" }}>
                {[
                  ["{number}", <Trans key="n">Sequential number</Trans>],
                  ["{year}", <Trans key="y">{`4-digit year (${new Date().getFullYear()})`}</Trans>],
                  ["{y}", <Trans key="y2">{`2-digit year (${String(new Date().getFullYear() % 100).padStart(2, "0")})`}</Trans>],
                  ["{month}", <Trans key="mo">{`2-digit month (${String(new Date().getMonth() + 1).padStart(2, "0")})`}</Trans>],
                  ["{m}", <Trans key="m">{`Month name (${new Date().toLocaleString("en", { month: "short" })})`}</Trans>],
                  ["{day}", <Trans key="d">{`Day of month (${String(new Date().getDate()).padStart(2, "0")})`}</Trans>],
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

        <Card title={<Trans>Logo</Trans>} style={{ marginBottom: 24 }}>
          <Space direction="vertical" size={12}>
            {organization.logo && (
              <img
                src={organization.logo}
                alt="logo"
                style={{
                  maxWidth: 240,
                  maxHeight: 80,
                  objectFit: "contain",
                  border: "1px solid #d9d9d9",
                  borderRadius: 6,
                  padding: 8,
                  display: "block",
                }}
              />
            )}
            <Upload
              accept="image/png,image/jpeg,image/jpg"
              showUploadList={false}
              customRequest={(data) => onLogoUpload(data)}
            >
              <Button icon={<UploadOutlined />}>
                {organization.logo ? t`Change logo` : t`Upload logo`}
              </Button>
            </Upload>
            <Text type="secondary" style={{ fontSize: 12 }}>
              <Trans>PNG or JPEG, shown on invoices and delivery notes.</Trans>
            </Text>
          </Space>
        </Card>

        <Divider />

        <Button
          type="primary"
          htmlType="submit"
          icon={<SaveOutlined />}
          loading={submitting}
          style={{ marginBottom: 40 }}
        >
          <Trans>Save</Trans>
        </Button>
      </Form>
    </div>
  );
}

export default SettingsInvoice;
