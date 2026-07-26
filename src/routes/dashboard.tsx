import { useCallback, useEffect, useState } from "react";
import { Card, Col, Row, Select, Statistic, Table, theme } from "antd";
import { Column } from "@ant-design/plots";
import { useAtomValue } from "jotai";
import { Trans } from "@lingui/react/macro";
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

const PERIOD_OPTIONS = [3, 6, 12, 24];

const Dashboard = () => {
  useLingui();
  const { i18n } = useLingui();
  const { token } = theme.useToken();
  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);
  const themeMode = useAtomValue(themeAtom);

  const [months, setMonths] = useState(12);
  const [data, setData] = useState<DashboardData | null>(null);
  const [loading, setLoading] = useState(false);

  const fetchDashboard = useCallback(() => {
    if (!organizationId) return;
    setLoading(true);
    GetDashboard(organizationId, months)
      .then(setData)
      .finally(() => setLoading(false));
  }, [organizationId, months]);

  useEffect(() => {
    fetchDashboard();
  }, [fetchDashboard]);

  const money = (cents: number) =>
    Intl.NumberFormat(i18n.locale, {
      style: "currency",
      currency: organization?.currency ?? "EUR",
      minimumFractionDigits: organization?.minimum_fraction_digits ?? undefined,
    }).format(cents / 100);

  const revenueTotal = (data?.revenueByMonth ?? []).reduce((sum, m) => sum + m.revenue, 0);

  return (
    <>
      <PageHeader
        icon={<DashboardOutlined />}
        title={<Trans>Dashboard</Trans>}
        actions={
          <Select
            value={months}
            onChange={setMonths}
            style={{ width: 160 }}
            options={PERIOD_OPTIONS.map((m) => ({
              value: m,
              label: <Trans>Last {m} months</Trans>,
            }))}
          />
        }
      />

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} md={8}>
          <Card loading={loading}>
            <Statistic
              title={<Trans>Revenue (selected period)</Trans>}
              value={money(revenueTotal)}
            />
          </Card>
        </Col>
        <Col xs={24} md={8}>
          <Card loading={loading}>
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
          <Card loading={loading}>
            <Statistic
              title={<Trans>Stock valuation</Trans>}
              value={money(data?.stockValuation.total ?? 0)}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col span={24}>
          <Card title={<Trans>Revenue over time</Trans>} loading={loading}>
            <Column
              data={data?.revenueByMonth ?? []}
              xField="month"
              yField="revenue"
              theme={themeMode === "dark" ? "classicDark" : "classic"}
              height={280}
              axis={{ y: { labelFormatter: (v: number) => money(v) } }}
              tooltip={{ items: [{ field: "revenue", valueFormatter: (v: number) => money(v) }] }}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} xl={12}>
          <Card title={<Trans>Outstanding invoices</Trans>} loading={loading}>
            <Row gutter={[8, 8]} style={{ marginBottom: 16 }}>
              <Col span={8}>
                <Statistic
                  title={<Trans>Current</Trans>}
                  value={money(data?.outstanding.current ?? 0)}
                />
              </Col>
              <Col span={8}>
                <Statistic
                  title={<Trans>1-30 days</Trans>}
                  value={money(data?.outstanding.days1To30 ?? 0)}
                />
              </Col>
              <Col span={8}>
                <Statistic
                  title={<Trans>31-60 days</Trans>}
                  value={money(data?.outstanding.days31To60 ?? 0)}
                />
              </Col>
              <Col span={8}>
                <Statistic
                  title={<Trans>61-90 days</Trans>}
                  value={money(data?.outstanding.days61To90 ?? 0)}
                />
              </Col>
              <Col span={8}>
                <Statistic
                  title={<Trans>90+ days</Trans>}
                  value={money(data?.outstanding.days90Plus ?? 0)}
                  styles={{
                    content: {
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
          <Card title={<Trans>Stock valuation</Trans>} loading={loading}>
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

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} xl={12}>
          <Card title={<Trans>Top clients</Trans>} loading={loading}>
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
          <Card title={<Trans>Top products</Trans>} loading={loading}>
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
