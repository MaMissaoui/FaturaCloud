import { useEffect, useMemo, useState } from "react";
import type { Import, ImportSummary, PurchaseOrder } from "src/types/models";
import { Link, useLocation, useNavigate } from "react-router";
import { Button, Col, Table, Row, Tag, Tooltip } from "antd";
import { atom, useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { ContainerOutlined } from "@ant-design/icons";
import filter from "lodash/filter";
import get from "lodash/get";
import includes from "lodash/includes";
import some from "lodash/some";
import toString from "lodash/toString";

import { GetImportSummaries } from "src/api";
import { importsAtom, setImportsAtom } from "src/atoms/import";
import { purchaseOrdersAtom, setPurchaseOrdersAtom } from "src/atoms/purchase-order";
import { organizationAtom, organizationIdAtom } from "src/atoms/organization";
import ImportForm from "src/components/imports/form";
import PageHeader from "src/components/page-header";
import { useDateFormatter } from "src/utils/date";
import { formatCents } from "src/utils/currency";

const searchAtom = atom<string>("");

const Imports = () => {
  const { i18n } = useLingui();
  const location = useLocation();
  const navigate = useNavigate();
  const imports = useAtomValue(importsAtom);
  const setImports = useSetAtom(setImportsAtom);
  const orders = useAtomValue(purchaseOrdersAtom);
  const setPurchaseOrders = useSetAtom(setPurchaseOrdersAtom);
  const organization = useAtomValue(organizationAtom);
  const organizationId = useAtomValue(organizationIdAtom);
  const [search, setSearch] = useAtom(searchAtom);
  const [loading, setLoading] = useState(false);
  const [summaries, setSummaries] = useState<Record<string, ImportSummary>>({});
  const formatDate = useDateFormatter();

  useEffect(() => {
    if (location.pathname === "/imports") {
      setLoading(true);
      setImports().finally(() => setLoading(false));
      // Needed for both this page's own "Purchase orders" column and the
      // drawer's linked-purchase-orders card — fetched here (once, on list
      // mount) rather than inside the drawer so it's already warm by the
      // time a row is clicked.
      setPurchaseOrders();
      if (organizationId) {
        GetImportSummaries(organizationId)
          .then(setSummaries)
          .catch(() => setSummaries({}));
      }
    }
  }, [location, setImports, setPurchaseOrders, organizationId]);

  // Order numbers per import, from the already-fetched purchaseOrdersAtom —
  // no extra request, same data the drawer's own linked-PO card filters.
  const ordersByImport = useMemo(() => {
    const map = new Map<string, PurchaseOrder[]>();
    for (const o of orders) {
      if (!o.importId) continue;
      const list = map.get(o.importId) ?? [];
      list.push(o);
      map.set(o.importId, list);
    }
    return map;
  }, [orders]);

  const searchImports = () => {
    return filter(imports, (imp: Import) => {
      return some(["importNumber", "notes"], (field) => {
        const value = get(imp, field);
        return includes(toString(value).toLowerCase(), search.toLowerCase());
      });
    });
  };

  const money = (cents: number) => formatCents(cents, organization?.currency ?? "EUR", i18n.locale);

  return (
    <>
      <PageHeader
        icon={<ContainerOutlined />}
        title={<Trans>Imports</Trans>}
        search={{ placeholder: t`Search text`, onChange: setSearch }}
        actions={
          <Link to="/imports" state={{ importModal: true }}>
            <Button type="primary" style={{ marginBottom: 10 }}>
              <Trans>New import</Trans>
            </Button>
          </Link>
        }
      />
      <Row>
        <Col span={24}>
          <Table
            dataSource={search ? searchImports() : imports}
            pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
            rowKey="id"
            loading={loading}
            onRow={(record: Import) => ({
              onClick: () =>
                navigate("/imports", { state: { importModal: true, importId: record.id } }),
              style: { cursor: "pointer" },
            })}
          >
            <Table.Column
              title={<Trans>Number</Trans>}
              key="importNumber"
              sorter={(a: Import, b: Import) => a.importNumber.localeCompare(b.importNumber)}
              render={(imp: Import) => (
                <Link
                  to="/imports"
                  state={{ importModal: true, importId: imp.id }}
                  onClick={(e) => e.stopPropagation()}
                >
                  {imp.importNumber}
                </Link>
              )}
            />
            <Table.Column
              title={<Trans>Date</Trans>}
              dataIndex="date"
              key="date"
              sorter={(a: Import, b: Import) => a.date - b.date}
              render={(date: number) => formatDate(date)}
            />
            <Table.Column
              title={<Trans>Purchase orders</Trans>}
              key="purchaseOrders"
              render={(imp: Import) => {
                const linked = ordersByImport.get(imp.id) ?? [];
                if (linked.length === 0) {
                  return <span style={{ color: "#999" }}>—</span>;
                }
                const shown = linked.slice(0, 3);
                const rest = linked.length - shown.length;
                return (
                  <span onClick={(e) => e.stopPropagation()}>
                    {shown.map((o) => (
                      <Link key={o.id} to={`/purchase-orders/${o.id}`}>
                        <Tag style={{ marginBottom: 2 }}>{o.orderNumber}</Tag>
                      </Link>
                    ))}
                    {rest > 0 && (
                      <Tooltip title={linked.map((o) => o.orderNumber).join(", ")}>
                        <Tag style={{ marginBottom: 2 }}>+{rest}</Tag>
                      </Tooltip>
                    )}
                  </span>
                );
              }}
            />
            <Table.Column
              title={<Trans>Committed value</Trans>}
              key="committedValue"
              align="right"
              sorter={(a: Import, b: Import) =>
                (summaries[a.id]?.totalCommittedValue ?? 0) -
                (summaries[b.id]?.totalCommittedValue ?? 0)
              }
              render={(imp: Import) => money(summaries[imp.id]?.totalCommittedValue ?? 0)}
            />
            <Table.Column
              title={<Trans>Freight</Trans>}
              dataIndex="freightCost"
              key="freightCost"
              align="right"
              sorter={(a: Import, b: Import) => a.freightCost - b.freightCost}
              render={(cents: number) => money(cents)}
            />
            <Table.Column
              title={<Trans>Customs</Trans>}
              dataIndex="customsCost"
              key="customsCost"
              align="right"
              sorter={(a: Import, b: Import) => a.customsCost - b.customsCost}
              render={(cents: number) => money(cents)}
            />
            <Table.Column title={<Trans>Notes</Trans>} dataIndex="notes" key="notes" ellipsis />
          </Table>
        </Col>
      </Row>

      <ImportForm />
    </>
  );
};

export default Imports;
