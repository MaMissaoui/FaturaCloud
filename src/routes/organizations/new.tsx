import { Alert, Form, Input, Select, Typography, Row, Col, Button, Card } from "antd";
import NumberInput from "src/components/number-input";
import { CloseOutlined, LogoutOutlined } from "@ant-design/icons";
import { atom, useAtom, useSetAtom, useAtomValue } from "jotai";
import { useEffect, useMemo } from "react";
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
import { logoutAtom } from "src/atoms/session";
import { CreateOrganization } from "src/api";
import { countries } from "src/utils/countries";
import { getDefaultFractionDigits } from "src/utils/currencies";
import { message } from "src/utils/message";
import { defaultTimezone, timezoneOptions } from "src/utils/timezones";

const { Title, Text } = Typography;

const submittingAtom = atom(false);
const currencies = compact(uniq(map(countries, "currency_code")));

const NewOrganization = () => {
  const navigate = useNavigate();
  const [form] = Form.useForm();
  const watchedTimezone = Form.useWatch("timezone", form);
  // ~420 zones; rebuilt only when the picked value changes (see the
  // Organizations drawer's identical select).
  const timezoneSelectOptions = useMemo(() => timezoneOptions(watchedTimezone), [watchedTimezone]);

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

  // A user who isn't a member of any organization yet lands here with no
  // Cancel (there's nowhere to go back to) and no layout header — logging
  // out is the way off this screen (issue #414).
  const logout = useSetAtom(logoutAtom);
  const hasOrganizations = organizations.length > 0;
  const handleLogout = () => {
    navigate("/login");
    logout();
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
            {!hasOrganizations && (
              <Alert
                type="info"
                showIcon
                style={{ marginTop: 16 }}
                message={<Trans>You are not a member of any organization yet</Trans>}
                description={
                  <Trans>
                    Ask an administrator to add you to their organization, then sign in again. Or
                    create your own organization below.
                  </Trans>
                }
              />
            )}
            <Form
              form={form}
              layout="vertical"
              onFinish={handleSubmit}
              style={{ marginTop: 24 }}
              // Prefilled with the browser's zone, like the Organizations
              // drawer: the server never infers one, and without it every
              // picked date reads a day early east of UTC (audit F156).
              initialValues={{ minimum_fraction_digits: 2, timezone: defaultTimezone() }}
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
                    <NumberInput min={0} max={10} style={{ width: "100%" }} />
                  </Form.Item>
                </Col>
              </Row>
              <Form.Item
                name="timezone"
                label={t`Time zone`}
                tooltip={t`The time zone this organization works in. Exported documents, accounting exports, document numbers and the Cash Book's daily totals use it to decide which day a date falls on.`}
              >
                <Select showSearch optionFilterProp="label" options={timezoneSelectOptions} />
              </Form.Item>
              <div style={{ display: "flex", justifyContent: "space-between" }}>
                <Button type="primary" htmlType="submit" disabled={submitting}>
                  <Trans>Create Organization</Trans>
                </Button>
                {hasOrganizations ? (
                  <Button
                    type="default"
                    onClick={handleCancel}
                    disabled={submitting}
                    icon={<CloseOutlined />}
                  >
                    <Trans>Cancel</Trans>
                  </Button>
                ) : (
                  <Button
                    type="default"
                    onClick={handleLogout}
                    disabled={submitting}
                    icon={<LogoutOutlined />}
                  >
                    <Trans>Sign out</Trans>
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
