import { useCallback, useEffect, useState } from "react";
import { Alert, Button, Col, DatePicker, Row, Space, Table, Tag, Typography } from "antd";
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
import { formatOrgCents } from "src/utils/currencies";

const BalanceSheetReport = () => {
  const { i18n } = useLingui();
  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);
  const dateFormat = useDatePickerFormat();

  const [asOfDate, setAsOfDate] = useState<Dayjs>(dayjs());
  const [report, setReport] = useState<BalanceSheet | null>(null);
  const [loading, setLoading] = useState(false);
  // A failed fetch used to reset report to null, which rendered identically
  // to a genuinely empty balance sheet — every total 0.00 and the equation
  // check hidden. Tracked separately so this page shows a real error instead
  // of a false all-clear.
  const [failed, setFailed] = useState(false);

  const refresh = useCallback(() => {
    if (!organizationId) return;
    setLoading(true);
    setFailed(false);
    GetBalanceSheet(organizationId, asOfDate.valueOf())
      .then(setReport)
      .catch(() => {
        setReport(null);
        setFailed(true);
      })
      .finally(() => setLoading(false));
  }, [organizationId, asOfDate]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const money = (cents: number) => formatOrgCents(cents, organization, i18n.locale);

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

      {failed && (
        <Alert
          style={{ marginTop: 16 }}
          type="error"
          showIcon
          message={<Trans>Couldn't load the balance sheet</Trans>}
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
                  <Typography.Text strong>
                    {failed ? "—" : money(report?.totalAssets ?? 0)}
                  </Typography.Text>
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
                  <Typography.Text strong>
                    {failed ? "—" : money(report?.totalLiabilities ?? 0)}
                  </Typography.Text>
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
                  <Typography.Text strong>
                    {failed ? "—" : money(report?.totalEquity ?? 0)}
                  </Typography.Text>
                </Table.Summary.Cell>
              </Table.Summary.Row>
            )}
          />
        </Col>
      </Row>

      {report && (
        <Row style={{ marginTop: 16 }}>
          <Col span={24}>
            <Space align="center">
              <Typography.Text strong>
                <Trans>Accounting equation check:</Trans>
              </Typography.Text>
              <Typography.Text>{money(report.totalAssets)}</Typography.Text>
              <Typography.Text strong>=</Typography.Text>
              <Typography.Text>
                {money(report.totalLiabilities)} + {money(report.totalEquity)}
              </Typography.Text>
              <Typography.Text strong>=</Typography.Text>
              <Typography.Text>
                {money(report.totalLiabilities + report.totalEquity)}
              </Typography.Text>
              {report.totalAssets === report.totalLiabilities + report.totalEquity ? (
                <Tag color="green">
                  <Trans>Balanced</Trans>
                </Tag>
              ) : (
                <Tag color="red">
                  <Trans>Out of balance</Trans>
                </Tag>
              )}
            </Space>
          </Col>
        </Row>
      )}
    </>
  );
};

export default BalanceSheetReport;
