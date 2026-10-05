import { useMemo, useState } from "react";
import type { ProductionOrder } from "src/types/models";
import { Link, useNavigate } from "react-router";
import { Button, Col, Empty, Row, Table, Tag, theme } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { plural, t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { DeploymentUnitOutlined } from "@ant-design/icons";

import { useDateFormatter } from "src/utils/date";
import {
  productionOrderStatusColor,
  productionOrderStatusLabel,
  type ProductionOrderStatus,
} from "src/types/production-order";
import { productionOrdersAtom, setProductionOrdersAtom } from "src/atoms/production-order";
import PageHeader from "src/components/page-header";
import DocumentFilters, { matchesDocumentFilters } from "src/components/document-filters";
import { useStatusChips } from "src/components/master-data/use-status-chips";
import type { Dayjs } from "dayjs";
import { useLoadOnPath } from "src/hooks/useLoadOnPath";

const ProductionOrders = () => {
  useLingui();
  const { token } = theme.useToken();
  const navigate = useNavigate();
  const formatDate = useDateFormatter();
  const orders = useAtomValue(productionOrdersAtom);
  const setOrders = useSetAtom(setProductionOrdersAtom);
  const [search, setSearch] = useState("");
  const [dateRange, setDateRange] = useState<[Dayjs, Dayjs] | null>(null);

  const loading = useLoadOnPath("/production-orders", () => setOrders());

  // useState + useMemo, matching bill-of-materials.tsx rather than the older
  // module-level searchAtom + unmemoized filter this used to share with the
  // settings pages: that combination wrote a global atom and rebuilt
  // dataSource on every keystroke, re-rendering every visible row (audit
  // 2026-09-14 F91).
  const matching = useMemo(
    () =>
      orders.filter((o: ProductionOrder) =>
        matchesDocumentFilters({
          search,
          searchFields: [o.orderNumber, o.finishedProductName],
          status: "",
          rowStatus: o.status,
          partyId: "",
          rowPartyId: "",
          dateRange,
          rowDate: o.date,
        }),
      ),
    [orders, search, dateRange],
  );
  // The status chips split what the search and date filters leave.
  const { shown, picked, bar } = useStatusChips(
    matching,
    (row) => row.status ?? "",
    [
      { key: "draft", label: <Trans context="document filter">Draft</Trans> },
      { key: "completed", label: <Trans context="production order filter">Completed</Trans> },
      {
        key: "cancelled",
        label: <Trans context="production order filter">Cancelled</Trans>,
        hideWhenEmpty: true,
      },
    ],
    t`Filter production orders`,
  );
  const hasFilters = !!(search || picked || dateRange);

  return (
    <>
      <PageHeader
        icon={<DeploymentUnitOutlined />}
        title={<Trans>Production Orders</Trans>}
        search={{
          placeholder: t`Search`,
          value: search,
          onChange: setSearch,
          allowClear: true,
          onClear: () => setSearch(""),
        }}
        filters={
          <DocumentFilters
            dateRange={dateRange}
            onDateRangeChange={setDateRange}
            dateLabel={t`Date`}
          />
        }
        actions={
          <Button type="primary" onClick={() => navigate("/production-orders/new")}>
            <Trans>New production order</Trans>
          </Button>
        }
      />

      <p style={{ margin: "4px 0 0", color: token.colorTextSecondary }}>
        {plural(orders.length, { one: "# production order", other: "# production orders" })}
      </p>
      {bar}

      <Row style={{ marginTop: 16 }}>
        <Col span={24}>
          <Table
            dataSource={shown}
            pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
            rowKey="id"
            loading={loading}
            locale={{
              emptyText: hasFilters ? (
                <Empty description={<Trans>No production orders match your filters</Trans>} />
              ) : (
                <Empty description={<Trans>No production orders yet</Trans>}>
                  <Link to="/production-orders/new">
                    <Button type="primary">
                      <Trans>Create your first production order</Trans>
                    </Button>
                  </Link>
                </Empty>
              ),
            }}
            onRow={(record: ProductionOrder) => ({
              onClick: () => navigate(`/production-orders/${record.id}`),
              onKeyDown: (e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  navigate(`/production-orders/${record.id}`);
                }
              },
              style: { cursor: "pointer" },
              tabIndex: 0,
            })}
          >
            <Table.Column
              title={<Trans>Order #</Trans>}
              key="orderNumber"
              sorter={(a: ProductionOrder, b: ProductionOrder) =>
                (a.orderNumber ?? "").localeCompare(b.orderNumber ?? "")
              }
              render={(o: ProductionOrder) => (
                <Link to={`/production-orders/${o.id}`} onClick={(e) => e.stopPropagation()}>
                  {o.orderNumber}
                </Link>
              )}
            />
            <Table.Column
              title={<Trans>Finished product</Trans>}
              dataIndex="finishedProductName"
              key="finishedProductName"
              sorter={(a: ProductionOrder, b: ProductionOrder) =>
                (a.finishedProductName ?? "").localeCompare(b.finishedProductName ?? "")
              }
              render={(v: string | null) => v ?? "—"}
            />
            <Table.Column
              title={<Trans>Quantity</Trans>}
              dataIndex="quantity"
              key="quantity"
              align="right"
              sorter={(a: ProductionOrder, b: ProductionOrder) => a.quantity - b.quantity}
            />
            <Table.Column
              title={<Trans>Import</Trans>}
              dataIndex="importNumber"
              sorter={(a: ProductionOrder, b: ProductionOrder) =>
                (a.importNumber ?? "").localeCompare(b.importNumber ?? "")
              }
              key="importNumber"
              render={(v: string | null) => v ?? "—"}
            />
            <Table.Column
              title={<Trans>Status</Trans>}
              dataIndex="status"
              key="status"
              sorter={(a: ProductionOrder, b: ProductionOrder) =>
                (a.status ?? "").localeCompare(b.status ?? "")
              }
              render={(status: string) => (
                <Tag color={productionOrderStatusColor[status as ProductionOrderStatus]}>
                  {productionOrderStatusLabel(status)}
                </Tag>
              )}
            />
            <Table.Column
              title={<Trans>Date</Trans>}
              dataIndex="date"
              key="date"
              sorter={(a: ProductionOrder, b: ProductionOrder) => (a.date ?? 0) - (b.date ?? 0)}
              render={(v: number) => (v ? formatDate(v) : "—")}
            />
          </Table>
        </Col>
      </Row>
    </>
  );
};

export default ProductionOrders;
