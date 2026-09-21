import type { CSSProperties, ReactNode } from "react";
import { Col, Input, Row, Space, Typography } from "antd";

const { Title } = Typography;

export interface PageHeaderSearchProps {
  value?: string;
  onChange: (value: string) => void;
  onSearch?: (value: string) => void;
  placeholder: string;
  allowClear?: boolean;
  onClear?: () => void;
  autoFocus?: boolean;
  // Extra props for the search input itself — used by the Cash Book's
  // combobox (role/aria-* + arrow-key handling), which drives the result
  // listbox from the field.
  inputProps?: Record<string, unknown>;
}

interface PageHeaderProps {
  icon: ReactNode;
  title: ReactNode;
  search?: PageHeaderSearchProps;
  extra?: ReactNode;
  // A filter bar rendered on its own full-width row under the title — the
  // place for a list's date/status/party filters, which don't fit alongside
  // the title, search and action on one line.
  filters?: ReactNode;
  actions?: ReactNode;
  style?: CSSProperties;
}

// Shared header for list pages: icon + title on the left, search + actions on
// the right, and an optional full-width filter row below. Was hand-rolled
// byte-for-byte-similar in every list/settings page.
const PageHeader = ({ icon, title, search, extra, filters, actions, style }: PageHeaderProps) => (
  <>
    <Row gutter={[16, 12]} style={style}>
      <Col xs={24} md={12}>
        <Title level={3} style={{ margin: 0 }}>
          <span style={{ marginRight: 8 }}>{icon}</span>
          {title}
        </Title>
      </Col>
      <Col
        xs={24}
        md={12}
        style={{ display: "flex", justifyContent: "flex-end", flexWrap: "wrap" }}
      >
        <Space wrap size="small" style={{ alignItems: "start" }}>
          {extra}
          {search && (
            <Input.Search
              placeholder={search.placeholder}
              aria-label={search.placeholder}
              value={search.value}
              onChange={(e) => search.onChange(e.target.value)}
              onSearch={search.onSearch}
              allowClear={search.allowClear}
              onClear={search.onClear}
              autoFocus={search.autoFocus}
              {...(search.inputProps as object)}
            />
          )}
          {actions}
        </Space>
      </Col>
    </Row>
    {filters && (
      <Row style={{ marginTop: 12 }}>
        <Col span={24}>{filters}</Col>
      </Row>
    )}
  </>
);

export default PageHeader;
