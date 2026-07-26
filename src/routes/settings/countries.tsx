import { useEffect, useMemo } from "react";
import { useLocation } from "react-router";
import { Col, Input, Row, Space, Switch, Table, Typography } from "antd";
import { atom, useAtom, useAtomValue, useSetAtom } from "jotai";
import { GlobalOutlined } from "@ant-design/icons";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import filter from "lodash/filter";
import includes from "lodash/includes";

import { localeAtom } from "src/atoms/generic";
import { activeCountriesAtom, setActiveCountriesAtom, toggleCountryActiveAtom } from "src/atoms/country";
import { COUNTRY_CODES, getCountryName } from "src/utils/country-codes";

const { Title, Text } = Typography;

const searchAtom = atom<string>("");

function SettingsCountries() {
  useLingui();
  const location = useLocation();

  const activeCountries = useAtomValue(activeCountriesAtom);
  const setActiveCountries = useSetAtom(setActiveCountriesAtom);
  const toggleCountryActive = useSetAtom(toggleCountryActiveAtom);
  const locale = useAtomValue(localeAtom);
  const [search, setSearch] = useAtom(searchAtom);

  useEffect(() => {
    if (location.pathname === "/settings/countries") {
      setActiveCountries();
    }
  }, [location, setActiveCountries]);

  const activeSet = useMemo(() => new Set(activeCountries), [activeCountries]);

  const rows = useMemo(
    () => COUNTRY_CODES.map((code) => ({ code, name: getCountryName(code, locale) })),
    [locale],
  );

  const filtered = search
    ? filter(
        rows,
        (row) => includes(row.name.toLowerCase(), search.toLowerCase()) || includes(row.code.toLowerCase(), search.toLowerCase()),
      )
    : rows;

  return (
    <>
      <Row>
        <Col span={12}>
          <Title level={3} style={{ marginTop: 0, marginBottom: 0 }}>
            <GlobalOutlined style={{ marginRight: 8 }} />
            <Trans>Countries</Trans>
          </Title>
        </Col>
        <Col span={12} style={{ display: "flex", justifyContent: "flex-end" }}>
          <Space style={{ alignItems: "start" }}>
            <Input.Search
              placeholder={t`Search`}
              onChange={(e) => setSearch(e.target.value)}
            />
          </Space>
        </Col>
      </Row>
      <Row style={{ marginTop: 8 }}>
        <Col span={24}>
          <Text type="secondary">
            <Trans>
              Only active countries appear in the country picklist on organizations, vendors and
              clients. Existing records keep whatever country they already have, even if it's
              later deactivated here.
            </Trans>
          </Text>
        </Col>
      </Row>

      <Row style={{ marginTop: 16 }}>
        <Col span={24}>
          <Table
            dataSource={filtered}
            pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
            rowKey="code"
          >
            <Table.Column
              title={<Trans>Code</Trans>}
              dataIndex="code"
              key="code"
              width={100}
              sorter={(a: { code: string }, b: { code: string }) => a.code.localeCompare(b.code)}
            />
            <Table.Column
              title={<Trans>Name</Trans>}
              dataIndex="name"
              key="name"
              sorter={(a: { name: string }, b: { name: string }) => a.name.localeCompare(b.name)}
            />
            <Table.Column
              title={<Trans>Active</Trans>}
              key="active"
              width={100}
              align="center"
              sorter={(a: { code: string }, b: { code: string }) =>
                Number(activeSet.has(a.code)) - Number(activeSet.has(b.code))
              }
              render={(row: { code: string }) => (
                <Switch
                  checked={activeSet.has(row.code)}
                  onChange={(checked) => toggleCountryActive({ code: row.code, active: checked })}
                />
              )}
            />
          </Table>
        </Col>
      </Row>
    </>
  );
}

export default SettingsCountries;
