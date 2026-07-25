import { useEffect, useState } from "react";
import type { InboundDelivery } from "src/types/models";
import { Link, useLocation, useNavigate } from "react-router";
import { Button, Col, Input, Row, Space, Table, Tag, Typography } from "antd";
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

const { Title } = Typography;
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
      <Row>
        <Col span={12}>
          <Title level={3} style={{ marginTop: 0, marginBottom: 0 }}>
            <ImportOutlined style={{ marginRight: 8 }} />
            <Trans>Goods Receipts</Trans>
          </Title>
        </Col>
        <Col span={12} style={{ display: "flex", justifyContent: "flex-end" }}>
          <Space style={{ alignItems: "start" }}>
            <Input.Search placeholder={t`Search`} onChange={(e) => setSearch(e.target.value)} />
            <Button type="primary" onClick={() => navigate("/inbound-deliveries/new")}>
              <Trans>New goods receipt</Trans>
            </Button>
          </Space>
        </Col>
      </Row>

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
