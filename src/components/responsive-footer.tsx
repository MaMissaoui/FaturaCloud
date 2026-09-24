import type { CSSProperties, ReactNode } from "react";
import { createPortal } from "react-dom";
import { Layout, theme } from "antd";

const { Footer } = Layout;

interface ResponsiveFooterProps {
  children: ReactNode;
  style?: CSSProperties;
}

const ResponsiveFooter = ({ children, style }: ResponsiveFooterProps) => {
  const { token } = theme.useToken();
  const footerEl = document.getElementById("footer");

  const footerContent = (
    // className hooks the phone-width wrap rule in src/styles/base.scss.
    <Footer
      className="responsive-footer"
      style={{
        position: "sticky",
        bottom: 0,
        zIndex: 1,
        padding: "0 16px",
        background: token.colorBgContainer,
        borderTop: `1px solid ${token.colorBorderSecondary}`,
        ...style,
      }}
    >
      {children}
    </Footer>
  );

  if (footerEl) {
    return createPortal(footerContent, footerEl);
  }

  return footerContent;
};

export default ResponsiveFooter;
