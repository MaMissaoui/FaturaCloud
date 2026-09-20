import { Button, Popconfirm, Table } from "antd";
import type { Organization } from "src/types/models";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import type { OrganizationUsageCount } from "src/api";

interface OrganizationsTableProps {
  dataSource: Organization[];
  loading: boolean;
  myOrgAdminIds: Set<string>;
  usageCounts: Record<string, OrganizationUsageCount>;
  onRowClick: (id: string) => void;
  onFetchUsageCount: (id: string) => void;
  onDelete: (id: string) => void;
}

// Purely presentational — every fetch/handler lives in the parent page
// (src/routes/organizations/index.tsx). Extracted from that file (F151,
// 2026-09-09) to cut its size, not to change when or how data loads.
export default function OrganizationsTable({
  dataSource,
  loading,
  myOrgAdminIds,
  usageCounts,
  onRowClick,
  onFetchUsageCount,
  onDelete,
}: OrganizationsTableProps) {
  return (
    <Table
      dataSource={dataSource}
      rowKey="id"
      loading={loading}
      pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
      size="middle"
      onRow={(record) => ({
        onClick: () => onRowClick(record.id),
        onKeyDown: (e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            onRowClick(record.id);
          }
        },
        style: { cursor: "pointer" },
        tabIndex: 0,
        role: "button",
      })}
    >
      <Table.Column
        title={<Trans>Name</Trans>}
        dataIndex="name"
        key="name"
        sorter={(a: Organization, b: Organization) => (a.name ?? "").localeCompare(b.name ?? "")}
        render={(name: string, record: Organization) => (
          <Button
            type="link"
            style={{ padding: 0, height: "auto" }}
            onClick={(e) => {
              e.stopPropagation();
              onRowClick(record.id);
            }}
          >
            {name}
          </Button>
        )}
      />
      <Table.Column
        title={<Trans>Code</Trans>}
        dataIndex="code"
        key="code"
        width={120}
        sorter={(a: Organization, b: Organization) => (a.code ?? "").localeCompare(b.code ?? "")}
      />
      <Table.Column
        title={<Trans>Email</Trans>}
        dataIndex="email"
        key="email"
        sorter={(a: Organization, b: Organization) => (a.email ?? "").localeCompare(b.email ?? "")}
      />
      <Table.Column
        title={<Trans>Phone</Trans>}
        dataIndex="phone"
        key="phone"
        width={150}
        sorter={(a: Organization, b: Organization) => (a.phone ?? "").localeCompare(b.phone ?? "")}
      />
      <Table.Column
        title="IBAN"
        dataIndex="iban"
        key="iban"
        width={200}
        sorter={(a: Organization, b: Organization) => (a.iban ?? "").localeCompare(b.iban ?? "")}
      />
      <Table.Column
        title={<Trans>Currency</Trans>}
        dataIndex="currency"
        key="currency"
        width={100}
        sorter={(a: Organization, b: Organization) =>
          (a.currency ?? "").localeCompare(b.currency ?? "")
        }
      />
      <Table.Column
        title=""
        key="actions"
        width={80}
        render={(_: unknown, record: Organization) => {
          if (!myOrgAdminIds.has(record.id)) return null;
          const counts = usageCounts[record.id];
          // Every field GetOrganizationUsageCount reports, so the warning
          // can't understate the blast radius when a new org-scoped table is
          // added (audit F125). Kept as a fixed list — rather than derived
          // from Object.keys — so each field gets a real translated label.
          const breakdown = counts
            ? [
                [counts.clients, t`client(s)`],
                [counts.vendors, t`vendor(s)`],
                [counts.invoices, t`invoice(s)`],
                [counts.orders, t`order(s)`],
                [counts.deliveries, t`delivery(ies)`],
                [counts.products, t`product(s)`],
                [counts.taxRates, t`tax rate(s)`],
                [counts.purchaseOrders, t`purchase order(s)`],
                [counts.inboundDeliveries, t`goods receipt(s)`],
                [counts.incomingInvoices, t`incoming invoice(s)`],
                [counts.productionOrders, t`production order(s)`],
                [counts.imports, t`import(s)`],
                [counts.stockMovements, t`stock movement(s)`],
                [counts.cashMovements, t`cash movement(s)`],
                [counts.productSerialNumbers, t`serial number(s)`],
                [counts.payments, t`payment(s)`],
                [counts.journalEntries, t`journal entr(y/ies)`],
                [counts.reconciliationGroups, t`reconciliation group(s)`],
                [counts.accounts, t`account(s)`],
                [counts.journals, t`journal(s)`],
                [counts.fiscalYears, t`fiscal year(s)`],
                [counts.fiscalPeriods, t`fiscal period(s)`],
                [counts.paymentTerms, t`payment term(s)`],
                [counts.unitsOfMeasure, t`unit(s) of measure`],
                [counts.documentTemplates, t`document template(s)`],
                [counts.documentTemplateSettings, t`template setting(s)`],
                [counts.documentNumberSettings, t`numbering setting(s)`],
                [counts.billOfMaterials, t`bill(s) of materials`],
                [counts.billOfMaterialsVersions, t`BOM version(s)`],
              ].filter(([n]) => (n as number) > 0)
            : [];
          return (
            <Popconfirm
              title={t`Delete this organization?`}
              description={
                breakdown.length > 0 ? (
                  <div style={{ maxWidth: 260 }}>
                    <div>
                      <Trans>This will permanently delete:</Trans>
                    </div>
                    <ul style={{ margin: "4px 0", paddingLeft: 18 }}>
                      {breakdown.map(([n, label]) => (
                        <li key={label as string}>
                          {n} {label}
                        </li>
                      ))}
                    </ul>
                    <div>
                      <Trans>This cannot be undone.</Trans>
                    </div>
                  </div>
                ) : (
                  <Trans>This cannot be undone.</Trans>
                )
              }
              onOpenChange={(open) => {
                if (open) onFetchUsageCount(record.id);
              }}
              onConfirm={(e) => {
                e?.stopPropagation();
                onDelete(record.id);
              }}
              onCancel={(e) => e?.stopPropagation()}
            >
              <Button size="small" danger onClick={(e) => e.stopPropagation()}>
                <Trans>Delete</Trans>
              </Button>
            </Popconfirm>
          );
        }}
      />
    </Table>
  );
}
