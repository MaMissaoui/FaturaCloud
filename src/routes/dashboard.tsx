import { useState } from "react";
import type { CSSProperties, ReactNode } from "react";
import { Link, useNavigate } from "react-router";
import { Alert, Button, Select, Skeleton, theme, Typography } from "antd";
import { Column } from "@ant-design/plots";
import { useAtomValue } from "jotai";
import { Trans, Plural } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { DashboardOutlined } from "@ant-design/icons";

import { organizationIdAtom, organizationAtom, myOrgRoleAtom } from "src/atoms/organization";
import { dashboardWidgetsForRole, isRouteAllowedForRole } from "src/layouts/role-menu";
import { themeAtom } from "src/atoms/generic";
import { GetDashboard } from "src/api";
import type { DashboardData, OutstandingInvoiceSummary } from "src/api";
import PageHeader from "src/components/page-header";
import OwedPanel from "src/components/dashboard/owed-panel";
import TillPanel from "src/components/dashboard/till-panel";
import { useFetch } from "src/hooks/useFetch";
import { formatOrgCents, numberFormatLocale } from "src/utils/currencies";

// Stock quantities are a display concern only — the organization's
// country-derived locale (falling back to the viewer's UI language) renders
// "2,5" not a hardcoded "."; whole values stay clean and fractions are capped
// at 2 decimals.
const formatQty = (qty: number, locale: string) =>
  new Intl.NumberFormat(locale, {
    minimumFractionDigits: 0,
    maximumFractionDigits: 2,
  }).format(qty);

const MONTH_OPTIONS = [3, 6, 12, 24];

// A rolling window ("m12") or a calendar year ("y2026") in one Select —
// years are generated at render time (not a fixed list) so "current
// year"/"last year" never go stale, and a handful of further-back years
// stay reachable without the list growing unbounded.
const CALENDAR_YEARS_BACK = 4;

type Period = { kind: "months"; months: number } | { kind: "year"; year: number };

const periodToValue = (p: Period) => (p.kind === "months" ? `m${p.months}` : `y${p.year}`);

const periodFromValue = (v: string): Period =>
  v[0] === "y"
    ? { kind: "year", year: Number(v.slice(1)) }
    : { kind: "months", months: Number(v.slice(1)) };

// Two sections side by side that stack on a narrow screen; a section a role
// can't see is left out, and the other takes the row.
const rowStyle: CSSProperties = {
  display: "flex",
  flexWrap: "wrap",
  gap: 24,
  alignItems: "stretch",
  marginTop: 24,
};

const capitalize = (s: string) => s.charAt(0).toLocaleUpperCase() + s.slice(1);

// The Dashboard (2026-10 redesign): what clients owe leads, with the till
// beside it, then sales, stock and the top lists. Each section follows the
// role's sections (dashboardWidgetsForRole, audit F147); the till is also
// only sent to a role that may use the Cash Book.
const Dashboard = () => {
  const { i18n } = useLingui();
  const { token } = theme.useToken();
  const navigate = useNavigate();
  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);
  const themeMode = useAtomValue(themeAtom);
  const orgRole = useAtomValue(myOrgRoleAtom);
  const show = dashboardWidgetsForRole(orgRole);
  const canOpen = (path: string) => isRouteAllowedForRole(orgRole, path);
  // A row-click handler set for a table row, or none when the role can't
  // open the target page (the row then isn't presented as clickable).
  const rowLink = (path: string, go: () => void) =>
    canOpen(path)
      ? {
          onClick: go,
          onKeyDown: (e: React.KeyboardEvent) => {
            if (e.key === "Enter" || e.key === " ") {
              e.preventDefault();
              go();
            }
          },
          style: { cursor: "pointer" },
          tabIndex: 0,
        }
      : {};
  const pageLink = (path: string, label: ReactNode) =>
    canOpen(path) ? <Link to={path}>{label}</Link> : undefined;
  const reportLink = (path: string) => pageLink(path, t`View full report`);

  const [period, setPeriod] = useState<Period>({ kind: "months", months: 12 });
  // A failed fetch must never read as "nothing owed, nothing to see" on the
  // one screen every user checks to decide what needs attention: on failure
  // only the error and its Retry show, never zeros.
  const {
    data,
    loading,
    failed,
    reload: fetchDashboard,
  } = useFetch<DashboardData | null>(
    organizationId ? [organizationId, periodToValue(period)] : null,
    () =>
      GetDashboard(
        organizationId!,
        period.kind === "year" ? { year: period.year } : { months: period.months },
      ),
    null,
  );

  const money = (cents: number) => formatOrgCents(cents, organization, i18n.locale);
  const qtyLocale = numberFormatLocale(organization?.country_code) ?? i18n.locale;
  const timeZone = organization?.timezone || undefined;
  const today = capitalize(
    new Intl.DateTimeFormat(i18n.locale, {
      weekday: "long",
      day: "numeric",
      month: "long",
      year: "numeric",
      timeZone,
    }).format(new Date()),
  );
  // A YYYY-MM-DD day of the till, as a weekday and date.
  const dayLabel = (date: string) =>
    capitalize(
      new Intl.DateTimeFormat(i18n.locale, {
        weekday: "long",
        day: "numeric",
        month: "long",
        timeZone: "UTC",
      }).format(new Date(`${date}T12:00:00Z`)),
    );

  const revenueTotal = (data?.revenueByMonth ?? []).reduce((sum, m) => sum + m.revenue, 0);
  const units = formatQty(data?.stockValuation.units ?? 0, qtyLocale);
  const threshold = formatQty(data?.lowStock.threshold ?? 0, qtyLocale);
  const till = show.till ? (data?.cashRegister ?? null) : null;
  const cashBookLink = pageLink("/cash-book", t`Open the Cash Book`);

  const sectionHead = (id: string, title: ReactNode, link?: ReactNode) => (
    <div
      style={{
        display: "flex",
        flexWrap: "wrap",
        justifyContent: "space-between",
        alignItems: "baseline",
        gap: 8,
        borderTop: `1px solid ${token.colorBorder}`,
        paddingTop: 14,
      }}
    >
      <h2 id={id} style={{ margin: 0, fontSize: 18, fontWeight: 600 }}>
        {title}
      </h2>
      {link}
    </div>
  );

  const rankedList = (
    items: { key: string; name: string; value: string; open?: () => void }[],
    empty: ReactNode,
  ) =>
    items.length === 0 ? (
      <Typography.Text type="secondary">{empty}</Typography.Text>
    ) : (
      <ol style={{ margin: 0, padding: 0, listStyle: "none" }}>
        {items.map((item) => (
          <li
            key={item.key}
            style={{
              display: "flex",
              justifyContent: "space-between",
              alignItems: "baseline",
              gap: 12,
              padding: "8px 0",
              borderBottom: `1px solid ${token.colorBorderSecondary}`,
            }}
          >
            {item.open ? (
              <Button
                type="link"
                onClick={item.open}
                style={{ padding: 0, height: "auto", whiteSpace: "normal", textAlign: "left" }}
              >
                {item.name}
              </Button>
            ) : (
              <span>{item.name}</span>
            )}
            <span style={{ fontWeight: 500, whiteSpace: "nowrap" }}>{item.value}</span>
          </li>
        ))}
      </ol>
    );

  const openProduct = (productId: string) =>
    canOpen("/products")
      ? () => navigate("/products", { state: { productModal: true, productId } })
      : undefined;
  const openClient = (clientId: string) =>
    canOpen("/clients")
      ? () => navigate("/clients", { state: { clientModal: true, clientId } })
      : undefined;

  return (
    <div style={{ fontVariantNumeric: "tabular-nums" }}>
      <PageHeader
        icon={<DashboardOutlined />}
        title={<Trans>Dashboard</Trans>}
        actions={
          show.sales ? (
            <Select
              aria-label={t`Sales period`}
              value={periodToValue(period)}
              onChange={(v) => setPeriod(periodFromValue(v))}
              style={{ width: 200 }}
              options={[
                {
                  label: t`Rolling window`,
                  options: MONTH_OPTIONS.map((m) => ({
                    value: `m${m}`,
                    label: <Trans>Last {m} months</Trans>,
                  })),
                },
                {
                  label: t`Calendar year`,
                  options: Array.from({ length: CALENDAR_YEARS_BACK + 1 }, (_, i) => {
                    const year = new Date().getFullYear() - i;
                    const label =
                      i === 0 ? (
                        <Trans>Current year ({year})</Trans>
                      ) : i === 1 ? (
                        <Trans>Last year ({year})</Trans>
                      ) : (
                        String(year)
                      );
                    return { value: `y${year}`, label };
                  }),
                },
              ]}
            />
          ) : undefined
        }
      />
      <Typography.Text type="secondary" style={{ display: "block", marginTop: 4 }}>
        {organization?.name ? `${organization.name} — ${today}` : today}
      </Typography.Text>

      {failed && (
        <Alert
          style={{ marginTop: 16 }}
          type="error"
          showIcon
          message={<Trans>Couldn't load the dashboard</Trans>}
          action={
            <Button size="small" onClick={fetchDashboard}>
              <Trans>Retry</Trans>
            </Button>
          }
        />
      )}

      {!failed && !data && loading && <Skeleton active style={{ marginTop: 24 }} />}

      {!failed && data && (
        <>
          {(show.receivables || till) && (
            <div style={rowStyle}>
              {show.receivables && (
                <OwedPanel
                  outstanding={data.outstanding}
                  followUp={data.loanFollowUp}
                  money={money}
                  invoiceRow={(inv: OutstandingInvoiceSummary) =>
                    rowLink(`/invoices/${inv.id}`, () => navigate(`/invoices/${inv.id}`))
                  }
                  reportLink={reportLink("/accounting/reports/ar-aging")}
                  cashBookLink={cashBookLink}
                />
              )}
              {till && (
                <TillPanel
                  till={till}
                  money={money}
                  dayLabel={dayLabel}
                  cashBookLink={cashBookLink}
                />
              )}
            </div>
          )}

          {(show.sales || show.stock) && (
            <div style={{ ...rowStyle, gap: "32px 48px", marginTop: 32 }}>
              {show.sales && (
                <section
                  aria-labelledby="dashboard-sales"
                  style={{
                    flex: "2 1 520px",
                    minWidth: 0,
                    display: "flex",
                    flexDirection: "column",
                    gap: 12,
                  }}
                >
                  {sectionHead(
                    "dashboard-sales",
                    <Trans>Revenue</Trans>,
                    reportLink("/reporting/revenue-trend"),
                  )}
                  <p style={{ margin: 0 }}>
                    <span style={{ fontSize: 24, fontWeight: 600 }}>{money(revenueTotal)}</span>{" "}
                    <Typography.Text type="secondary">
                      <Trans>in the selected period</Trans>
                    </Typography.Text>
                  </p>
                  <div role="img" aria-label={t`Column chart showing revenue over time`}>
                    <Column
                      data={data.revenueByMonth}
                      xField="month"
                      yField="revenue"
                      theme={themeMode === "dark" ? "classicDark" : "classic"}
                      height={220}
                      style={{ fill: token.colorPrimary }}
                      axis={{ y: { labelFormatter: (v: number) => money(v) } }}
                      tooltip={{
                        items: [
                          {
                            field: "revenue",
                            name: t`Revenue`,
                            valueFormatter: (v: number) => money(v),
                          },
                        ],
                      }}
                    />
                  </div>
                </section>
              )}

              {show.stock && (
                <section
                  aria-labelledby="dashboard-stock"
                  style={{
                    flex: "1 1 300px",
                    minWidth: 0,
                    display: "flex",
                    flexDirection: "column",
                    gap: 12,
                  }}
                >
                  {sectionHead(
                    "dashboard-stock",
                    <Trans>Stock</Trans>,
                    reportLink("/accounting/reports/inventory-valuation"),
                  )}
                  <p style={{ margin: 0 }}>
                    <span style={{ fontSize: 24, fontWeight: 600, overflowWrap: "anywhere" }}>
                      {money(data.stockValuation.total)}
                    </span>
                    <br />
                    <Typography.Text type="secondary">
                      <Plural
                        value={data.stockValuation.productCount}
                        one={`# stock-tracked product, ${units} units`}
                        other={`# stock-tracked products, ${units} units`}
                      />
                    </Typography.Text>
                  </p>
                  <h3 style={{ margin: "4px 0 0", fontSize: 15, fontWeight: 600 }}>
                    <Trans>{threshold} units or fewer</Trans>
                  </h3>
                  {rankedList(
                    data.lowStock.items.slice(0, 6).map((p) => ({
                      key: p.productId,
                      name: p.name,
                      value: formatQty(p.quantity, qtyLocale),
                      open: openProduct(p.productId),
                    })),
                    <Trans>No product is running low.</Trans>,
                  )}
                  {data.lowStock.count > 6 &&
                    pageLink(
                      "/inventory",
                      <Plural
                        value={data.lowStock.count}
                        one="See all # products in Inventory"
                        other="See all # products in Inventory"
                      />,
                    )}
                </section>
              )}
            </div>
          )}

          {show.sales && (
            <div style={{ ...rowStyle, gap: "32px 48px", marginTop: 32 }}>
              <section
                aria-labelledby="dashboard-top-clients"
                style={{ flex: "1 1 320px", minWidth: 0 }}
              >
                {sectionHead(
                  "dashboard-top-clients",
                  <Trans>Top clients</Trans>,
                  reportLink("/reporting/sales-by-client"),
                )}
                <div style={{ marginTop: 4 }}>
                  {rankedList(
                    data.topClients.map((c) => ({
                      key: c.clientId,
                      name: c.name,
                      value: money(c.revenue),
                      open: openClient(c.clientId),
                    })),
                    <Trans>No revenue in this period</Trans>,
                  )}
                </div>
              </section>
              <section
                aria-labelledby="dashboard-top-products"
                style={{ flex: "1 1 320px", minWidth: 0 }}
              >
                {sectionHead(
                  "dashboard-top-products",
                  <Trans>Top products</Trans>,
                  reportLink("/reporting/sales-by-product"),
                )}
                <div style={{ marginTop: 4 }}>
                  {rankedList(
                    data.topProducts.map((p) => ({
                      key: p.productId,
                      name: p.name,
                      value: money(p.revenue),
                      open: openProduct(p.productId),
                    })),
                    <Trans>No revenue in this period</Trans>,
                  )}
                </div>
              </section>
            </div>
          )}
        </>
      )}
    </div>
  );
};

export default Dashboard;
