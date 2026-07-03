import { useEffect, useMemo, useState } from "react";
import { useLocation, useNavigate } from "react-router";
import { Button, Drawer, Form, Input, InputNumber, Popconfirm, Select, Space, Switch, theme, Typography } from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { DeleteOutlined } from "@ant-design/icons";
import get from "lodash/get";

import { productIdAtom, productAtom, productsAtom, deleteProductAtom } from "src/atoms/product";
import { taxRatesAtom, setTaxRatesAtom } from "src/atoms/tax-rate";
import ScrollShadow from "src/components/scroll-shadow";

const UNIT_OPTIONS = ["hour", "day", "week", "month", "piece", "kg", "g", "lb", "oz", "l", "ml", "m", "km"];

// Derives a product code from its name (e.g. "Steel Bracket" -> "STEEL-BRACKET"),
// appending "-2", "-3", ... if that code is already used by another product.
const deriveProductCode = (name: string, existingCodes: Set<string>): string => {
  const base = name.toUpperCase().replace(/[^A-Z0-9]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 24) || "PRODUCT";
  if (!existingCodes.has(base)) return base;
  let suffix = 2;
  while (existingCodes.has(`${base}-${suffix}`)) suffix += 1;
  return `${base}-${suffix}`;
};

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

const ProductForm = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const [form] = Form.useForm();

  const [productId, setProductId] = useAtom(productIdAtom);
  const products = useAtomValue(productsAtom);
  const setProduct = useSetAtom(productAtom);
  const [submitting, setSubmitting] = useState(false);
  const deleteProduct = useSetAtom(deleteProductAtom);
  const [codeTouched, setCodeTouched] = useState(false);
  const nameValue = Form.useWatch("name", form);

  const taxRates = useAtomValue(taxRatesAtom);
  const setTaxRates = useSetAtom(setTaxRatesAtom);

  const isVisible = get(location.state, "productModal", false);

  const product = useMemo(() => {
    if (!productId) return null;
    return products.find((p: any) => p.id === productId) ?? null;
  }, [products, productId]);

  useEffect(() => {
    if (isVisible) setTaxRates();
  }, [isVisible, setTaxRates]);

  useEffect(() => {
    const navProductId = get(location.state, "productId");
    if (isVisible && navProductId) {
      setProductId(navProductId);
    } else if (!isVisible) {
      setProductId(null);
      form.resetFields();
    }
  }, [isVisible, location.state, setProductId, form]);

  // Reset the "did the user type their own code" flag each time the drawer
  // opens for a brand-new product, so auto-propose kicks back in.
  useEffect(() => {
    if (isVisible && !productId) setCodeTouched(false);
  }, [isVisible, productId]);

  // Propose a code derived from the name for new products, unless the user
  // has already typed one in themselves.
  useEffect(() => {
    if (productId || codeTouched || !nameValue) return;
    const existingCodes = new Set(products.map((p: any) => p.sku).filter(Boolean));
    form.setFieldValue("sku", deriveProductCode(nameValue, existingCodes));
  }, [nameValue, productId, codeTouched, products, form]);

  useEffect(() => {
    if (product) {
      form.setFieldsValue({
        ...product,
        price: product.price / 100,
        unitCost: product.unitCost != null ? product.unitCost / 100 : null,
        stockEnabled: product.stockEnabled === 1,
      });
    } else if (!productId) {
      form.resetFields();
    }
  }, [product, productId, form]);

  const handleClose = () => {
    setProductId(null);
    form.resetFields();
    navigate(location.pathname, { state: { productModal: false } });
  };

  const handleSubmit = async (values: any) => {
    setSubmitting(true);
    await setProduct({
      ...values,
      price: Math.round((values.price ?? 0) * 100),
      unitCost: values.unitCost != null ? Math.round(values.unitCost * 100) : null,
      stockEnabled: values.type === "product" && values.stockEnabled ? 1 : 0,
    });
    handleClose();
    setSubmitting(false);
  };

  const handleDelete = async () => {
    if (productId) {
      setSubmitting(true);
      await deleteProduct(productId);
      handleClose();
      setSubmitting(false);
    }
  };

  return (
    <Drawer
      title={productId ? <Trans>Edit product</Trans> : <Trans>New product</Trans>}
      open={isVisible}
      placement="right"
      width={480}
      onClose={handleClose}
      footer={
        <div style={{ display: "flex", justifyContent: "space-between" }}>
          <div>
            {productId && (
              <Popconfirm
                title={<Trans>Are you sure you want to delete this product?</Trans>}
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
      <ScrollShadow>
        <Form
          form={form}
          layout="vertical"
          onFinish={handleSubmit}
          initialValues={{ type: "service", stockEnabled: false }}
        >
          <Section><Trans>Details</Trans></Section>
          <Form.Item name="name" label={<Trans>Name</Trans>} rules={[{ required: true, message: t`Name is required` }]}>
            <Input placeholder={t`Product or service name`} />
          </Form.Item>

          <Form.Item name="type" label={<Trans>Type</Trans>} rules={[{ required: true }]}>
            <Select onChange={(val) => { if (val === "service") form.setFieldValue("stockEnabled", false); }}>
              <Select.Option value="service"><Trans>Service</Trans></Select.Option>
              <Select.Option value="product"><Trans>Product</Trans></Select.Option>
            </Select>
          </Form.Item>

          <Form.Item noStyle shouldUpdate={(prev, cur) => prev.type !== cur.type}>
            {({ getFieldValue }) =>
              getFieldValue("type") === "product" ? (
                <Form.Item name="stockEnabled" label={<Trans>Track inventory</Trans>} valuePropName="checked">
                  <Switch />
                </Form.Item>
              ) : null
            }
          </Form.Item>

          <Form.Item name="description" label={<Trans>Description</Trans>}>
            <Input.TextArea rows={3} placeholder={t`Optional description or notes`} />
          </Form.Item>

          <Form.Item
            name="sku"
            label={<Trans>SKU / Product code</Trans>}
            rules={[{ required: true, message: t`Product code is required` }]}
          >
            <Input placeholder={t`e.g. SVC-001`} onChange={() => setCodeTouched(true)} />
          </Form.Item>

          <Section><Trans>Pricing</Trans></Section>
          <Form.Item name="price" label={<Trans>Price</Trans>} rules={[{ required: true, message: t`Price is required` }]}>
            <InputNumber min={0} precision={2} step={0.01} style={{ width: "100%" }} placeholder="0.00" />
          </Form.Item>

          <Form.Item name="unitCost" label={<Trans>Cost price</Trans>}>
            <InputNumber min={0} precision={2} step={0.01} style={{ width: "100%" }} placeholder="0.00" />
          </Form.Item>

          <Form.Item name="unit" label={<Trans>Unit</Trans>}>
            <Select allowClear showSearch placeholder={t`Select or type a unit`}>
              {UNIT_OPTIONS.map((u) => (
                <Select.Option key={u} value={u}>{u}</Select.Option>
              ))}
            </Select>
          </Form.Item>

          <Form.Item name="taxRateId" label={<Trans>Default tax rate</Trans>}>
            <Select allowClear placeholder={t`Select tax rate`}>
              {taxRates.map((tr: any) => (
                <Select.Option key={tr.id} value={tr.id}>
                  {tr.name} ({tr.percentage}%)
                </Select.Option>
              ))}
            </Select>
          </Form.Item>
        </Form>
      </ScrollShadow>
    </Drawer>
  );
};

export default ProductForm;
