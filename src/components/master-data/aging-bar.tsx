import { theme } from "antd";

import type { OutstandingBucket } from "src/api";
import { AGING_BUCKETS, agingBucketLabel } from "./aging";

// AgingBar is the compact form of the Dashboard's owed bar, for a summary
// panel: a 12px bar split by how late the money is, then only the buckets
// that hold something, with their amounts.
export default function AgingBar({
  amounts,
  money,
}: {
  amounts: Record<OutstandingBucket, number>;
  money: (cents: number) => string;
}) {
  const { token } = theme.useToken();
  const shown = AGING_BUCKETS.filter((b) => amounts[b.key] > 0);
  if (shown.length === 0) return null;
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
      <div
        aria-hidden="true"
        style={{ display: "flex", height: 12, borderRadius: 3, overflow: "hidden", gap: 2 }}
      >
        {shown.map((b) => (
          <div
            key={b.key}
            style={{ flex: `${amounts[b.key]} 1 0`, minWidth: 6, background: b.color }}
          />
        ))}
      </div>
      <dl
        style={{
          margin: 0,
          display: "grid",
          gridTemplateColumns: "1fr auto",
          gap: "4px 16px",
          fontSize: 13,
          color: token.colorTextSecondary,
        }}
      >
        {shown.map((b) => (
          <div key={b.key} style={{ display: "contents" }}>
            <dt style={{ display: "flex", alignItems: "center", gap: 8 }}>
              <span
                aria-hidden="true"
                style={{
                  width: 10,
                  height: 10,
                  borderRadius: 2,
                  background: b.color,
                  flex: "none",
                }}
              />
              {agingBucketLabel(b.key)}
            </dt>
            <dd style={{ margin: 0, textAlign: "right", fontVariantNumeric: "tabular-nums" }}>
              {money(amounts[b.key])}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  );
}
