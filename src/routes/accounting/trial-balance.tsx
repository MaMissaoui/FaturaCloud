import { useEffect, useState } from "react";
import type { TrialBalanceRow } from "src/types/models";
import { useLocation } from "react-router";
import { Col, Row, Select, Table, Typography } from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import { atom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { TableOutlined } from "@ant-design/icons";
import sum from "lodash/sum";

import { GetTrialBalance } from "src/api";
import { organizationIdAtom } from "src/atoms/organization";
import {
  fiscalYearsAtom,
  setFiscalYearsAtom,
  fiscalPeriodsAtom,
  loadFiscalPeriodsAtom,
} from "src/atoms/fiscal-period";
import PageHeader from "src/components/page-header";

const fiscalYearFilterAtom = atom<string>("");
const fiscalPeriodFilterAtom = atom<string>("");

const TrialBalance = () => {
  useLingui();
  const location = useLocation();
  const organizationId = useAtomValue(organizationIdAtom);

  const fiscalYears = useAtomValue(fiscalYearsAtom);
  const setFiscalYears = useSetAtom(setFiscalYearsAtom);
  const fiscalPeriodsByYear = useAtomValue(fiscalPeriodsAtom);
  const loadFiscalPeriods = useSetAtom(loadFiscalPeriodsAtom);

  const [fiscalYearId, setFiscalYearId] = useAtom(fiscalYearFilterAtom);
  const [fiscalPeriodId, setFiscalPeriodId] = useAtom(fiscalPeriodFilterAtom);

  const [rows, setRows] = useState<TrialBalanceRow[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (location.pathname === "/accounting/trial-balance") {
      setFiscalYears();
    }
  }, [location, setFiscalYears]);

  useEffect(() => {
    if (fiscalYearId) loadFiscalPeriods(fiscalYearId);
  }, [fiscalYearId, loadFiscalPeriods]);

  useEffect(() => {
    if (!organizationId) return;
    setLoading(true);
    GetTrialBalance(organizationId, fiscalPeriodId || undefined)
      .then(setRows)
      .catch(() => setRows([]))
      .finally(() => setLoading(false));
  }, [organizationId, fiscalPeriodId]);

  const periods = fiscalYearId ? (fiscalPeriodsByYear[fiscalYearId] ?? []) : [];
  const totalDebit = sum(rows.map((r) => r.debit)) / 100;
  const totalCredit = sum(rows.map((r) => r.credit)) / 100;

  return (
    <>
      <PageHeader
        icon={<TableOutlined />}
        title={<Trans>Trial Balance</Trans>}
        extra={
          <>
            <Select
              allowClear
              placeholder={t`All fiscal years`}
              style={{ width: 180 }}
              value={fiscalYearId || undefined}
              onChange={(v) => {
                setFiscalYearId(v ?? "");
                setFiscalPeriodId("");
              }}
              options={fiscalYears.map((y) => ({ value: y.id, label: y.name }))}
            />
            <Select
              allowClear
              disabled={!fiscalYearId}
              placeholder={t`All periods`}
              style={{ width: 180, marginLeft: 8 }}
              value={fiscalPeriodId || undefined}
              onChange={(v) => setFiscalPeriodId(v ?? "")}
              options={periods.map((p) => ({ value: p.id, label: p.name }))}
            />
          </>
        }
      />
      <Row style={{ marginTop: 16 }}>
        <Col span={24}>
          <Table
            dataSource={rows}
            pagination={{ hideOnSinglePage: true, defaultPageSize: 50 }}
            rowKey="accountId"
            loading={loading}
            summary={() => (
              <Table.Summary.Row>
                <Table.Summary.Cell index={0} colSpan={2}>
                  <Typography.Text strong>
                    <Trans>Total</Trans>
                  </Typography.Text>
                </Table.Summary.Cell>
                <Table.Summary.Cell index={2} align="right">
                  <Typography.Text strong>{totalDebit.toFixed(2)}</Typography.Text>
                </Table.Summary.Cell>
                <Table.Summary.Cell index={3} align="right">
                  <Typography.Text strong>{totalCredit.toFixed(2)}</Typography.Text>
                </Table.Summary.Cell>
              </Table.Summary.Row>
            )}
          >
            <Table.Column
              title={<Trans>Code</Trans>}
              dataIndex="code"
              key="code"
              width={100}
              sorter={(a: TrialBalanceRow, b: TrialBalanceRow) => a.code.localeCompare(b.code)}
              defaultSortOrder="ascend"
            />
            <Table.Column title={<Trans>Name</Trans>} dataIndex="name" key="name" />
            <Table.Column
              title={<Trans>Debit</Trans>}
              dataIndex="debit"
              key="debit"
              align="right"
              render={(v: number) => (v / 100).toFixed(2)}
            />
            <Table.Column
              title={<Trans>Credit</Trans>}
              dataIndex="credit"
              key="credit"
              align="right"
              render={(v: number) => (v / 100).toFixed(2)}
            />
          </Table>
        </Col>
      </Row>
    </>
  );
};

export default TrialBalance;
