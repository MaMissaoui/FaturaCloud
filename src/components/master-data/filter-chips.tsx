import type { ReactNode } from "react";
import { theme } from "antd";

export interface FilterChip<K extends string> {
  key: K;
  label: ReactNode;
  count?: number;
}

// FilterChips is a single-select row of toggle buttons with counts, for the
// few slices of a master-data list that matter ("Owe money", "Businesses").
// Real buttons with aria-pressed, so they are reachable and announced.
export default function FilterChips<K extends string>({
  chips,
  value,
  onChange,
  ariaLabel,
}: {
  chips: FilterChip<K>[];
  value: K;
  onChange: (key: K) => void;
  ariaLabel: string;
}) {
  const { token } = theme.useToken();
  return (
    <div role="group" aria-label={ariaLabel} style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
      {chips.map((chip) => {
        const on = chip.key === value;
        return (
          <button
            key={chip.key}
            type="button"
            aria-pressed={on}
            onClick={() => onChange(chip.key)}
            style={{
              height: 32,
              padding: "0 12px",
              borderRadius: 16,
              cursor: "pointer",
              font: "inherit",
              border: `1px solid ${on ? token.colorPrimary : token.colorBorder}`,
              background: on ? token.colorPrimaryBg : token.colorBgContainer,
              color: on ? token.colorPrimaryText : token.colorText,
              whiteSpace: "nowrap",
            }}
          >
            {chip.label}
            {chip.count !== undefined && (
              <span
                style={{
                  marginLeft: 6,
                  color: on ? token.colorPrimaryText : token.colorTextSecondary,
                  fontVariantNumeric: "tabular-nums",
                }}
              >
                {chip.count}
              </span>
            )}
          </button>
        );
      })}
    </div>
  );
}
