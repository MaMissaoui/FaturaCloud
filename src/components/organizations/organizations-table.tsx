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
      onRow={(record) => ({ onClick: () => onRowClick(record.id), style: { cursor: "pointer" } })}
    >
      <Table.Column
        title={<Trans>Name</Trans>}
        dataIndex="name"
        key="name"
        sorter={(a: Organization, b: Organization) => (a.name ?? "").localeCompare(b.name ?? "")}
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
          const breakdown = counts
            ? [
                [counts.clients, t`client(s)`],
                [counts.vendors, t`vendor(s)`],
                [counts.invoices, t`invoice(s)`],
                [counts.products, t`product(s)`],
                [counts.orders, t`order(s)`],
                [counts.purchaseOrders, t`purchase order(s)`],
                [counts.inboundDeliveries, t`goods receipt(s)`],
                [counts.incomingInvoices, t`incoming invoice(s)`],
                [counts.deliveries, t`delivery(ies)`],
                [counts.taxRates, t`tax rate(s)`],
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
