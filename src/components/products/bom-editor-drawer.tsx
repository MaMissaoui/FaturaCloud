import { useEffect, useMemo, useState } from "react";
import { useLocation, useNavigate } from "react-router";
import {
  Alert,
  Button,
  Drawer,
  Empty,
  Form,
  InputNumber,
  Popconfirm,
  Radio,
  Select,
  Space,
  Table,
  theme,
} from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import get from "lodash/get";

import type { BOMVersion, BOMVersionDetail, BOMVersionLine } from "src/types/models";
import { productsAtom, setProductsAtom } from "src/atoms/product";
import {
  GetProductBOM,
  ReplaceProductBOM,
  GetBOMVersions,
  GetBOMVersion,
  RestoreBOMVersion,
} from "src/api";
import { message } from "src/utils/message";
import { useDateFormatter } from "src/utils/date";
import ScrollShadow from "src/components/scroll-shadow";
import BOMFields from "src/components/products/bom-fields";

// Quantities in this drawer's form are always expressed "per batch of
// batchSize units" — roundDisplay keeps that scaling from producing ugly
// floats in the input (e.g. 1/3 of a batch of 3) without pretending to be
// db/product_bom.go's own roundBOMQuantity, which is the authority on the
// stored canonical per-unit value.
const roundDisplay = (q: number) => Math.round(q * 10000) / 10000;

// Opened via router state (`{ bomModal: true, productId }`) — the same
// shape products/form.tsx's own drawer already uses for `productModal`, so
// its "Open in Bill of Materials screen" link can navigate here directly.
// A dedicated Form scoped to just the recipe, saved immediately via
// PUT /products/{id}/bom, rather than bundled into a product-wide save —
// this is the one place a bill of materials is actually maintained now;
// see BOMFields' own comment for why the row UI itself isn't duplicated.
//
// The list page's "New recipe" button opens this same drawer with no
// `productId` in state — `pickedProductId` covers that case with an
// in-drawer product picker, so there's no separate "create" component or
// route, just this drawer's edit flow one step earlier.
//
// Version history (db/product_bom.go's bill_of_materials_versions): every
// save snapshots the resulting new recipe as a version. This drawer shows
// "Current" plus each past version as a chip; selecting a past one swaps
// the editable form for a read-only view of that version's lines, with a
// "Restore this version?" confirmation in the footer instead of Save.
// Batch size is a pure entry-helper (see roundBOMQuantity's comment in
// db/product_bom.go) — the fields on screen are always "for a batch of N
// units", divided down to per-unit before the API call, and reloaded from
// whatever batchSize the latest version was saved at rather than always
// resetting to 1.
const BOMEditorDrawer = ({ onSaved }: { onSaved: () => void }) => {
  const location = useLocation();
  const navigate = useNavigate();
  const { token } = theme.useToken();
  const [form] = Form.useForm();
  const formatDate = useDateFormatter();
  const [submitting, setSubmitting] = useState(false);
  // loadFailed gates Save: an empty form after a failed fetch looks exactly
  // like "no components yet", and saving from it would wipe a real recipe
  // (F84). reloadToken is what the Retry button bumps to re-run the loader.
  const [loadFailed, setLoadFailed] = useState(false);
  const [loading, setLoading] = useState(false);
  const [reloadToken, setReloadToken] = useState(0);
  const [pickedProductId, setPickedProductId] = useState<string | null>(null);
  const [batchSize, setBatchSize] = useState(1);
  const [versions, setVersions] = useState<BOMVersion[]>([]);
  // null = editing the current recipe; otherwise the id of a past version
  // being browsed read-only.
  const [selectedVersionId, setSelectedVersionId] = useState<string | null>(null);
  const [viewedVersion, setViewedVersion] = useState<BOMVersionDetail | null>(null);
  const [restoring, setRestoring] = useState(false);

  const products = useAtomValue(productsAtom);
  const setProducts = useSetAtom(setProductsAtom);

  const isVisible = get(location.state, "bomModal", false);
  const routeProductId: string | null = get(location.state, "productId", null);
  const productId = routeProductId ?? pickedProductId;

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

  const finishedProductOptions = useMemo(
    () =>
      products
        .filter((p) => p.category === "finished")
        .map((p) => ({ value: p.id, label: p.sku ? `${p.name} (${p.sku})` : p.name })),
    [products],
  );

  useEffect(() => {
    if (isVisible) setProducts();
  }, [isVisible, setProducts]);

  // Reset the picker each time the drawer opens fresh (routeProductId only
  // set once, on the navigate() call that opened it) so a previous "New
  // recipe" pick doesn't leak into the next time it's opened that way.
  useEffect(() => {
    if (isVisible && !routeProductId) setPickedProductId(null);
  }, [isVisible, routeProductId]);

  // Load the current recipe and its version history together — a single
  // Promise.all rather than two independent effects, so the batch-size
  // scaling below always has both the lines and the latest version's
  // batchSize by the time it runs instead of racing on load order.
  //
  // A failure here must be visible and must block Save (F84): an empty form
  // is indistinguishable from "this product has no components yet", and
  // saving from that state would replace a real recipe with nothing.
  useEffect(() => {
    setSelectedVersionId(null);
    setViewedVersion(null);
    if (isVisible && productId) {
      let cancelled = false;
      setLoadFailed(false);
      setLoading(true);
      Promise.all([GetProductBOM(productId), GetBOMVersions(productId)])
        .then(([lines, v]) => {
          if (cancelled) return;
          setVersions(v);
          const latestBatchSize = v[0]?.batchSize ?? 1;
          setBatchSize(latestBatchSize);
          form.setFieldValue(
            "bom",
            lines.map((l) => ({
              componentProductId: l.componentProductId,
              quantityPerUnit: roundDisplay(l.quantityPerUnit * latestBatchSize),
            })),
          );
        })
        .catch((error) => {
          if (cancelled) return;
          setLoadFailed(true);
          form.setFieldValue("bom", []);
          setVersions([]);
          setBatchSize(1);
          message.error(
            error instanceof Error ? error.message : t`Failed to load bill of materials`,
          );
        })
        .finally(() => {
          if (!cancelled) setLoading(false);
        });
      // Switching product in the picker mid-flight must not let the previous
      // product's recipe land in the form.
      return () => {
        cancelled = true;
      };
    }
    setLoadFailed(false);
    setLoading(false);
    form.setFieldValue("bom", []);
    setVersions([]);
    setBatchSize(1);
  }, [isVisible, productId, form, reloadToken]);

  const handleClose = () => {
    form.resetFields();
    setPickedProductId(null);
    setVersions([]);
    setSelectedVersionId(null);
    setViewedVersion(null);
    setBatchSize(1);
    navigate(location.pathname, { state: { bomModal: false } });
  };

  // Changing the batch size rescales whatever's already on screen so each
  // row's underlying per-unit meaning stays the same — typing 3 at a batch
  // of 1 then switching to a batch of 6 shows 6, not a recipe that silently
  // doubled.
  const handleBatchSizeChange = (value: number | null) => {
    const newBatch = value && value > 0 ? Math.round(value) : 1;
    if (newBatch === batchSize) return;
    const currentBom: { componentProductId: string; quantityPerUnit: number }[] =
      form.getFieldValue("bom") ?? [];
    form.setFieldValue(
      "bom",
      currentBom.map((line) => ({
        ...line,
        quantityPerUnit:
          line.quantityPerUnit != null
            ? roundDisplay((line.quantityPerUnit / batchSize) * newBatch)
            : line.quantityPerUnit,
      })),
    );
    setBatchSize(newBatch);
  };

  const handleSelectVersion = (versionId: string | null) => {
    setSelectedVersionId(versionId);
    setViewedVersion(null);
    if (versionId && productId) {
      GetBOMVersion(productId, versionId)
        .then(setViewedVersion)
        .catch((error) => {
          // Without this the drawer sat on loading={!viewedVersion} forever
          // with Restore disabled and no way out but closing (F84).
          message.error(error instanceof Error ? error.message : t`Failed to load version`);
          setSelectedVersionId(null);
        });
    }
  };

  const handleSubmit = async (values: {
    bom?: { componentProductId: string; quantityPerUnit: number }[];
  }) => {
    if (!productId) return;
    setSubmitting(true);
    try {
      const lines = (values.bom ?? []).map((l) => ({
        componentProductId: l.componentProductId,
        quantityPerUnit: batchSize === 1 ? l.quantityPerUnit : l.quantityPerUnit / batchSize,
      }));
      await ReplaceProductBOM(productId, lines, batchSize);
      message.success(t`Bill of materials saved`);
      onSaved();
      handleClose();
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Failed to save bill of materials`);
    } finally {
      setSubmitting(false);
    }
  };

  const handleRestore = async () => {
    if (!productId || !selectedVersionId) return;
    setRestoring(true);
    try {
      await RestoreBOMVersion(productId, selectedVersionId);
      message.success(t`Version restored`);
      onSaved();
      const [lines, v] = await Promise.all([GetProductBOM(productId), GetBOMVersions(productId)]);
      setVersions(v);
      const latestBatchSize = v[0]?.batchSize ?? 1;
      setBatchSize(latestBatchSize);
      form.setFieldValue(
        "bom",
        lines.map((l) => ({
          componentProductId: l.componentProductId,
          quantityPerUnit: roundDisplay(l.quantityPerUnit * latestBatchSize),
        })),
      );
      setSelectedVersionId(null);
      setViewedVersion(null);
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Failed to restore version`);
    } finally {
      setRestoring(false);
    }
  };

  const showRecipe = productId && (!product || product.category === "finished");
  const viewingPastVersion = selectedVersionId !== null;

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
          {viewingPastVersion ? (
            <Popconfirm
              title={<Trans>Restore this version?</Trans>}
              description={
                <Trans>
                  This replaces the current recipe with v{viewedVersion?.versionNumber}'s components
                  and quantities.
                </Trans>
              }
              okText={<Trans>Restore</Trans>}
              cancelText={<Trans>Cancel</Trans>}
              onConfirm={handleRestore}
            >
              <Button type="primary" loading={restoring} disabled={!viewedVersion}>
                <Trans>Restore this version</Trans>
              </Button>
            </Popconfirm>
          ) : (
            <Button
              type="primary"
              loading={submitting}
              disabled={
                !productId ||
                (!!product && product.category !== "finished") ||
                loadFailed ||
                loading
              }
              onClick={() => form.submit()}
            >
              <Trans>Save</Trans>
            </Button>
          )}
        </Space>
      }
    >
      <ScrollShadow>
        {loadFailed && (
          <Alert
            type="error"
            showIcon
            style={{ marginBottom: 16 }}
            message={<Trans>Couldn't load this bill of materials</Trans>}
            description={
              <Trans>
                Saving is disabled until it loads, so an empty form can't overwrite the saved
                recipe.
              </Trans>
            }
            action={
              <Button size="small" onClick={() => setReloadToken((n) => n + 1)}>
                <Trans>Retry</Trans>
              </Button>
            }
          />
        )}
        {!routeProductId && (
          <div style={{ marginBottom: 16 }}>
            <div style={{ marginBottom: 8, fontWeight: 500 }}>
              <Trans>Finished product</Trans>
            </div>
            <Select
              showSearch
              style={{ width: "100%" }}
              placeholder={t`Select a finished product`}
              optionFilterProp="label"
              options={finishedProductOptions}
              value={pickedProductId ?? undefined}
              onChange={setPickedProductId}
            />
          </div>
        )}

        {showRecipe && versions.length > 0 && (
          <div style={{ marginBottom: 16 }}>
            <Radio.Group
              value={selectedVersionId ?? "current"}
              onChange={(e) =>
                handleSelectVersion(e.target.value === "current" ? null : e.target.value)
              }
              optionType="button"
              buttonStyle="solid"
              size="small"
            >
              <Radio.Button value="current">
                <Trans>Current</Trans>
              </Radio.Button>
              {versions.map((v) => (
                <Radio.Button key={v.id} value={v.id}>
                  v{v.versionNumber} · {v.createdAt ? formatDate(Number(v.createdAt)) : ""}
                </Radio.Button>
              ))}
            </Radio.Group>
          </div>
        )}

        {!productId ? (
          <Empty description={<Trans>Pick a finished product above to define its recipe.</Trans>} />
        ) : product && product.category !== "finished" ? (
          <Empty
            description={
              <Trans>
                Only a "Finished good" product can have a bill of materials — change this product's
                category first.
              </Trans>
            }
          />
        ) : viewingPastVersion ? (
          <Table
            dataSource={viewedVersion?.lines ?? []}
            rowKey="id"
            loading={!viewedVersion}
            size="small"
            pagination={false}
          >
            <Table.Column
              title={<Trans>Component</Trans>}
              key="component"
              render={(l: BOMVersionLine) =>
                l.componentSku ? `${l.componentName} (${l.componentSku})` : l.componentName
              }
            />
            <Table.Column
              title={<Trans>Qty per unit</Trans>}
              dataIndex="quantityPerUnit"
              key="quantityPerUnit"
              align="right"
            />
          </Table>
        ) : (
          <Form form={form} layout="vertical" onFinish={handleSubmit} initialValues={{ bom: [] }}>
            <Form.Item label={<Trans>Batch size</Trans>} style={{ maxWidth: 160 }}>
              <InputNumber
                min={1}
                precision={0}
                style={{ width: "100%" }}
                value={batchSize}
                onChange={handleBatchSizeChange}
              />
            </Form.Item>
            <div style={{ marginBottom: 12, color: token.colorTextSecondary, fontSize: 12 }}>
              {batchSize === 1 ? (
                <Trans>Quantities below are per single unit.</Trans>
              ) : (
                <Trans>Quantities below are for a batch of {batchSize} units.</Trans>
              )}
            </div>
            <BOMFields fieldName="bom" componentOptions={componentOptions} />
          </Form>
        )}
      </ScrollShadow>
    </Drawer>
  );
};

export default BOMEditorDrawer;
