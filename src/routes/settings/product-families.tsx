import { useEffect, useMemo, useState } from "react";
import type { ProductFamily } from "src/types/models";
import { Link, useLocation, useNavigate } from "react-router";
import { Button, Col, Row, Table, Empty } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { ClusterOutlined } from "@ant-design/icons";

import { productFamiliesAtom, setProductFamiliesAtom } from "src/atoms/product-family";
import ProductFamilyForm from "src/components/product-families/form";
import PageHeader from "src/components/page-header";

function SettingsProductFamilies() {
  useLingui();
  const location = useLocation();
  const navigate = useNavigate();

  const productFamilies = useAtomValue(productFamiliesAtom);
  const setProductFamilies = useSetAtom(setProductFamiliesAtom);
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (location.pathname === "/settings/product-families") {
      setLoading(true);
      setProductFamilies().finally(() => setLoading(false));
    }
  }, [location, setProductFamilies]);

  const filtered = useMemo(() => {
    const term = search.trim().toLowerCase();
    if (!term) return productFamilies;
    return productFamilies.filter((f: ProductFamily) => f.name.toLowerCase().includes(term));
  }, [productFamilies, search]);

  return (
    <>
      <PageHeader
        icon={<ClusterOutlined />}
        title={<Trans>Product families</Trans>}
        search={{ placeholder: t`Search`, onChange: setSearch }}
        actions={
          <Link to="/settings/product-families" state={{ productFamilyModal: true }}>
            <Button type="primary">
              <Trans>New product family</Trans>
            </Button>
          </Link>
        }
      />

      <Row style={{ marginTop: 16 }}>
        <Col span={24}>
          <Table
            dataSource={filtered}
            pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
            rowKey="id"
            loading={loading}
            locale={{
              emptyText: search ? (
                <Empty description={<Trans>No families match your search</Trans>} />
              ) : (
                <Empty description={<Trans>No product families yet</Trans>}>
                  <Link to="/settings/product-families" state={{ productFamilyModal: true }}>
                    <Button type="primary">
                      <Trans>Create your first product family</Trans>
                    </Button>
                  </Link>
                </Empty>
              ),
            }}
            onRow={(record: ProductFamily) => ({
              onClick: () =>
                navigate("/settings/product-families", {
                  state: { productFamilyModal: true, productFamilyId: record.id },
                }),
              onKeyDown: (e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  navigate("/settings/product-families", {
                    state: { productFamilyModal: true, productFamilyId: record.id },
                  });
                }
              },
              style: { cursor: "pointer" },
              tabIndex: 0,
              role: "button",
            })}
          >
            <Table.Column
              title={<Trans>Name</Trans>}
              key="name"
              sorter={(a: ProductFamily, b: ProductFamily) => a.name.localeCompare(b.name)}
              render={(f: ProductFamily) => (
                <Link
                  to="/settings/product-families"
                  state={{ productFamilyModal: true, productFamilyId: f.id }}
                  onClick={(e) => e.stopPropagation()}
                >
                  {f.name}
                </Link>
              )}
            />
          </Table>
        </Col>
      </Row>

      <ProductFamilyForm />
    </>
  );
}

export default SettingsProductFamilies;
