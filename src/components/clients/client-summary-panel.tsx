import type { CSSProperties, ReactNode } from "react";
import { Link } from "react-router";
import { Alert, Button, Skeleton, Space, Tag, theme } from "antd";
import { useAtomValue } from "jotai";
import dayjs from "dayjs";
import { Plural, Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";

import { GetClientSummary, type ClientSummary, type OutstandingBucket } from "src/api";
import type { Client } from "src/types/client";
import { myOrgRoleSyncAtom, organizationAtom } from "src/atoms/organization";
import { roleCanUseCashBook } from "src/layouts/role-menu";
import { paymentMethodLabel } from "src/types/payment";
import { formatMoneyUnits, formatOrgCents } from "src/utils/currencies";
import { useFetch } from "src/hooks/useFetch";
import { formatAddressOneLine } from "src/utils/address";
import { useDateFormatter } from "src/utils/date";
import AgingBar from "src/components/master-data/aging-bar";
import { lateTextColor } from "src/components/master-data/aging";
import { themeAtom } from "src/atoms/generic";
import { isBusinessClient, parseClientEmails } from "src/components/clients/client-kind";

// ClientSummaryPanel is the Clients screen's right-hand panel: what the
// client owes and how late, the invoices still open, their buying and paying
// history and their contact details, with the actions that follow from it.
// It loads GET /api/clients/{id}/summary each time the picked client changes.
export default function ClientSummaryPanel({
  client,
  onEdit,
}: {
  client: Client;
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
  } = useFetch<ClientSummary | null>([client.id], () => GetClientSummary(client.id), null);

  const money = (cents: number) => formatOrgCents(cents, organization, i18n.locale);
  const secondary: CSSProperties = { color: token.colorTextSecondary };
  const rule: CSSProperties = {
    borderTop: `1px solid ${token.colorBorderSecondary}`,
    paddingTop: 16,
  };
  const business = isBusinessClient(client);
  const emails = parseClientEmails(client.emails);
  const taxNumber = client.vatin || client.tax_number;
  const address = formatAddressOneLine(client) || client.address || "";
  const city = client.city || client.address || "";

  const kindLine = [
    business ? t`Business` : t`Private individual`,
    city,
    summary?.firstPurchase
      ? t`client since ${dayjs(summary.firstPurchase).format("MMMM YYYY")}`
      : "",
  ]
    .filter(Boolean)
    .join(", ");

  const contact: { label: string; value: ReactNode }[] = [
    {
      label: t`Phone`,
      value: client.phone ? <a href={`tel:${client.phone}`}>{client.phone}</a> : null,
    },
    { label: t`ID number`, value: client.identity_number || null },
    { label: t`Tax number`, value: taxNumber || null },
    { label: t`Address`, value: address || null },
    { label: t`Code`, value: client.code || null },
    { label: t`E-mail`, value: emails.length ? emails.join(", ") : null },
  ];

  return (
    <section
      aria-labelledby="client-summary-name"
      style={{ display: "flex", flexDirection: "column", gap: 20 }}
    >
      <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
        <div
          style={{
            display: "flex",
            flexWrap: "wrap",
            justifyContent: "space-between",
            alignItems: "flex-start",
            gap: 8,
          }}
        >
          <h2
            id="client-summary-name"
            style={{ margin: 0, fontSize: 20, lineHeight: "28px", fontWeight: 600 }}
          >
            {client.name}
          </h2>
          {client.importBatchId && (
            <Tag style={{ marginInlineEnd: 0 }}>
              <Trans>Imported from the paper register</Trans>
            </Tag>
          )}
        </div>
        <p style={{ margin: 0, ...secondary }}>{kindLine}</p>
      </div>

      <Space wrap>
        {summary && summary.owed > 0 && roleCanUseCashBook(role) && (
          <Link to="/cash-book">
            <Button type="primary">
              <Trans>Open Cash Book</Trans>
            </Button>
          </Link>
        )}
        <Link to="/invoices/new">
          <Button>
            <Trans>New invoice</Trans>
          </Button>
        </Link>
        <Button onClick={onEdit}>
          <Trans>Edit</Trans>
        </Button>
      </Space>

      {failed && (
        <Alert
          type="error"
          showIcon
          title={<Trans>The client's balance could not be loaded.</Trans>}
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
              <Trans>Owes</Trans>
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
                        value={summary.openInvoices.length}
                        one="on # invoice, overdue"
                        other="on # invoices, all overdue"
                      />
                    ) : (
                      <Plural
                        value={summary.openInvoices.length}
                        one="on # invoice"
                        other="on # invoices"
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

          {summary.openInvoices.length > 0 && (
            <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
              <h3 style={{ margin: 0, fontSize: 15, fontWeight: 600 }}>
                <Trans>Unpaid invoices</Trans>
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
                  {[...summary.openInvoices]
                    .sort((a, b) => b.daysOverdue - a.daysOverdue)
                    .map((inv) => {
                      const due = inv.dueDate ? formatDate(inv.dueDate) : "";
                      const foreign =
                        organization?.currency &&
                        inv.currency &&
                        inv.currency !== organization.currency;
                      return (
                        <tr key={inv.id}>
                          <td style={td(token, "left")}>
                            <Link to={`/invoices/${inv.id}`}>{inv.number}</Link>
                            {due && (
                              <div style={{ ...secondary, fontSize: 12 }}>
                                <Trans>due {due}</Trans>
                              </div>
                            )}
                          </td>
                          <td
                            style={{
                              ...td(token, "right"),
                              ...(inv.daysOverdue > 90
                                ? { color: lateTextColor(dark), fontWeight: 500 }
                                : {}),
                            }}
                          >
                            {inv.daysOverdue > 0 ? t`${inv.daysOverdue} d` : t`Not due`}
                          </td>
                          <td style={td(token, "right")}>
                            {money(inv.total)}
                            {foreign && (
                              <div style={{ fontSize: 12, ...secondary }}>
                                {formatMoneyUnits(
                                  inv.foreignTotal / 100,
                                  inv.currency,
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

          {summary.invoiceCount > 0 && (
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
                {t`${summary.invoiceCount} purchases for ${money(summary.billedTotal)}, of which ${money(summary.paidTotal)} paid in ${summary.paymentCount} payments.`}
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
        {contact.map(({ label, value }) => (
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
