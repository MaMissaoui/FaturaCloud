import { useMemo, useState } from "react";
import type { Vendor } from "src/types/models";
import { Link, Outlet, useNavigate } from "react-router";
import { Button, Col, Empty, Space, Table, Row, Tag, theme, Tooltip, Typography } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { plural, t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { PhoneOutlined, SolutionOutlined } from "@ant-design/icons";
import isEmpty from "lodash/isEmpty";
import filter from "lodash/filter";
import get from "lodash/get";
import includes from "lodash/includes";
import some from "lodash/some";
import toString from "lodash/toString";

import { vendorsAtom, setVendorsAtom } from "src/atoms/vendor";
import { organizationAtom, organizationIdAtom } from "src/atoms/organization";
import { GetVendorSummaries, type VendorSummaryList } from "src/api";
import VendorForm from "src/components/vendors/form";
import VendorSummaryPanel from "src/components/vendors/vendor-summary-panel";
import MassDataExcelActions from "src/components/mass-data/mass-data-excel-actions";
import FilterChips from "src/components/master-data/filter-chips";
import HeadlineFigure from "src/components/master-data/headline-figure";
import ListWithPanel from "src/components/master-data/list-with-panel";
import { useSummariesEnabled } from "src/components/master-data/use-summaries-enabled";
import { useWideScreen } from "src/components/master-data/use-wide-screen";
import PageHeader from "src/components/page-header";
import { formatAddressOneLine } from "src/utils/address";
import { formatOrgCents } from "src/utils/currencies";
import { useDateFormatter } from "src/utils/date";
import { useFetch } from "src/hooks/useFetch";
import { useLoadOnPath } from "src/hooks/useLoadOnPath";

type VendorFilter = "all" | "owing" | "overdue";

const SEARCH_FIELDS = [
  "name",
  "code",
  "registration_number",
  "emails",
  "phone",
  "vatin",
  "website",
  "city",
];

const Vendors = () => {
  const { i18n } = useLingui();
  const { token } = theme.useToken();
  const navigate = useNavigate();
  const vendors = useAtomValue(vendorsAtom);
  const setVendors = useSetAtom(setVendorsAtom);
  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);
  const formatDate = useDateFormatter();
  const [search, setSearch] = useState("");
  const [chip, setChip] = useState<VendorFilter>("all");
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const loading = useLoadOnPath("/vendors", () => setVendors());

  const wide = useWideScreen();

  // Summaries (what the organization owes each vendor, last purchase, the
  // panel) are on when the organization has them switched on and the role may
  // see vendor balances. A failed load (the setting switched off meanwhile, a
  // 403) falls back to the plain list rather than showing an error.
  const summariesEnabled = useSummariesEnabled("vendor-balances");
  const { data: summaries, failed: summariesFailed } = useFetch<VendorSummaryList | null>(
    summariesEnabled && organizationId ? [organizationId, vendors.length] : null,
    () => GetVendorSummaries(organizationId!),
    null,
  );
  const showSummaries = summariesEnabled && !summariesFailed;

  const summaryById = useMemo(
    () => new Map((summaries?.vendors ?? []).map((row) => [row.vendorId, row])),
    [summaries],
  );

  const searched = useMemo(
    () =>
      filter(vendors, (vendor: Vendor) => {
        const needle = search.toLowerCase();
        const fieldsMatch = some(SEARCH_FIELDS, (field) =>
          includes(toString(get(vendor, field)).toLowerCase(), needle),
        );
        return fieldsMatch || includes(formatAddressOneLine(vendor).toLowerCase(), needle);
      }),
    [vendors, search],
  );

  const shown = useMemo(() => {
    if (!showSummaries) return searched;
    const owed = (v: Vendor) => summaryById.get(v.id)?.owed ?? 0;
    const overdue = (v: Vendor) => summaryById.get(v.id)?.overdue ?? 0;
    switch (chip) {
      case "owing":
        return searched.filter((v) => owed(v) > 0).sort((a, b) => owed(b) - owed(a));
      case "overdue":
        return searched.filter((v) => overdue(v) > 0).sort((a, b) => overdue(b) - overdue(a));
      default:
        return searched;
    }
  }, [showSummaries, chip, searched, summaryById]);

  const selected = selectedId ? (vendors.find((v) => v.id === selectedId) ?? null) : null;
  const money = (cents: number) => formatOrgCents(cents, organization, i18n.locale);
  const openForm = (vendorId?: string) =>
    navigate("/vendors", { state: { vendorModal: true, vendorId } });
  const pick = (vendor: Vendor) => (showSummaries ? setSelectedId(vendor.id) : openForm(vendor.id));

  // A muted dash for an empty cell keeps the columns narrow; the panel spells
  // out "Not set".
  const notSet = (
    <Typography.Text type="secondary" aria-label={t`Not set`}>
      —
    </Typography.Text>
  );

  const newVendorButton = (
    <Link to="/vendors" state={{ vendorModal: true }}>
      <Button type="primary">
        <Trans>New vendor</Trans>
      </Button>
    </Link>
  );
  const excel = organizationId && (
    <MassDataExcelActions
      organizationId={organizationId}
      resource="vendors"
      filenamePrefix="vendors"
      onImported={() => setVendors()}
      compact
    />
  );

  const emptyText = search ? (
    <Empty description={<Trans>No vendors match your search</Trans>} />
  ) : (
    <Empty description={<Trans>No vendors yet</Trans>}>
      <Link to="/vendors" state={{ vendorModal: true }}>
        <Button type="primary">
          <Trans>Create your first vendor</Trans>
        </Button>
      </Link>
    </Empty>
  );

  const rowProps = (record: Vendor) => ({
    onClick: () => pick(record),
    onKeyDown: (e: React.KeyboardEvent) => {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        pick(record);
      }
    },
    style: { cursor: "pointer" },
    tabIndex: 0,
    role: "button",
    "aria-pressed": showSummaries ? record.id === selectedId : undefined,
  });

  // Summaries off: the plain list, as before the redesign.
  if (!showSummaries) {
    return (
      <>
        <PageHeader
          icon={<SolutionOutlined />}
          title={<Trans>Vendors</Trans>}
          search={{ placeholder: t`Search`, value: search, onChange: setSearch }}
          actions={
            <Space wrap>
              {excel}
              {newVendorButton}
            </Space>
          }
        />
        <Row>
          <Col span={24}>
            <Table
              dataSource={shown}
              pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
              rowKey="id"
              loading={loading}
              locale={{ emptyText }}
              onRow={rowProps}
            >
              <Table.Column
                title={<Trans>Name</Trans>}
                key="name"
                sorter={(a: Vendor, b: Vendor) => (a.name ?? "").localeCompare(b.name ?? "")}
                render={(vendor) => (
                  <Link
                    to={`/vendors`}
                    state={{ vendorModal: true, vendorId: vendor.id }}
                    onClick={(e) => e.stopPropagation()}
                  >
                    {vendor.name}
                  </Link>
                )}
              />
              <Table.Column
                title={<Trans>Code</Trans>}
                dataIndex="code"
                key="code"
                width={100}
                sorter={(a: Vendor, b: Vendor) => (a.code ?? "").localeCompare(b.code ?? "")}
              />
              <Table.Column
                title={<Trans>Address</Trans>}
                key="address"
                sorter={(a: Vendor, b: Vendor) =>
                  formatAddressOneLine(a).localeCompare(formatAddressOneLine(b))
                }
                render={(vendor: Vendor) => formatAddressOneLine(vendor)}
              />
              <Table.Column
                title={<Trans>Emails</Trans>}
                dataIndex="emails"
                key="emails"
                sorter={(a: Vendor, b: Vendor) => (a.emails ?? "").localeCompare(b.emails ?? "")}
                render={(emails: string) => {
                  if (!emails) return "";
                  let parsed: string[];
                  try {
                    parsed = JSON.parse(emails);
                  } catch {
                    return "";
                  }
                  return parsed.map((email: string) => <Tag key={email}>{email}</Tag>);
                }}
              />
              <Table.Column
                title={<Trans>Phone</Trans>}
                dataIndex="phone"
                key="phone"
                sorter={(a: Vendor, b: Vendor) => (a.phone ?? "").localeCompare(b.phone ?? "")}
                render={(phone) => {
                  if (!isEmpty(phone)) {
                    return (
                      <a href={`tel:${phone}`} onClick={(e) => e.stopPropagation()}>
                        <PhoneOutlined />
                        {` ${phone}`}
                      </a>
                    );
                  }
                }}
              />
              <Table.Column
                title={<Trans>VATIN</Trans>}
                dataIndex="vatin"
                key="vatin"
                sorter={(a: Vendor, b: Vendor) => (a.vatin ?? "").localeCompare(b.vatin ?? "")}
              />
            </Table>
            <Outlet />
          </Col>
        </Row>

        <VendorForm />
      </>
    );
  }

  const owingCount = summaries?.owingCount ?? 0;
  const overdueCount = summaries?.overdueCount ?? 0;
  const list = (
    <>
      <Table
        dataSource={shown}
        pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
        rowKey="id"
        loading={loading}
        locale={{ emptyText }}
        onRow={rowProps}
        // A class, not a row style: the pinned "You owe" cell paints its own
        // background, so the highlight is set on every cell in base.scss.
        rowClassName={(record: Vendor) =>
          record.id === selectedId ? "master-data-selected-row" : ""
        }
        size="middle"
        scroll={{ x: "max-content" }}
      >
        <Table.Column
          title={<Trans>Vendor</Trans>}
          key="name"
          sorter={(a: Vendor, b: Vendor) => (a.name ?? "").localeCompare(b.name ?? "")}
          render={(vendor: Vendor) => (
            <span
              style={{
                display: "inline-flex",
                alignItems: "center",
                gap: 8,
                maxWidth: 320,
                whiteSpace: "nowrap",
              }}
            >
              <Tooltip title={vendor.name}>
                <Typography.Link
                  ellipsis
                  style={{ maxWidth: 220 }}
                  onClick={(e) => {
                    e.stopPropagation();
                    pick(vendor);
                  }}
                >
                  {vendor.name}
                </Typography.Link>
              </Tooltip>
              {vendor.code && <Tag style={{ marginInlineEnd: 0 }}>{vendor.code}</Tag>}
            </span>
          )}
        />
        {/* Only from 1440px, where the panel beside the list leaves room; the panel shows the city. */}
        {wide && (
          <Table.Column
            title={<Trans>City</Trans>}
            key="city"
            render={(vendor: Vendor) =>
              vendor.city ? <span style={{ whiteSpace: "nowrap" }}>{vendor.city}</span> : notSet
            }
          />
        )}
        <Table.Column
          title={<Trans>Phone</Trans>}
          key="phone"
          render={(vendor: Vendor) =>
            vendor.phone ? (
              <a
                href={`tel:${vendor.phone}`}
                onClick={(e) => e.stopPropagation()}
                style={{ whiteSpace: "nowrap" }}
              >
                {vendor.phone}
              </a>
            ) : (
              notSet
            )
          }
        />
        <Table.Column
          title={<Trans>Last purchase</Trans>}
          key="lastPurchase"
          sorter={(a: Vendor, b: Vendor) =>
            (summaryById.get(a.id)?.lastPurchase ?? 0) - (summaryById.get(b.id)?.lastPurchase ?? 0)
          }
          render={(vendor: Vendor) => {
            const last = summaryById.get(vendor.id)?.lastPurchase;
            return last ? <span style={{ whiteSpace: "nowrap" }}>{formatDate(last)}</span> : "";
          }}
        />
        <Table.Column
          title={<Trans>You owe</Trans>}
          key="owed"
          align="right"
          // Pinned, so what is owed stays in view when the table is wider than
          // its column and scrolls (1280px with the panel open).
          fixed="right"
          sorter={(a: Vendor, b: Vendor) =>
            (summaryById.get(a.id)?.owed ?? 0) - (summaryById.get(b.id)?.owed ?? 0)
          }
          render={(vendor: Vendor) => {
            const owed = summaryById.get(vendor.id)?.owed ?? 0;
            return owed > 0 ? (
              <span
                style={{
                  whiteSpace: "nowrap",
                  fontWeight: 500,
                  fontVariantNumeric: "tabular-nums",
                }}
              >
                {money(owed)}
              </span>
            ) : (
              ""
            );
          }}
        />
      </Table>
      <Outlet />
    </>
  );

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
      <PageHeader
        icon={<SolutionOutlined />}
        title={<Trans>Vendors</Trans>}
        search={{
          placeholder: t`Name, phone or tax number`,
          value: search,
          onChange: setSearch,
        }}
        actions={
          <Space wrap>
            {excel}
            {newVendorButton}
          </Space>
        }
      />
      <p style={{ margin: "-12px 0 0", color: token.colorTextSecondary }}>
        {plural(vendors.length, { one: "# vendor", other: "# vendors" })}
      </p>

      <div
        style={{
          display: "flex",
          flexWrap: "wrap",
          alignItems: "flex-end",
          justifyContent: "space-between",
          gap: "12px 24px",
          borderTop: `1px solid ${token.colorBorderSecondary}`,
          paddingTop: 20,
        }}
      >
        <HeadlineFigure
          label={<Trans>What you owe your vendors</Trans>}
          value={summaries ? money(summaries.totalOwed) : "…"}
          note={
            summaries
              ? plural(owingCount, { one: "across # vendor", other: "across # vendors" })
              : undefined
          }
        />
        <FilterChips<VendorFilter>
          ariaLabel={t`Filter vendors`}
          value={chip}
          onChange={setChip}
          chips={[
            { key: "all", label: <Trans>All</Trans>, count: vendors.length },
            { key: "owing", label: <Trans>You owe them</Trans>, count: owingCount },
            { key: "overdue", label: <Trans>Overdue</Trans>, count: overdueCount },
          ]}
        />
      </div>

      <ListWithPanel
        list={list}
        panelOpen={!!selected}
        onClosePanel={() => setSelectedId(null)}
        panel={
          selected ? (
            <VendorSummaryPanel
              key={selected.id}
              vendor={selected}
              onEdit={() => openForm(selected.id)}
            />
          ) : (
            <p style={{ margin: 0, color: token.colorTextSecondary }}>
              <Trans>Select a vendor to see what you owe them and your recent purchases.</Trans>
            </p>
          )
        }
      />

      <VendorForm />
    </div>
  );
};

export default Vendors;
