import {
  Button,
  Card,
  Col,
  Divider,
  Form,
  Input,
  InputNumber,
  Popconfirm,
  Row,
  Select,
  Space,
  Typography,
} from "antd";
import { atom, useAtom, useSetAtom } from "jotai";
import { HomeOutlined, SaveOutlined } from "@ant-design/icons";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import isEmpty from "lodash/isEmpty";

import {
  organizationAtom,
  setOrganizationsAtom,
  deleteOrganizationAtom,
} from "src/atoms/organization";
import { DATE_FORMATS, type DateFormatKey, getDateFormatLabel } from "src/utils/date";
import { countries } from "src/utils/countries";

const { Title, Text } = Typography;
const { TextArea } = Input;

const submittingAtom = atom(false);

function SettingsOrganization() {
  useLingui();
  const [form] = Form.useForm();

  const setOrganizations = useSetAtom(setOrganizationsAtom);
  const deleteOrganization = useSetAtom(deleteOrganizationAtom);
  const [organization, setOrganization] = useAtom(organizationAtom);
  const [submitting, setSubmitting] = useAtom(submittingAtom);

  const onSubmit = async (values: object) => {
    setSubmitting(true);
    setOrganization(values);
    setOrganizations();
    setSubmitting(false);
  };

  const onDelete = () => {
    setSubmitting(true);
    deleteOrganization();
    setSubmitting(false);
  };

  if (isEmpty(organization)) return null;

  return (
    <div style={{ maxWidth: 720 }}>
      <Title level={4} style={{ marginTop: 0, marginBottom: 20 }}>
        <HomeOutlined style={{ marginRight: 8 }} />
        <Trans>Organization</Trans>
      </Title>

      <Form form={form} layout="vertical" onFinish={onSubmit} initialValues={organization}>
        <Card title={<Trans>Details</Trans>} style={{ marginBottom: 16 }}>
          <Text type="secondary" style={{ display: "block", marginBottom: 16 }}>
            <Trans>Basic information about your organization shown on invoices and documents.</Trans>
          </Text>
          <Row gutter={[16, 0]}>
            <Col xs={24} md={12}>
              <Form.Item
                label={t`Name`}
                name="name"
                rules={[{ required: true, message: t`This field is required!` }]}
              >
                <Input />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item label={t`Country`} name="country">
                <Select placeholder={t`Select country`} showSearch>
                  {countries.map((country) => (
                    <Select.Option key={country.name} value={country.name}>
                      {country.name}
                    </Select.Option>
                  ))}
                </Select>
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item label={t`E-mail`} name="email">
                <Input />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item label={t`Phone`} name="phone">
                <Input />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item label={t`Website`} name="website">
                <Input />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item label={t`Registration number`} name="registration_number">
                <Input />
              </Form.Item>
            </Col>
            <Col xs={24}>
              <Form.Item label={t`Address`} name="address">
                <TextArea rows={3} />
              </Form.Item>
            </Col>
          </Row>
        </Card>

        <Card title={<Trans>Banking</Trans>} style={{ marginBottom: 16 }}>
          <Row gutter={[16, 0]}>
            <Col xs={24} md={12}>
              <Form.Item label={t`Bank name`} name="bank_name">
                <Input />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item label={t`IBAN`} name="iban">
                <Input />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item label="VATIN" name="vatin">
                <Input />
              </Form.Item>
            </Col>
          </Row>
        </Card>

        <Card title={<Trans>Formatting</Trans>} style={{ marginBottom: 24 }}>
          <Row gutter={[16, 0]}>
            <Col xs={24} md={12}>
              <Form.Item label={t`Date format`} name="date_format">
                <Select placeholder={t`Select date format`}>
                  {Object.keys(DATE_FORMATS).map((key) => (
                    <Select.Option
                      key={key}
                      value={key === "AUTO" ? null : DATE_FORMATS[key as DateFormatKey]}
                    >
                      {getDateFormatLabel(key as DateFormatKey)}
                    </Select.Option>
                  ))}
                </Select>
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item label={t`Decimal places`} name="minimum_fraction_digits">
                <InputNumber min={0} max={10} style={{ width: "100%" }} />
              </Form.Item>
            </Col>
          </Row>
        </Card>

        <Divider />

        <Space>
          <Button
            type="primary"
            htmlType="submit"
            icon={<SaveOutlined />}
            loading={submitting}
          >
            <Trans>Save</Trans>
          </Button>
          <Popconfirm
            title={t`Are you sure delete this organization?`}
            onConfirm={onDelete}
            okText={t`Yes`}
            cancelText={t`No!`}
          >
            <Button danger loading={submitting}>
              <Trans>Delete organization</Trans>
            </Button>
          </Popconfirm>
        </Space>
      </Form>
    </div>
  );
}

export default SettingsOrganization;
