import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router";
import { Alert, Badge, Button, Col, Row, Table, Tooltip } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { BuildOutlined, PlusOutlined } from "@ant-design/icons";

import type { Product } from "src/types/models";
import { organizationIdAtom } from "src/atoms/organization";
import { productsAtom, setProductsAtom } from "src/atoms/product";
import { GetBOMSummaries } from "src/api";
import BOMEditorDrawer from "src/components/products/bom-editor-drawer";
import PageHeader from "src/components/page-header";
import { unitLabel } from "src/utils/units";

// A focused surface for maintaining a finished product's recipe — the
// product edit drawer (src/components/products/form.tsx) still has the
// same "Bill of Materials" card for editing one while already in that
// record, but this is the one place to see every finished product's
// recipe status at a glance (which ones still have none defined, most
// useful right after a batch of new finished goods is created) and jump
// straight into editing without opening the full product form first.
const BillOfMaterials = () => {
  useLingui();
  const navigate = useNavigate();
  const organizationId = useAtomValue(organizationIdAtom);
  const products = useAtomValue(productsAtom);
  const setProducts = useSetAtom(setProductsAtom);

  const [summaries, setSummaries] = useState<Record<string, number>>({});
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState("");
  // A failed summaries fetch used to leave summaries as {} and let every
  // finished product fall through to the "None / no recipe defined yet"
  // warning badge — telling the user correct recipes are missing (the F85
  // failure mode). Tracked separately so a failed load shows a real error
  // instead of a per-row false claim, while a genuinely-zero-count product
  // still keeps its true empty-state badge.
  const [failed, setFailed] = useState(false);

  const finishedProducts = useMemo(
    () => products.filter((p) => p.category === "finished"),
    [products],
  );

  // Client-side filter — the full finished-product list is already loaded
  // via productsAtom (same source the "New recipe" picker and the Products
  // page's component/finished pickers already rely on), so a search box
  // here doesn't need its own server round trip the way Products' paginated
  // list does.
  const filteredProducts = useMemo(() => {
    const term = search.trim().toLowerCase();
    if (!term) return finishedProducts;
    return finishedProducts.filter(
      (p) => p.name.toLowerCase().includes(term) || (p.sku ?? "").toLowerCase().includes(term),
    );
  }, [finishedProducts, search]);

  const refresh = useCallback(() => {
    if (!organizationId) return;
    setLoading(true);
    setFailed(false);
    Promise.all([setProducts(), GetBOMSummaries(organizationId)])
      .then(([, rows]) => {
        setSummaries(Object.fromEntries(rows.map((r) => [r.finishedProductId, r.componentCount])));
      })
      .catch(() => {
        setSummaries({});
        setFailed(true);
      })
      .finally(() => setLoading(false));
  }, [organizationId, setProducts]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  return (
    <>
      <PageHeader
        icon={<BuildOutlined />}
        title={<Trans>Bill of Materials</Trans>}
        search={{
          placeholder: t`Search by name or SKU`,
          value: search,
          onChange: setSearch,
          allowClear: true,
        }}
        actions={
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={() => navigate("/bill-of-materials", { state: { bomModal: true } })}
          >
            <Trans>New recipe</Trans>
          </Button>
        }
      />

      {failed && (
        <Alert
          style={{ marginTop: 16 }}
          type="error"
          showIcon
          message={<Trans>Couldn't load the bill-of-materials summaries</Trans>}
          action={
            <Button size="small" onClick={refresh}>
              <Trans>Retry</Trans>
            </Button>
          }
        />
      )}

      <Row style={{ marginTop: 16 }}>
        <Col span={24}>
          <Table
            dataSource={filteredProducts}
            rowKey="id"
            loading={loading}
            pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
            locale={{
              emptyText: search ? (
                <Trans>No finished-good products match "{search}"</Trans>
              ) : (
                <Trans>
                  No finished-good products yet — set a product's category to "Finished good" to
                  define a recipe for it.
                </Trans>
              ),
            }}
            onRow={(record: Product) => ({
              onClick: () =>
                navigate("/bill-of-materials", { state: { bomModal: true, productId: record.id } }),
              onKeyDown: (e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  navigate("/bill-of-materials", {
                    state: { bomModal: true, productId: record.id },
                  });
                }
              },
              style: { cursor: "pointer" },
              tabIndex: 0,
              role: "link",
            })}
          >
            <Table.Column
              title={<Trans>Name</Trans>}
              dataIndex="name"
              key="name"
              sorter={(a: Product, b: Product) => (a.name ?? "").localeCompare(b.name ?? "")}
              defaultSortOrder="ascend"
              render={(name: string, record: Product) => (
                <Link
                  to="/bill-of-materials"
                  state={{ bomModal: true, productId: record.id }}
                  onClick={(e) => e.stopPropagation()}
                >
                  {name}
                </Link>
              )}
            />
            <Table.Column
              title={<Trans>SKU</Trans>}
              dataIndex="sku"
              key="sku"
              sorter={(a: Product, b: Product) => (a.sku ?? "").localeCompare(b.sku ?? "")}
            />
            <Table.Column
              title={<Trans>Unit</Trans>}
              dataIndex="unit"
              sorter={(a: Product, b: Product) => (a.unit ?? "").localeCompare(b.unit ?? "")}
              key="unit"
              render={(unit: string | null) => (unit ? unitLabel(unit) : "—")}
            />
            <Table.Column
              title={<Trans>Components</Trans>}
              sorter={(a: Product, b: Product) => (summaries[a.id] ?? 0) - (summaries[b.id] ?? 0)}
              key="components"
              align="center"
              render={(p: Product) => {
                // A failed summaries load must not read as "this product has
                // no recipe" — show nothing rather than the warning badge.
                if (failed) return "—";
                const count = summaries[p.id] ?? 0;
                return count > 0 ? (
                  <Badge status="success" text={count} />
                ) : (
                  <Tooltip title={t`No recipe defined yet`}>
                    <Badge status="warning" text={t`None`} />
                  </Tooltip>
                );
              }}
            />
          </Table>
        </Col>
      </Row>
      <BOMEditorDrawer onSaved={refresh} />
    </>
  );
};

export default BillOfMaterials;
