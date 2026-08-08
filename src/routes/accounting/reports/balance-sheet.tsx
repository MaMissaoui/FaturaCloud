import { useEffect, useState } from "react";
import { Col, DatePicker, Row, Table, Typography } from "antd";
import { useAtomValue } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { FundOutlined } from "@ant-design/icons";
import dayjs, { type Dayjs } from "dayjs";

import { GetBalanceSheet } from "src/api";
import type { BalanceSheet, BalanceSheetLine } from "src/types/models";
import { organizationIdAtom, organizationAtom } from "src/atoms/organization";
import { useDatePickerFormat } from "src/utils/date";
import PageHeader from "src/components/page-header";

const BalanceSheetReport = () => {
  const { i18n } = useLingui();
  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);
  const dateFormat = useDatePickerFormat();

  const [asOfDate, setAsOfDate] = useState<Dayjs>(dayjs());
  const [report, setReport] = useState<BalanceSheet | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!organizationId) return;
    setLoading(true);
    GetBalanceSheet(organizationId, asOfDate.valueOf())
      .then(setReport)
      .catch(() => setReport(null))
      .finally(() => setLoading(false));
  }, [organizationId, asOfDate]);

  const money = (cents: number) =>
    Intl.NumberFormat(i18n.locale, {
      style: "currency",
      currency: organization?.currency ?? "EUR",
      minimumFractionDigits: organization?.minimum_fraction_digits ?? undefined,
    }).format(cents / 100);

  const columns = [
    { title: <Trans>Code</Trans>, dataIndex: "code", key: "code", width: 100 },
    { title: <Trans>Account</Trans>, dataIndex: "name", key: "name" },
    {
      title: <Trans>Amount</Trans>,
      dataIndex: "amount",
      key: "amount",
      align: "right" as const,
      render: (v: number) => money(v),
    },
  ];

  const equityRows = report
    ? [
        ...(report.equity ?? []),
        {
          accountId: "current-earnings",
          code: "",
          name: t`Current earnings`,
          amount: report.currentEarnings,
        },
      ]
    : [];

  return (
    <>
      <PageHeader
        icon={<FundOutlined />}
        title={<Trans>Balance Sheet</Trans>}
        extra={
          <DatePicker
            value={asOfDate}
            format={dateFormat}
            allowClear={false}
            onChange={(d) => d && setAsOfDate(d)}
          />
        }
      />

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} xl={12}>
          <Typography.Title level={5}>
            <Trans>Assets</Trans>
          </Typography.Title>
          <Table<BalanceSheetLine>
            dataSource={report?.assets ?? []}
            columns={columns}
            rowKey="accountId"
            loading={loading}
            pagination={false}
            summary={() => (
              <Table.Summary.Row>
                <Table.Summary.Cell index={0} colSpan={2}>
                  <Typography.Text strong>
                    <Trans>Total assets</Trans>
                  </Typography.Text>
                </Table.Summary.Cell>
                <Table.Summary.Cell index={2} align="right">
                  <Typography.Text strong>{money(report?.totalAssets ?? 0)}</Typography.Text>
                </Table.Summary.Cell>
              </Table.Summary.Row>
            )}
          />
        </Col>
        <Col xs={24} xl={12}>
          <Typography.Title level={5}>
            <Trans>Liabilities</Trans>
          </Typography.Title>
          <Table<BalanceSheetLine>
            dataSource={report?.liabilities ?? []}
            columns={columns}
            rowKey="accountId"
            loading={loading}
            pagination={false}
            summary={() => (
              <Table.Summary.Row>
                <Table.Summary.Cell index={0} colSpan={2}>
                  <Typography.Text strong>
                    <Trans>Total liabilities</Trans>
                  </Typography.Text>
                </Table.Summary.Cell>
                <Table.Summary.Cell index={2} align="right">
                  <Typography.Text strong>{money(report?.totalLiabilities ?? 0)}</Typography.Text>
                </Table.Summary.Cell>
              </Table.Summary.Row>
            )}
            style={{ marginBottom: 24 }}
          />

          <Typography.Title level={5}>
            <Trans>Equity</Trans>
          </Typography.Title>
          <Table<BalanceSheetLine>
            dataSource={equityRows}
            columns={columns}
            rowKey="accountId"
            loading={loading}
            pagination={false}
            summary={() => (
              <Table.Summary.Row>
                <Table.Summary.Cell index={0} colSpan={2}>
                  <Typography.Text strong>
                    <Trans>Total equity</Trans>
                  </Typography.Text>
                </Table.Summary.Cell>
                <Table.Summary.Cell index={2} align="right">
                  <Typography.Text strong>{money(report?.totalEquity ?? 0)}</Typography.Text>
                </Table.Summary.Cell>
              </Table.Summary.Row>
            )}
          />
        </Col>
      </Row>
    </>
  );
};

export default BalanceSheetReport;
