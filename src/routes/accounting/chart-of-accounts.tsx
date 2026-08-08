import { useEffect, useState } from "react";
import type { Account } from "src/types/models";
import { Link, Outlet, useLocation, useNavigate } from "react-router";
import { Button, Table, Tag, Col, Row } from "antd";
import { atom, useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { BankOutlined } from "@ant-design/icons";
import filter from "lodash/filter";
import find from "lodash/find";
import includes from "lodash/includes";
import toLower from "lodash/toLower";

import { accountsAtom, setAccountsAtom } from "src/atoms/account";
import AccountForm from "src/components/accounting/account-form";
import PageHeader from "src/components/page-header";

const searchAtom = atom<string>("");

const accountTypeColor: Record<string, string> = {
  asset: "blue",
  liability: "orange",
  equity: "purple",
  revenue: "green",
  expense: "volcano",
};

const accountTypeLabel = (type: string): string => {
  switch (type) {
    case "asset":
      return t`Asset`;
    case "liability":
      return t`Liability`;
    case "equity":
      return t`Equity`;
    case "revenue":
      return t`Revenue`;
    case "expense":
      return t`Expense`;
    default:
      return type;
  }
};

const ChartOfAccounts = () => {
  useLingui();
  const location = useLocation();
  const navigate = useNavigate();
  const accounts = useAtomValue(accountsAtom);
  const setAccounts = useSetAtom(setAccountsAtom);
  const [search, setSearch] = useAtom(searchAtom);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (location.pathname === "/accounting/chart-of-accounts") {
      setLoading(true);
      setAccounts().finally(() => setLoading(false));
    }
  }, [location, setAccounts]);

  const filtered = search
    ? filter(accounts, (a: Account) => includes(toLower(`${a.code} ${a.name}`), toLower(search)))
    : accounts;

  return (
    <>
      <PageHeader
        icon={<BankOutlined />}
        title={<Trans>Chart of Accounts</Trans>}
        search={{ placeholder: t`Search text`, onChange: setSearch }}
        actions={
          <Link to="/accounting/chart-of-accounts" state={{ accountModal: true }}>
            <Button type="primary">
              <Trans>New account</Trans>
            </Button>
          </Link>
        }
      />
      <Row>
        <Col span={24}>
          <Table
            dataSource={filtered}
            pagination={{ defaultPageSize: 50, showSizeChanger: true, hideOnSinglePage: true }}
            rowKey="id"
            loading={loading}
            onRow={(record: Account) => ({
              onClick: () =>
                navigate("/accounting/chart-of-accounts", {
                  state: { accountModal: true, accountId: record.id },
                }),
              style: { cursor: "pointer" },
            })}
          >
            <Table.Column
              title={<Trans>Code</Trans>}
              dataIndex="code"
              key="code"
              width={100}
              sorter={(a: Account, b: Account) => a.code.localeCompare(b.code)}
              defaultSortOrder="ascend"
            />
            <Table.Column
              title={<Trans>Name</Trans>}
              key="name"
              sorter={(a: Account, b: Account) => a.name.localeCompare(b.name)}
              render={(account: Account) => (
                <Link
                  to="/accounting/chart-of-accounts"
                  state={{ accountModal: true, accountId: account.id }}
                  onClick={(e) => e.stopPropagation()}
                  style={account.isGroup ? { fontWeight: 600 } : undefined}
                >
                  {account.name}
                </Link>
              )}
            />
            <Table.Column
              title={<Trans>Type</Trans>}
              dataIndex="type"
              key="type"
              sorter={(a: Account, b: Account) => a.type.localeCompare(b.type)}
              render={(type: string) => (
                <Tag color={accountTypeColor[type]}>{accountTypeLabel(type)}</Tag>
              )}
            />
            <Table.Column
              title={<Trans>Parent</Trans>}
              dataIndex="parentId"
              key="parentId"
              render={(parentId: string | null) => {
                if (!parentId) return "—";
                const parent = find(accounts, { id: parentId });
                return parent ? `${parent.code} · ${parent.name}` : "—";
              }}
            />
            <Table.Column
              title={<Trans>Group</Trans>}
              dataIndex="isGroup"
              key="isGroup"
              align="center"
              width={90}
              render={(isGroup: number) => (isGroup ? <Tag>{t`Header`}</Tag> : null)}
            />
            <Table.Column
              title={<Trans>Active</Trans>}
              dataIndex="isActive"
              key="isActive"
              align="center"
              width={90}
              render={(isActive: number) =>
                isActive ? (
                  <Tag color="green">{t`Active`}</Tag>
                ) : (
                  <Tag color="default">{t`Inactive`}</Tag>
                )
              }
            />
          </Table>
          <Outlet />
        </Col>
      </Row>

      <AccountForm />
    </>
  );
};

export default ChartOfAccounts;
