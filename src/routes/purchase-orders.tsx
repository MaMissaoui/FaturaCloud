import { useEffect, useState } from "react";
import type { PurchaseOrder } from "src/types/models";
import { Link, useLocation, useNavigate } from "react-router";
import { Button, Col, Row, Table, Tag } from "antd";
import { atom, useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { ShoppingCartOutlined } from "@ant-design/icons";
import filter from "lodash/filter";
import includes from "lodash/includes";

import { useDateFormatter } from "src/utils/date";
import {
  PURCHASE_ORDER_STATUSES,
  purchaseOrderStatusColor,
  purchaseOrderStatusLabel,
  type PurchaseOrderStatus,
} from "src/types/purchase-order";
import { purchaseOrdersAtom, setPurchaseOrdersAtom } from "src/atoms/purchase-order";
import PageHeader from "src/components/page-header";

const searchAtom = atom<string>("");

const PurchaseOrders = () => {
  useLingui();
  const location = useLocation();
  const navigate = useNavigate();
  const formatDate = useDateFormatter();
  const orders = useAtomValue(purchaseOrdersAtom);
  const setOrders = useSetAtom(setPurchaseOrdersAtom);
  const [search, setSearch] = useAtom(searchAtom);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (location.pathname === "/purchase-orders") {
      setLoading(true);
      setOrders().finally(() => setLoading(false));
    }
  }, [location, setOrders]);

  const filtered = search
    ? filter(
        orders,
        (o: PurchaseOrder) =>
          includes((o.orderNumber ?? "").toLowerCase(), search.toLowerCase()) ||
          includes((o.vendorName ?? "").toLowerCase(), search.toLowerCase()),
      )
    : orders;

  return (
    <>
      <PageHeader
        icon={<ShoppingCartOutlined />}
        title={<Trans>Purchase Orders</Trans>}
        search={{ placeholder: t`Search`, onChange: setSearch }}
        actions={
          <Button type="primary" onClick={() => navigate("/purchase-orders/new")}>
            <Trans>New purchase order</Trans>
          </Button>
        }
      />

      <Row style={{ marginTop: 16 }}>
        <Col span={24}>
          <Table
            dataSource={filtered}
            pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
            rowKey="id"
            loading={loading}
            onRow={(record: PurchaseOrder) => ({
              onClick: () => navigate(`/purchase-orders/${record.id}`),
              style: { cursor: "pointer" },
            })}
          >
            <Table.Column
              title={<Trans>Order #</Trans>}
              key="orderNumber"
              sorter={(a: PurchaseOrder, b: PurchaseOrder) =>
                (a.orderNumber ?? "").localeCompare(b.orderNumber ?? "")
              }
              render={(o: PurchaseOrder) => (
                <Link to={`/purchase-orders/${o.id}`} onClick={(e) => e.stopPropagation()}>
                  {o.orderNumber}
                </Link>
              )}
            />
            <Table.Column
              title={<Trans>Vendor</Trans>}
              dataIndex="vendorName"
              key="vendorName"
              sorter={(a: PurchaseOrder, b: PurchaseOrder) =>
                (a.vendorName ?? "").localeCompare(b.vendorName ?? "")
              }
              render={(v: string | null) => v ?? "—"}
            />
            <Table.Column
              title={<Trans>Status</Trans>}
              dataIndex="status"
              key="status"
              // Filters are built inside the component body (not at module
              // scope) so the labels follow the active locale.
              filters={PURCHASE_ORDER_STATUSES.map((s) => ({
                text: purchaseOrderStatusLabel(s),
                value: s,
              }))}
              onFilter={(value, record: PurchaseOrder) => record.status === value}
              sorter={(a: PurchaseOrder, b: PurchaseOrder) =>
                (a.status ?? "").localeCompare(b.status ?? "")
              }
              render={(status: string) => (
                <Tag color={purchaseOrderStatusColor[status as PurchaseOrderStatus]}>
                  {purchaseOrderStatusLabel(status)}
                </Tag>
              )}
            />
            <Table.Column
              title={<Trans>Order date</Trans>}
              dataIndex="orderDate"
              key="orderDate"
              sorter={(a: PurchaseOrder, b: PurchaseOrder) => (a.orderDate ?? 0) - (b.orderDate ?? 0)}
              render={(v: number) => (v ? formatDate(v) : "—")}
            />
            <Table.Column
              title={<Trans>Expected date</Trans>}
              dataIndex="expectedDate"
              key="expectedDate"
              sorter={(a: PurchaseOrder, b: PurchaseOrder) =>
                (a.expectedDate ?? 0) - (b.expectedDate ?? 0)
              }
              render={(v: number | null) => (v ? formatDate(v) : "—")}
            />
          </Table>
        </Col>
      </Row>
    </>
  );
};

export default PurchaseOrders;
