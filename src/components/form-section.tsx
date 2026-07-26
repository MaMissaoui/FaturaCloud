import { theme, Typography } from "antd";

// Section heading inside a drawer/modal form. Was copy-pasted byte-identically
// into the clients, vendors, products, tax-rates and stock-movement forms
// before being lifted here.
const Section = ({ children }: { children: React.ReactNode }) => {
  const { token } = theme.useToken();
  return (
    <Typography.Text
      strong
      style={{ color: token.colorPrimary, display: "block", marginBottom: 12, marginTop: 4 }}
    >
      {children}
    </Typography.Text>
  );
};

export default Section;
