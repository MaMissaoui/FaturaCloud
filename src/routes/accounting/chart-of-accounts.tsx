import { useEffect, useMemo, useState } from "react";
import type { Account } from "src/types/models";
import { Link, Outlet, useLocation, useNavigate } from "react-router";
import { Button, Table, Tag, Col, Row, Space, Empty } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { BankOutlined } from "@ant-design/icons";
import filter from "lodash/filter";
import find from "lodash/find";
import includes from "lodash/includes";
import toLower from "lodash/toLower";

import { accountsAtom, setAccountsAtom } from "src/atoms/account";
import { organizationIdAtom } from "src/atoms/organization";
import AccountForm from "src/components/accounting/account-form";
import MassDataExcelActions from "src/components/mass-data/mass-data-excel-actions";
import PageHeader from "src/components/page-header";

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
  const organizationId = useAtomValue(organizationIdAtom);
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (location.pathname === "/accounting/chart-of-accounts") {
      setLoading(true);
      setAccounts().finally(() => setLoading(false));
    }
  }, [location, setAccounts]);

  const filtered = useMemo(
    () =>
      filter(accounts, (a: Account) => includes(toLower(`${a.code} ${a.name}`), toLower(search))),
    [accounts, search],
  );

  return (
    <>
      <PageHeader
        icon={<BankOutlined />}
        title={<Trans>Chart of Accounts</Trans>}
        search={{ placeholder: t`Search text`, value: search, onChange: setSearch }}
        actions={
          <Space wrap>
            {organizationId && (
              <MassDataExcelActions
                organizationId={organizationId}
                resource="accounts"
                filenamePrefix="chart-of-accounts"
                onImported={() => setAccounts()}
              />
            )}
            <Link to="/accounting/chart-of-accounts" state={{ accountModal: true }}>
              <Button type="primary">
                <Trans>New account</Trans>
              </Button>
            </Link>
          </Space>
        }
      />
      <Row>
        <Col span={24}>
          <Table
            dataSource={filtered}
            pagination={{ defaultPageSize: 50, showSizeChanger: true, hideOnSinglePage: true }}
            rowKey="id"
            loading={loading}
            locale={{
              emptyText: search ? (
                <Empty description={<Trans>No accounts match your search</Trans>} />
              ) : (
                <Empty description={<Trans>No accounts yet</Trans>}>
                  <Link to="/accounting/chart-of-accounts" state={{ accountModal: true }}>
                    <Button type="primary">
                      <Trans>Create your first account</Trans>
                    </Button>
                  </Link>
                </Empty>
              ),
            }}
            onRow={(record: Account) => ({
              onClick: () =>
                navigate("/accounting/chart-of-accounts", {
                  state: { accountModal: true, accountId: record.id },
                }),
              onKeyDown: (e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  navigate("/accounting/chart-of-accounts", {
                    state: { accountModal: true, accountId: record.id },
                  });
                }
              },
              style: { cursor: "pointer" },
              tabIndex: 0,
              role: "button",
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
              sorter={(a: Account, b: Account) => {
                const pa = find(accounts, { id: a.parentId }) as Account | undefined;
                const pb = find(accounts, { id: b.parentId }) as Account | undefined;
                return `${pa?.code ?? ""}`.localeCompare(`${pb?.code ?? ""}`);
              }}
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
              sorter={(a: Account, b: Account) => a.isGroup - b.isGroup}
              render={(isGroup: number) => (isGroup ? <Tag>{t`Header`}</Tag> : null)}
            />
            <Table.Column
              title={<Trans>Active</Trans>}
              dataIndex="isActive"
              key="isActive"
              align="center"
              width={90}
              sorter={(a: Account, b: Account) => a.isActive - b.isActive}
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
