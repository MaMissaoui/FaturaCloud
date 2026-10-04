import { useMemo, useState } from "react";
import type { Client } from "src/types/models";
import { Link, Outlet, useNavigate } from "react-router";
import { Button, Col, Empty, Space, Table, Row, Tag, theme, Tooltip, Typography } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { PhoneOutlined, TeamOutlined } from "@ant-design/icons";
import isEmpty from "lodash/isEmpty";
import filter from "lodash/filter";
import get from "lodash/get";
import includes from "lodash/includes";
import some from "lodash/some";
import toString from "lodash/toString";

import { clientsAtom, setClientsAtom } from "src/atoms/client";
import { organizationAtom, organizationIdAtom } from "src/atoms/organization";
import { GetClientSummaries, type ClientSummaryList } from "src/api";
import ClientForm from "src/components/clients/form";
import ClientSummaryPanel from "src/components/clients/client-summary-panel";
import { duplicateNameKeys, isBusinessClient, nameKey } from "src/components/clients/client-kind";
import MassDataExcelActions from "src/components/mass-data/mass-data-excel-actions";
import FilterChips from "src/components/master-data/filter-chips";
import HeadlineFigure from "src/components/master-data/headline-figure";
import ListWithPanel from "src/components/master-data/list-with-panel";
import { useSummariesEnabled } from "src/components/master-data/use-summaries-enabled";
import PageHeader from "src/components/page-header";
import { formatAddressOneLine } from "src/utils/address";
import { formatOrgCents } from "src/utils/currencies";
import { useDateFormatter } from "src/utils/date";
import { useFetch } from "src/hooks/useFetch";
import { useLoadOnPath } from "src/hooks/useLoadOnPath";

type ClientFilter = "all" | "owing" | "business" | "private";

const SEARCH_FIELDS = [
  "name",
  "code",
  "registration_number",
  "emails",
  "phone",
  "vatin",
  "website",
  "identity_number",
  "city",
];

const Clients = () => {
  const { i18n } = useLingui();
  const { token } = theme.useToken();
  const navigate = useNavigate();
  const clients = useAtomValue(clientsAtom);
  const setClients = useSetAtom(setClientsAtom);
  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);
  const formatDate = useDateFormatter();
  const [search, setSearch] = useState("");
  const [chip, setChip] = useState<ClientFilter>("all");
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const loading = useLoadOnPath("/clients", () => setClients());

  // Summaries (what each client owes, last purchase, the panel) are on when
  // the organization has them switched on and the role may see client
  // balances. A failed load (the setting switched off meanwhile, a 403)
  // falls back to the plain list rather than showing an error.
  const summariesEnabled = useSummariesEnabled("client-balances");
  const { data: summaries, failed: summariesFailed } = useFetch<ClientSummaryList | null>(
    summariesEnabled && organizationId ? [organizationId, clients.length] : null,
    () => GetClientSummaries(organizationId!),
    null,
  );
  const showSummaries = summariesEnabled && !summariesFailed;

  const summaryById = useMemo(
    () => new Map((summaries?.clients ?? []).map((row) => [row.clientId, row])),
    [summaries],
  );
  const duplicates = useMemo(() => duplicateNameKeys(clients), [clients]);
  const businessCount = useMemo(() => clients.filter(isBusinessClient).length, [clients]);

  const searched = useMemo(
    () =>
      filter(clients, (client: Client) => {
        const needle = search.toLowerCase();
        const fieldsMatch = some(SEARCH_FIELDS, (field) =>
          includes(toString(get(client, field)).toLowerCase(), needle),
        );
        return fieldsMatch || includes(formatAddressOneLine(client).toLowerCase(), needle);
      }),
    [clients, search],
  );

  const shown = useMemo(() => {
    if (!showSummaries) return searched;
    switch (chip) {
      case "owing":
        return searched
          .filter((c) => (summaryById.get(c.id)?.owed ?? 0) > 0)
          .sort((a, b) => (summaryById.get(b.id)?.owed ?? 0) - (summaryById.get(a.id)?.owed ?? 0));
      case "business":
        return searched.filter(isBusinessClient);
      case "private":
        return searched.filter((c) => !isBusinessClient(c));
      default:
        return searched;
    }
  }, [showSummaries, chip, searched, summaryById]);

  const selected = selectedId ? (clients.find((c) => c.id === selectedId) ?? null) : null;
  const money = (cents: number) => formatOrgCents(cents, organization, i18n.locale);
  const openForm = (clientId?: string) =>
    navigate("/clients", { state: { clientModal: true, clientId } });
  const pick = (client: Client) => (showSummaries ? setSelectedId(client.id) : openForm(client.id));

  // A muted dash for an empty cell keeps the columns narrow; the panel spells
  // out "Not set".
  const notSet = (
    <Typography.Text type="secondary" aria-label={t`Not set`}>
      —
    </Typography.Text>
  );

  const newClientButton = (
    <Link to="/clients" state={{ clientModal: true }}>
      <Button type="primary">
        <Trans>New client</Trans>
      </Button>
    </Link>
  );
  const excel = organizationId && (
    <MassDataExcelActions
      organizationId={organizationId}
      resource="clients"
      filenamePrefix="clients"
      onImported={() => setClients()}
      compact
    />
  );

  const emptyText = search ? (
    <Empty description={<Trans>No clients match your search</Trans>} />
  ) : (
    <Empty description={<Trans>No clients yet</Trans>}>
      <Link to="/clients" state={{ clientModal: true }}>
        <Button type="primary">
          <Trans>Create your first client</Trans>
        </Button>
      </Link>
    </Empty>
  );

  const rowProps = (record: Client) => ({
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
          icon={<TeamOutlined />}
          title={<Trans>Clients</Trans>}
          search={{ placeholder: t`Search`, value: search, onChange: setSearch }}
          actions={
            <Space wrap>
              {excel}
              {newClientButton}
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
                sorter={(a: Client, b: Client) => (a.name ?? "").localeCompare(b.name ?? "")}
                render={(client) => (
                  <Link
                    to={`/clients`}
                    state={{ clientModal: true, clientId: client.id }}
                    onClick={(e) => e.stopPropagation()}
                  >
                    {client.name}
                  </Link>
                )}
              />
              <Table.Column
                title={<Trans>Code</Trans>}
                dataIndex="code"
                key="code"
                width={100}
                sorter={(a: Client, b: Client) => (a.code ?? "").localeCompare(b.code ?? "")}
              />
              <Table.Column
                title={<Trans>Address</Trans>}
                key="address"
                sorter={(a: Client, b: Client) =>
                  formatAddressOneLine(a).localeCompare(formatAddressOneLine(b))
                }
                render={(client: Client) => formatAddressOneLine(client)}
              />
              <Table.Column
                title={<Trans>Emails</Trans>}
                dataIndex="emails"
                key="emails"
                sorter={(a: Client, b: Client) => (a.emails ?? "").localeCompare(b.emails ?? "")}
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
                sorter={(a: Client, b: Client) => (a.phone ?? "").localeCompare(b.phone ?? "")}
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
                sorter={(a: Client, b: Client) => (a.vatin ?? "").localeCompare(b.vatin ?? "")}
              />
              <Table.Column
                title={<Trans>Identity number</Trans>}
                dataIndex="identity_number"
                key="identity_number"
                sorter={(a: Client, b: Client) =>
                  (a.identity_number ?? "").localeCompare(b.identity_number ?? "")
                }
              />
            </Table>
            <Outlet />
          </Col>
        </Row>

        <ClientForm />
      </>
    );
  }

  const owingCount = summaries?.owingCount ?? 0;
  const list = (
    <>
      <Table
        dataSource={shown}
        pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
        rowKey="id"
        loading={loading}
        locale={{ emptyText }}
        onRow={rowProps}
        // A class, not a row style: the pinned Owes cell paints its own
        // background, so the highlight is set on every cell in base.scss.
        rowClassName={(record: Client) =>
          record.id === selectedId ? "master-data-selected-row" : ""
        }
        size="middle"
        scroll={{ x: "max-content" }}
      >
        <Table.Column
          title={<Trans>Client</Trans>}
          key="name"
          sorter={(a: Client, b: Client) => (a.name ?? "").localeCompare(b.name ?? "")}
          render={(client: Client) => (
            <span
              style={{
                display: "inline-flex",
                alignItems: "center",
                gap: 8,
                maxWidth: 320,
                whiteSpace: "nowrap",
              }}
            >
              <Tooltip title={client.name}>
                <Typography.Link
                  ellipsis
                  style={{ maxWidth: 200 }}
                  onClick={(e) => {
                    e.stopPropagation();
                    pick(client);
                  }}
                >
                  {client.name}
                </Typography.Link>
              </Tooltip>
              {isBusinessClient(client) && (
                <Tag color="blue" style={{ marginInlineEnd: 0 }}>
                  <Trans>Business</Trans>
                </Tag>
              )}
              {duplicates.has(nameKey(client.name)) && (
                <Tag style={{ marginInlineEnd: 0 }}>
                  <Trans>Possible duplicate</Trans>
                </Tag>
              )}
            </span>
          )}
        />
        <Table.Column
          title={<Trans>City</Trans>}
          key="city"
          // Hidden below 1440px (src/styles/base.scss), where the panel beside
          // the list leaves no room; the panel shows the city.
          className="master-data-wide-only"
          render={(client: Client) => {
            const city = client.city || client.address;
            return city ? <span style={{ whiteSpace: "nowrap" }}>{city}</span> : notSet;
          }}
        />
        <Table.Column
          title={<Trans>Phone</Trans>}
          key="phone"
          render={(client: Client) =>
            client.phone ? (
              <a
                href={`tel:${client.phone}`}
                onClick={(e) => e.stopPropagation()}
                style={{ whiteSpace: "nowrap" }}
              >
                {client.phone}
              </a>
            ) : (
              notSet
            )
          }
        />
        <Table.Column
          title={<Trans>Last purchase</Trans>}
          key="lastPurchase"
          sorter={(a: Client, b: Client) =>
            (summaryById.get(a.id)?.lastPurchase ?? 0) - (summaryById.get(b.id)?.lastPurchase ?? 0)
          }
          render={(client: Client) => {
            const last = summaryById.get(client.id)?.lastPurchase;
            return last ? <span style={{ whiteSpace: "nowrap" }}>{formatDate(last)}</span> : "";
          }}
        />
        <Table.Column
          title={<Trans>Owes</Trans>}
          key="owed"
          align="right"
          // Pinned, so what a client owes stays in view when the table is
          // wider than its column and scrolls (1280px with the panel open).
          fixed="right"
          sorter={(a: Client, b: Client) =>
            (summaryById.get(a.id)?.owed ?? 0) - (summaryById.get(b.id)?.owed ?? 0)
          }
          render={(client: Client) => {
            const owed = summaryById.get(client.id)?.owed ?? 0;
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
        icon={<TeamOutlined />}
        title={<Trans>Clients</Trans>}
        search={{
          placeholder: t`Name, phone, ID or tax number`,
          value: search,
          onChange: setSearch,
        }}
        actions={
          <Space wrap>
            {excel}
            {newClientButton}
          </Space>
        }
      />
      <p style={{ margin: "-12px 0 0", color: token.colorTextSecondary }}>
        {t`${clients.length} clients, ${businessCount} of them businesses`}
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
          label={<Trans>What your clients owe you</Trans>}
          value={summaries ? money(summaries.totalOwed) : "…"}
          note={summaries ? t`across ${owingCount} clients` : undefined}
        />
        <FilterChips<ClientFilter>
          ariaLabel={t`Filter clients`}
          value={chip}
          onChange={setChip}
          chips={[
            { key: "all", label: <Trans>All</Trans>, count: clients.length },
            { key: "owing", label: <Trans>Owe money</Trans>, count: owingCount },
            { key: "business", label: <Trans>Businesses</Trans>, count: businessCount },
            {
              key: "private",
              label: <Trans>Private</Trans>,
              count: clients.length - businessCount,
            },
          ]}
        />
      </div>

      <ListWithPanel
        list={list}
        panelOpen={!!selected}
        onClosePanel={() => setSelectedId(null)}
        panel={
          selected ? (
            <ClientSummaryPanel
              key={selected.id}
              client={selected}
              onEdit={() => openForm(selected.id)}
            />
          ) : (
            <p style={{ margin: 0, color: token.colorTextSecondary }}>
              <Trans>Select a client to see what they owe and their recent activity.</Trans>
            </p>
          )
        }
      />

      <ClientForm />
    </div>
  );
};

export default Clients;
