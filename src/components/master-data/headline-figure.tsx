import type { ReactNode } from "react";
import { theme } from "antd";

// HeadlineFigure is the one number a master-data screen leads with ("What
// your clients owe you"), in the Dashboard's idiom: a secondary label, a
// large figure and a short note after it.
export default function HeadlineFigure({
  label,
  value,
  note,
  size = 30,
}: {
  label: ReactNode;
  value: ReactNode;
  note?: ReactNode;
  size?: number;
}) {
  const { token } = theme.useToken();
  return (
    <div>
      <p style={{ margin: "0 0 4px", color: token.colorTextSecondary }}>{label}</p>
      <p
        style={{
          margin: 0,
          display: "flex",
          flexWrap: "wrap",
          alignItems: "baseline",
          gap: "4px 12px",
        }}
      >
        <span
          style={{
            fontSize: size,
            lineHeight: 1.1,
            fontWeight: 600,
            fontVariantNumeric: "tabular-nums",
          }}
        >
          {value}
        </span>
        {note && <span style={{ color: token.colorTextSecondary }}>{note}</span>}
      </p>
    </div>
  );
}
