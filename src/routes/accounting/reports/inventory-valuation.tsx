import { useCallback, useEffect, useState } from "react";
import { Alert, Button, Card, Col, Row, Statistic, Table, theme, Typography } from "antd";
import { useAtomValue } from "jotai";
import { Trans } from "@lingui/react/macro";
import { useLingui } from "@lingui/react";
import { GoldOutlined } from "@ant-design/icons";
import { Link } from "react-router";

import { GetInventoryValuation } from "src/api";
import type { InventoryValuation, InventoryValuationLine } from "src/api";
import { organizationIdAtom, organizationAtom } from "src/atoms/organization";
import PageHeader from "src/components/page-header";
import { formatOrgCents } from "src/utils/currencies";

const InventoryValuationReport = () => {
  const { i18n } = useLingui();
  const { token } = theme.useToken();
  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);

  const [report, setReport] = useState<InventoryValuation | null>(null);
  const [loading, setLoading] = useState(false);
  // A failed fetch used to reset report to null, which the Statistic cards
  // below then rendered identically to "still loading" — both as
  // 0.00/0.00/0.00, and a zero Difference is this report's *good* outcome,
  // so a slow load or a transient error looked exactly like "books
  // reconcile." Tracked separately so the cards can show a real error
  // instead of a false all-clear.
  const [failed, setFailed] = useState(false);

  const refresh = useCallback(() => {
    if (!organizationId) return;
    setLoading(true);
    setFailed(false);
    GetInventoryValuation(organizationId)
      .then(setReport)
      .catch(() => {
        setReport(null);
        setFailed(true);
      })
      .finally(() => setLoading(false));
  }, [organizationId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const money = (cents: number) => formatOrgCents(cents, organization, i18n.locale);

  return (
    <>
      <PageHeader icon={<GoldOutlined />} title={<Trans>Inventory Valuation</Trans>} />

      {failed && (
        <Alert
          style={{ marginTop: 16 }}
          type="error"
          showIcon
          message={<Trans>Couldn't load the inventory valuation report</Trans>}
          action={
            <Button size="small" onClick={refresh}>
              <Trans>Retry</Trans>
            </Button>
          }
        />
      )}

      <Row gutter={[8, 8]} style={{ marginTop: 16, marginBottom: 16 }}>
        <Col xs={12} md={8}>
          <Card>
            <Statistic
              title={<Trans>GL balance</Trans>}
              value={failed ? "—" : money(report?.glBalance ?? 0)}
              loading={loading}
            />
          </Card>
        </Col>
        <Col xs={12} md={8}>
          <Card>
            <Statistic
              title={<Trans>Computed value</Trans>}
              value={failed ? "—" : money(report?.computedValue ?? 0)}
              loading={loading}
            />
          </Card>
        </Col>
        <Col xs={12} md={8}>
          <Card>
            <Statistic
              title={<Trans>Difference</Trans>}
              value={failed ? "—" : money(report?.difference ?? 0)}
              loading={loading}
              styles={{
                content: {
                  color: !failed && (report?.difference ?? 0) !== 0 ? token.colorError : undefined,
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
            summary={() =>
              report?.products?.length ? (
                <Table.Summary.Row>
                  <Table.Summary.Cell index={0} colSpan={2}>
                    <Typography.Text strong>
                      <Trans>Total</Trans>
                    </Typography.Text>
                  </Table.Summary.Cell>
                  <Table.Summary.Cell index={2} align="right">
                    <Typography.Text strong>
                      {report.products.reduce((sum, p) => sum + p.quantity, 0)}
                    </Typography.Text>
                  </Table.Summary.Cell>
                  <Table.Summary.Cell index={3} align="right">
                    <Typography.Text strong>{money(report.computedValue)}</Typography.Text>
                  </Table.Summary.Cell>
                </Table.Summary.Row>
              ) : null
            }
          >
            <Table.Column
              title={<Trans>Product</Trans>}
              dataIndex="name"
              key="name"
              sorter={(a: InventoryValuationLine, b: InventoryValuationLine) =>
                a.name.localeCompare(b.name)
              }
              render={(name: string, product: InventoryValuationLine) => (
                <Link to="/products" state={{ productModal: true, productId: product.productId }}>
                  {name}
                </Link>
              )}
            />
            <Table.Column
              title={<Trans>SKU</Trans>}
              dataIndex="sku"
              key="sku"
              sorter={(a: InventoryValuationLine, b: InventoryValuationLine) =>
                (a.sku ?? "").localeCompare(b.sku ?? "")
              }
            />
            <Table.Column
              title={<Trans>Quantity</Trans>}
              dataIndex="quantity"
              key="quantity"
              align="right"
              sorter={(a: InventoryValuationLine, b: InventoryValuationLine) =>
                a.quantity - b.quantity
              }
            />
            <Table.Column
              title={<Trans>Value</Trans>}
              key="value"
              align="right"
              sorter={(a: InventoryValuationLine, b: InventoryValuationLine) => a.value - b.value}
              render={(product: InventoryValuationLine) => money(product.value)}
            />
          </Table>
        </Col>
      </Row>
    </>
  );
};

export default InventoryValuationReport;
