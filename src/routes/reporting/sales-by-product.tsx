import { useEffect, useState } from "react";
import { Card, DatePicker, Table } from "antd";
import { Bar } from "@ant-design/plots";
import { useAtomValue } from "jotai";
import { Trans } from "@lingui/react/macro";
import { useLingui } from "@lingui/react";
import { AppstoreOutlined } from "@ant-design/icons";
import dayjs, { type Dayjs } from "dayjs";

import { organizationIdAtom, organizationAtom } from "src/atoms/organization";
import { themeAtom } from "src/atoms/generic";
import { GetSalesByProduct } from "src/api";
import type { ProductRevenue } from "src/api";
import PageHeader from "src/components/page-header";
import { useDatePickerFormat } from "src/utils/date";

const { RangePicker } = DatePicker;

const SalesByProduct = () => {
  const { i18n } = useLingui();
  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);
  const themeMode = useAtomValue(themeAtom);
  const dateFormat = useDatePickerFormat();

  const [range, setRange] = useState<[Dayjs, Dayjs]>([dayjs().subtract(12, "month"), dayjs()]);
  const [rows, setRows] = useState<ProductRevenue[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!organizationId) return;
    setLoading(true);
    GetSalesByProduct(organizationId, range[0].startOf("day").valueOf(), range[1].endOf("day").valueOf())
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
        icon={<AppstoreOutlined />}
        title={<Trans>Sales by Product</Trans>}
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
        title={rows.length > 20 ? <Trans>Top 20 by revenue</Trans> : undefined}
      >
        <Bar
          data={rows.slice(0, 20)}
          xField="name"
          yField="revenue"
          theme={themeMode === "dark" ? "classicDark" : "classic"}
          height={280}
          axis={{ y: { labelFormatter: (v: number) => money(v) } }}
          tooltip={{ items: [{ field: "revenue", valueFormatter: (v: number) => money(v) }] }}
        />
      </Card>

      <Table
        style={{ marginTop: 16 }}
        dataSource={rows}
        rowKey="productId"
        loading={loading}
        pagination={{ hideOnSinglePage: true, defaultPageSize: 50 }}
        locale={{ emptyText: <Trans>No revenue in this period</Trans> }}
      >
        <Table.Column title={<Trans>Product</Trans>} dataIndex="name" key="name" />
        <Table.Column
          title={<Trans>Revenue</Trans>}
          key="revenue"
          align="right"
          render={(row: ProductRevenue) => money(row.revenue)}
        />
      </Table>
    </>
  );
};

export default SalesByProduct;
