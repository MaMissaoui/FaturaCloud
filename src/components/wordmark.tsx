import { theme } from "antd";

// Text-based "FaturaCloud" wordmark. Rendered as live text (not an image) so it
// stays crisp at any size and adapts to the active light/dark theme:
// "Fatura" uses the theme text color, "Cloud" uses the brand primary blue.
export default function Wordmark({ fontSize = 18 }: { fontSize?: number }) {
  const {
    token: { colorText, colorPrimary },
  } = theme.useToken();

  return (
    <span
      style={{
        fontSize,
        fontWeight: 700,
        lineHeight: 1,
        letterSpacing: 0.2,
        whiteSpace: "nowrap",
        userSelect: "none",
      }}
    >
      <span style={{ color: colorText }}>Fatura</span>
      <span style={{ color: colorPrimary, fontWeight: 500 }}>Cloud</span>
    </span>
  );
}
