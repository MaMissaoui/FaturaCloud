import { useEffect } from "react";
import { Link, Outlet, useLocation, useNavigate } from "react-router";
import { Button, Col, Input, Row, Space, Table, Typography } from "antd";
import { atom, useAtom, useAtomValue, useSetAtom } from "jotai";
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

const { Title } = Typography;

const searchAtom = atom<string>("");

function SettingsTaxRates() {
  useLingui();
  const location = useLocation();
  const navigate = useNavigate();

  const taxRates = useAtomValue(taxRatesAtom);
  const setTaxRates = useSetAtom(setTaxRatesAtom);
  const [search, setSearch] = useAtom(searchAtom);

  useEffect(() => {
    if (location.pathname === "/settings/tax-rates") {
      setTaxRates();
    }
  }, [location, setTaxRates]);

  const filtered = search
    ? filter(taxRates, (tr: any) =>
        some(["name", "description", "percentage"], (field) =>
          includes(toString(get(tr, field)).toLowerCase(), search.toLowerCase()),
        ),
      )
    : taxRates;

  return (
    <>
      <Row>
        <Col span={12}>
          <Title level={3} style={{ marginTop: 0, marginBottom: 0 }}>
            <CalculatorOutlined style={{ marginRight: 8 }} />
            <Trans>Tax rates</Trans>
          </Title>
        </Col>
        <Col span={12} style={{ display: "flex", justifyContent: "flex-end" }}>
          <Space style={{ alignItems: "start" }}>
            <Input.Search
              placeholder={t`Search`}
              onChange={(e) => setSearch(e.target.value)}
            />
            <Link to="/settings/tax-rates/new">
              <Button type="primary">
                <Trans>New tax rate</Trans>
              </Button>
            </Link>
          </Space>
        </Col>
      </Row>

      <Row style={{ marginTop: 16 }}>
        <Col span={24}>
          <Table
            dataSource={filtered}
            pagination={false}
            rowKey="id"
            onRow={(record: any) => ({
              onClick: () => navigate(`/settings/tax-rates/${record.id}`),
              style: { cursor: "pointer" },
            })}
          >
            <Table.Column
              title={<Trans>Name</Trans>}
              key="name"
              sorter={(a: any, b: any) => a.name.localeCompare(b.name)}
              render={(tr) => (
                <Link to={`/settings/tax-rates/${tr.id}`} onClick={(e) => e.stopPropagation()}>{tr.name}</Link>
              )}
            />
            <Table.Column
              title={<Trans>Description</Trans>}
              dataIndex="description"
              key="description"
              sorter={(a: any, b: any) => (a.description ?? "").localeCompare(b.description ?? "")}
            />
            <Table.Column
              title={<Trans>Percentage</Trans>}
              align="right"
              dataIndex="percentage"
              key="percentage"
              sorter={(a: any, b: any) => a.percentage - b.percentage}
              render={(percentage) => `${percentage} %`}
            />
            <Table.Column
              title={<Trans>Default</Trans>}
              align="center"
              dataIndex="isDefault"
              key="isDefault"
              sorter={(a: any, b: any) => (a.isDefault ? 1 : 0) - (b.isDefault ? 1 : 0)}
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
