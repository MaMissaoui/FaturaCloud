import { useEffect, useMemo, useState } from "react";
import { useLocation, useNavigate } from "react-router";
import {
  Button,
  Card,
  Col,
  Drawer,
  Form,
  Input,
  InputNumber,
  Popconfirm,
  Row,
  Select,
  Space,
  Typography,
} from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { DeleteOutlined } from "@ant-design/icons";
import get from "lodash/get";
import map from "lodash/map";
import { GetVendorDocumentCount } from "src/api";

import { vendorIdAtom, vendorAtom, vendorsAtom, deleteVendorAtom } from "src/atoms/vendor";
import { generateClientCode } from "src/utils/client";
import { currencies } from "src/utils/currencies";
import ScrollShadow from "src/components/scroll-shadow";

const VendorForm = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const [form] = Form.useForm();

  const [vendorId, setVendorId] = useAtom(vendorIdAtom);
  const vendors = useAtomValue(vendorsAtom);
  const setVendor = useSetAtom(vendorAtom);
  const [submitting, setSubmitting] = useState(false);
  const deleteVendor = useSetAtom(deleteVendorAtom);
  const [documentCount, setDocumentCount] = useState<number | null>(null);

  const isVisible = get(location.state, "vendorModal", false);

  const vendor = useMemo(() => {
    if (!vendorId) return null;
    const v = vendors.find((x: any) => x.id === vendorId);
    if (!v) return null;
    let emails: string[] = [];
    try {
      emails = v.emails ? JSON.parse(v.emails) : [];
    } catch {
      emails = [];
    }
    return { ...v, emails };
  }, [vendors, vendorId]);

  const handleClose = () => {
    setVendorId(null);
    form.resetFields();
    navigate(location.pathname, { state: { vendorModal: false } });
  };

  const handleSubmit = async (values: any) => {
    setSubmitting(true);
    await setVendor(values);
    setVendorId(null);
    navigate(location.pathname, { state: { vendorModal: false } });
    form.resetFields();
    setSubmitting(false);
  };

  const handleDelete = async () => {
    if (vendorId) {
      setSubmitting(true);
      const deleted = await deleteVendor(vendorId);
      // Deletion is refused while purchasing documents still reference the
      // vendor; keep the drawer open so the message stays in context.
      if (deleted) handleClose();
      setSubmitting(false);
    }
  };

  useEffect(() => {
    const navVendorId = get(location.state, "vendorId");
    if (isVisible && navVendorId) {
      setVendorId(navVendorId);
    } else if (!isVisible) {
      setVendorId(null);
      form.resetFields();
    }
  }, [isVisible, location.state, setVendorId, form]);

  useEffect(() => {
    if (vendor) {
      form.setFieldsValue(vendor);
    } else if (!vendorId) {
      form.resetFields();
    }
  }, [vendor, vendorId, form]);

  useEffect(() => {
    if (vendorId) {
      GetVendorDocumentCount(vendorId)
        .then(setDocumentCount)
        .catch(() => setDocumentCount(0));
    } else {
      setDocumentCount(null);
    }
  }, [vendorId]);

  const canDelete = documentCount === 0;

  return (
    <Drawer
      title={vendorId ? <Trans>Edit vendor</Trans> : <Trans>New vendor</Trans>}
      open={isVisible}
      placement="right"
      size={640}
      onClose={handleClose}
      footer={
        <div style={{ display: "flex", justifyContent: "space-between" }}>
          <div>
            {vendorId && canDelete && (
              <Popconfirm
                title={<Trans>Are you sure you want to delete this vendor?</Trans>}
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
            {vendorId && documentCount !== null && documentCount > 0 && (
              <Typography.Text type="secondary">
                <Trans>Used by {documentCount} document(s)</Trans>
              </Typography.Text>
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
      <ScrollShadow>
        <Form form={form} layout="vertical" onFinish={handleSubmit}>
          <Card title={<Trans>Contact</Trans>} style={{ marginBottom: 12 }}>
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
                      if (!vendorId) form.setFieldValue("code", generateClientCode(e.target.value));
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
              <Col xs={24}>
                <Form.Item name="address" label={<Trans>Address</Trans>} style={{ marginBottom: 0 }}>
                  <Input.TextArea rows={2} placeholder={t`Address`} />
                </Form.Item>
              </Col>
            </Row>
          </Card>

          <Card title={<Trans>Purchasing</Trans>} style={{ marginBottom: 16 }}>
            <Row gutter={[16, 0]}>
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
              <Col xs={24} md={12}>
                <Form.Item name="paymentTermsDays" label={<Trans>Payment terms (days)</Trans>}>
                  <InputNumber
                    placeholder={t`Payment terms (days)`}
                    min={0}
                    precision={0}
                    style={{ width: "100%" }}
                  />
                </Form.Item>
              </Col>
            </Row>
          </Card>
        </Form>
      </ScrollShadow>
    </Drawer>
  );
};

export default VendorForm;
