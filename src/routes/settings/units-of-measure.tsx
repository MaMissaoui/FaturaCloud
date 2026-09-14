import { useEffect, useState } from "react";
import type { UnitOfMeasure } from "src/types/models";
import { Link, useLocation, useNavigate } from "react-router";
import { Button, Col, Row, Table } from "antd";
import { atom, useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { CheckSquareOutlined, ColumnWidthOutlined } from "@ant-design/icons";
import filter from "lodash/filter";
import includes from "lodash/includes";

import { unitsOfMeasureAtom, setUnitsOfMeasureAtom } from "src/atoms/unit-of-measure";
import UnitOfMeasureForm from "src/components/units-of-measure/form";
import PageHeader from "src/components/page-header";

const searchAtom = atom<string>("");

function SettingsUnitsOfMeasure() {
  useLingui();
  const location = useLocation();
  const navigate = useNavigate();

  const unitsOfMeasure = useAtomValue(unitsOfMeasureAtom);
  const setUnitsOfMeasure = useSetAtom(setUnitsOfMeasureAtom);
  const [search, setSearch] = useAtom(searchAtom);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (location.pathname === "/settings/units-of-measure") {
      setLoading(true);
      setUnitsOfMeasure().finally(() => setLoading(false));
    }
  }, [location, setUnitsOfMeasure]);

  const filtered = search
    ? filter(unitsOfMeasure, (u: UnitOfMeasure) =>
        includes(u.name.toLowerCase(), search.toLowerCase()),
      )
    : unitsOfMeasure;

  return (
    <>
      <PageHeader
        icon={<ColumnWidthOutlined />}
        title={<Trans>Units of measure</Trans>}
        search={{ placeholder: t`Search`, onChange: setSearch }}
        actions={
          <Link to="/settings/units-of-measure" state={{ unitOfMeasureModal: true }}>
            <Button type="primary">
              <Trans>New unit of measure</Trans>
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
            onRow={(record: UnitOfMeasure) => ({
              onClick: () =>
                navigate("/settings/units-of-measure", {
                  state: { unitOfMeasureModal: true, unitOfMeasureId: record.id },
                }),
              style: { cursor: "pointer" },
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
