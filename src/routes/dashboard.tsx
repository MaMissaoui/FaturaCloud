import { useCallback, useEffect, useState } from "react";
import type { CSSProperties } from "react";
import { Card, Col, Row, Select, Statistic, Table, theme } from "antd";
import { Column } from "@ant-design/plots";
import { useAtomValue } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { DashboardOutlined } from "@ant-design/icons";

import { organizationIdAtom, organizationAtom } from "src/atoms/organization";
import { themeAtom } from "src/atoms/generic";
import { GetDashboard } from "src/api";
import type {
  DashboardData,
  OutstandingInvoiceSummary,
  StockValuationItem,
  ClientRevenue,
  ProductRevenue,
} from "src/api";
import PageHeader from "src/components/page-header";
import { formatOrgCents } from "src/utils/currencies";

const MONTH_OPTIONS = [3, 6, 12, 24];

// The "Outstanding invoices" card packs 5 Statistics into a half-width
// column — antd's Statistic value has no wrap/overflow handling of its own,
// so a large organization's real total (e.g. "TND 4,515,363.83") overflowed
// horizontally straight into the next column's value instead of wrapping.
// Module-level (not inline) so the object is referentially stable across
// renders, same reasoning as orders/details.tsx's getOrderStatusColor.
const outstandingStatisticStyle: CSSProperties = {
  whiteSpace: "normal",
  wordBreak: "break-word",
  fontSize: 18,
};

// A rolling window ("m12") or a calendar year ("y2026") in one Select —
// years are generated at render time (not a fixed list) so "current
// year"/"last year" never go stale, and a handful of further-back years
// stay reachable without the list growing unbounded.
const CALENDAR_YEARS_BACK = 4;

type Period = { kind: "months"; months: number } | { kind: "year"; year: number };

const periodToValue = (p: Period) => (p.kind === "months" ? `m${p.months}` : `y${p.year}`);

const periodFromValue = (v: string): Period =>
  v[0] === "y"
    ? { kind: "year", year: Number(v.slice(1)) }
    : { kind: "months", months: Number(v.slice(1)) };

const Dashboard = () => {
  useLingui();
  const { i18n } = useLingui();
  const { token } = theme.useToken();
  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);
  const themeMode = useAtomValue(themeAtom);

  const [period, setPeriod] = useState<Period>({ kind: "months", months: 12 });
  const [data, setData] = useState<DashboardData | null>(null);
  const [loading, setLoading] = useState(false);

  const fetchDashboard = useCallback(() => {
    if (!organizationId) return;
    setLoading(true);
    GetDashboard(
      organizationId,
      period.kind === "year" ? { year: period.year } : { months: period.months },
    )
      .then(setData)
      .finally(() => setLoading(false));
  }, [organizationId, period]);

  useEffect(() => {
    fetchDashboard();
  }, [fetchDashboard]);

  const money = (cents: number) => formatOrgCents(cents, organization, i18n.locale);

  const revenueTotal = (data?.revenueByMonth ?? []).reduce((sum, m) => sum + m.revenue, 0);

  return (
    <>
      <PageHeader
        icon={<DashboardOutlined />}
        title={<Trans>Dashboard</Trans>}
        actions={
          <Select
            value={periodToValue(period)}
            onChange={(v) => setPeriod(periodFromValue(v))}
            style={{ width: 180 }}
            options={[
              {
                label: t`Rolling window`,
                options: MONTH_OPTIONS.map((m) => ({
                  value: `m${m}`,
                  label: <Trans>Last {m} months</Trans>,
                })),
              },
              {
                label: t`Calendar year`,
                options: Array.from({ length: CALENDAR_YEARS_BACK + 1 }, (_, i) => {
                  const year = new Date().getFullYear() - i;
                  const label =
                    i === 0 ? (
                      <Trans>Current year ({year})</Trans>
                    ) : i === 1 ? (
                      <Trans>Last year ({year})</Trans>
                    ) : (
                      String(year)
                    );
                  return { value: `y${year}`, label };
                }),
              },
            ]}
          />
        }
      />

      <Row gutter={[16, 16]} style={{ marginTop: 12 }}>
        <Col xs={24} md={8}>
          <Card size="small" loading={loading}>
            <Statistic
              title={<Trans>Revenue (selected period)</Trans>}
              value={money(revenueTotal)}
            />
          </Card>
        </Col>
        <Col xs={24} md={8}>
          <Card size="small" loading={loading}>
            <Statistic
              title={<Trans>Outstanding</Trans>}
              value={money(data?.outstanding.total ?? 0)}
              styles={{
                content: {
                  color: (data?.outstanding.total ?? 0) > 0 ? token.colorError : undefined,
                },
              }}
            />
          </Card>
        </Col>
        <Col xs={24} md={8}>
          <Card size="small" loading={loading}>
            <Statistic
              title={<Trans>Stock valuation</Trans>}
              value={money(data?.stockValuation.total ?? 0)}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 12 }}>
        <Col span={24}>
          <Card size="small" title={<Trans>Revenue over time</Trans>} loading={loading}>
            <Column
              data={data?.revenueByMonth ?? []}
              xField="month"
              yField="revenue"
              theme={themeMode === "dark" ? "classicDark" : "classic"}
              height={220}
              axis={{ y: { labelFormatter: (v: number) => money(v) } }}
              tooltip={{
                items: [
                  { field: "revenue", name: t`Revenue`, valueFormatter: (v: number) => money(v) },
                ],
              }}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 12 }}>
        <Col xs={24} xl={12}>
          <Card size="small" title={<Trans>Outstanding invoices</Trans>} loading={loading}>
            <Row gutter={[8, 12]} style={{ marginBottom: 12 }}>
              <Col xs={12} sm={8}>
                <Statistic
                  title={<Trans>Current</Trans>}
                  value={money(data?.outstanding.current ?? 0)}
                  styles={{ content: outstandingStatisticStyle }}
                />
              </Col>
              <Col xs={12} sm={8}>
                <Statistic
                  title={<Trans>1-30 days</Trans>}
                  value={money(data?.outstanding.days1To30 ?? 0)}
                  styles={{ content: outstandingStatisticStyle }}
                />
              </Col>
              <Col xs={12} sm={8}>
                <Statistic
                  title={<Trans>31-60 days</Trans>}
                  value={money(data?.outstanding.days31To60 ?? 0)}
                  styles={{ content: outstandingStatisticStyle }}
                />
              </Col>
              <Col xs={12} sm={8}>
                <Statistic
                  title={<Trans>61-90 days</Trans>}
                  value={money(data?.outstanding.days61To90 ?? 0)}
                  styles={{ content: outstandingStatisticStyle }}
                />
              </Col>
              <Col xs={12} sm={8}>
                <Statistic
                  title={<Trans>90+ days</Trans>}
                  value={money(data?.outstanding.days90Plus ?? 0)}
                  styles={{
                    content: {
                      ...outstandingStatisticStyle,
                      color: (data?.outstanding.days90Plus ?? 0) > 0 ? token.colorError : undefined,
                    },
                  }}
                />
              </Col>
            </Row>
            <Table
              dataSource={data?.outstanding.invoices ?? []}
              rowKey="id"
              size="small"
              pagination={{ pageSize: 5, hideOnSinglePage: true }}
              locale={{ emptyText: <Trans>No outstanding invoices</Trans> }}
            >
              <Table.Column title={<Trans>Invoice</Trans>} dataIndex="number" key="number" />
              <Table.Column title={<Trans>Client</Trans>} dataIndex="clientName" key="clientName" />
              <Table.Column
                title={<Trans>Days overdue</Trans>}
                dataIndex="daysOverdue"
                key="daysOverdue"
                align="right"
                render={(days: number) => (days > 0 ? days : "—")}
              />
              <Table.Column
                title={<Trans>Total</Trans>}
                key="total"
                align="right"
                render={(inv: OutstandingInvoiceSummary) => money(inv.total)}
              />
            </Table>
          </Card>
        </Col>

        <Col xs={24} xl={12}>
          <Card size="small" title={<Trans>Stock valuation</Trans>} loading={loading}>
            <Table
              dataSource={data?.stockValuation.items ?? []}
              rowKey="productId"
              size="small"
              pagination={false}
              locale={{ emptyText: <Trans>No stock-tracked products</Trans> }}
            >
              <Table.Column title={<Trans>Product</Trans>} dataIndex="name" key="name" />
              <Table.Column
                title={<Trans>Quantity</Trans>}
                dataIndex="quantity"
                key="quantity"
                align="right"
                render={(qty: number) => (qty % 1 === 0 ? qty : qty.toFixed(2))}
              />
              <Table.Column
                title={<Trans>Value</Trans>}
                key="value"
                align="right"
                render={(item: StockValuationItem) => money(item.value)}
              />
            </Table>
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 12 }}>
        <Col xs={24} xl={12}>
          <Card size="small" title={<Trans>Top clients</Trans>} loading={loading}>
            <Table
              dataSource={data?.topClients ?? []}
              rowKey="clientId"
              size="small"
              pagination={false}
              locale={{ emptyText: <Trans>No revenue in this period</Trans> }}
            >
              <Table.Column title={<Trans>Client</Trans>} dataIndex="name" key="name" />
              <Table.Column
                title={<Trans>Revenue</Trans>}
                key="revenue"
                align="right"
                render={(c: ClientRevenue) => money(c.revenue)}
              />
            </Table>
          </Card>
        </Col>

        <Col xs={24} xl={12}>
          <Card size="small" title={<Trans>Top products</Trans>} loading={loading}>
            <Table
              dataSource={data?.topProducts ?? []}
              rowKey="productId"
              size="small"
              pagination={false}
              locale={{ emptyText: <Trans>No revenue in this period</Trans> }}
            >
              <Table.Column title={<Trans>Product</Trans>} dataIndex="name" key="name" />
              <Table.Column
                title={<Trans>Revenue</Trans>}
                key="revenue"
                align="right"
                render={(p: ProductRevenue) => money(p.revenue)}
              />
            </Table>
          </Card>
        </Col>
      </Row>
    </>
  );
};

export default Dashboard;
