import { useState } from "react";
import type { ReactNode } from "react";
import { Table, theme, Typography } from "antd";
import { Trans, Plural } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";

import type {
  LoanFollowUp,
  OutstandingBucket,
  OutstandingInvoiceSummary,
  OutstandingSummary,
} from "src/api";

// The aging ramp: one hue, darker with age, so the buckets differ in
// lightness and not only in colour. "Not due yet" sits outside the ramp in a
// neutral blue-grey: it isn't late.
const BUCKETS: { key: OutstandingBucket; color: string }[] = [
  { key: "current", color: "#8FA3BF" },
  { key: "days1To30", color: "#F3C29B" },
  { key: "days31To60", color: "#E08A4E" },
  { key: "days61To90", color: "#B9531A" },
  { key: "days90Plus", color: "#6E2E0A" },
];

const bucketLabel = (key: OutstandingBucket) => {
  switch (key) {
    case "current":
      return t`Not due yet`;
    case "days1To30":
      return t`1 to 30 days late`;
    case "days31To60":
      return t`31 to 60 days late`;
    case "days61To90":
      return t`61 to 90 days late`;
    case "days90Plus":
      return t`More than 90 days late`;
  }
};

interface Props {
  outstanding: OutstandingSummary;
  followUp: LoanFollowUp;
  money: (cents: number) => string;
  // Row click/keyboard props for an invoice, empty when the role can't open it.
  invoiceRow: (invoice: OutstandingInvoiceSummary) => object;
  reportLink?: ReactNode;
  cashBookLink?: ReactNode;
}

// "What your clients owe you": the total, one bar split by how late it is,
// the invoices of the bucket picked under it (the oldest non-empty one by
// default), and the customers who have stopped paying.
const OwedPanel = ({
  outstanding,
  followUp,
  money,
  invoiceRow,
  reportLink,
  cashBookLink,
}: Props) => {
  const { token } = theme.useToken();
  const [picked, setPicked] = useState<OutstandingBucket | null>(null);

  const amounts: Record<OutstandingBucket, number> = {
    current: outstanding.current,
    days1To30: outstanding.days1To30,
    days31To60: outstanding.days31To60,
    days61To90: outstanding.days61To90,
    days90Plus: outstanding.days90Plus,
  };
  const counts = outstanding.invoices.reduce(
    (acc, inv) => ({ ...acc, [inv.bucket]: (acc[inv.bucket] ?? 0) + 1 }),
    {} as Partial<Record<OutstandingBucket, number>>,
  );
  const shown = BUCKETS.filter((b) => amounts[b.key] > 0);
  const oldest = [...shown].reverse()[0]?.key ?? null;
  const selected = picked && amounts[picked] > 0 ? picked : oldest;
  const invoices = outstanding.invoices.filter((inv) => inv.bucket === selected);
  const count = outstanding.invoices.length;
  const allLate = outstanding.current === 0;
  const stale = followUp.customers.filter((c) => c.stale);
  const approaching = followUp.customers.filter((c) => !c.stale);
  const staleAfter = followUp.staleAfterDays;

  return (
    <section
      aria-labelledby="dashboard-owed"
      style={{
        flex: "2 1 560px",
        minWidth: 0,
        background: token.colorBgContainer,
        borderRadius: token.borderRadiusLG,
        padding: "clamp(16px, 3vw, 32px)",
        display: "flex",
        flexDirection: "column",
        gap: 20,
      }}
    >
      <div>
        <h2
          id="dashboard-owed"
          style={{
            margin: "0 0 6px",
            fontSize: 16,
            fontWeight: 500,
            color: token.colorTextSecondary,
          }}
        >
          <Trans>What your clients owe you</Trans>
        </h2>
        <p
          style={{
            margin: 0,
            display: "flex",
            flexWrap: "wrap",
            alignItems: "baseline",
            gap: "4px 16px",
          }}
        >
          <span
            style={{
              fontSize: "clamp(32px, 4.5vw, 52px)",
              lineHeight: 1.05,
              fontWeight: 600,
              letterSpacing: "-0.02em",
              overflowWrap: "anywhere",
            }}
          >
            {money(outstanding.total)}
          </span>
          {count > 0 && (
            <Typography.Text type="secondary" style={{ fontSize: 15 }}>
              {allLate ? (
                <Plural
                  value={count}
                  one="on # unpaid invoice, past its due date"
                  other="on # unpaid invoices, all past their due date"
                />
              ) : (
                <Plural value={count} one="on # unpaid invoice" other="on # unpaid invoices" />
              )}
            </Typography.Text>
          )}
        </p>
      </div>

      {count === 0 ? (
        <Typography.Text type="secondary">
          <Trans>No outstanding invoices</Trans>
        </Typography.Text>
      ) : (
        <>
          <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
            <div
              aria-hidden="true"
              style={{ display: "flex", height: 18, gap: 2, borderRadius: 4, overflow: "hidden" }}
            >
              {shown.map((b) => (
                <div
                  key={b.key}
                  style={{
                    flex: `${amounts[b.key]} 1 0`,
                    minWidth: 6,
                    background: b.color,
                    opacity: b.key === selected ? 1 : 0.45,
                  }}
                />
              ))}
            </div>
            <div
              role="group"
              aria-label={t`How late`}
              style={{
                display: "grid",
                gridTemplateColumns: "repeat(auto-fit, minmax(150px, 1fr))",
                gap: 8,
              }}
            >
              {shown.map((b) => {
                const on = b.key === selected;
                return (
                  <button
                    key={b.key}
                    type="button"
                    className="dashboard-bucket"
                    aria-pressed={on}
                    onClick={() => setPicked(b.key)}
                    style={{
                      textAlign: "left",
                      cursor: "pointer",
                      padding: "10px 12px",
                      minHeight: 44,
                      borderRadius: token.borderRadius,
                      color: token.colorText,
                      background: on ? token.colorFillTertiary : token.colorBgContainer,
                      border: `1px solid ${on ? token.colorText : token.colorBorderSecondary}`,
                    }}
                  >
                    <span
                      style={{
                        display: "flex",
                        alignItems: "center",
                        gap: 8,
                        fontSize: 13,
                        color: token.colorTextSecondary,
                      }}
                    >
                      <span
                        style={{
                          width: 10,
                          height: 10,
                          borderRadius: 2,
                          background: b.color,
                          flex: "none",
                        }}
                      />
                      {bucketLabel(b.key)}
                    </span>
                    <span
                      style={{
                        display: "block",
                        fontSize: 17,
                        fontWeight: 600,
                        marginTop: 4,
                        overflowWrap: "anywhere",
                      }}
                    >
                      {money(amounts[b.key])}
                    </span>
                    <span
                      style={{ display: "block", fontSize: 13, color: token.colorTextSecondary }}
                    >
                      <Plural value={counts[b.key] ?? 0} one="# invoice" other="# invoices" />
                    </span>
                  </button>
                );
              })}
            </div>
          </div>

          {selected && (
            <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
              <div
                style={{
                  display: "flex",
                  flexWrap: "wrap",
                  justifyContent: "space-between",
                  alignItems: "baseline",
                  gap: 8,
                }}
              >
                <h3 style={{ margin: 0, fontSize: 15, fontWeight: 600 }}>
                  {bucketLabel(selected)}
                </h3>
                {reportLink}
              </div>
              <Table<OutstandingInvoiceSummary>
                dataSource={invoices}
                rowKey="id"
                size="small"
                pagination={{ pageSize: 5, hideOnSinglePage: true }}
                onRow={invoiceRow}
                columns={[
                  { title: t`Invoice`, dataIndex: "number", key: "number" },
                  { title: t`Client`, dataIndex: "clientName", key: "clientName" },
                  {
                    title: t`Days overdue`,
                    dataIndex: "daysOverdue",
                    key: "daysOverdue",
                    align: "right",
                    render: (days: number) => (days > 0 ? days : "—"),
                  },
                  {
                    title: t`Still owed`,
                    key: "total",
                    align: "right",
                    render: (_, inv) => (
                      <span style={{ whiteSpace: "nowrap" }}>{money(inv.total)}</span>
                    ),
                  },
                ]}
              />
            </div>
          )}
        </>
      )}

      {followUp.customers.length > 0 && (
        <div
          style={{
            background: token.colorWarningBg,
            borderRadius: token.borderRadius,
            padding: "14px 16px",
            display: "flex",
            flexDirection: "column",
            gap: 10,
          }}
        >
          {stale.length > 0 && (
            <FollowUpList
              title={
                <Plural
                  value={followUp.staleCount}
                  one={`# client hasn't paid anything for more than ${staleAfter} days`}
                  other={`# clients haven't paid anything for more than ${staleAfter} days`}
                />
              }
              customers={stale}
              money={money}
            />
          )}
          {approaching.length > 0 && (
            <FollowUpList
              title={
                <Plural
                  value={followUp.approachingCount}
                  one={`# client is close to ${staleAfter} days without paying`}
                  other={`# clients are close to ${staleAfter} days without paying`}
                />
              }
              customers={approaching}
              money={money}
            />
          )}
          {cashBookLink}
        </div>
      )}
    </section>
  );
};

const FollowUpList = ({
  title,
  customers,
  money,
}: {
  title: ReactNode;
  customers: LoanFollowUp["customers"];
  money: (cents: number) => string;
}) => (
  <div>
    <p style={{ margin: "0 0 4px", fontWeight: 600 }}>{title}</p>
    <ul style={{ margin: 0, padding: 0, listStyle: "none" }}>
      {customers.map((c) => (
        <li
          key={c.clientId}
          style={{
            display: "flex",
            flexWrap: "wrap",
            justifyContent: "space-between",
            gap: "0 12px",
          }}
        >
          <span>
            {c.clientName}{" "}
            <Typography.Text type="secondary">
              <Plural value={c.idleDays} one="(# day)" other="(# days)" />
            </Typography.Text>
          </span>
          <span style={{ whiteSpace: "nowrap", fontWeight: 500 }}>{money(c.outstanding)}</span>
        </li>
      ))}
    </ul>
  </div>
);

export default OwedPanel;
