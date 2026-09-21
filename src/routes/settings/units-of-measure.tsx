import { useEffect, useMemo, useState } from "react";
import type { UnitOfMeasure } from "src/types/models";
import { Link, useLocation, useNavigate } from "react-router";
import { Button, Col, Row, Space, Table, Empty } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { CheckSquareOutlined, ColumnWidthOutlined } from "@ant-design/icons";

import { unitsOfMeasureAtom, setUnitsOfMeasureAtom } from "src/atoms/unit-of-measure";
import { organizationIdAtom } from "src/atoms/organization";
import UnitOfMeasureForm from "src/components/units-of-measure/form";
import MassDataExcelActions from "src/components/mass-data/mass-data-excel-actions";
import PageHeader from "src/components/page-header";

function SettingsUnitsOfMeasure() {
  useLingui();
  const location = useLocation();
  const navigate = useNavigate();

  const unitsOfMeasure = useAtomValue(unitsOfMeasureAtom);
  const setUnitsOfMeasure = useSetAtom(setUnitsOfMeasureAtom);
  const organizationId = useAtomValue(organizationIdAtom);
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (location.pathname === "/settings/units-of-measure") {
      setLoading(true);
      setUnitsOfMeasure().finally(() => setLoading(false));
    }
  }, [location, setUnitsOfMeasure]);

  // useState + useMemo, matching bill-of-materials.tsx — see the same note
  // in production-orders.tsx (audit 2026-09-14 F91).
  const filtered = useMemo(() => {
    const term = search.trim().toLowerCase();
    if (!term) return unitsOfMeasure;
    return unitsOfMeasure.filter((u: UnitOfMeasure) => u.name.toLowerCase().includes(term));
  }, [unitsOfMeasure, search]);

  return (
    <>
      <PageHeader
        icon={<ColumnWidthOutlined />}
        title={<Trans>Units of measure</Trans>}
        search={{ placeholder: t`Search`, onChange: setSearch }}
        actions={
          <Space wrap>
            {organizationId && (
              <MassDataExcelActions
                organizationId={organizationId}
                resource="units-of-measure"
                filenamePrefix="units-of-measure"
                onImported={() => setUnitsOfMeasure()}
              />
            )}
            <Link to="/settings/units-of-measure" state={{ unitOfMeasureModal: true }}>
              <Button type="primary">
                <Trans>New unit of measure</Trans>
              </Button>
            </Link>
          </Space>
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
                <Empty description={<Trans>No units match your search</Trans>} />
              ) : (
                <Empty description={<Trans>No units of measure yet</Trans>}>
                  <Link to="/settings/units-of-measure" state={{ unitOfMeasureModal: true }}>
                    <Button type="primary">
                      <Trans>Create your first unit of measure</Trans>
                    </Button>
                  </Link>
                </Empty>
              ),
            }}
            onRow={(record: UnitOfMeasure) => ({
              onClick: () =>
                navigate("/settings/units-of-measure", {
                  state: { unitOfMeasureModal: true, unitOfMeasureId: record.id },
                }),
              onKeyDown: (e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  navigate("/settings/units-of-measure", {
                    state: { unitOfMeasureModal: true, unitOfMeasureId: record.id },
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
              sorter={(a: UnitOfMeasure, b: UnitOfMeasure) => a.name.localeCompare(b.name)}
              render={(u: UnitOfMeasure) => (
                <Link
                  to="/settings/units-of-measure"
                  state={{ unitOfMeasureModal: true, unitOfMeasureId: u.id }}
                  onClick={(e) => e.stopPropagation()}
                >
                  {u.name}
                </Link>
              )}
            />
            <Table.Column
              title={<Trans>Default</Trans>}
              align="center"
              dataIndex="isDefault"
              key="isDefault"
              sorter={(a: UnitOfMeasure, b: UnitOfMeasure) =>
                (a.isDefault ? 1 : 0) - (b.isDefault ? 1 : 0)
              }
              render={(value) => (value ? <CheckSquareOutlined /> : "—")}
            />
          </Table>
        </Col>
      </Row>

      <UnitOfMeasureForm />
    </>
  );
}

export default SettingsUnitsOfMeasure;
