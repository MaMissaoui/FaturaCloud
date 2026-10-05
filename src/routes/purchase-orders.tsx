import { useMemo, useState } from "react";
import type { PurchaseOrder } from "src/types/models";
import { Link, useNavigate } from "react-router";
import { Button, Col, Empty, Row, Table, Tag, theme } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { plural, t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { ShoppingCartOutlined } from "@ant-design/icons";
import filter from "lodash/filter";

import { useDateFormatter } from "src/utils/date";
import {
  purchaseOrderStatusColor,
  purchaseOrderStatusLabel,
  type PurchaseOrderStatus,
} from "src/types/purchase-order";
import { purchaseOrdersAtom, setPurchaseOrdersAtom } from "src/atoms/purchase-order";
import { vendorsAtom, setVendorsAtom } from "src/atoms/vendor";
import PageHeader from "src/components/page-header";
import DocumentFilters, { matchesDocumentFilters } from "src/components/document-filters";
import { useStatusChips } from "src/components/master-data/use-status-chips";
import type { Dayjs } from "dayjs";
import { useLoadOnPath } from "src/hooks/useLoadOnPath";

const PurchaseOrders = () => {
  useLingui();
  const { token } = theme.useToken();
  const navigate = useNavigate();
  const formatDate = useDateFormatter();
  const orders = useAtomValue(purchaseOrdersAtom);
  const setOrders = useSetAtom(setPurchaseOrdersAtom);
  const setVendors = useSetAtom(setVendorsAtom);
  const [search, setSearch] = useState("");
  const [vendorFilter, setVendorFilter] = useState("");
  const [dateRange, setDateRange] = useState<[Dayjs, Dayjs] | null>(null);
  const vendors = useAtomValue(vendorsAtom);

  const loading = useLoadOnPath("/purchase-orders", () => {
    setVendors();
    return setOrders();
  });

  const vendorOptions = useMemo(
    () =>
      (vendors as any[]).map((v) => ({
        value: v.id,
        label: [v.name, v.code ? `· ${v.code}` : v.phone].filter(Boolean).join(" "),
      })),
    [vendors],
  );

  // What the search, party and date filters leave; the status chips split it.
  const matching = useMemo(
    () =>
      filter(orders, (o: PurchaseOrder) =>
        matchesDocumentFilters({
          search,
          searchFields: [o.orderNumber, o.vendorName],
          status: "",
          rowStatus: o.status,
          partyId: vendorFilter,
          rowPartyId: o.vendorId,
          dateRange,
          rowDate: o.orderDate,
        }),
      ),
    [orders, search, vendorFilter, dateRange],
  );
  const { shown, picked, bar } = useStatusChips(
    matching,
    (row) => row.status ?? "",
    [
      { key: "draft", label: <Trans context="document filter">Draft</Trans> },
      { key: "confirmed", label: <Trans context="document filter">Confirmed</Trans> },
      { key: "received", label: <Trans context="document filter">Received</Trans> },
      {
        key: "cancelled",
        label: <Trans context="document filter">Cancelled</Trans>,
        hideWhenEmpty: true,
      },
    ],
    t`Filter purchase orders`,
  );
  const hasFilters = !!(search || picked || vendorFilter || dateRange);

  return (
    <>
      <PageHeader
        icon={<ShoppingCartOutlined />}
        title={<Trans>Purchase Orders</Trans>}
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
            dateLabel={t`Order date`}
            partyOptions={vendorOptions}
            partyValue={vendorFilter}
            onPartyChange={setVendorFilter}
            partyPlaceholder={t`All vendors`}
          />
        }
        actions={
          <Button type="primary" onClick={() => navigate("/purchase-orders/new")}>
            <Trans>New purchase order</Trans>
          </Button>
        }
      />

      <p style={{ margin: "4px 0 0", color: token.colorTextSecondary }}>
        {plural(orders.length, { one: "# purchase order", other: "# purchase orders" })}
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
                <Empty description={<Trans>No purchase orders match your filters</Trans>} />
              ) : (
                <Empty description={<Trans>No purchase orders yet</Trans>}>
                  <Link to="/purchase-orders/new">
                    <Button type="primary">
                      <Trans>Create your first purchase order</Trans>
                    </Button>
                  </Link>
                </Empty>
              ),
            }}
            onRow={(record: PurchaseOrder) => ({
              onClick: () => navigate(`/purchase-orders/${record.id}`),
              onKeyDown: (e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  navigate(`/purchase-orders/${record.id}`);
                }
              },
              style: { cursor: "pointer" },
              tabIndex: 0,
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
              title={<Trans>Import</Trans>}
              key="importNumber"
              sorter={(a: PurchaseOrder, b: PurchaseOrder) =>
                (a.importNumber ?? "").localeCompare(b.importNumber ?? "")
              }
              render={(o: PurchaseOrder) =>
                o.importNumber ? (
                  <Link
                    to="/imports"
                    state={{ importModal: true, importId: o.importId }}
                    onClick={(e) => e.stopPropagation()}
                  >
                    {o.importNumber}
                  </Link>
                ) : (
                  "—"
                )
              }
            />
            <Table.Column
              title={<Trans>Status</Trans>}
              dataIndex="status"
              key="status"
              // Filters are built inside the component body (not at module
              // scope) so the labels follow the active locale.
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
              sorter={(a: PurchaseOrder, b: PurchaseOrder) =>
                (a.orderDate ?? 0) - (b.orderDate ?? 0)
              }
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
