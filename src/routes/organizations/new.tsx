import { Form, Input, InputNumber, Select, Typography, Row, Col, Button, Card } from "antd";
import { CloseOutlined } from "@ant-design/icons";
import { atom, useAtom, useSetAtom, useAtomValue } from "jotai";
import { useEffect } from "react";
import { useNavigate } from "react-router";
import { nanoid } from "nanoid";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import compact from "lodash/compact";
import map from "lodash/map";
import uniq from "lodash/uniq";

import {
  organizationsAtom,
  organizationIdAtom,
  setOrganizationsAtom,
  isCashbookAtom,
} from "src/atoms/organization";
import { CreateOrganization } from "src/api";
import { countries } from "src/utils/countries";
import { getDefaultFractionDigits } from "src/utils/currencies";
import { message } from "src/utils/message";

const { Title, Text } = Typography;

const submittingAtom = atom(false);
const currencies = compact(uniq(map(countries, "currency_code")));

const NewOrganization = () => {
  const navigate = useNavigate();
  const [form] = Form.useForm();

  // Atoms
  const organizations = useAtomValue(organizationsAtom);
  const [submitting, setSubmitting] = useAtom(submittingAtom);
  const setOrganizations = useSetAtom(setOrganizationsAtom);
  const setOrganizationId = useSetAtom(organizationIdAtom);

  // The restricted cashbook role has no business creating organizations —
  // bounce it back to the counter screen (the org switcher's "New
  // organization" entry is already hidden for it).
  const isCashbook = useAtomValue(isCashbookAtom);
  useEffect(() => {
    if (isCashbook) navigate("/cash-book", { replace: true });
  }, [isCashbook, navigate]);

  // Calls the API directly rather than going through organizationAtom:
  // that atom swallows a failed save (it toasts and returns), so this page
  // would navigate to the settings screen even when nothing was created.
  // Keeping the error here lets it stay on the form and surface the reason,
  // the same shape the organizations list's edit drawer uses.
  const handleSubmit = async (values: any) => {
    setSubmitting(true);
    try {
      const newOrg = await CreateOrganization({
        ...values,
        id: nanoid(),
        currency: values.currency || "EUR",
        minimum_fraction_digits: values.minimum_fraction_digits ?? 2,
        due_days: 7,
        overdueCharge: 0,
        invoiceNumberFormat: "#{number}",
        invoiceNumberCounter: 0,
      });
      setOrganizations();
      setOrganizationId(newOrg.id);
      message.success(t`Organization created`);
      // Navigate to the organization settings page after creation
      navigate("/settings/organization");
    } catch (error) {
      console.error("Failed to create organization:", error);
      message.error(error instanceof Error ? error.message : t`Organization creation failed`);
    } finally {
      setSubmitting(false);
    }
  };

  const handleCancel = () => {
    navigate("/");
  };

  return (
    <>
      <Row style={{ marginTop: 100 }} justify="center">
        <Col xs={24} md={16} lg={12} style={{ padding: "0 16px" }}>
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
              <Form.Item
                name="name"
                label={t`Name`}
                rules={[{ required: true, message: t`Please input name!` }]}
              >
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
                    <Select
                      showSearch
                      onChange={(currency: string) =>
                        form.setFieldValue(
                          "minimum_fraction_digits",
                          getDefaultFractionDigits(currency),
                        )
                      }
                    >
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
