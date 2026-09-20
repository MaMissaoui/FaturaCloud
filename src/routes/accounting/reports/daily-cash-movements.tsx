import { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Button,
  Card,
  Col,
  DatePicker,
  Row,
  Select,
  Space,
  Statistic,
  Table,
  Typography,
} from "antd";
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
  // A failed fetch used to reset rows to [], which rendered identically to a
  // genuinely activity-free range ("No activity in this range") — a
  // transient 500 told an accountant there were no movements. Tracked
  // separately so this page shows a real error instead of a false all-clear.
  const [failed, setFailed] = useState(false);

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

  const refresh = useCallback(() => {
    if (!organizationId || !accountId) return;
    setLoading(true);
    setFailed(false);
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
        setFailed(true);
      })
      .finally(() => setLoading(false));
  }, [organizationId, accountId, range]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const money = (cents: number) => formatOrgCents(cents, organization, i18n.locale);

  return (
    <>
      <PageHeader icon={<WalletOutlined />} title={<Trans>Daily Cash Movements</Trans>} />

      {failed && (
        <Alert
          style={{ marginTop: 16 }}
          type="error"
          showIcon
          message={<Trans>Couldn't load the daily cash movements</Trans>}
          action={
            <Button size="small" onClick={refresh}>
              <Trans>Retry</Trans>
            </Button>
          }
        />
      )}

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
        <>
          {rows.length > 0 && (
            <Row gutter={[8, 8]} style={{ marginTop: 16, marginBottom: 16 }}>
              <Col xs={12} md={6}>
                <Card>
                  <Statistic
                    title={<Trans>Opening balance</Trans>}
                    value={money(rows[0]?.opening ?? 0)}
                  />
                </Card>
              </Col>
              <Col xs={12} md={6}>
                <Card>
                  <Statistic
                    title={<Trans>Total in</Trans>}
                    value={money(rows.reduce((sum, r) => sum + r.in, 0))}
                  />
                </Card>
              </Col>
              <Col xs={12} md={6}>
                <Card>
                  <Statistic
                    title={<Trans>Total out</Trans>}
                    value={money(rows.reduce((sum, r) => sum + r.out, 0))}
                  />
                </Card>
              </Col>
              <Col xs={12} md={6}>
                <Card>
                  <Statistic
                    title={<Trans>Closing balance</Trans>}
                    value={money(rows[rows.length - 1]?.closing ?? 0)}
                  />
                </Card>
              </Col>
            </Row>
          )}
          <Table
            dataSource={rows}
            rowKey="date"
            loading={loading}
            pagination={{ hideOnSinglePage: true, defaultPageSize: 31 }}
            locale={{ emptyText: failed ? "—" : <Trans>No activity in this range</Trans> }}
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
        </>
      )}
    </>
  );
};

export default DailyCashMovements;
