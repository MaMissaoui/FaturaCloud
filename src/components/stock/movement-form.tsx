import { useEffect, useState } from "react";
import { useLocation, useNavigate } from "react-router";
import { Button, Drawer, Form, Input, InputNumber, Select, Space, theme, Typography } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import get from "lodash/get";
import filter from "lodash/filter";

import { productsAtom } from "src/atoms/product";
import { createStockMovementAtom } from "src/atoms/stock";

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

const MovementForm = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const [form] = Form.useForm();

  const products = useAtomValue(productsAtom);
  const createMovement = useSetAtom(createStockMovementAtom);
  const [submitting, setSubmitting] = useState(false);

  const trackedProducts = filter(products, (p: any) => p.stockEnabled);

  const isVisible = get(location.state, "movementModal", false);
  const preselectedProductId = get(location.state, "movementProductId", null);

  useEffect(() => {
    if (isVisible) {
      form.resetFields();
      if (preselectedProductId) form.setFieldValue("productId", preselectedProductId);
      form.setFieldValue("type", "in");
    }
  }, [isVisible, preselectedProductId, form]);

  const handleClose = () => {
    form.resetFields();
    navigate(location.pathname, { state: { movementModal: false } });
  };

  const handleSubmit = async (values: any) => {
    setSubmitting(true);

    const product = products.find((p: any) => p.id === values.productId);
    const rawQty: number = values.quantity ?? 0;

    let signedQty: number;
    if (values.type === "out" || values.type === "count_subtraction") {
      signedQty = -rawQty;
    } else if (values.type === "adjustment") {
      signedQty = rawQty - (product?.stockQuantity ?? 0);
    } else {
      signedQty = rawQty;
    }

    await createMovement({
      productId: values.productId,
      type: values.type,
      quantity: signedQty,
      unitCost: values.unitCost != null ? Math.round(values.unitCost * 100) : null,
      note: values.note || null,
      reference: values.reference || null,
    });

    handleClose();
    setSubmitting(false);
  };

  return (
    <Drawer
      title={<Trans>Record stock movement</Trans>}
      open={isVisible}
      placement="right"
      width={480}
      onClose={handleClose}
      footer={
        <Space style={{ justifyContent: "flex-end", width: "100%", display: "flex" }}>
          <Button onClick={handleClose}><Trans>Cancel</Trans></Button>
          <Button type="primary" loading={submitting} onClick={() => form.submit()}>
            <Trans>Save</Trans>
          </Button>
        </Space>
      }
    >
      <Form form={form} layout="vertical" onFinish={handleSubmit}>
        <Section><Trans>Movement</Trans></Section>
        <Form.Item name="productId" label={<Trans>Product</Trans>} rules={[{ required: true, message: t`Select a product` }]}>
          <Select showSearch optionFilterProp="children" placeholder={t`Select product`}>
            {trackedProducts.map((p: any) => (
              <Select.Option key={p.id} value={p.id}>
                {p.name}{p.sku ? ` (${p.sku})` : ""}
              </Select.Option>
            ))}
          </Select>
        </Form.Item>

        <Form.Item name="type" label={<Trans>Type</Trans>} rules={[{ required: true }]}>
          <Select>
            <Select.Option value="in"><Trans>Stock in — receive goods</Trans></Select.Option>
            <Select.Option value="out"><Trans>Stock out — consume / sell</Trans></Select.Option>
            <Select.Option value="adjustment"><Trans>Adjustment — set count to</Trans></Select.Option>
            <Select.Option value="count_addition"><Trans>Stock count — surplus found (add)</Trans></Select.Option>
            <Select.Option value="count_subtraction"><Trans>Stock count — shortage found (subtract)</Trans></Select.Option>
          </Select>
        </Form.Item>

        <Form.Item noStyle shouldUpdate={(prev, cur) => prev.type !== cur.type}>
          {({ getFieldValue }) => {
            const type = getFieldValue("type");
            const label =
              type === "adjustment"
                ? t`New stock count`
                : type === "out"
                  ? t`Quantity consumed / sold`
                  : type === "count_addition"
                    ? t`Surplus quantity found`
                    : type === "count_subtraction"
                      ? t`Shortage quantity found`
                      : t`Quantity received`;
            return (
              <Form.Item name="quantity" label={label} rules={[{ required: true, message: t`Quantity is required` }]}>
                <InputNumber min={0} precision={2} step={1} style={{ width: "100%" }} placeholder="0" />
              </Form.Item>
            );
          }}
        </Form.Item>

        <Form.Item noStyle shouldUpdate={(prev, cur) => prev.type !== cur.type}>
          {({ getFieldValue }) =>
            getFieldValue("type") === "in" ? (
              <Form.Item name="unitCost" label={<Trans>Purchase price per unit</Trans>}>
                <InputNumber min={0} precision={2} step={0.01} style={{ width: "100%" }} placeholder="0.00" />
              </Form.Item>
            ) : null
          }
        </Form.Item>

        <Section><Trans>Details</Trans></Section>
        <Form.Item name="reference" label={<Trans>Reference</Trans>}>
          <Input placeholder={t`PO number, invoice ID, …`} />
        </Form.Item>

        <Form.Item name="note" label={<Trans>Note</Trans>}>
          <Input.TextArea rows={2} placeholder={t`Optional note`} />
        </Form.Item>
      </Form>
    </Drawer>
  );
};

export default MovementForm;
