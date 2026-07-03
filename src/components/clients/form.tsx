import { useEffect, useMemo, useState } from "react";
import { useLocation, useNavigate } from "react-router";
import { Button, Drawer, Form, Input, Popconfirm, Select, Space, theme, Typography } from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { DeleteOutlined } from "@ant-design/icons";
import get from "lodash/get";
import { GetClientInvoiceCount } from "src/api";

import { clientIdAtom, clientAtom, clientsAtom, deleteClientAtom } from "src/atoms/client";
import { generateClientCode } from "src/utils/client";

const Section = ({ children }: { children: React.ReactNode }) => {
  const { token } = theme.useToken();
  return (
    <Typography.Text
      strong
      style={{ color: token.colorPrimary, display: "block", marginBottom: 12, marginTop: 4 }}
    >
      {children}
    </Typography.Text>
  );
};

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
    navigate(location.pathname, { state: { clientModal: false } });
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
      width={480}
      onClose={handleClose}
      footer={
        <div style={{ display: "flex", justifyContent: "space-between" }}>
          <div>
            {clientId && (
              <Popconfirm
                title={
                  <div>
                    <div><Trans>Are you sure you want to delete this client?</Trans></div>
                    {invoiceCount !== null && invoiceCount > 0 && (
                      <div style={{ color: "#ff4d4f", marginTop: 4 }}>
                        <Trans>Warning: This will also delete {invoiceCount} related invoice(s).</Trans>
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
            <Button onClick={handleClose}><Trans>Cancel</Trans></Button>
            <Button type="primary" loading={submitting} onClick={() => form.submit()}>
              <Trans>Save</Trans>
            </Button>
          </Space>
        </div>
      }
    >
      <Form form={form} layout="vertical" onFinish={handleSubmit}>
        <Section><Trans>Contact</Trans></Section>
        <Form.Item name="name" label={<Trans>Name</Trans>} rules={[{ required: true, message: t`Please input name!` }]}>
          <Input
            placeholder={t`Name`}
            onChange={(e) => {
              if (!clientId) form.setFieldValue("code", generateClientCode(e.target.value));
            }}
          />
        </Form.Item>
        <Form.Item name="code" label={<Trans>Code</Trans>}>
          <Input placeholder={t`Code`} maxLength={10} />
        </Form.Item>
        <Form.Item name="emails" label={<Trans>E-mails</Trans>}>
          <Select placeholder={t`E-mails`} mode="tags" tokenSeparators={[",", ";"]} />
        </Form.Item>
        <Form.Item name="phone" label={<Trans>Phone</Trans>}>
          <Input placeholder={t`Phone`} />
        </Form.Item>
        <Form.Item name="website" label={<Trans>Website</Trans>}>
          <Input placeholder={t`Website`} />
        </Form.Item>
        <Form.Item name="vatin" label={<Trans>VAT Number</Trans>}>
          <Input placeholder={t`VAT Number`} />
        </Form.Item>

        <Section><Trans>Address</Trans></Section>
        <Form.Item name="address" label={<Trans>Address</Trans>}>
          <Input.TextArea rows={4} placeholder={t`Address`} />
        </Form.Item>
      </Form>
    </Drawer>
  );
};

export default ClientForm;
