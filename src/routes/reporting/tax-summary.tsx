import { useEffect, useState } from "react";
import { Alert, Button, Col, DatePicker, Row, Table, Tooltip, Typography } from "antd";
import { useAtomValue } from "jotai";
import { Trans } from "@lingui/react/macro";
import { useLingui } from "@lingui/react";
import { CalculatorOutlined } from "@ant-design/icons";
import dayjs, { type Dayjs } from "dayjs";

import { organizationIdAtom, organizationAtom } from "src/atoms/organization";
import { GetTaxSummary } from "src/api";
import type { TaxSummary as TaxSummaryData, TaxSummaryLine } from "src/api";
import PageHeader from "src/components/page-header";
import { formatOrgCents } from "src/utils/currencies";
import { useDatePickerFormat } from "src/utils/date";
import { useTaxRateCategoryLabels } from "src/utils/tax-rate-categories";

const { RangePicker } = DatePicker;

const TaxSummary = () => {
  const { i18n } = useLingui();
  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);
  const dateFormat = useDatePickerFormat();

  const [range, setRange] = useState<[Dayjs, Dayjs]>([dayjs().subtract(12, "month"), dayjs()]);
  const [summary, setSummary] = useState<TaxSummaryData | null>(null);
  const [loading, setLoading] = useState(false);
  // A failed fetch used to reset summary to null, which the two tables and
  // the net liability below then rendered identically to a genuinely empty
  // period (0.00 everywhere) — a slow load or transient error looked exactly
  // like "nothing to report." Tracked separately so the page can show a real
  // error instead of a false all-clear, the same fix already applied to the
  // other accounting/reporting screens.
  const [failed, setFailed] = useState(false);
  const categoryLabels = useTaxRateCategoryLabels();

  const refresh = () => {
    if (!organizationId) return;
    setLoading(true);
    setFailed(false);
    GetTaxSummary(
      organizationId,
      range[0].startOf("day").valueOf(),
      range[1].endOf("day").valueOf(),
    )
      .then(setSummary)
      .catch(() => {
        setSummary(null);
        setFailed(true);
      })
      .finally(() => setLoading(false));
  };

  useEffect(refresh, [organizationId, range]);

  const money = (cents: number) => formatOrgCents(cents, organization, i18n.locale);

  const renderName = (line: TaxSummaryLine) =>
    line.taxRateId ? line.name : <Trans>Unrated</Trans>;

  const columns = [
    { title: <Trans>Rate</Trans>, key: "name", render: renderName },
    {
      title: <Trans>Category</Trans>,
      dataIndex: "categoryCode",
      key: "categoryCode",
      render: (code: string) =>
        code ? (
          <Tooltip title={categoryLabels[code as keyof typeof categoryLabels] ?? code}>
            {code}
          </Tooltip>
        ) : (
          "—"
        ),
    },
    {
      title: <Trans>%</Trans>,
      dataIndex: "percentage",
      key: "percentage",
      align: "right" as const,
      render: (v: number) => `${v}%`,
    },
    {
      title: <Trans>Base</Trans>,
      dataIndex: "base",
      key: "base",
      align: "right" as const,
      render: (v: number) => money(v),
    },
    {
      title: <Trans>Tax</Trans>,
      dataIndex: "tax",
      key: "tax",
      align: "right" as const,
      render: (v: number) => money(v),
    },
  ];

  const totalTax = (lines: TaxSummaryLine[]) => lines.reduce((sum, l) => sum + l.tax, 0);
  const totalOutputTax = summary ? totalTax(summary.output) : 0;
  const totalInputTax = summary ? totalTax(summary.input) : 0;
  const netVatLiability = totalOutputTax - totalInputTax;

  return (
    <>
      <PageHeader
        icon={<CalculatorOutlined />}
        title={<Trans>Tax Summary</Trans>}
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

      {failed && (
        <Alert
          style={{ marginTop: 16 }}
          type="error"
          showIcon
          message={<Trans>Couldn't load the tax summary</Trans>}
          action={
            <Button size="small" onClick={refresh}>
              <Trans>Retry</Trans>
            </Button>
          }
        />
      )}

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} xl={12}>
          <Typography.Title level={5}>
            <Trans>Output VAT (Sales)</Trans>
          </Typography.Title>
          <Table<TaxSummaryLine>
            dataSource={failed ? [] : (summary?.output ?? [])}
            columns={columns}
            rowKey="taxRateId"
            loading={loading}
            pagination={false}
            summary={(data) => (
              <Table.Summary.Row>
                <Table.Summary.Cell index={0} colSpan={4}>
                  <Typography.Text strong>
                    <Trans>Total output VAT</Trans>
                  </Typography.Text>
                </Table.Summary.Cell>
                <Table.Summary.Cell index={4} align="right">
                  <Typography.Text strong>
                    {failed ? "—" : money(totalTax(data as TaxSummaryLine[]))}
                  </Typography.Text>
                </Table.Summary.Cell>
              </Table.Summary.Row>
            )}
          />
        </Col>
        <Col xs={24} xl={12}>
          <Typography.Title level={5}>
            <Trans>Input VAT (Purchases)</Trans>
          </Typography.Title>
          <Table<TaxSummaryLine>
            dataSource={failed ? [] : (summary?.input ?? [])}
            columns={columns}
            rowKey="taxRateId"
            loading={loading}
            pagination={false}
            summary={(data) => (
              <Table.Summary.Row>
                <Table.Summary.Cell index={0} colSpan={4}>
                  <Typography.Text strong>
                    <Trans>Total input VAT</Trans>
                  </Typography.Text>
                </Table.Summary.Cell>
                <Table.Summary.Cell index={4} align="right">
                  <Typography.Text strong>
                    {failed ? "—" : money(totalTax(data as TaxSummaryLine[]))}
                  </Typography.Text>
                </Table.Summary.Cell>
              </Table.Summary.Row>
            )}
          />
        </Col>
      </Row>

      {/* The number that actually matters for a filing — output VAT owed
          less input VAT reclaimable — which the two totals above never
          combined into one figure. */}
      <Row style={{ marginTop: 16 }}>
        <Col span={24}>
          <Typography.Title level={5}>
            <Trans>Net VAT liability</Trans>
          </Typography.Title>
          <Typography.Text
            strong
            style={{ fontSize: 20 }}
            type={!failed && netVatLiability < 0 ? "success" : undefined}
          >
            {failed ? "—" : money(netVatLiability)}
          </Typography.Text>
          <br />
          <Typography.Text type="secondary">
            {!failed && netVatLiability < 0 ? (
              <Trans>Reclaimable — input VAT exceeds output VAT for this period</Trans>
            ) : (
              <Trans>Owed — output VAT exceeds input VAT for this period</Trans>
            )}
          </Typography.Text>
        </Col>
      </Row>
    </>
  );
};

export default TaxSummary;
