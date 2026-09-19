import { useCallback, useEffect, useState } from "react";
import { Card, Col, Row, Statistic, Table, theme, Typography } from "antd";
import { useAtomValue } from "jotai";
import { Trans } from "@lingui/react/macro";
import { useLingui } from "@lingui/react";
import { ClockCircleOutlined } from "@ant-design/icons";

import { GetReceivableAging } from "src/api";
import type { OutstandingInvoiceSummary, OutstandingSummary } from "src/api";
import { organizationIdAtom, organizationAtom } from "src/atoms/organization";
import PageHeader from "src/components/page-header";
import { formatOrgCents, numberFormatLocale } from "src/utils/currencies";
import { formatCents } from "src/utils/currency";

const ReceivableAging = () => {
  const { i18n } = useLingui();
  const { token } = theme.useToken();
  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);

  const [summary, setSummary] = useState<OutstandingSummary | null>(null);
  const [loading, setLoading] = useState(false);

  const refresh = useCallback(() => {
    if (!organizationId) return;
    setLoading(true);
    GetReceivableAging(organizationId)
      .then(setSummary)
      .catch(() => setSummary(null))
      .finally(() => setLoading(false));
  }, [organizationId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const money = (cents: number) => formatOrgCents(cents, organization, i18n.locale);

  return (
    <>
      <PageHeader icon={<ClockCircleOutlined />} title={<Trans>AR Aging</Trans>} />

      <Row gutter={[8, 8]} style={{ marginTop: 16, marginBottom: 16 }}>
        <Col xs={12} md={4}>
          <Card>
            <Statistic title={<Trans>Total</Trans>} value={money(summary?.total ?? 0)} />
          </Card>
        </Col>
        <Col xs={12} md={4}>
          <Card>
            <Statistic title={<Trans>Current</Trans>} value={money(summary?.current ?? 0)} />
          </Card>
        </Col>
        <Col xs={12} md={4}>
          <Card>
            <Statistic title={<Trans>1-30 days</Trans>} value={money(summary?.days1To30 ?? 0)} />
          </Card>
        </Col>
        <Col xs={12} md={4}>
          <Card>
            <Statistic title={<Trans>31-60 days</Trans>} value={money(summary?.days31To60 ?? 0)} />
          </Card>
        </Col>
        <Col xs={12} md={4}>
          <Card>
            <Statistic title={<Trans>61-90 days</Trans>} value={money(summary?.days61To90 ?? 0)} />
          </Card>
        </Col>
        <Col xs={12} md={4}>
          <Card>
            <Statistic
              title={<Trans>90+ days</Trans>}
              value={money(summary?.days90Plus ?? 0)}
              styles={{
                content: {
                  color: (summary?.days90Plus ?? 0) > 0 ? token.colorError : undefined,
                },
              }}
            />
          </Card>
        </Col>
      </Row>

      <Row>
        <Col span={24}>
          <Table
            dataSource={summary?.invoices ?? []}
            rowKey="id"
            loading={loading}
            pagination={{ hideOnSinglePage: true, defaultPageSize: 50 }}
            locale={{ emptyText: <Trans>No outstanding invoices</Trans> }}
            summary={() =>
              summary?.invoices?.length ? (
                <Table.Summary.Row>
                  <Table.Summary.Cell index={0} colSpan={3}>
                    <Typography.Text strong>
                      <Trans>Total</Trans>
                    </Typography.Text>
                  </Table.Summary.Cell>
                  <Table.Summary.Cell index={3} align="right">
                    <Typography.Text strong>{money(summary.total)}</Typography.Text>
                  </Table.Summary.Cell>
                </Table.Summary.Row>
              ) : null
            }
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
              title={<Trans>Balance due</Trans>}
              key="total"
              align="right"
              render={(inv: OutstandingInvoiceSummary) => (
                <>
                  {money(inv.total)}
                  {organization?.currency && inv.currency !== organization.currency && (
                    <div style={{ color: token.colorTextSecondary, fontSize: 12 }}>
                      {formatCents(
                        inv.foreignTotal,
                        inv.currency,
                        numberFormatLocale(organization?.country_code) ?? i18n.locale,
                      )}
                    </div>
                  )}
                </>
              )}
            />
          </Table>
        </Col>
      </Row>
    </>
  );
};

export default ReceivableAging;
