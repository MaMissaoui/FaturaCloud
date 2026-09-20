import { useEffect, useRef, useState } from "react";
import { Alert, Button, Card, DatePicker, Table } from "antd";
import { Bar } from "@ant-design/plots";
import { useAtomValue } from "jotai";
import { Link } from "react-router";
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
import { formatOrgCents } from "src/utils/currencies";
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
  // A failed fetch used to reset rows to [], which rendered identically to a
  // genuinely spend-free period — tracked separately so the page can show a
  // real error instead of a false all-clear.
  const [failed, setFailed] = useState(false);
  // requestIdRef guards against an in-flight earlier range's request
  // overwriting a newer one, the same shape as products.tsx's search guard.
  const requestIdRef = useRef(0);

  const refresh = () => {
    if (!organizationId) return;
    const requestId = ++requestIdRef.current;
    setLoading(true);
    setFailed(false);
    GetPurchasesByVendor(
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
          message={<Trans>Couldn't load purchases by vendor</Trans>}
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
              rows.length > 20 ? <Trans>Top 20 by spend</Trans> : <Trans>Spend by vendor</Trans>
            }
          >
            <div role="img" aria-label={t`Bar chart showing spend by vendor`}>
              <Bar
                data={rows.slice(0, 20)}
                xField="name"
                yField="spend"
                theme={themeMode === "dark" ? "classicDark" : "classic"}
                // Fixed height regardless of row count made G2Plot auto-hide
                // overlapping category-axis labels once this list neared 20 items —
                // scale with the actual number of bars instead.
                height={Math.max(280, Math.min(rows.length, 20) * 32)}
                axis={{ y: { labelFormatter: (v: number) => money(v) } }}
                tooltip={{
                  items: [
                    { field: "spend", name: t`Spend`, valueFormatter: (v: number) => money(v) },
                  ],
                }}
              />
            </div>
          </Card>

          <Table
            style={{ marginTop: 16 }}
            dataSource={rows}
            rowKey="vendorId"
            loading={loading}
            pagination={{ hideOnSinglePage: true, defaultPageSize: 50 }}
            locale={{ emptyText: <Trans>No purchases in this period</Trans> }}
          >
            <Table.Column
              title={<Trans>Vendor</Trans>}
              key="name"
              render={(row: VendorSpend) => (
                <Link to="/vendors" state={{ vendorModal: true, vendorId: row.vendorId }}>
                  {row.name}
                </Link>
              )}
            />
            <Table.Column
              title={<Trans>Spend</Trans>}
              key="spend"
              align="right"
              render={(row: VendorSpend) => money(row.spend)}
            />
          </Table>
        </>
      )}
    </>
  );
};

export default PurchasesByVendor;
