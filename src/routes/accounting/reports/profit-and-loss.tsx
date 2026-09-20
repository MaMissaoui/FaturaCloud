import { useCallback, useEffect, useMemo, useState } from "react";
import { Alert, Button, Card, Col, Row, Select, Table, Typography, theme } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { LineChartOutlined } from "@ant-design/icons";

import { GetProfitAndLoss } from "src/api";
import type { ProfitAndLoss, ProfitAndLossLine } from "src/types/models";
import { organizationIdAtom, organizationAtom } from "src/atoms/organization";
import { fiscalYearsAtom, setFiscalYearsAtom } from "src/atoms/fiscal-period";
import PageHeader from "src/components/page-header";
import { formatOrgCents } from "src/utils/currencies";

const ProfitAndLossReport = () => {
  const { i18n } = useLingui();
  const { token } = theme.useToken();
  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);
  const fiscalYears = useAtomValue(fiscalYearsAtom);
  const setFiscalYears = useSetAtom(setFiscalYearsAtom);

  const [fiscalYearId, setFiscalYearId] = useState<string>("");
  const [report, setReport] = useState<ProfitAndLoss | null>(null);
  const [loading, setLoading] = useState(false);
  // A failed fetch used to reset report to null, which rendered identically
  // to a genuinely empty period — every total 0.00 and a green Net income
  // card. Tracked separately so this page shows a real error instead of a
  // false all-clear.
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    setFiscalYears();
  }, [setFiscalYears]);

  // Default to the fiscal year covering today, falling back to the most
  // recent one — same "pick a sensible starting scope" idea as the trial
  // balance page, just auto-selected since a P&L with no range is
  // meaningless (the backend rejects an all-zero range outright).
  useEffect(() => {
    if (fiscalYearId || fiscalYears.length === 0) return;
    const now = Date.now();
    const current = fiscalYears.find((y) => y.startDate <= now && now <= y.endDate);
    setFiscalYearId((current ?? fiscalYears[0]).id);
  }, [fiscalYears, fiscalYearId]);

  const selectedYear = useMemo(
    () => fiscalYears.find((y) => y.id === fiscalYearId),
    [fiscalYears, fiscalYearId],
  );

  const refresh = useCallback(() => {
    if (!organizationId || !selectedYear) return;
    setLoading(true);
    setFailed(false);
    GetProfitAndLoss(organizationId, selectedYear.startDate, selectedYear.endDate)
      .then(setReport)
      .catch(() => {
        setReport(null);
        setFailed(true);
      })
      .finally(() => setLoading(false));
  }, [organizationId, selectedYear]);

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

  return (
    <>
      <PageHeader
        icon={<LineChartOutlined />}
        title={<Trans>Profit &amp; Loss</Trans>}
        extra={
          <Select
            placeholder={t`Select a fiscal year`}
            style={{ width: 180 }}
            value={fiscalYearId || undefined}
            onChange={setFiscalYearId}
            options={fiscalYears.map((y) => ({ value: y.id, label: y.name }))}
          />
        }
      />

      {failed && (
        <Alert
          style={{ marginTop: 16 }}
          type="error"
          showIcon
          message={<Trans>Couldn't load the profit &amp; loss report</Trans>}
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
            <Trans>Revenue</Trans>
          </Typography.Title>
          <Table<ProfitAndLossLine>
            dataSource={report?.revenue ?? []}
            columns={columns}
            rowKey="accountId"
            loading={loading}
            pagination={false}
            summary={() => (
              <Table.Summary.Row>
                <Table.Summary.Cell index={0} colSpan={2}>
                  <Typography.Text strong>
                    <Trans>Total revenue</Trans>
                  </Typography.Text>
                </Table.Summary.Cell>
                <Table.Summary.Cell index={2} align="right">
                  <Typography.Text strong>
                    {failed ? "—" : money(report?.totalRevenue ?? 0)}
                  </Typography.Text>
                </Table.Summary.Cell>
              </Table.Summary.Row>
            )}
          />
        </Col>
        <Col xs={24} xl={12}>
          <Typography.Title level={5}>
            <Trans>Expenses</Trans>
          </Typography.Title>
          <Table<ProfitAndLossLine>
            dataSource={report?.expenses ?? []}
            columns={columns}
            rowKey="accountId"
            loading={loading}
            pagination={false}
            summary={() => (
              <Table.Summary.Row>
                <Table.Summary.Cell index={0} colSpan={2}>
                  <Typography.Text strong>
                    <Trans>Total expenses</Trans>
                  </Typography.Text>
                </Table.Summary.Cell>
                <Table.Summary.Cell index={2} align="right">
                  <Typography.Text strong>
                    {failed ? "—" : money(report?.totalExpenses ?? 0)}
                  </Typography.Text>
                </Table.Summary.Cell>
              </Table.Summary.Row>
            )}
          />
        </Col>
      </Row>

      <Row style={{ marginTop: 24 }}>
        <Col span={24}>
          <Card
            style={{
              background: failed
                ? undefined
                : (report?.netIncome ?? 0) >= 0
                  ? token.colorSuccessBg
                  : token.colorErrorBg,
            }}
          >
            <Row justify="space-between" align="middle">
              <Col>
                <Typography.Title level={4} style={{ margin: 0 }}>
                  <Trans>Net income</Trans>
                </Typography.Title>
              </Col>
              <Col>
                <Typography.Title
                  level={3}
                  style={{
                    margin: 0,
                    color: failed
                      ? undefined
                      : (report?.netIncome ?? 0) >= 0
                        ? token.colorSuccess
                        : token.colorError,
                  }}
                >
                  {failed ? "—" : money(report?.netIncome ?? 0)}
                </Typography.Title>
              </Col>
            </Row>
          </Card>
        </Col>
      </Row>
    </>
  );
};

export default ProfitAndLossReport;
