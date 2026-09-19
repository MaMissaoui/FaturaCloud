import { useEffect, useState } from "react";
import { Alert, Button, Card, DatePicker, Table } from "antd";
import { Bar } from "@ant-design/plots";
import { useAtomValue } from "jotai";
import { Link } from "react-router";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { AppstoreOutlined } from "@ant-design/icons";
import dayjs, { type Dayjs } from "dayjs";

import { organizationIdAtom, organizationAtom } from "src/atoms/organization";
import { themeAtom } from "src/atoms/generic";
import { GetSalesByProduct } from "src/api";
import type { ProductRevenue } from "src/api";
import PageHeader from "src/components/page-header";
import { formatOrgCents } from "src/utils/currencies";
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
  // A failed fetch used to reset rows to [], which rendered identically to a
  // genuinely revenue-free period — tracked separately so the page can show
  // a real error instead of a false all-clear.
  const [failed, setFailed] = useState(false);

  const refresh = () => {
    if (!organizationId) return;
    setLoading(true);
    setFailed(false);
    GetSalesByProduct(
      organizationId,
      range[0].startOf("day").valueOf(),
      range[1].endOf("day").valueOf(),
    )
      .then(setRows)
      .catch(() => {
        setRows([]);
        setFailed(true);
      })
      .finally(() => setLoading(false));
  };

  useEffect(refresh, [organizationId, range]);

  const money = (cents: number) => formatOrgCents(cents, organization, i18n.locale);

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
          message={<Trans>Couldn't load sales by product</Trans>}
          action={
            <Button size="small" onClick={refresh}>
              <Trans>Retry</Trans>
            </Button>
          }
        />
      )}

      {!failed && (
        <>
          <Card
            style={{ marginTop: 16 }}
            loading={loading}
            title={
              rows.length > 20 ? (
                <Trans>Top 20 by revenue</Trans>
              ) : (
                <Trans>Revenue by product</Trans>
              )
            }
          >
            <div role="img" aria-label={t`Bar chart showing revenue by product`}>
              <Bar
                data={rows.slice(0, 20)}
                xField="name"
                yField="revenue"
                theme={themeMode === "dark" ? "classicDark" : "classic"}
                // Fixed height regardless of row count made G2Plot auto-hide
                // overlapping category-axis labels once this list neared 20 items —
                // scale with the actual number of bars instead.
                height={Math.max(280, Math.min(rows.length, 20) * 32)}
                axis={{ y: { labelFormatter: (v: number) => money(v) } }}
                tooltip={{
                  items: [
                    { field: "revenue", name: t`Revenue`, valueFormatter: (v: number) => money(v) },
                  ],
                }}
              />
            </div>
          </Card>

          <Table
            style={{ marginTop: 16 }}
            dataSource={rows}
            rowKey="productId"
            loading={loading}
            pagination={{ hideOnSinglePage: true, defaultPageSize: 50 }}
            locale={{ emptyText: <Trans>No revenue in this period</Trans> }}
          >
            <Table.Column
              title={<Trans>Product</Trans>}
              key="name"
              render={(row: ProductRevenue) => (
                <Link to="/products" state={{ productModal: true, productId: row.productId }}>
                  {row.name}
                </Link>
              )}
            />
            <Table.Column
              title={<Trans>Revenue</Trans>}
              key="revenue"
              align="right"
              render={(row: ProductRevenue) => money(row.revenue)}
            />
          </Table>
        </>
      )}
    </>
  );
};

export default SalesByProduct;
