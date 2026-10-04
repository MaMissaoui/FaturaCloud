import type { CSSProperties, ReactNode } from "react";
import { Link } from "react-router";
import { Alert, Button, Skeleton, Space, theme } from "antd";
import { useAtomValue } from "jotai";
import dayjs from "dayjs";
import { Plural, Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";

import { GetVendorSummary, type OutstandingBucket, type VendorSummary } from "src/api";
import type { Vendor } from "src/types/vendor";
import { myOrgRoleSyncAtom, organizationAtom } from "src/atoms/organization";
import { roleCanSeeMenuItem } from "src/layouts/role-menu";
import { paymentMethodLabel } from "src/types/payment";
import { formatMoneyUnits, formatOrgCents } from "src/utils/currencies";
import { useFetch } from "src/hooks/useFetch";
import { formatAddressOneLine } from "src/utils/address";
import { useDateFormatter } from "src/utils/date";
import AgingBar from "src/components/master-data/aging-bar";
import { lateTextColor } from "src/components/master-data/aging";
import { themeAtom } from "src/atoms/generic";
import { parseClientEmails } from "src/components/clients/client-kind";

// VendorSummaryPanel is the Vendors screen's right-hand panel, the purchases
// mirror of ClientSummaryPanel: what the organization owes the vendor and how
// late, the incoming invoices still open, the buying and paying history and
// the contact details, with the actions that follow from it. It loads
// GET /api/vendors/{id}/summary each time the picked vendor changes.
export default function VendorSummaryPanel({
  vendor,
  onEdit,
}: {
  vendor: Vendor;
  onEdit: () => void;
}) {
  const { i18n } = useLingui();
  const { token } = theme.useToken();
  const organization = useAtomValue(organizationAtom);
  const role = useAtomValue(myOrgRoleSyncAtom);
  const dark = useAtomValue(themeAtom) === "dark";
  const formatDate = useDateFormatter();
  const {
    data: summary,
    loading,
    failed,
    reload,
  } = useFetch<VendorSummary | null>([vendor.id], () => GetVendorSummary(vendor.id), null);

  const money = (cents: number) => formatOrgCents(cents, organization, i18n.locale);
  const secondary: CSSProperties = { color: token.colorTextSecondary };
  const rule: CSSProperties = {
    borderTop: `1px solid ${token.colorBorderSecondary}`,
    paddingTop: 16,
  };
  const emails = parseClientEmails(vendor.emails);
  const address = formatAddressOneLine(vendor);

  const kindLine = [
    vendor.city || "",
    summary?.firstPurchase
      ? t`vendor since ${dayjs(summary.firstPurchase).format("MMMM YYYY")}`
      : "",
  ]
    .filter(Boolean)
    .join(", ");

  // The core contact rows always show ("Not set" when empty, as on the
  // Clients panel); the optional ones only when filled in.
  const contact: { label: string; value: ReactNode; optional?: boolean }[] = [
    {
      label: t`Phone`,
      value: vendor.phone ? <a href={`tel:${vendor.phone}`}>{vendor.phone}</a> : null,
    },
    { label: t`Tax number`, value: vendor.vatin || null },
    {
      label: t`Registration number`,
      value: vendor.registration_number || null,
      optional: true,
    },
    { label: t`Address`, value: address || null },
    { label: t`Code`, value: vendor.code || null },
    { label: t`E-mail`, value: emails.length ? emails.join(", ") : null },
    {
      label: t`Website`,
      value: vendor.website ? (
        <a href={websiteHref(vendor.website)} target="_blank" rel="noreferrer">
          {vendor.website}
        </a>
      ) : null,
      optional: true,
    },
    {
      label: t`Payment terms`,
      value: vendor.paymentTermsDays ? t`${vendor.paymentTermsDays} days` : null,
      optional: true,
    },
    { label: t`Currency`, value: vendor.defaultCurrency || null, optional: true },
  ];

  return (
    <section
      aria-labelledby="vendor-summary-name"
      style={{ display: "flex", flexDirection: "column", gap: 20 }}
    >
      <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
        <h2
          id="vendor-summary-name"
          style={{ margin: 0, fontSize: 20, lineHeight: "28px", fontWeight: 600 }}
        >
          {vendor.name}
        </h2>
        {kindLine && <p style={{ margin: 0, ...secondary }}>{kindLine}</p>}
      </div>

      <Space wrap>
        {roleCanSeeMenuItem(role, "group-purchasing", "incoming-invoices") && (
          <Link to="/incoming-invoices/new">
            <Button>
              <Trans>New incoming invoice</Trans>
            </Button>
          </Link>
        )}
        {roleCanSeeMenuItem(role, "group-purchasing", "purchase-orders") && (
          <Link to="/purchase-orders/new">
            <Button>
              <Trans>New purchase order</Trans>
            </Button>
          </Link>
        )}
        <Button onClick={onEdit}>
          <Trans>Edit</Trans>
        </Button>
      </Space>

      {failed && (
        <Alert
          type="error"
          showIcon
          title={<Trans>What you owe this vendor could not be loaded.</Trans>}
          action={
            <Button size="small" onClick={reload}>
              <Trans>Retry</Trans>
            </Button>
          }
        />
      )}
      {loading && !summary && <Skeleton active paragraph={{ rows: 6 }} />}

      {summary && (
        <>
          <div style={{ ...rule, display: "flex", flexDirection: "column", gap: 10 }}>
            <p style={{ margin: 0, ...secondary }}>
              <Trans>You owe</Trans>
            </p>
            {summary.owed > 0 ? (
              <>
                <p
                  style={{
                    margin: 0,
                    display: "flex",
                    flexWrap: "wrap",
                    alignItems: "baseline",
                    gap: "4px 10px",
                  }}
                >
                  <span
                    style={{
                      fontSize: 28,
                      lineHeight: 1.1,
                      fontWeight: 600,
                      fontVariantNumeric: "tabular-nums",
                    }}
                  >
                    {money(summary.owed)}
                  </span>
                  <span style={secondary}>
                    {summary.current === 0 ? (
                      <Plural
                        value={summary.openBills.length}
                        one="on # incoming invoice, overdue"
                        other="on # incoming invoices, all overdue"
                      />
                    ) : (
                      <Plural
                        value={summary.openBills.length}
                        one="on # incoming invoice"
                        other="on # incoming invoices"
                      />
                    )}
                  </span>
                </p>
                <AgingBar
                  money={money}
                  amounts={
                    {
                      current: summary.current,
                      days1To30: summary.days1To30,
                      days31To60: summary.days31To60,
                      days61To90: summary.days61To90,
                      days90Plus: summary.days90Plus,
                    } satisfies Record<OutstandingBucket, number>
                  }
                />
              </>
            ) : (
              <p style={{ margin: 0, fontSize: 20, fontWeight: 600 }}>
                <Trans>Nothing owed</Trans>
              </p>
            )}
          </div>

          {summary.openBills.length > 0 && (
            <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
              <h3 style={{ margin: 0, fontSize: 15, fontWeight: 600 }}>
                <Trans>Unpaid incoming invoices</Trans>
              </h3>
              <table
                style={{
                  width: "100%",
                  borderCollapse: "collapse",
                  fontVariantNumeric: "tabular-nums",
                }}
              >
                <thead>
                  <tr style={{ textAlign: "left", fontSize: 13, ...secondary }}>
                    <th scope="col" style={th(token, "left")}>
                      <Trans>Invoice</Trans>
                    </th>
                    <th scope="col" style={th(token, "right")}>
                      <Trans>Days late</Trans>
                    </th>
                    <th scope="col" style={th(token, "right")}>
                      <Trans>Still owed</Trans>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {[...summary.openBills]
                    .sort((a, b) => b.daysOverdue - a.daysOverdue)
                    .map((bill) => {
                      const due = bill.dueDate ? formatDate(bill.dueDate) : "";
                      const foreign =
                        organization?.currency &&
                        bill.currency &&
                        bill.currency !== organization.currency;
                      return (
                        <tr key={bill.id}>
                          <td style={td(token, "left")}>
                            <Link to={`/incoming-invoices/${bill.id}`}>
                              {bill.number || <Trans>No number</Trans>}
                            </Link>
                            {due && (
                              <div style={{ ...secondary, fontSize: 12 }}>
                                <Trans>due {due}</Trans>
                              </div>
                            )}
                          </td>
                          <td
                            style={{
                              ...td(token, "right"),
                              ...(bill.daysOverdue > 90
                                ? { color: lateTextColor(dark), fontWeight: 500 }
                                : {}),
                            }}
                          >
                            {bill.daysOverdue > 0 ? t`${bill.daysOverdue} d` : t`Not due`}
                          </td>
                          <td style={td(token, "right")}>
                            {money(bill.total)}
                            {foreign && (
                              <div style={{ fontSize: 12, ...secondary }}>
                                {formatMoneyUnits(
                                  bill.foreignTotal / 100,
                                  bill.currency,
                                  i18n.locale,
                                )}
                              </div>
                            )}
                          </td>
                        </tr>
                      );
                    })}
                </tbody>
              </table>
            </div>
          )}

          {summary.billCount > 0 && (
            <div
              style={{
                background: token.colorFillQuaternary,
                borderRadius: 6,
                padding: "12px 14px",
                display: "flex",
                flexDirection: "column",
                gap: 4,
              }}
            >
              <p style={{ margin: 0 }}>
                {t`${summary.billCount} incoming invoices for ${money(summary.billedTotal)}, of which ${money(summary.paidTotal)} paid in ${summary.paymentCount} payments.`}
              </p>
              {summary.lastPayment && (
                <p style={{ margin: 0, ...secondary }}>
                  {t`Last payment: ${money(summary.lastPayment.amount)} (${paymentMethodLabel(summary.lastPayment.method)}) on ${formatDate(summary.lastPayment.date)}.`}
                </p>
              )}
            </div>
          )}
        </>
      )}

      <dl
        style={{
          ...rule,
          margin: 0,
          display: "grid",
          gridTemplateColumns: "max-content 1fr",
          gap: "8px 16px",
        }}
      >
        {contact
          .filter((row) => row.value || !row.optional)
          .map(({ label, value }) => (
            <div key={label} style={{ display: "contents" }}>
              <dt style={secondary}>{label}</dt>
              <dd style={{ margin: 0, overflowWrap: "anywhere", ...(value ? {} : secondary) }}>
                {value ?? <Trans>Not set</Trans>}
              </dd>
            </div>
          ))}
      </dl>
    </section>
  );
}

// A website stored without a scheme ("acme.tn") would otherwise resolve as a
// path inside the app.
const websiteHref = (site: string) => (/^https?:\/\//i.test(site) ? site : `https://${site}`);

type Token = ReturnType<typeof theme.useToken>["token"];

const th = (token: Token, align: "left" | "right"): CSSProperties => ({
  fontWeight: 500,
  padding: "6px 8px",
  textAlign: align,
  borderBottom: `1px solid ${token.colorBorderSecondary}`,
});

const td = (token: Token, align: "left" | "right"): CSSProperties => ({
  padding: "8px",
  // Amounts and day counts never break across lines; the invoice cell's
  // due date sits on its own line instead.
  whiteSpace: align === "right" ? "nowrap" : undefined,
  textAlign: align,
  borderBottom: `1px solid ${token.colorBorderSecondary}`,
  verticalAlign: "top",
});
