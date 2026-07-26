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
}

interface PageHeaderProps {
  icon: ReactNode;
  title: ReactNode;
  search?: PageHeaderSearchProps;
  extra?: ReactNode;
  actions?: ReactNode;
  style?: CSSProperties;
}

// Shared header for list pages: icon + title on the left, search + actions on
// the right. Was hand-rolled byte-for-byte-similar in every list/settings page.
const PageHeader = ({ icon, title, search, extra, actions, style }: PageHeaderProps) => (
  <Row style={style}>
    <Col span={12}>
      <Title level={3} style={{ margin: 0 }}>
        <span style={{ marginRight: 8 }}>{icon}</span>
        {title}
      </Title>
    </Col>
    <Col span={12} style={{ display: "flex", justifyContent: "flex-end" }}>
      <Space style={{ alignItems: "start" }}>
        {extra}
        {search && (
          <Input.Search
            placeholder={search.placeholder}
            value={search.value}
            onChange={(e) => search.onChange(e.target.value)}
            onSearch={search.onSearch}
            allowClear={search.allowClear}
            onClear={search.onClear}
          />
        )}
        {actions}
      </Space>
    </Col>
  </Row>
);

export default PageHeader;
