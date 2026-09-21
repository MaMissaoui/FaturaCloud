import { useEffect, useMemo, useState } from "react";
import type { TaxRate } from "src/types/models";
import { Link, Outlet, useLocation, useNavigate } from "react-router";
import { Button, Col, Row, Space, Table, Empty } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { CalculatorOutlined, CheckSquareOutlined } from "@ant-design/icons";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import filter from "lodash/filter";
import includes from "lodash/includes";
import some from "lodash/some";
import get from "lodash/get";
import toString from "lodash/toString";

import { taxRatesAtom, setTaxRatesAtom } from "src/atoms/tax-rate";
import { organizationIdAtom } from "src/atoms/organization";
import MassDataExcelActions from "src/components/mass-data/mass-data-excel-actions";
import PageHeader from "src/components/page-header";

function SettingsTaxRates() {
  useLingui();
  const location = useLocation();
  const navigate = useNavigate();

  const taxRates = useAtomValue(taxRatesAtom);
  const setTaxRates = useSetAtom(setTaxRatesAtom);
  const organizationId = useAtomValue(organizationIdAtom);
  const [search, setSearch] = useState("");

  useEffect(() => {
    if (location.pathname === "/settings/tax-rates") {
      setTaxRates();
    }
  }, [location, setTaxRates]);

  const filtered = useMemo(
    () =>
      filter(taxRates, (tr: TaxRate) =>
        some(["name", "description", "percentage"], (field) =>
          includes(toString(get(tr, field)).toLowerCase(), search.toLowerCase()),
        ),
      ),
    [taxRates, search],
  );

  return (
    <>
      <PageHeader
        icon={<CalculatorOutlined />}
        title={<Trans>Tax rates</Trans>}
        search={{ placeholder: t`Search`, value: search, onChange: setSearch }}
        actions={
          <Space wrap>
            {organizationId && (
              <MassDataExcelActions
                organizationId={organizationId}
                resource="tax-rates"
                filenamePrefix="tax-rates"
                onImported={() => setTaxRates()}
              />
            )}
            <Link to="/settings/tax-rates/new">
              <Button type="primary">
                <Trans>New tax rate</Trans>
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
            locale={{
              emptyText: search ? (
                <Empty description={<Trans>No tax rates match your search</Trans>} />
              ) : (
                <Empty description={<Trans>No tax rates yet</Trans>}>
                  <Link to="/settings/tax-rates/new">
                    <Button type="primary">
                      <Trans>Create your first tax rate</Trans>
                    </Button>
                  </Link>
                </Empty>
              ),
            }}
            onRow={(record: TaxRate) => ({
              onClick: () => navigate(`/settings/tax-rates/${record.id}`),
              onKeyDown: (e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  navigate(`/settings/tax-rates/${record.id}`);
                }
              },
              style: { cursor: "pointer" },
              tabIndex: 0,
              role: "link",
            })}
          >
            <Table.Column
              title={<Trans>Name</Trans>}
              key="name"
              sorter={(a: TaxRate, b: TaxRate) => a.name.localeCompare(b.name)}
              render={(tr) => (
                <Link to={`/settings/tax-rates/${tr.id}`} onClick={(e) => e.stopPropagation()}>
                  {tr.name}
                </Link>
              )}
            />
            <Table.Column
              title={<Trans>Description</Trans>}
              dataIndex="description"
              key="description"
              sorter={(a: TaxRate, b: TaxRate) =>
                (a.description ?? "").localeCompare(b.description ?? "")
              }
            />
            <Table.Column
              title={<Trans>Percentage</Trans>}
              align="right"
              dataIndex="percentage"
              key="percentage"
              sorter={(a: TaxRate, b: TaxRate) => a.percentage - b.percentage}
              render={(percentage) => `${percentage} %`}
            />
            <Table.Column
              title={<Trans>Default</Trans>}
              align="center"
              dataIndex="isDefault"
              key="isDefault"
              sorter={(a: TaxRate, b: TaxRate) => (a.isDefault ? 1 : 0) - (b.isDefault ? 1 : 0)}
              render={(value) => (value ? <CheckSquareOutlined /> : "—")}
            />
          </Table>
        </Col>
      </Row>

      <Outlet />
    </>
  );
}

export default SettingsTaxRates;
