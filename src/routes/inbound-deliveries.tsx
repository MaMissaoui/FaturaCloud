import { useEffect, useState } from "react";
import type { InboundDelivery } from "src/types/models";
import { Link, useLocation, useNavigate } from "react-router";
import { Button, Col, Row, Table, Tag } from "antd";
import { atom, useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { ImportOutlined } from "@ant-design/icons";
import filter from "lodash/filter";
import includes from "lodash/includes";

import { useDateFormatter } from "src/utils/date";
import {
  INBOUND_DELIVERY_STATUSES,
  inboundDeliveryStatusColor,
  inboundDeliveryStatusLabel,
  type InboundDeliveryStatus,
} from "src/types/inbound-delivery";
import { inboundDeliveriesAtom, setInboundDeliveriesAtom } from "src/atoms/inbound-delivery";
import PageHeader from "src/components/page-header";

const searchAtom = atom<string>("");

const InboundDeliveries = () => {
  useLingui();
  const location = useLocation();
  const navigate = useNavigate();
  const formatDate = useDateFormatter();
  const deliveries = useAtomValue(inboundDeliveriesAtom);
  const setDeliveries = useSetAtom(setInboundDeliveriesAtom);
  const [search, setSearch] = useAtom(searchAtom);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (location.pathname === "/inbound-deliveries") {
      setLoading(true);
      setDeliveries().finally(() => setLoading(false));
    }
  }, [location, setDeliveries]);

  const filtered = search
    ? filter(
        deliveries,
        (d: InboundDelivery) =>
          includes((d.deliveryNumber ?? "").toLowerCase(), search.toLowerCase()) ||
          includes((d.vendorName ?? "").toLowerCase(), search.toLowerCase()) ||
          includes((d.orderNumber ?? "").toLowerCase(), search.toLowerCase()),
      )
    : deliveries;

  return (
    <>
      <PageHeader
        icon={<ImportOutlined />}
        title={<Trans>Goods Receipts</Trans>}
        search={{ placeholder: t`Search`, onChange: setSearch }}
        actions={
          <Button type="primary" onClick={() => navigate("/inbound-deliveries/new")}>
            <Trans>New goods receipt</Trans>
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
            onRow={(record: InboundDelivery) => ({
              onClick: () => navigate(`/inbound-deliveries/${record.id}`),
              style: { cursor: "pointer" },
            })}
          >
            <Table.Column
              title={<Trans>Receipt #</Trans>}
              key="deliveryNumber"
              sorter={(a: InboundDelivery, b: InboundDelivery) =>
                (a.deliveryNumber ?? "").localeCompare(b.deliveryNumber ?? "")
              }
              render={(d: InboundDelivery) => (
                <Link to={`/inbound-deliveries/${d.id}`} onClick={(e) => e.stopPropagation()}>
                  {d.deliveryNumber}
                </Link>
              )}
            />
            <Table.Column
              title={<Trans>Vendor</Trans>}
              dataIndex="vendorName"
              key="vendorName"
              sorter={(a: InboundDelivery, b: InboundDelivery) =>
                (a.vendorName ?? "").localeCompare(b.vendorName ?? "")
              }
              render={(v: string | null) => v ?? "—"}
            />
            <Table.Column
              title={<Trans>Purchase order</Trans>}
              dataIndex="orderNumber"
              key="orderNumber"
              sorter={(a: InboundDelivery, b: InboundDelivery) =>
                (a.orderNumber ?? "").localeCompare(b.orderNumber ?? "")
              }
              render={(v: string | null) => v ?? "—"}
            />
            <Table.Column
              title={<Trans>Status</Trans>}
              dataIndex="status"
              key="status"
              filters={INBOUND_DELIVERY_STATUSES.map((s) => ({
                text: inboundDeliveryStatusLabel(s),
                value: s,
              }))}
              onFilter={(value, record: InboundDelivery) => record.status === value}
              sorter={(a: InboundDelivery, b: InboundDelivery) =>
                (a.status ?? "").localeCompare(b.status ?? "")
              }
              render={(status: string) => (
                <Tag color={inboundDeliveryStatusColor[status as InboundDeliveryStatus]}>
                  {inboundDeliveryStatusLabel(status)}
                </Tag>
              )}
            />
            <Table.Column
              title={<Trans>Receipt date</Trans>}
              dataIndex="deliveryDate"
              key="deliveryDate"
              sorter={(a: InboundDelivery, b: InboundDelivery) =>
                (a.deliveryDate ?? 0) - (b.deliveryDate ?? 0)
              }
              render={(v: number) => (v ? formatDate(v) : "—")}
            />
          </Table>
        </Col>
      </Row>
    </>
  );
};

export default InboundDeliveries;
