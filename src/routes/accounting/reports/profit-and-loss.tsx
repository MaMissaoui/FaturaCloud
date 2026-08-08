import { useEffect, useMemo, useState } from "react";
import { Col, Row, Select, Table, Typography } from "antd";
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

const ProfitAndLossReport = () => {
  const { i18n } = useLingui();
  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);
  const fiscalYears = useAtomValue(fiscalYearsAtom);
  const setFiscalYears = useSetAtom(setFiscalYearsAtom);

  const [fiscalYearId, setFiscalYearId] = useState<string>("");
  const [report, setReport] = useState<ProfitAndLoss | null>(null);
  const [loading, setLoading] = useState(false);

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

  useEffect(() => {
    if (!organizationId || !selectedYear) return;
    setLoading(true);
    GetProfitAndLoss(organizationId, selectedYear.startDate, selectedYear.endDate)
      .then(setReport)
      .catch(() => setReport(null))
      .finally(() => setLoading(false));
  }, [organizationId, selectedYear]);

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
                  <Typography.Text strong>{money(report?.totalRevenue ?? 0)}</Typography.Text>
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
                  <Typography.Text strong>{money(report?.totalExpenses ?? 0)}</Typography.Text>
                </Table.Summary.Cell>
              </Table.Summary.Row>
            )}
          />
        </Col>
      </Row>

      <Row style={{ marginTop: 24 }}>
        <Col span={24} style={{ textAlign: "right" }}>
          <Typography.Title level={4}>
            <Trans>Net income</Trans>: {money(report?.netIncome ?? 0)}
          </Typography.Title>
        </Col>
      </Row>
    </>
  );
};

export default ProfitAndLossReport;
