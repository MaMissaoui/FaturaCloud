import { useEffect, useState } from "react";
import { Card, DatePicker, Switch, Table } from "antd";
import { Column } from "@ant-design/plots";
import { useAtomValue } from "jotai";
import { Trans } from "@lingui/react/macro";
import { useLingui } from "@lingui/react";
import { LineChartOutlined } from "@ant-design/icons";
import dayjs, { type Dayjs } from "dayjs";

import { organizationIdAtom, organizationAtom } from "src/atoms/organization";
import { themeAtom } from "src/atoms/generic";
import { GetRevenueTrend } from "src/api";
import type { MonthlyRevenue } from "src/api";
import PageHeader from "src/components/page-header";
import { useDatePickerFormat } from "src/utils/date";

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

  useEffect(() => {
    if (!organizationId) return;
    setLoading(true);
    GetRevenueTrend(organizationId, range[0].startOf("day").valueOf(), range[1].endOf("day").valueOf())
      .then(setRows)
      .catch(() => setRows([]))
      .finally(() => setLoading(false));
  }, [organizationId, range]);

  const money = (cents: number) =>
    Intl.NumberFormat(i18n.locale, {
      style: "currency",
      currency: organization?.currency ?? "EUR",
      minimumFractionDigits: organization?.minimum_fraction_digits ?? undefined,
    }).format(cents / 100);

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
            onChange={(values) => {
              if (values?.[0] && values?.[1]) setRange([values[0], values[1]]);
            }}
          />
        }
      />

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
            <Table.Column title={<Trans>Month</Trans>} dataIndex="month" key="month" />
            <Table.Column
              title={<Trans>Revenue</Trans>}
              key="revenue"
              align="right"
              render={(row: MonthlyRevenue) => money(row.revenue)}
            />
          </Table>
        ) : (
          <Column
            data={rows}
            xField="month"
            yField="revenue"
            theme={themeMode === "dark" ? "classicDark" : "classic"}
            height={320}
            axis={{ y: { labelFormatter: (v: number) => money(v) } }}
            tooltip={{ items: [{ field: "revenue", valueFormatter: (v: number) => money(v) }] }}
          />
        )}
      </Card>
    </>
  );
};

export default RevenueTrend;
