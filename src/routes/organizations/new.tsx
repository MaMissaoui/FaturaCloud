import { Form, Input, InputNumber, Select, Typography, Row, Col, Button, Card } from "antd";
import { CloseOutlined } from "@ant-design/icons";
import { atom, useAtom, useSetAtom, useAtomValue } from "jotai";
import { useNavigate } from "react-router";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import compact from "lodash/compact";
import map from "lodash/map";
import uniq from "lodash/uniq";

import { organizationAtom, organizationsAtom, organizationIdAtom } from "src/atoms/organization";
import { countries } from "src/utils/countries";

const { Title, Text } = Typography;

const submittingAtom = atom(false);
const currencies = compact(uniq(map(countries, "currency_code")));

const NewOrganization = () => {
  const navigate = useNavigate();
  const [form] = Form.useForm();

  // Atoms
  const setOrganization = useSetAtom(organizationAtom);
  const organizations = useAtomValue(organizationsAtom);
  const [submitting, setSubmitting] = useAtom(submittingAtom);
  const setOrganizationId = useSetAtom(organizationIdAtom);

  const handleSubmit = async (values: any) => {
    setSubmitting(true);
    // Clear organizationId to ensure we create a new organization
    setOrganizationId(null);
    await setOrganization(values);
    setSubmitting(false);
    // Navigate to the organization settings page after creation
    navigate("/settings/organization");
  };

  const handleCancel = () => {
    navigate("/");
  };

  return (
    <>
      <Row style={{ marginTop: 100 }}>
        <Col span={12} offset={6}>
          <Card>
            <Text type="secondary" style={{ fontWeight: 400 }}>
              <Trans>Add a new organization to your account</Trans>
            </Text>
            <Title level={3} style={{ margin: 0 }}>
              <Trans>New Organization</Trans>
            </Title>
            <Form
              form={form}
              layout="vertical"
              onFinish={handleSubmit}
              style={{ marginTop: 24 }}
              initialValues={{ minimum_fraction_digits: 2 }}
            >
              <Form.Item name="name" rules={[{ required: true, message: t`Please input name!` }]}>
                <Input placeholder={t`Name`} />
              </Form.Item>
              <Row gutter={16}>
                <Col span={12}>
                  <Form.Item name="country" label={t`Country`}>
                    <Select showSearch>
                      {countries.map((country) => (
                        <Select.Option key={country.name} value={country.name}>
                          {country.name}
                        </Select.Option>
                      ))}
                    </Select>
                  </Form.Item>
                </Col>
                <Col span={6}>
                  <Form.Item name="currency" label={t`Currency`}>
                    <Select showSearch>
                      {currencies.map((currency) => (
                        <Select.Option key={currency} value={currency}>
                          {currency}
                        </Select.Option>
                      ))}
                    </Select>
                  </Form.Item>
                </Col>
                <Col span={6}>
                  <Form.Item name="minimum_fraction_digits" label={t`Decimal places`}>
                    <InputNumber min={0} max={10} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
              </Row>
              <div style={{ display: "flex", justifyContent: "space-between" }}>
                <Button type="primary" htmlType="submit" disabled={submitting}>
                  <Trans>Create Organization</Trans>
                </Button>
                {organizations.length > 0 && (
                  <Button
                    type="default"
                    onClick={handleCancel}
                    disabled={submitting}
                    icon={<CloseOutlined />}
                  >
                    <Trans>Cancel</Trans>
                  </Button>
                )}
              </div>
            </Form>
          </Card>
        </Col>
      </Row>
    </>
  );
};

export default NewOrganization;
