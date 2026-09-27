import { useEffect, useMemo, useState } from "react";
import { useLocation, useNavigate } from "react-router";
import { Button, Drawer, Form, Input, Popconfirm, Space } from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { DeleteOutlined } from "@ant-design/icons";
import get from "lodash/get";

import {
  productFamilyIdAtom,
  productFamilyAtom,
  productFamiliesAtom,
  deleteProductFamilyAtom,
} from "src/atoms/product-family";

const ProductFamilyForm = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const [form] = Form.useForm();

  const [productFamilyId, setProductFamilyId] = useAtom(productFamilyIdAtom);
  const productFamilies = useAtomValue(productFamiliesAtom);
  const setProductFamily = useSetAtom(productFamilyAtom);
  const deleteProductFamily = useSetAtom(deleteProductFamilyAtom);
  const [submitting, setSubmitting] = useState(false);

  const isVisible = get(location.state, "productFamilyModal", false);

  const productFamily = useMemo(() => {
    if (!productFamilyId) return null;
    return productFamilies.find((f) => f.id === productFamilyId) ?? null;
  }, [productFamilies, productFamilyId]);

  const handleClose = () => {
    setProductFamilyId(null);
    form.resetFields();
    navigate(location.pathname, { state: { productFamilyModal: false } });
  };

  const handleSubmit = async (values: { name: string }) => {
    setSubmitting(true);
    try {
      await setProductFamily(values);
      handleClose();
    } catch {
      // setProductFamily already toasted the error — keep the drawer open
      // with the user's input intact rather than closing on a failed save.
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async () => {
    if (productFamilyId) {
      setSubmitting(true);
      const deleted = await deleteProductFamily(productFamilyId);
      if (deleted) handleClose();
      setSubmitting(false);
    }
  };

  useEffect(() => {
    const navId = get(location.state, "productFamilyId");
    if (isVisible && navId) {
      setProductFamilyId(navId);
    } else if (!isVisible) {
      setProductFamilyId(null);
      form.resetFields();
    }
  }, [isVisible, location.state, setProductFamilyId, form]);

  useEffect(() => {
    if (productFamily) {
      form.setFieldsValue({ ...productFamily });
    } else if (!productFamilyId) {
      form.resetFields();
    }
  }, [productFamily, productFamilyId, form]);

  return (
    <Drawer
      title={
        productFamilyId ? <Trans>Edit product family</Trans> : <Trans>New product family</Trans>
      }
      open={isVisible}
      placement="right"
      size={420}
      onClose={handleClose}
      footer={
        <div style={{ display: "flex", justifyContent: "space-between" }}>
          <div>
            {productFamilyId && (
              <Popconfirm
                title={<Trans>Are you sure you want to delete this product family?</Trans>}
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
          <Input placeholder={t`e.g. Washing machine, Refrigerator`} />
        </Form.Item>
      </Form>
    </Drawer>
  );
};

export default ProductFamilyForm;
