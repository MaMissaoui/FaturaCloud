import { useEffect, useState } from "react";
import { Card, DatePicker, Table } from "antd";
import { Bar } from "@ant-design/plots";
import { useAtomValue } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { ShoppingCartOutlined } from "@ant-design/icons";
import dayjs, { type Dayjs } from "dayjs";

import { organizationIdAtom, organizationAtom } from "src/atoms/organization";
import { themeAtom } from "src/atoms/generic";
import { GetPurchasesByVendor } from "src/api";
import type { VendorSpend } from "src/api";
import PageHeader from "src/components/page-header";
import { useDatePickerFormat } from "src/utils/date";

const { RangePicker } = DatePicker;

const PurchasesByVendor = () => {
  const { i18n } = useLingui();
  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);
  const themeMode = useAtomValue(themeAtom);
  const dateFormat = useDatePickerFormat();

  const [range, setRange] = useState<[Dayjs, Dayjs]>([dayjs().subtract(12, "month"), dayjs()]);
  const [rows, setRows] = useState<VendorSpend[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!organizationId) return;
    setLoading(true);
    GetPurchasesByVendor(
      organizationId,
      range[0].startOf("day").valueOf(),
      range[1].endOf("day").valueOf(),
    )
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
        icon={<ShoppingCartOutlined />}
        title={<Trans>Purchases by Vendor</Trans>}
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
        title={rows.length > 20 ? <Trans>Top 20 by spend</Trans> : undefined}
      >
        <Bar
          data={rows.slice(0, 20)}
          xField="name"
          yField="spend"
          theme={themeMode === "dark" ? "classicDark" : "classic"}
          height={280}
          axis={{ y: { labelFormatter: (v: number) => money(v) } }}
          tooltip={{ items: [{ field: "spend", name: t`Spend`, valueFormatter: (v: number) => money(v) }] }}
        />
      </Card>

      <Table
        style={{ marginTop: 16 }}
        dataSource={rows}
        rowKey="vendorId"
        loading={loading}
        pagination={{ hideOnSinglePage: true, defaultPageSize: 50 }}
        locale={{ emptyText: <Trans>No purchases in this period</Trans> }}
      >
        <Table.Column title={<Trans>Vendor</Trans>} dataIndex="name" key="name" />
        <Table.Column
          title={<Trans>Spend</Trans>}
          key="spend"
          align="right"
          render={(row: VendorSpend) => money(row.spend)}
        />
      </Table>
    </>
  );
};

export default PurchasesByVendor;
