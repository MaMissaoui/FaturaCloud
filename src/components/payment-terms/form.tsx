import { useEffect, useMemo, useState } from "react";
import { useLocation, useNavigate } from "react-router";
import { Button, Checkbox, Drawer, Form, Input, Popconfirm, Space } from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { DeleteOutlined } from "@ant-design/icons";
import get from "lodash/get";

import {
  paymentTermIdAtom,
  paymentTermAtom,
  paymentTermsAtom,
  deletePaymentTermAtom,
} from "src/atoms/payment-term";

const PaymentTermForm = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const [form] = Form.useForm();

  const [paymentTermId, setPaymentTermId] = useAtom(paymentTermIdAtom);
  const paymentTerms = useAtomValue(paymentTermsAtom);
  const setPaymentTerm = useSetAtom(paymentTermAtom);
  const deletePaymentTerm = useSetAtom(deletePaymentTermAtom);
  const [submitting, setSubmitting] = useState(false);

  const isVisible = get(location.state, "paymentTermModal", false);

  const paymentTerm = useMemo(() => {
    if (!paymentTermId) return null;
    return paymentTerms.find((pt: any) => pt.id === paymentTermId) ?? null;
  }, [paymentTerms, paymentTermId]);

  const handleClose = () => {
    setPaymentTermId(null);
    form.resetFields();
    navigate(location.pathname, { state: { paymentTermModal: false } });
  };

  const handleSubmit = async (values: any) => {
    setSubmitting(true);
    try {
      await setPaymentTerm(values);
      handleClose();
    } catch {
      // setPaymentTerm already toasted the error — keep the drawer open
      // with the user's input intact rather than closing on a failed save.
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async () => {
    if (paymentTermId) {
      setSubmitting(true);
      const deleted = await deletePaymentTerm(paymentTermId);
      if (deleted) handleClose();
      setSubmitting(false);
    }
  };

  useEffect(() => {
    const navId = get(location.state, "paymentTermId");
    if (isVisible && navId) {
      setPaymentTermId(navId);
    } else if (!isVisible) {
      setPaymentTermId(null);
      form.resetFields();
    }
  }, [isVisible, location.state, setPaymentTermId, form]);

  useEffect(() => {
    if (paymentTerm) {
      form.setFieldsValue({ ...paymentTerm, isDefault: !!paymentTerm.isDefault });
    } else if (!paymentTermId) {
      form.resetFields();
    }
  }, [paymentTerm, paymentTermId, form]);

  return (
    <Drawer
      title={paymentTermId ? <Trans>Edit payment term</Trans> : <Trans>New payment term</Trans>}
      open={isVisible}
      placement="right"
      size={420}
      onClose={handleClose}
      footer={
        <div style={{ display: "flex", justifyContent: "space-between" }}>
          <div>
            {paymentTermId && (
              <Popconfirm
                title={<Trans>Are you sure you want to delete this payment term?</Trans>}
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
      <Form form={form} layout="vertical" onFinish={handleSubmit}>
        <Form.Item
          name="name"
          label={<Trans>Name</Trans>}
          rules={[{ required: true, message: t`This field is required!` }]}
        >
          <Input placeholder={t`e.g. Net 30`} />
        </Form.Item>
        <Form.Item name="isDefault" valuePropName="checked">
          <Checkbox>
            <Trans>Default for new invoices</Trans>
          </Checkbox>
        </Form.Item>
      </Form>
    </Drawer>
  );
};

export default PaymentTermForm;
