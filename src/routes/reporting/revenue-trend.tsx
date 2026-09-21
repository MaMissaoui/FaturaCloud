import { useEffect, useRef, useState } from "react";
import { Alert, Button, Card, DatePicker, Switch, Table } from "antd";
import { Column } from "@ant-design/plots";
import { useAtomValue } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { LineChartOutlined } from "@ant-design/icons";
import dayjs, { type Dayjs } from "dayjs";

import { organizationIdAtom, organizationAtom } from "src/atoms/organization";
import { themeAtom } from "src/atoms/generic";
import { GetRevenueTrend } from "src/api";
import type { MonthlyRevenue } from "src/api";
import PageHeader from "src/components/page-header";
import { formatOrgCents } from "src/utils/currencies";
import { useDatePickerFormat } from "src/utils/date";
import { moneySorter, textSorter } from "src/utils/sort";

const { RangePicker } = DatePicker;

const RevenueTrend = () => {
  const { i18n } = useLingui();
  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);
  const themeMode = useAtomValue(themeAtom);
  const dateFormat = useDatePickerFormat();

  const [range, setRange] = useState<[Dayjs, Dayjs]>([dayjs().subtract(12, "month"), dayjs()]);
  const [rows, setRows] = useState<MonthlyRevenue[]>([]);
  const [showTable, setShowTable] = useState(false);
  const [loading, setLoading] = useState(false);
  // A failed fetch used to reset rows to [], which rendered identically to a
  // genuinely revenue-free period — a slow load or transient error looked
  // exactly like "no sales." Tracked separately so the page can show a real
  // error instead of a false all-clear.
  const [failed, setFailed] = useState(false);
  // requestIdRef guards against an in-flight earlier range's request
  // overwriting a newer one, the same shape as products.tsx's search guard.
  const requestIdRef = useRef(0);

  const refresh = () => {
    if (!organizationId) return;
    const requestId = ++requestIdRef.current;
    setLoading(true);
    setFailed(false);
    GetRevenueTrend(
      organizationId,
      range[0].startOf("day").valueOf(),
      range[1].endOf("day").valueOf(),
    )
      .then((data) => {
        if (requestId !== requestIdRef.current) return;
        setRows(data);
      })
      .catch(() => {
        if (requestId !== requestIdRef.current) return;
        setRows([]);
        setFailed(true);
      })
      .finally(() => {
        if (requestId === requestIdRef.current) setLoading(false);
      });
  };

  useEffect(refresh, [organizationId, range]);

  const money = (cents: number) => formatOrgCents(cents, organization, i18n.locale);

  // month is "YYYY-MM" (db/sales_reports.go's strftime output) — parsed as
  // the 1st of that month and rendered via dayjs' active locale rather than
  // shown raw, so a German/French user sees "März 2026"/"mars 2026" instead
  // of "2026-03". The chart's own x-axis keeps the raw string (an AntV
  // category axis, not a text render) — only this table cell needed it.
  const formatMonth = (month: string) => dayjs(`${month}-01`).format("MMMM YYYY");

  return (
    <>
      <PageHeader
        icon={<LineChartOutlined />}
        title={<Trans>Revenue Trend</Trans>}
        extra={
          <RangePicker
            value={range}
            format={dateFormat}
            allowClear={false}
            presets={[
              { label: t`Last 7 days`, value: [dayjs().subtract(7, "day"), dayjs()] },
              { label: t`Last 30 days`, value: [dayjs().subtract(30, "day"), dayjs()] },
              { label: t`This month`, value: [dayjs().startOf("month"), dayjs().endOf("month")] },
              {
                label: t`Last month`,
                value: [
                  dayjs().subtract(1, "month").startOf("month"),
                  dayjs().subtract(1, "month").endOf("month"),
                ],
              },
              { label: t`This year`, value: [dayjs().startOf("year"), dayjs().endOf("year")] },
            ]}
            onChange={(values) => {
              if (values?.[0] && values?.[1]) setRange([values[0], values[1]]);
            }}
          />
        }
      />

      {failed && (
        <Alert
          style={{ marginTop: 16 }}
          type="error"
          showIcon
          message={<Trans>Couldn't load the revenue trend</Trans>}
          action={
            <Button size="small" onClick={refresh}>
              <Trans>Retry</Trans>
            </Button>
          }
        />
      )}

      {!failed && (
        <Card
          style={{ marginTop: 16 }}
          loading={loading}
          title={<Trans>Revenue by month</Trans>}
          extra={
            <Switch
              checked={showTable}
              onChange={setShowTable}
              checkedChildren={<Trans>Table</Trans>}
              unCheckedChildren={<Trans>Chart</Trans>}
            />
          }
        >
          {showTable ? (
            <Table
              dataSource={rows}
              rowKey="month"
              size="small"
              pagination={{ hideOnSinglePage: true, defaultPageSize: 50 }}
              locale={{ emptyText: <Trans>No revenue in this period</Trans> }}
            >
              <Table.Column
                title={<Trans>Month</Trans>}
                key="month"
                sorter={textSorter((row: MonthlyRevenue) => row.month)}
                render={(row: MonthlyRevenue) => formatMonth(row.month)}
              />
              <Table.Column
                title={<Trans>Revenue</Trans>}
                key="revenue"
                align="right"
                sorter={moneySorter((row: MonthlyRevenue) => row.revenue)}
                render={(row: MonthlyRevenue) => money(row.revenue)}
              />
            </Table>
          ) : (
            <div role="img" aria-label={t`Bar chart showing monthly revenue data`}>
              <Column
                data={failed ? [] : rows}
                xField="month"
                yField="revenue"
                theme={themeMode === "dark" ? "classicDark" : "classic"}
                height={320}
                axis={{ y: { labelFormatter: (v: number) => money(v) } }}
                tooltip={{
                  items: [
                    { field: "revenue", name: t`Revenue`, valueFormatter: (v: number) => money(v) },
                  ],
                }}
              />
            </div>
          )}
        </Card>
      )}
    </>
  );
};

export default RevenueTrend;
