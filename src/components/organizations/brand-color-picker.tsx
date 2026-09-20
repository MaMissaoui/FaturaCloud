import { CheckOutlined, StopOutlined } from "@ant-design/icons";
import { Tooltip, theme } from "antd";
import { t } from "@lingui/core/macro";

// Curated, contrast-checked presets — Phase 1 is deliberately a fixed swatch
// set, not a free-form color picker, so a chosen color can't land on a hue
// antd's theme algorithm can't derive a readable button/text ramp from. Must
// stay in sync with brandColorPalette in db/organization.go, the
// server-side source of truth: the API rejects any value not on that list.
//
// Labels are functions, not module-scope strings, so the `t` call happens at
// render time and re-reads the active locale — same reasoning as
// invoiceStateLabel in src/types/invoice.ts. The hex values stay untranslated.
const SWATCHES: { label: () => string; value: string }[] = [
  { label: () => t`Indigo`, value: "#2E4CAE" },
  { label: () => t`Teal`, value: "#0F7C74" },
  { label: () => t`Emerald`, value: "#1D8A5D" },
  { label: () => t`Navy`, value: "#16325C" },
  { label: () => t`Violet`, value: "#6D3FBF" },
  { label: () => t`Terracotta`, value: "#B5502E" },
  { label: () => t`Slate`, value: "#475569" },
];

interface BrandColorPickerProps {
  value?: string | null;
  onChange?: (value: string) => void;
}

export default function BrandColorPicker({ value, onChange }: BrandColorPickerProps) {
  const { token } = theme.useToken();
  const selected = value || "";

  const renderSwatch = (hex: string, label: string) => {
    const isSelected = selected === hex;
    return (
      <Tooltip title={label} key={hex || "default"}>
        <button
          type="button"
          aria-label={label}
          aria-pressed={isSelected}
          onClick={() => onChange?.(hex)}
          style={{
            width: 28,
            height: 28,
            borderRadius: "50%",
            border: "none",
            padding: 0,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            cursor: "pointer",
            background: hex || token.colorBgContainer,
            boxShadow: isSelected
              ? `0 0 0 2px ${token.colorBgContainer}, 0 0 0 4px ${token.colorPrimary}`
              : `0 0 0 1px ${token.colorBorder}`,
          }}
        >
          {isSelected ? (
            <CheckOutlined style={{ fontSize: 13, color: hex ? "#fff" : token.colorText }} />
          ) : (
            hex === "" && <StopOutlined style={{ fontSize: 13, color: token.colorTextTertiary }} />
          )}
        </button>
      </Tooltip>
    );
  };

  return (
    <div
      role="group"
      aria-label={t`Brand color`}
      style={{ display: "flex", gap: 10, flexWrap: "wrap" }}
    >
      {renderSwatch("", t`Default`)}
      {SWATCHES.map((s) => renderSwatch(s.value, s.label()))}
    </div>
  );
}
