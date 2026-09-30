import type { CSSProperties, ReactNode } from "react";
import { theme } from "antd";

import type { LoanTone } from "src/components/cash-book/loan-register-model";

// A receipt-book slip: the new layout's one visual device. Its colour says
// where the money stands, like the copies of a carbon receipt book: the
// warning tint for a balance still owed, the error tint for a loan with no
// payment for too long, a plain bordered slip once settled. The dashed top
// edge is the tear line. Theme tokens, so it follows dark mode and the
// organization's colours.
export const useSlipColors = (tone: LoanTone) => {
  const { token } = theme.useToken();
  switch (tone) {
    case "open":
      return { background: token.colorWarningBg, border: token.colorWarningBorder };
    case "stale":
      return { background: token.colorErrorBg, border: token.colorErrorBorder };
    case "settled":
      return { background: token.colorBgContainer, border: token.colorBorder };
  }
};

const Slip = ({
  tone,
  children,
  style,
}: {
  tone: LoanTone;
  children: ReactNode;
  style?: CSSProperties;
}) => {
  const { token } = theme.useToken();
  const { background, border } = useSlipColors(tone);
  return (
    <div
      style={{
        background,
        border: `1px solid ${border}`,
        borderTop: `2px dashed ${token.colorTextQuaternary}`,
        borderRadius: token.borderRadiusSM,
        padding: "12px 14px",
        minWidth: 0,
        ...style,
      }}
    >
      {children}
    </div>
  );
};

export default Slip;
