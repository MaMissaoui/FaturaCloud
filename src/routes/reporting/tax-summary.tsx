import { useEffect, useState } from "react";
import { Col, DatePicker, Row, Table, Typography } from "antd";
import { useAtomValue } from "jotai";
import { Trans } from "@lingui/react/macro";
import { useLingui } from "@lingui/react";
import { CalculatorOutlined } from "@ant-design/icons";
import dayjs, { type Dayjs } from "dayjs";

import { organizationIdAtom, organizationAtom } from "src/atoms/organization";
import { GetTaxSummary } from "src/api";
import type { TaxSummary as TaxSummaryData, TaxSummaryLine } from "src/api";
import PageHeader from "src/components/page-header";
import { useDatePickerFormat } from "src/utils/date";

const { RangePicker } = DatePicker;

const TaxSummary = () => {
  const { i18n } = useLingui();
  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);
  const dateFormat = useDatePickerFormat();

  const [range, setRange] = useState<[Dayjs, Dayjs]>([dayjs().subtract(12, "month"), dayjs()]);
  const [summary, setSummary] = useState<TaxSummaryData | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!organizationId) return;
    setLoading(true);
    GetTaxSummary(organizationId, range[0].startOf("day").valueOf(), range[1].endOf("day").valueOf())
      .then(setSummary)
      .catch(() => setSummary(null))
      .finally(() => setLoading(false));
  }, [organizationId, range]);

  const money = (cents: number) =>
    Intl.NumberFormat(i18n.locale, {
      style: "currency",
      currency: organization?.currency ?? "EUR",
      minimumFractionDigits: organization?.minimum_fraction_digits ?? undefined,
    }).format(cents / 100);

  const renderName = (line: TaxSummaryLine) => (line.taxRateId ? line.name : <Trans>Unrated</Trans>);

  const columns = [
    { title: <Trans>Rate</Trans>, key: "name", render: renderName },
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

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} xl={12}>
          <Typography.Title level={5}>
            <Trans>Output VAT (Sales)</Trans>
          </Typography.Title>
          <Table<TaxSummaryLine>
            dataSource={summary?.output ?? []}
            columns={columns}
            rowKey="taxRateId"
            loading={loading}
            pagination={false}
            summary={(data) => (
              <Table.Summary.Row>
                <Table.Summary.Cell index={0} colSpan={3}>
                  <Typography.Text strong>
                    <Trans>Total output VAT</Trans>
                  </Typography.Text>
                </Table.Summary.Cell>
                <Table.Summary.Cell index={3} align="right">
                  <Typography.Text strong>{money(totalTax(data as TaxSummaryLine[]))}</Typography.Text>
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
            dataSource={summary?.input ?? []}
            columns={columns}
            rowKey="taxRateId"
            loading={loading}
            pagination={false}
            summary={(data) => (
              <Table.Summary.Row>
                <Table.Summary.Cell index={0} colSpan={3}>
                  <Typography.Text strong>
                    <Trans>Total input VAT</Trans>
                  </Typography.Text>
                </Table.Summary.Cell>
                <Table.Summary.Cell index={3} align="right">
                  <Typography.Text strong>{money(totalTax(data as TaxSummaryLine[]))}</Typography.Text>
                </Table.Summary.Cell>
              </Table.Summary.Row>
            )}
          />
        </Col>
      </Row>
    </>
  );
};

export default TaxSummary;
