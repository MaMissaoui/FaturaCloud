import { useEffect, useMemo, useState } from "react";
import { useLocation, useNavigate } from "react-router";
import { Button, Drawer, Empty, Form, Space } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import get from "lodash/get";

import { productsAtom, setProductsAtom } from "src/atoms/product";
import { GetProductBOM, ReplaceProductBOM } from "src/api";
import { message } from "src/utils/message";
import ScrollShadow from "src/components/scroll-shadow";
import BOMFields from "src/components/products/bom-fields";

// Opened via router state (`{ bomModal: true, productId }`) — the same
// shape products/form.tsx's own drawer already uses for `productModal`, so
// its "Open in Bill of Materials screen" link can navigate here directly.
// A dedicated Form scoped to just the recipe, saved immediately via
// PUT /products/{id}/bom, rather than bundled into a product-wide save —
// this is the one place a bill of materials is actually maintained now;
// see BOMFields' own comment for why the row UI itself isn't duplicated.
const BOMEditorDrawer = ({ onSaved }: { onSaved: () => void }) => {
  const location = useLocation();
  const navigate = useNavigate();
  const [form] = Form.useForm();
  const [submitting, setSubmitting] = useState(false);

  const products = useAtomValue(productsAtom);
  const setProducts = useSetAtom(setProductsAtom);

  const isVisible = get(location.state, "bomModal", false);
  const productId: string | null = get(location.state, "productId", null);

  const product = useMemo(
    () => (productId ? (products.find((p) => p.id === productId) ?? null) : null),
    [products, productId],
  );

  const componentOptions = useMemo(
    () =>
      products
        .filter((p) => p.category === "component")
        .map((p) => ({ value: p.id, label: p.sku ? `${p.name} (${p.sku})` : p.name })),
    [products],
  );

  useEffect(() => {
    if (isVisible) setProducts();
  }, [isVisible, setProducts]);

  useEffect(() => {
    if (isVisible && productId) {
      GetProductBOM(productId).then((lines) => {
        form.setFieldValue(
          "bom",
          lines.map((l) => ({
            componentProductId: l.componentProductId,
            quantityPerUnit: l.quantityPerUnit,
          })),
        );
      });
    } else {
      form.setFieldValue("bom", []);
    }
  }, [isVisible, productId, form]);

  const handleClose = () => {
    form.resetFields();
    navigate(location.pathname, { state: { bomModal: false } });
  };

  const handleSubmit = async (values: {
    bom?: { componentProductId: string; quantityPerUnit: number }[];
  }) => {
    if (!productId) return;
    setSubmitting(true);
    try {
      await ReplaceProductBOM(productId, values.bom ?? []);
      message.success(t`Bill of materials saved`);
      onSaved();
      handleClose();
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Failed to save bill of materials`);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Drawer
      title={
        product ? (
          <Trans>Bill of Materials — {product.name}</Trans>
        ) : (
          <Trans>Bill of Materials</Trans>
        )
      }
      open={isVisible}
      placement="right"
      size={560}
      onClose={handleClose}
      footer={
        <Space style={{ display: "flex", justifyContent: "flex-end" }}>
          <Button onClick={handleClose}>
            <Trans>Cancel</Trans>
          </Button>
          <Button
            type="primary"
            loading={submitting}
            disabled={!!product && product.category !== "finished"}
            onClick={() => form.submit()}
          >
            <Trans>Save</Trans>
          </Button>
        </Space>
      }
    >
      <ScrollShadow>
        {product && product.category !== "finished" ? (
          <Empty
            description={
              <Trans>
                Only a "Finished good" product can have a bill of materials — change this product's
                category first.
              </Trans>
            }
          />
        ) : (
          <Form form={form} layout="vertical" onFinish={handleSubmit} initialValues={{ bom: [] }}>
            <BOMFields fieldName="bom" componentOptions={componentOptions} />
          </Form>
        )}
      </ScrollShadow>
    </Drawer>
  );
};

export default BOMEditorDrawer;
