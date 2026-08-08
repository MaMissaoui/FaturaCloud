import { useCallback, useEffect, useState } from "react";
import { Card, Col, Row, Statistic, Table, theme } from "antd";
import { useAtomValue } from "jotai";
import { Trans } from "@lingui/react/macro";
import { useLingui } from "@lingui/react";
import { FieldTimeOutlined } from "@ant-design/icons";

import { GetPayableAging } from "src/api";
import type { OutstandingBillSummary, PayableAgingSummary } from "src/api";
import { organizationIdAtom, organizationAtom } from "src/atoms/organization";
import PageHeader from "src/components/page-header";

const PayableAging = () => {
  const { i18n } = useLingui();
  const { token } = theme.useToken();
  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);

  const [summary, setSummary] = useState<PayableAgingSummary | null>(null);
  const [loading, setLoading] = useState(false);

  const refresh = useCallback(() => {
    if (!organizationId) return;
    setLoading(true);
    GetPayableAging(organizationId)
      .then(setSummary)
      .catch(() => setSummary(null))
      .finally(() => setLoading(false));
  }, [organizationId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const money = (cents: number) =>
    Intl.NumberFormat(i18n.locale, {
      style: "currency",
      currency: organization?.currency ?? "EUR",
      minimumFractionDigits: organization?.minimum_fraction_digits ?? undefined,
    }).format(cents / 100);

  return (
    <>
      <PageHeader icon={<FieldTimeOutlined />} title={<Trans>AP Aging</Trans>} />

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
            dataSource={summary?.bills ?? []}
            rowKey="id"
            loading={loading}
            pagination={{ hideOnSinglePage: true, defaultPageSize: 50 }}
            locale={{ emptyText: <Trans>No outstanding bills</Trans> }}
          >
            <Table.Column title={<Trans>Bill</Trans>} dataIndex="number" key="number" />
            <Table.Column title={<Trans>Vendor</Trans>} dataIndex="vendorName" key="vendorName" />
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
              render={(bill: OutstandingBillSummary) => money(bill.total)}
            />
          </Table>
        </Col>
      </Row>
    </>
  );
};

export default PayableAging;
