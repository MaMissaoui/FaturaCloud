import { useEffect, useState } from "react";
import type { IncomingInvoice } from "src/types/models";
import { Link, useLocation, useNavigate } from "react-router";
import { Button, Col, Input, Row, Space, Table, Tag, Typography } from "antd";
import { atom, useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { AuditOutlined } from "@ant-design/icons";
import filter from "lodash/filter";
import includes from "lodash/includes";

import { useDateFormatter } from "src/utils/date";
import { getFormattedNumber } from "src/utils/currencies";
import { centsToUnits } from "src/utils/currency";
import {
  INCOMING_INVOICE_STATES,
  incomingInvoiceStateColor,
  incomingInvoiceStateLabel,
  type IncomingInvoiceState,
} from "src/types/incoming-invoice";
import { organizationAtom } from "src/atoms/organization";
import { incomingInvoicesAtom, setIncomingInvoicesAtom } from "src/atoms/incoming-invoice";

const { Title } = Typography;
const searchAtom = atom<string>("");

const IncomingInvoices = () => {
  const { i18n } = useLingui();
  const location = useLocation();
  const navigate = useNavigate();
  const formatDate = useDateFormatter();
  const organization = useAtomValue(organizationAtom);
  const invoices = useAtomValue(incomingInvoicesAtom);
  const setInvoices = useSetAtom(setIncomingInvoicesAtom);
  const [search, setSearch] = useAtom(searchAtom);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (location.pathname === "/incoming-invoices") {
      setLoading(true);
      setInvoices().finally(() => setLoading(false));
    }
  }, [location, setInvoices]);

  const filtered = search
    ? filter(
        invoices,
        (i: IncomingInvoice) =>
          includes((i.vendorInvoiceNumber ?? "").toLowerCase(), search.toLowerCase()) ||
          includes((i.vendorName ?? "").toLowerCase(), search.toLowerCase()) ||
          includes((i.reference ?? "").toLowerCase(), search.toLowerCase()),
      )
    : invoices;

  return (
    <>
      <Row>
        <Col span={12}>
          <Title level={3} style={{ marginTop: 0, marginBottom: 0 }}>
            <AuditOutlined style={{ marginRight: 8 }} />
            <Trans>Incoming Invoices</Trans>
          </Title>
        </Col>
        <Col span={12} style={{ display: "flex", justifyContent: "flex-end" }}>
          <Space style={{ alignItems: "start" }}>
            <Input.Search placeholder={t`Search`} onChange={(e) => setSearch(e.target.value)} />
            <Button type="primary" onClick={() => navigate("/incoming-invoices/new")}>
              <Trans>New incoming invoice</Trans>
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
            onRow={(record: IncomingInvoice) => ({
              onClick: () => navigate(`/incoming-invoices/${record.id}`),
              style: { cursor: "pointer" },
            })}
          >
            <Table.Column
              title={<Trans>Vendor invoice #</Trans>}
              key="vendorInvoiceNumber"
              sorter={(a: IncomingInvoice, b: IncomingInvoice) =>
                (a.vendorInvoiceNumber ?? "").localeCompare(b.vendorInvoiceNumber ?? "")
              }
              render={(i: IncomingInvoice) => (
                <Link to={`/incoming-invoices/${i.id}`} onClick={(e) => e.stopPropagation()}>
                  {i.vendorInvoiceNumber}
                </Link>
              )}
            />
            <Table.Column
              title={<Trans>Vendor</Trans>}
              dataIndex="vendorName"
              key="vendorName"
              sorter={(a: IncomingInvoice, b: IncomingInvoice) =>
                (a.vendorName ?? "").localeCompare(b.vendorName ?? "")
              }
              render={(v: string | null) => v ?? "—"}
            />
            <Table.Column
              title={<Trans>Purchase order</Trans>}
              dataIndex="orderNumber"
              key="orderNumber"
              render={(v: string | null) => v ?? "—"}
            />
            <Table.Column
              title={<Trans>State</Trans>}
              dataIndex="state"
              key="state"
              filters={INCOMING_INVOICE_STATES.map((s) => ({
                text: incomingInvoiceStateLabel(s),
                value: s,
              }))}
              onFilter={(value, record: IncomingInvoice) => record.state === value}
              render={(state: string, record: IncomingInvoice) => (
                <Space size={4}>
                  <Tag color={incomingInvoiceStateColor[state as IncomingInvoiceState]}>
                    {incomingInvoiceStateLabel(state)}
                  </Tag>
                  {record.matchOverride === 1 && (
                    <Tag color="warning">
                      <Trans>Override</Trans>
                    </Tag>
                  )}
                </Space>
              )}
            />
            <Table.Column
              title={<Trans>Date</Trans>}
              dataIndex="date"
              key="date"
              sorter={(a: IncomingInvoice, b: IncomingInvoice) => (a.date ?? 0) - (b.date ?? 0)}
              render={(v: number) => (v ? formatDate(v) : "—")}
            />
            <Table.Column
              title={<Trans>Due date</Trans>}
              dataIndex="dueDate"
              key="dueDate"
              sorter={(a: IncomingInvoice, b: IncomingInvoice) =>
                (a.dueDate ?? 0) - (b.dueDate ?? 0)
              }
              render={(v: number | null) => (v ? formatDate(v) : "—")}
            />
            <Table.Column
              title={<Trans>Total</Trans>}
              dataIndex="total"
              key="total"
              align="right"
              sorter={(a: IncomingInvoice, b: IncomingInvoice) => (a.total ?? 0) - (b.total ?? 0)}
              render={(total: number, record: IncomingInvoice) =>
                getFormattedNumber(centsToUnits(total), record.currency, i18n.locale, organization)
              }
            />
          </Table>
        </Col>
      </Row>
    </>
  );
};

export default IncomingInvoices;
