import { useCallback, useEffect, useState } from "react";
import { Card, Col, Row, Statistic, Table, theme } from "antd";
import { useAtomValue } from "jotai";
import { Trans } from "@lingui/react/macro";
import { useLingui } from "@lingui/react";
import { GoldOutlined } from "@ant-design/icons";

import { GetInventoryValuation } from "src/api";
import type { InventoryValuation, InventoryValuationLine } from "src/api";
import { organizationIdAtom, organizationAtom } from "src/atoms/organization";
import PageHeader from "src/components/page-header";

const InventoryValuationReport = () => {
  const { i18n } = useLingui();
  const { token } = theme.useToken();
  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);

  const [report, setReport] = useState<InventoryValuation | null>(null);
  const [loading, setLoading] = useState(false);

  const refresh = useCallback(() => {
    if (!organizationId) return;
    setLoading(true);
    GetInventoryValuation(organizationId)
      .then(setReport)
      .catch(() => setReport(null))
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
      <PageHeader icon={<GoldOutlined />} title={<Trans>Inventory Valuation</Trans>} />

      <Row gutter={[8, 8]} style={{ marginTop: 16, marginBottom: 16 }}>
        <Col xs={12} md={8}>
          <Card>
            <Statistic title={<Trans>GL balance</Trans>} value={money(report?.glBalance ?? 0)} />
          </Card>
        </Col>
        <Col xs={12} md={8}>
          <Card>
            <Statistic
              title={<Trans>Computed value</Trans>}
              value={money(report?.computedValue ?? 0)}
            />
          </Card>
        </Col>
        <Col xs={12} md={8}>
          <Card>
            <Statistic
              title={<Trans>Difference</Trans>}
              value={money(report?.difference ?? 0)}
              styles={{
                content: {
                  color: (report?.difference ?? 0) !== 0 ? token.colorError : undefined,
                },
              }}
            />
          </Card>
        </Col>
      </Row>

      <Row>
        <Col span={24}>
          <Table
            dataSource={report?.products ?? []}
            rowKey="productId"
            loading={loading}
            pagination={{ hideOnSinglePage: true, defaultPageSize: 50 }}
            locale={{ emptyText: <Trans>No stock-enabled products</Trans> }}
          >
            <Table.Column title={<Trans>Product</Trans>} dataIndex="name" key="name" />
            <Table.Column
              title={<Trans>Quantity</Trans>}
              dataIndex="quantity"
              key="quantity"
              align="right"
            />
            <Table.Column
              title={<Trans>Value</Trans>}
              key="value"
              align="right"
              render={(product: InventoryValuationLine) => money(product.value)}
            />
          </Table>
        </Col>
      </Row>
    </>
  );
};

export default InventoryValuationReport;
