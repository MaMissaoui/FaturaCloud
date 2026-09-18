import { useEffect, useState } from "react";
import { DatePicker, Select, Space, Table, Typography } from "antd";
import { useAtomValue } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { WalletOutlined } from "@ant-design/icons";
import dayjs, { type Dayjs } from "dayjs";

import { organizationIdAtom, organizationAtom } from "src/atoms/organization";
import { GetAccounts, GetDailyCashMovements } from "src/api";
import type { DailyCashMovementRow } from "src/api";
import type { Account } from "src/types/models";
import PageHeader from "src/components/page-header";
import { formatOrgCents } from "src/utils/currencies";
import { useDatePickerFormat } from "src/utils/date";

const { RangePicker } = DatePicker;
const { Option } = Select;

// Day boundaries in the returned rows are UTC — see db/gl_reports.go's
// DailyCashMovementRow doc comment for why (no per-organization timezone
// anywhere in this app). row.date is a plain "YYYY-MM-DD" label with no
// time component, so plain dayjs(row.date) just relabels that calendar date
// in the active locale's format — no timezone conversion happens (and none
// should: the string already names the exact UTC day this row is for).
const DailyCashMovements = () => {
  const { i18n } = useLingui();
  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);
  const dateFormat = useDatePickerFormat();

  const [accounts, setAccounts] = useState<Account[]>([]);
  const [accountId, setAccountId] = useState<string>("");
  const [range, setRange] = useState<[Dayjs, Dayjs]>([dayjs().subtract(6, "day"), dayjs()]);
  const [rows, setRows] = useState<DailyCashMovementRow[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!organizationId) return;
    GetAccounts(organizationId)
      .then((accts) => setAccounts(accts.filter((a) => !a.isGroup)))
      .catch((error) => console.error("Failed to fetch accounts:", error));
  }, [organizationId]);

  // Defaults to the organization's configured cash register account once
  // both the account list and the organization have loaded, but only ever
  // once — a user picking a different account to review afterward must not
  // get silently reset back on an unrelated re-render.
  const [defaulted, setDefaulted] = useState(false);
  useEffect(() => {
    if (defaulted || !organization?.defaultCashRegisterAccountId || accounts.length === 0) return;
    setAccountId(organization.defaultCashRegisterAccountId);
    setDefaulted(true);
  }, [defaulted, organization, accounts]);

  useEffect(() => {
    if (!organizationId || !accountId) return;
    setLoading(true);
    GetDailyCashMovements(
      organizationId,
      accountId,
      range[0].startOf("day").valueOf(),
      range[1].endOf("day").valueOf(),
    )
      .then(setRows)
      .catch((error) => {
        console.error("Failed to fetch daily cash movements:", error);
        setRows([]);
      })
      .finally(() => setLoading(false));
  }, [organizationId, accountId, range]);

  const money = (cents: number) => formatOrgCents(cents, organization, i18n.locale);

  return (
    <>
      <PageHeader icon={<WalletOutlined />} title={<Trans>Daily Cash Movements</Trans>} />

      <Space style={{ marginTop: 16, marginBottom: 16 }} wrap>
        <Select
          value={accountId || undefined}
          onChange={setAccountId}
          placeholder={t`Account`}
          showSearch
          optionFilterProp="children"
          style={{ minWidth: 220 }}
        >
          {accounts.map((a) => (
            <Option key={a.id} value={a.id}>
              {a.code} — {a.name}
            </Option>
          ))}
        </Select>
        <RangePicker
          value={range}
          onChange={(dates) => dates?.[0] && dates?.[1] && setRange([dates[0], dates[1]])}
          format={dateFormat}
          allowClear={false}
        />
      </Space>

      {!accountId ? (
        <Typography.Text type="secondary">
          <Trans>Pick an account to see its daily movements.</Trans>
        </Typography.Text>
      ) : (
        <Table
          dataSource={rows}
          rowKey="date"
          loading={loading}
          pagination={{ hideOnSinglePage: true, defaultPageSize: 31 }}
          locale={{ emptyText: <Trans>No activity in this range</Trans> }}
        >
          <Table.Column
            title={<Trans>Date</Trans>}
            key="date"
            render={(row: DailyCashMovementRow) => dayjs(row.date).format(dateFormat)}
          />
          <Table.Column
            title={<Trans>Opening balance</Trans>}
            key="opening"
            align="right"
            render={(row: DailyCashMovementRow) => money(row.opening)}
          />
          <Table.Column
            title={<Trans>In</Trans>}
            key="in"
            align="right"
            render={(row: DailyCashMovementRow) => money(row.in)}
          />
          <Table.Column
            title={<Trans>Out</Trans>}
            key="out"
            align="right"
            render={(row: DailyCashMovementRow) => money(row.out)}
          />
          <Table.Column
            title={<Trans>Closing balance</Trans>}
            key="closing"
            align="right"
            render={(row: DailyCashMovementRow) => <strong>{money(row.closing)}</strong>}
          />
        </Table>
      )}
    </>
  );
};

export default DailyCashMovements;
