import type { ReactNode } from "react";
import { theme } from "antd";
import { Trans } from "@lingui/react/macro";

import type { DashboardCashRegister } from "src/api";

interface Props {
  till: DashboardCashRegister;
  money: (cents: number) => string;
  // The till's "yesterday" date (YYYY-MM-DD) as a weekday and date.
  dayLabel: (date: string) => string;
  cashBookLink?: ReactNode;
}

const signed = (sign: string, money: (cents: number) => string, cents: number) =>
  (cents > 0 ? sign : "") + money(cents);

// The till: what's in it now, today's movements, and how yesterday closed.
const TillPanel = ({ till, money, dayLabel, cashBookLink }: Props) => {
  const { token } = theme.useToken();
  const row = (label: ReactNode, value: string) => (
    <>
      <dt>{label}</dt>
      <dd style={{ margin: 0, textAlign: "right", fontWeight: 500, whiteSpace: "nowrap" }}>
        {value}
      </dd>
    </>
  );
  const group = (title: ReactNode, rows: ReactNode) => (
    <div style={{ paddingTop: 14, borderTop: `1px solid ${token.colorPrimaryBorder}` }}>
      <p style={{ margin: "0 0 8px", fontSize: 13, color: token.colorTextSecondary }}>{title}</p>
      <dl style={{ margin: 0, display: "grid", gridTemplateColumns: "1fr auto", gap: "8px 16px" }}>
        {rows}
      </dl>
    </div>
  );

  return (
    <section
      aria-labelledby="dashboard-till"
      style={{
        flex: "1 1 300px",
        minWidth: 0,
        background: token.colorPrimaryBg,
        borderRadius: token.borderRadiusLG,
        padding: "clamp(16px, 3vw, 28px)",
        display: "flex",
        flexDirection: "column",
        gap: 16,
      }}
    >
      <h2
        id="dashboard-till"
        style={{ margin: 0, fontSize: 16, fontWeight: 500, color: token.colorTextSecondary }}
      >
        <Trans>Cash register</Trans>
      </h2>
      <div>
        <p style={{ margin: "0 0 4px", color: token.colorTextSecondary }}>
          <Trans>In the till now</Trans>
        </p>
        <p
          style={{
            margin: 0,
            fontSize: "clamp(28px, 3.5vw, 40px)",
            lineHeight: 1.05,
            fontWeight: 600,
            overflowWrap: "anywhere",
          }}
        >
          {money(till.today.closing)}
        </p>
      </div>
      {group(
        <Trans>Today</Trans>,
        <>
          {row(<Trans>Opening balance</Trans>, money(till.today.opening))}
          {row(<Trans>In</Trans>, signed("+", money, till.today.in))}
          {row(<Trans>Out</Trans>, signed("−", money, till.today.out))}
        </>,
      )}
      {group(
        dayLabel(till.yesterday.date),
        <>
          {row(<Trans>In</Trans>, signed("+", money, till.yesterday.in))}
          {row(<Trans>Out</Trans>, signed("−", money, till.yesterday.out))}
          {row(<Trans>Closing balance</Trans>, money(till.yesterday.closing))}
        </>,
      )}
      {cashBookLink && <div style={{ marginTop: "auto" }}>{cashBookLink}</div>}
    </section>
  );
};

export default TillPanel;
