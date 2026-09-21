import { useEffect, useMemo, useState } from "react";
import type { Import, ImportSummary, PurchaseOrder } from "src/types/models";
import { Link, useLocation, useNavigate } from "react-router";
import { Button, Col, Empty, Table, Row, Tag, Tooltip } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
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
import { vendorsAtom, setVendorsAtom } from "src/atoms/vendor";
import { organizationAtom, organizationIdAtom } from "src/atoms/organization";
import ImportForm from "src/components/imports/form";
import PageHeader from "src/components/page-header";
import DocumentFilters from "src/components/document-filters";
import type { Dayjs } from "dayjs";
import { useDateFormatter } from "src/utils/date";
import { formatOrgCents } from "src/utils/currencies";

const Imports = () => {
  const { i18n } = useLingui();
  const location = useLocation();
  const navigate = useNavigate();
  const imports = useAtomValue(importsAtom);
  const setImports = useSetAtom(setImportsAtom);
  const orders = useAtomValue(purchaseOrdersAtom);
  const setPurchaseOrders = useSetAtom(setPurchaseOrdersAtom);
  const vendors = useAtomValue(vendorsAtom);
  const setVendors = useSetAtom(setVendorsAtom);
  const organization = useAtomValue(organizationAtom);
  const organizationId = useAtomValue(organizationIdAtom);
  // useState + useMemo, matching production-orders.tsx rather than the older
  // module-level searchAtom + unmemoized filter: that wrote a global atom and
  // rebuilt the dataSource on every keystroke, re-rendering every visible row
  // (audit 2026-09-19 F130).
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(false);
  const [summaries, setSummaries] = useState<Record<string, ImportSummary>>({});
  const [dateRange, setDateRange] = useState<[Dayjs, Dayjs] | null>(null);
  const [vendorFilter, setVendorFilter] = useState("");
  const formatDate = useDateFormatter();

  useEffect(() => {
    if (location.pathname === "/imports") {
      setLoading(true);
      setVendors();
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
  }, [location, setImports, setPurchaseOrders, setVendors, organizationId]);

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

  const vendorOptions = useMemo(
    () =>
      (vendors as any[]).map((v) => ({
        value: v.id,
        label: [v.name, v.code ? `· ${v.code}` : v.phone].filter(Boolean).join(" "),
      })),
    [vendors],
  );

  const hasFilters = !!(search || dateRange || vendorFilter);

  const filteredImports = useMemo(() => {
    const term = search.trim().toLowerCase();
    return filter(imports, (imp: Import) => {
      // An import has no vendor of its own — it matches when any of its
      // linked purchase orders belongs to the selected vendor.
      if (
        vendorFilter &&
        !(ordersByImport.get(imp.id) ?? []).some((o) => o.vendorId === vendorFilter)
      ) {
        return false;
      }
      if (
        term &&
        !some(["importNumber", "notes"], (field) =>
          includes(toString(get(imp, field)).toLowerCase(), term),
        )
      ) {
        return false;
      }
      if (dateRange?.[0] && (imp.date ?? 0) < dateRange[0].startOf("day").valueOf()) return false;
      if (dateRange?.[1] && (imp.date ?? 0) > dateRange[1].endOf("day").valueOf()) return false;
      return true;
    });
  }, [imports, search, dateRange, vendorFilter, ordersByImport]);

  // formatOrgCents applies the organization's own minimum_fraction_digits
  // (the same helper every other money screen uses) — formatCents ignored it,
  // so Imports rendered a different number of decimals than the rest of the
  // app for the same organization.
  const money = (cents: number) => formatOrgCents(cents, organization, i18n.locale);

  return (
    <>
      <PageHeader
        icon={<ContainerOutlined />}
        title={<Trans>Imports</Trans>}
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
            status=""
            onStatusChange={() => {}}
            statusOptions={[]}
            partyOptions={vendorOptions}
            partyValue={vendorFilter}
            onPartyChange={setVendorFilter}
            partyPlaceholder={t`All vendors`}
          />
        }
        actions={
          <Link to="/imports" state={{ importModal: true }}>
            <Button type="primary" style={{ marginBottom: 10 }}>
              <Trans>New import</Trans>
            </Button>
          </Link>
        }
      />
      <Row style={{ marginTop: 16 }}>
        <Col span={24}>
          <Table
            dataSource={filteredImports}
            pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
            rowKey="id"
            loading={loading}
            locale={{
              emptyText: hasFilters ? (
                <Empty description={<Trans>No imports match your filters</Trans>} />
              ) : (
                <Empty description={<Trans>No imports yet</Trans>}>
                  <Link to="/imports" state={{ importModal: true }}>
                    <Button type="primary">
                      <Trans>Create your first import</Trans>
                    </Button>
                  </Link>
                </Empty>
              ),
            }}
            onRow={(record: Import) => ({
              onClick: () =>
                navigate("/imports", { state: { importModal: true, importId: record.id } }),
              onKeyDown: (e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  navigate("/imports", { state: { importModal: true, importId: record.id } });
                }
              },
              style: { cursor: "pointer" },
              tabIndex: 0,
              role: "button",
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
              sorter={(a: Import, b: Import) =>
                (ordersByImport.get(a.id)?.length ?? 0) - (ordersByImport.get(b.id)?.length ?? 0)
              }
              render={(imp: Import) => {
                const linked = ordersByImport.get(imp.id) ?? [];
                if (linked.length === 0) {
                  return "—";
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
            <Table.Column
              title={<Trans>Notes</Trans>}
              dataIndex="notes"
              key="notes"
              ellipsis
              sorter={(a: Import, b: Import) => (a.notes ?? "").localeCompare(b.notes ?? "")}
            />
          </Table>
        </Col>
      </Row>

      <ImportForm />
    </>
  );
};

export default Imports;
