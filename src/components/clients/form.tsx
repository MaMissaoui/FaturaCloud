import { useEffect, useMemo, useState } from "react";
import { useLocation, useNavigate } from "react-router";
import {
  Button,
  Card,
  Col,
  Collapse,
  Drawer,
  Form,
  Input,
  Popconfirm,
  Row,
  Select,
  Space,
  theme,
} from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { DeleteOutlined } from "@ant-design/icons";
import get from "lodash/get";
import map from "lodash/map";
import { GetClientInvoiceCount } from "src/api";

import { clientIdAtom, clientAtom, clientsAtom, deleteClientAtom } from "src/atoms/client";
import { generateClientCode } from "src/utils/client";
import { currencies } from "src/utils/currencies";
import { useCountryOptions } from "src/hooks/useCountryOptions";
import ScrollShadow from "src/components/scroll-shadow";

// Which collapsed panel each field lives in, so a validation error on a
// hidden field opens its panel (a collapsed field's error is otherwise
// invisible — see docs/ui-consistency-plan.md's Tier 1.2 footgun note).
const PANEL_OF_FIELD: Record<string, string> = {
  address: "cashbook",
  phone2: "cashbook",
  phone3: "cashbook",
  identity_number: "cashbook",
  iban: "cashbook",
  guarantor: "cashbook",
  tax_number: "einvoicing",
  default_buyer_reference: "einvoicing",
};

const ClientForm = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const [form] = Form.useForm();
  const { token } = theme.useToken();

  const [clientId, setClientId] = useAtom(clientIdAtom);
  const clients = useAtomValue(clientsAtom);
  const setClient = useSetAtom(clientAtom);
  const [submitting, setSubmitting] = useState(false);
  const deleteClient = useSetAtom(deleteClientAtom);
  const [invoiceCount, setInvoiceCount] = useState<number | null>(null);
  const [activeKeys, setActiveKeys] = useState<string[]>([]);

  const isVisible = get(location.state, "clientModal", false);

  const client = useMemo(() => {
    if (!clientId) return null;
    const c = clients.find((x: any) => x.id === clientId);
    if (!c) return null;
    let emails: string[] = [];
    try {
      emails = c.emails ? JSON.parse(c.emails) : [];
    } catch {
      emails = [];
    }
    return { ...c, emails };
  }, [clients, clientId]);

  const watchedCountryCode = Form.useWatch("country_code", form);
  const countryOptions = useCountryOptions(client?.country_code);

  const handleClose = () => {
    setClientId(null);
    form.resetFields();
    setActiveKeys([]);
    navigate(location.pathname, { state: { clientModal: false } });
  };

  const handleFinishFailed = ({
    errorFields,
  }: {
    errorFields: { name: (string | number)[] }[];
  }) => {
    const panels = errorFields
      .map((field) => PANEL_OF_FIELD[String(field.name[0])])
      .filter(Boolean);
    if (panels.length) {
      setActiveKeys((keys) => [...new Set([...keys, ...panels])]);
    }
  };

  const handleSubmit = async (values: any) => {
    setSubmitting(true);
    try {
      await setClient(values);
      handleClose();
    } catch {
      // setClient already toasted the error — keep the drawer open with
      // the user's input intact rather than closing on a failed save.
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async () => {
    if (clientId) {
      setSubmitting(true);
      await deleteClient(clientId);
      handleClose();
      setSubmitting(false);
    }
  };

  useEffect(() => {
    const navClientId = get(location.state, "clientId");
    if (isVisible && navClientId) {
      setClientId(navClientId);
    } else if (!isVisible) {
      setClientId(null);
      form.resetFields();
    }
  }, [isVisible, location.state, setClientId, form]);

  useEffect(() => {
    if (client) {
      form.setFieldsValue(client);
    } else if (!clientId) {
      form.resetFields();
    }
  }, [client, clientId, form]);

  useEffect(() => {
    if (clientId) {
      GetClientInvoiceCount(clientId)
        .then(setInvoiceCount)
        .catch(() => setInvoiceCount(0));
    } else {
      setInvoiceCount(null);
    }
  }, [clientId]);

  return (
    <Drawer
      title={clientId ? <Trans>Edit client</Trans> : <Trans>New client</Trans>}
      open={isVisible}
      placement="right"
      size={640}
      onClose={handleClose}
      footer={
        <div style={{ display: "flex", justifyContent: "space-between" }}>
          <div>
            {clientId && (
              <Popconfirm
                title={
                  <div>
                    <div>
                      <Trans>Are you sure you want to delete this client?</Trans>
                    </div>
                    {invoiceCount !== null && invoiceCount > 0 && (
                      <div style={{ color: token.colorError, marginTop: 4 }}>
                        <Trans>
                          Warning: This will also delete {invoiceCount} related invoice(s).
                        </Trans>
                      </div>
                    )}
                  </div>
                }
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
          onFinishFailed={handleFinishFailed}
        >
          <Card size="small" title={<Trans>Contact</Trans>} style={{ marginBottom: 12 }}>
            <Row gutter={[16, 0]}>
              <Col xs={24} md={16}>
                <Form.Item
                  name="name"
                  label={<Trans>Name</Trans>}
                  rules={[{ required: true, message: t`Please input name!` }]}
                >
                  <Input
                    placeholder={t`Name`}
                    onChange={(e) => {
                      if (!clientId) form.setFieldValue("code", generateClientCode(e.target.value));
                    }}
                  />
                </Form.Item>
              </Col>
              <Col xs={24} md={8}>
                <Form.Item name="code" label={<Trans>Code</Trans>}>
                  <Input placeholder={t`Code`} maxLength={10} />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                {/* Tunisia's Matricule Fiscal (MF) is the same "this party's
                    tax ID" concept the vatin column already stores for
                    every other country — relabeled, not a new field, since
                    Tunisia has no VAT-number-shaped identifier to store
                    alongside it. */}
                {watchedCountryCode === "TN" ? (
                  <Form.Item
                    name="vatin"
                    label={<Trans>Matricule Fiscal (MF)</Trans>}
                    tooltip={<Trans>e.g. 1234567A/B/C/000</Trans>}
                  >
                    <Input placeholder={t`e.g. 1234567A/B/C/000`} />
                  </Form.Item>
                ) : (
                  <Form.Item name="vatin" label={<Trans>VAT Number</Trans>}>
                    <Input placeholder={t`VAT Number`} />
                  </Form.Item>
                )}
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="defaultCurrency" label={<Trans>Default currency</Trans>}>
                  <Select placeholder={t`Default currency`} allowClear showSearch>
                    {map(currencies, (currency) => (
                      <Select.Option value={currency} key={currency}>
                        {currency}
                      </Select.Option>
                    ))}
                  </Select>
                </Form.Item>
              </Col>
              <Col xs={24}>
                <Form.Item name="emails" label={<Trans>E-mails</Trans>}>
                  <Select placeholder={t`E-mails`} mode="tags" tokenSeparators={[",", ";"]} />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="phone" label={<Trans>Phone</Trans>}>
                  <Input placeholder={t`Phone`} />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item
                  name="website"
                  label={<Trans>Website</Trans>}
                  style={{ marginBottom: 0 }}
                >
                  <Input placeholder={t`Website`} />
                </Form.Item>
              </Col>
            </Row>
          </Card>

          <Card size="small" title={<Trans>Address</Trans>} style={{ marginBottom: 12 }}>
            <Row gutter={[16, 0]}>
              <Col xs={24} md={16}>
                <Form.Item name="street" label={<Trans>Street</Trans>}>
                  <Input placeholder={t`Street`} />
                </Form.Item>
              </Col>
              <Col xs={24} md={8}>
                <Form.Item name="house_number" label={<Trans>House number</Trans>}>
                  <Input placeholder={t`House number`} />
                </Form.Item>
              </Col>
              <Col xs={24} md={6}>
                <Form.Item
                  name="postal_code"
                  label={<Trans>Postal code</Trans>}
                  style={{ marginBottom: 0 }}
                >
                  <Input placeholder={t`Postal code`} />
                </Form.Item>
              </Col>
              <Col xs={24} md={10}>
                <Form.Item name="city" label={<Trans>City</Trans>} style={{ marginBottom: 0 }}>
                  <Input placeholder={t`City`} />
                </Form.Item>
              </Col>
              <Col xs={24} md={8}>
                <Form.Item
                  name="country_code"
                  label={<Trans>Country</Trans>}
                  style={{ marginBottom: 0 }}
                >
                  <Select
                    showSearch
                    allowClear
                    placeholder={t`Select a country`}
                    options={countryOptions}
                    filterOption={(input, option) =>
                      (option?.label ?? "").toLowerCase().includes(input.toLowerCase())
                    }
                  />
                </Form.Item>
              </Col>
            </Row>
          </Card>

          <Collapse
            size="small"
            activeKey={activeKeys}
            onChange={(keys) => setActiveKeys(keys as string[])}
            items={[
              {
                // Cash Book lookup/credit fields: collapsed by default so the
                // drawer fits without scrolling; every value still loads and
                // saves (forceRender), and an error inside opens the panel.
                key: "cashbook",
                label: <Trans>Cash Book</Trans>,
                forceRender: true,
                children: (
                  <Row gutter={[16, 0]}>
                    <Col xs={24}>
                      <Form.Item
                        name="address"
                        label={<Trans>Address</Trans>}
                        tooltip={
                          <Trans>
                            Free-text address for the Cash Book's quick "New customer" form. The
                            structured Address fields are what appears on documents.
                          </Trans>
                        }
                      >
                        <Input placeholder={t`Address`} />
                      </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                      <Form.Item name="phone2" label={<Trans>Phone 2</Trans>}>
                        <Input placeholder={t`Phone 2`} />
                      </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                      <Form.Item name="phone3" label={<Trans>Phone 3</Trans>}>
                        <Input placeholder={t`Phone 3`} />
                      </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                      <Form.Item
                        name="identity_number"
                        label={<Trans>Identity number</Trans>}
                        tooltip={
                          <Trans>
                            Personal ID/CIN card number — used to look up this client in Cash Book,
                            not a tax ID.
                          </Trans>
                        }
                      >
                        <Input placeholder={t`Identity number`} />
                      </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                      <Form.Item
                        name="iban"
                        label={<Trans>IBAN</Trans>}
                        tooltip={
                          <Trans>
                            Search/reference only — used to look up this client in Cash Book, not
                            this client's bank account for payments.
                          </Trans>
                        }
                      >
                        <Input placeholder={t`IBAN`} />
                      </Form.Item>
                    </Col>
                    <Col xs={24}>
                      <Form.Item
                        name="guarantor"
                        label={<Trans>Guarantor</Trans>}
                        style={{ marginBottom: 0 }}
                      >
                        <Input placeholder={t`Guarantor`} />
                      </Form.Item>
                    </Col>
                  </Row>
                ),
              },
              {
                key: "einvoicing",
                label: <Trans>E-invoicing</Trans>,
                forceRender: true,
                children: (
                  <Row gutter={[16, 0]}>
                    <Col xs={24} md={12}>
                      <Form.Item name="tax_number" label={<Trans>Tax number</Trans>}>
                        <Input placeholder={t`Tax number`} />
                      </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                      <Form.Item
                        name="default_buyer_reference"
                        label={<Trans>Default buyer reference</Trans>}
                        tooltip={t`Copied into new invoices for this client, e.g. a Leitweg-ID for German B2G.`}
                        style={{ marginBottom: 0 }}
                      >
                        <Input placeholder={t`Default buyer reference`} />
                      </Form.Item>
                    </Col>
                  </Row>
                ),
              },
            ]}
          />
        </Form>
      </ScrollShadow>
    </Drawer>
  );
};

export default ClientForm;
