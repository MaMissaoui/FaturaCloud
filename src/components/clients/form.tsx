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
} from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { DeleteOutlined } from "@ant-design/icons";
import get from "lodash/get";
import { GetClientInvoiceCount } from "src/api";

import { clientIdAtom, clientAtom, clientsAtom, deleteClientAtom } from "src/atoms/client";
import { generateClientCode } from "src/utils/client";
import ScrollShadow from "src/components/scroll-shadow";

// Fields inside the collapsed "E-invoicing" panel — used to auto-expand it if
// validation fails on a field the user can't currently see.
const E_INVOICING_FIELDS = [
  "tax_number",
  "street",
  "house_number",
  "postal_code",
  "city",
  "country_code",
  "default_buyer_reference",
];

const ClientForm = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const [form] = Form.useForm();

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

  const handleClose = () => {
    setClientId(null);
    form.resetFields();
    setActiveKeys([]);
    navigate(location.pathname, { state: { clientModal: false } });
  };

  const handleFinishFailed = ({ errorFields }: { errorFields: { name: (string | number)[] }[] }) => {
    const hasHiddenError = errorFields.some((field) => E_INVOICING_FIELDS.includes(String(field.name[0])));
    if (hasHiddenError) {
      setActiveKeys((keys) => (keys.includes("einvoicing") ? keys : [...keys, "einvoicing"]));
    }
  };

  const handleSubmit = async (values: any) => {
    setSubmitting(true);
    await setClient(values);
    setClientId(null);
    navigate(location.pathname, { state: { clientModal: false } });
    form.resetFields();
    setSubmitting(false);
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
                      <div style={{ color: "#ff4d4f", marginTop: 4 }}>
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
        <Form form={form} layout="vertical" onFinish={handleSubmit} onFinishFailed={handleFinishFailed}>
          <Card title={<Trans>Contact</Trans>} style={{ marginBottom: 16 }}>
            <Row gutter={[16, 0]}>
              <Col xs={24}>
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
              <Col xs={24} md={12}>
                <Form.Item name="code" label={<Trans>Code</Trans>}>
                  <Input placeholder={t`Code`} maxLength={10} />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="vatin" label={<Trans>VAT Number</Trans>}>
                  <Input placeholder={t`VAT Number`} />
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
                <Form.Item name="website" label={<Trans>Website</Trans>}>
                  <Input placeholder={t`Website`} />
                </Form.Item>
              </Col>
            </Row>
          </Card>

          <Card title={<Trans>Address</Trans>} style={{ marginBottom: 16 }}>
            <Form.Item name="address" label={<Trans>Address</Trans>} style={{ marginBottom: 0 }}>
              <Input.TextArea rows={3} placeholder={t`Address`} />
            </Form.Item>
          </Card>

          <Collapse
            activeKey={activeKeys}
            onChange={(keys) => setActiveKeys(keys as string[])}
            items={[
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
                        name="country_code"
                        label={<Trans>Country code</Trans>}
                        rules={[{ len: 2, message: t`Use the 2-letter ISO 3166-1 code!` }]}
                      >
                        <Input
                          maxLength={2}
                          placeholder={t`e.g. DE`}
                          onChange={(e) =>
                            form.setFieldValue("country_code", e.target.value.toUpperCase())
                          }
                        />
                      </Form.Item>
                    </Col>
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
                    <Col xs={24} md={8}>
                      <Form.Item name="postal_code" label={<Trans>Postal code</Trans>}>
                        <Input placeholder={t`Postal code`} />
                      </Form.Item>
                    </Col>
                    <Col xs={24} md={16}>
                      <Form.Item name="city" label={<Trans>City</Trans>}>
                        <Input placeholder={t`City`} />
                      </Form.Item>
                    </Col>
                    <Col xs={24}>
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
