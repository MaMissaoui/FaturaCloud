import { useState } from "react";
import { Alert, Button, DatePicker, Segmented, Select, Space, Table, Tag, Typography } from "antd";
import { useAtomValue } from "jotai";
import { Link } from "react-router";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { ReloadOutlined } from "@ant-design/icons";
import type { Dayjs } from "dayjs";

import {
  GetOrganizationAuditEvents,
  GetOrganizationMembers,
  GetPlatformAuditEvents,
  type AuditEvent,
  type AuditEventPage,
  type OrganizationMember,
} from "src/api";
import { isPlatformAdminAtom } from "src/atoms/auth";
import { isOrgAdminAtom, organizationIdAtom } from "src/atoms/organization";
import { useFetch } from "src/hooks/useFetch";
import { useDatePickerFormat, useDateTimeFormatter } from "src/utils/date";
import { message } from "src/utils/message";

const { Title, Paragraph, Text } = Typography;

// Stable empty values while nothing is loaded (see useFetch).
const NO_PAGE: AuditEventPage = { events: [], next: "" };
const NO_MEMBERS: OrganizationMember[] = [];

// What each route resource is, for the Area column. A resource missing here
// shows its raw name.
const resourceLabel = (resource: string): string => {
  switch (resource) {
    case "invoices":
      return t`Invoice`;
    case "cash-sales":
      return t`Cash sale`;
    case "incoming-invoices":
      return t`Incoming invoice`;
    case "orders":
      return t`Order`;
    case "purchase-orders":
      return t`Purchase order`;
    case "deliveries":
      return t`Delivery`;
    case "inbound-deliveries":
      return t`Goods receipt`;
    case "production-orders":
      return t`Production order`;
    case "clients":
      return t`Client`;
    case "vendors":
      return t`Vendor`;
    case "products":
      return t`Product`;
    case "product-families":
      return t`Product family`;
    case "payments":
      return t`Payment`;
    case "cash-movements":
      return t`Cash movement`;
    case "stock-movements":
      return t`Stock movement`;
    case "imports":
      return t`Import`;
    case "journal-entries":
      return t`Journal entry`;
    case "journals":
      return t`Journal`;
    case "accounts":
      return t`Account`;
    case "fiscal-years":
      return t`Fiscal year`;
    case "fiscal-periods":
      return t`Fiscal period`;
    case "tax-rates":
      return t`Tax rate`;
    case "payment-terms":
      return t`Payment term`;
    case "units-of-measure":
      return t`Unit of measure`;
    case "document-templates":
      return t`Document template`;
    case "document-number-settings":
      return t`Document numbering`;
    case "loan-imports":
      return t`Loan register import`;
    case "organizations":
      return t`Organization`;
    case "logo":
      return t`Logo`;
    case "reset":
      return t`Organization data`;
    case "members":
      return t`Member`;
    case "users":
      return t`User`;
    case "backups":
    case "backup":
      return t`Backup`;
    case "restore":
      return t`Database`;
    case "countries":
      return t`Country`;
    default:
      return resource;
  }
};

// What was done, from the method and the route's last part.
const actionLabel = (event: AuditEvent): string => {
  const last = event.route.split("/").pop() ?? "";
  if (event.resource === "reset") return t`Reset`;
  if (event.resource === "restore" || last === "restore") return t`Restored`;
  switch (last) {
    case "state":
    case "status":
      return t`Changed state`;
    case "post":
      return t`Posted`;
    case "reverse":
      return t`Reversed`;
    case "void":
      return t`Voided`;
    case "close":
      return t`Closed`;
    case "import":
      return t`Imported`;
    case "payments":
      return t`Payment recorded`;
    case "bom":
      return t`Changed recipe`;
    case "orientation":
      return t`Changed orientation`;
  }
  switch (event.method) {
    case "POST":
      return event.resource === "logo" || event.resource === "document-templates"
        ? t`Uploaded`
        : t`Created`;
    case "DELETE":
      return t`Deleted`;
    default:
      return t`Updated`;
  }
};

// Document states as the lists name them.
const stateLabel = (state: string): string => {
  switch (state) {
    case "draft":
      return t`Draft`;
    case "sent":
      return t`Sent`;
    case "paid":
      return t`Paid`;
    case "cancelled":
      return t`Cancelled`;
    case "approved":
      return t`Approved`;
    case "confirmed":
      return t`Confirmed`;
    case "shipped":
      return t`Shipped`;
    case "delivered":
      return t`Delivered`;
    case "received":
      return t`Received`;
    case "completed":
      return t`Completed`;
    case "posted":
      return t`Posted`;
    case "reversed":
      return t`Reversed`;
    case "voided":
      return t`Voided`;
    case "open":
      return t`Open`;
    case "closed":
      return t`Closed`;
    default:
      return state;
  }
};

// Where a document opens, for the ones with a page of their own.
const documentPath = (event: AuditEvent): string | null => {
  if (!event.entityId || event.method === "DELETE") return null;
  switch (event.resource) {
    case "invoices":
    case "cash-sales":
      return `/invoices/${event.entityId}`;
    case "incoming-invoices":
    case "orders":
    case "purchase-orders":
    case "deliveries":
    case "inbound-deliveries":
    case "production-orders":
      return `/${event.resource}/${event.entityId}`;
    case "journal-entries":
      return `/accounting/journal-entries/${event.entityId}`;
    default:
      return null;
  }
};

type Scope = "organization" | "platform";

// The activity history (api/audit.go): every change made over the last two
// years — who, when, what — for the organization's admins, plus the
// platform's own (users, backups, restores) for platform admins.
const SettingsActivity = () => {
  const organizationId = useAtomValue(organizationIdAtom);
  const isOrgAdmin = useAtomValue(isOrgAdminAtom);
  const isPlatformAdmin = useAtomValue(isPlatformAdminAtom);
  const formatDateTime = useDateTimeFormatter();
  const dateFormat = useDatePickerFormat();

  const [pickedScope, setScope] = useState<Scope>("organization");
  const scope: Scope = !isOrgAdmin ? "platform" : !isPlatformAdmin ? "organization" : pickedScope;
  const [userId, setUserId] = useState<string | undefined>();
  const [range, setRange] = useState<[Dayjs | null, Dayjs | null] | null>(null);
  // Pages loaded past the first, for the filters they were loaded with.
  const [more, setMore] = useState<{ key: string; page: AuditEventPage } | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);

  const from = range?.[0]?.startOf("day").valueOf();
  const to = range?.[1]?.add(1, "day").startOf("day").valueOf();
  const query = { userId: scope === "organization" ? userId : undefined, from, to };
  const canRead = scope === "organization" ? isOrgAdmin && !!organizationId : isPlatformAdmin;
  const key = canRead
    ? [
        scope,
        scope === "organization" ? organizationId : "",
        query.userId ?? "",
        from ?? 0,
        to ?? 0,
      ]
    : null;
  const keyString = JSON.stringify(key);
  const fetchPage = (before?: string) =>
    scope === "organization"
      ? GetOrganizationAuditEvents(organizationId!, { ...query, before })
      : GetPlatformAuditEvents({ ...query, before });

  const first = useFetch<AuditEventPage>(key, () => fetchPage(), NO_PAGE);
  const { data: members } = useFetch<OrganizationMember[]>(
    isOrgAdmin && organizationId ? [organizationId] : null,
    () => GetOrganizationMembers(organizationId!),
    NO_MEMBERS,
  );

  const extra = more?.key === keyString ? more.page : null;
  const events = extra ? [...first.data.events, ...extra.events] : first.data.events;
  const next = extra ? extra.next : first.data.next;

  if (!isOrgAdmin && !isPlatformAdmin) {
    return (
      <Alert
        type="warning"
        showIcon
        message={<Trans>Only an organization admin can see the activity history.</Trans>}
      />
    );
  }

  const loadMore = async () => {
    setLoadingMore(true);
    try {
      const page = await fetchPage(next);
      setMore({
        key: keyString,
        page: { events: [...(extra?.events ?? []), ...page.events], next: page.next },
      });
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Could not load more activity`);
    } finally {
      setLoadingMore(false);
    }
  };

  return (
    <div style={{ maxWidth: 1200 }}>
      <Title level={3}>
        <Trans>Activity</Trans>
      </Title>
      <Paragraph type="secondary">
        {scope === "organization" ? (
          <Trans>
            Every change made in this organization over the last two years: who made it, when, and
            to which document.
          </Trans>
        ) : (
          <Trans>
            Changes outside any organization over the last two years: users, backups and database
            restores.
          </Trans>
        )}
      </Paragraph>

      <Space wrap style={{ marginBottom: 16 }}>
        {isOrgAdmin && isPlatformAdmin && (
          <Segmented<Scope>
            value={scope}
            onChange={setScope}
            options={[
              { value: "organization", label: t`This organization` },
              { value: "platform", label: t`Platform` },
            ]}
          />
        )}
        {scope === "organization" && (
          <Select
            allowClear
            showSearch
            optionFilterProp="label"
            placeholder={t`Everyone`}
            style={{ width: 240 }}
            value={userId}
            onChange={setUserId}
            options={members.map((m) => ({ value: m.userId, label: m.displayName || m.email }))}
            aria-label={t`Filter by user`}
          />
        )}
        <DatePicker.RangePicker
          value={range}
          onChange={setRange}
          format={dateFormat}
          allowEmpty={[true, true]}
        />
        <Button icon={<ReloadOutlined />} onClick={first.reload} aria-label={t`Refresh`} />
      </Space>

      {first.failed ? (
        <Alert
          type="error"
          showIcon
          message={<Trans>The activity history could not be loaded.</Trans>}
          action={
            <Button size="small" onClick={first.reload}>
              <Trans>Retry</Trans>
            </Button>
          }
        />
      ) : (
        <Table<AuditEvent>
          rowKey="id"
          size="small"
          loading={first.loading}
          dataSource={events}
          pagination={false}
          scroll={{ x: "max-content" }}
          columns={[
            {
              title: t`When`,
              dataIndex: "createdAt",
              render: (at: number) => (
                <span style={{ whiteSpace: "nowrap" }}>{formatDateTime(at)}</span>
              ),
            },
            {
              title: t`Who`,
              dataIndex: "userEmail",
              render: (email: string) => email || <Text type="secondary">—</Text>,
            },
            {
              title: t`Action`,
              key: "action",
              render: (_, event) => (
                <span style={{ whiteSpace: "nowrap" }}>
                  {resourceLabel(event.resource)} · {actionLabel(event)}
                </span>
              ),
            },
            {
              title: t`Document`,
              key: "document",
              render: (_, event) => {
                const name = event.entityLabel || event.entityId;
                if (!name) return <Text type="secondary">—</Text>;
                const path = documentPath(event);
                return path ? <Link to={path}>{name}</Link> : name;
              },
            },
            {
              title: t`Change`,
              key: "change",
              render: (_, event) =>
                event.toState ? (
                  <span style={{ whiteSpace: "nowrap" }}>
                    {event.fromState && <Tag>{stateLabel(event.fromState)}</Tag>}
                    {event.fromState && "→ "}
                    <Tag color="blue">{stateLabel(event.toState)}</Tag>
                  </span>
                ) : null,
            },
          ]}
          footer={
            next
              ? () => (
                  <Button onClick={loadMore} loading={loadingMore}>
                    <Trans>Load more</Trans>
                  </Button>
                )
              : undefined
          }
        />
      )}
    </div>
  );
};

export default SettingsActivity;
