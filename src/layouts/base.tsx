import { useState } from "react";
import { Outlet, Link, useLocation, useNavigate } from "react-router";
import { Button, Divider, Layout, Menu, Select, Space, Row, Col, message, theme } from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import {
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  FileTextOutlined,
  TeamOutlined,
  SettingOutlined,
  FileOutlined,
  CalculatorOutlined,
  PlusOutlined,
  DatabaseOutlined,
  AppstoreOutlined,
  InboxOutlined,
  ShoppingOutlined,
  SendOutlined,
  CommentOutlined,
  LogoutOutlined,
  UserOutlined,
  ApartmentOutlined,
} from "@ant-design/icons";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { match } from "path-to-regexp";
import compact from "lodash/compact";
import isEmpty from "lodash/isEmpty";
import join from "lodash/join";
import map from "lodash/map";
import take from "lodash/take";
import toUpper from "lodash/toUpper";

import { siderAtom, localeAtom } from "src/atoms/generic";
import { organizationsAtom, organizationIdAtom, organizationAtom } from "src/atoms/organization";
import { currentUserAtom, isAdminAtom } from "src/atoms/auth";
import { Logout } from "src/api";
import FeedbackModal from "src/components/feedback-modal";
import { dynamicActivate, locales } from "src/utils/lingui";

const { Content, Header, Sider } = Layout;
const { Option } = Select;

export default function BaseLayout() {
  const { i18n } = useLingui();
  const location = useLocation();
  const navigate = useNavigate();

  const [, contextHolder] = message.useMessage();
  const {
    token: { colorBgContainer, borderRadiusLG },
  } = theme.useToken();

  // Feedback modal state
  const [feedbackModalOpen, setFeedbackModalOpen] = useState(false);

  // Organizations
  const organizations = useAtomValue(organizationsAtom);

  // Organization
  const organizationId = useAtomValue(organizationIdAtom);
  const setOrganizationId = useSetAtom(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);

  // Locale
  const setLocale = useSetAtom(localeAtom);

  // Sider
  const [siderCollapsed, setSiderCollapsed] = useAtom(siderAtom);

  // Auth
  const currentUser = useAtomValue(currentUserAtom);
  const isAdmin = useAtomValue(isAdminAtom);
  const handleLogout = () => {
    Logout();
    navigate("/login");
  };

  // If no organizationId is set, redirect to index page
  if (!organizationId) {
    navigate("/");
    return null;
  }

  // If organizationId exists but organization is null (not found), clear the invalid ID and redirect
  if (organizationId && organization === null) {
    setOrganizationId(null);
    navigate("/");
    return null;
  }

  // Active menu item detection
  let openKeys: string[] = [];
  let selectedKeys: string[] = [];
  const matchFn = match(`/*path`, { decode: decodeURIComponent });
  const matchResult = matchFn(location.pathname);
  if (matchResult && matchResult.params.path) {
    const pathString = Array.isArray(matchResult.params.path)
      ? matchResult.params.path.join("/")
      : matchResult.params.path;
    const pathArray = pathString.split("/");
    openKeys = pathArray[0] === "settings" ? ["settings"] : [];
    selectedKeys = [join(take(compact(pathArray), 2), ".")];
  }

  if (!organization) {
    return (
      <div
        style={{ display: "flex", justifyContent: "center", alignItems: "center", height: "100vh" }}
      >
        Loading...
      </div>
    );
  }

  return (
    <Layout hasSider style={{ minHeight: "100vh", width: "100%" }}>
      <Sider
        trigger={null}
        collapsible
        collapsed={siderCollapsed}
        style={{ overflow: "auto", height: "100vh", position: "fixed", left: 0, top: 0, bottom: 0 }}
      >
        <div className="logo" style={{ margin: siderCollapsed ? "8px 8px 16px" : "8px 16px 16px", textAlign: "center" }}>
          <Link to="/invoices">
            <img src={siderCollapsed ? "/logo-minimal.png" : "/logo-light.png"} alt="Fatura" style={{ width: siderCollapsed ? "100%" : "45%", height: "auto" }} />
          </Link>
        </div>
        <Menu
          theme="dark"
          mode="inline"
          defaultOpenKeys={openKeys}
          defaultSelectedKeys={selectedKeys}
          items={[
            {
              type: "group" as const,
              label: <Trans>Sales</Trans>,
              key: "group-sales",
              children: [
                {
                  icon: <FileTextOutlined />,
                  label: (
                    <Link to="/invoices">
                      <Trans>Invoices</Trans>
                    </Link>
                  ),
                  key: "invoices",
                },
                {
                  icon: <SendOutlined />,
                  label: (
                    <Link to="/deliveries">
                      <Trans>Outbound Deliveries</Trans>
                    </Link>
                  ),
                  key: "deliveries",
                },
                {
                  icon: <ShoppingOutlined />,
                  label: (
                    <Link to="/orders">
                      <Trans>Orders</Trans>
                    </Link>
                  ),
                  key: "orders",
                },
              ],
            },
            {
              type: "group" as const,
              label: <Trans>Inventory</Trans>,
              key: "group-inventory",
              children: [
                {
                  icon: <InboxOutlined />,
                  label: (
                    <Link to="/inventory">
                      <Trans>Inventory</Trans>
                    </Link>
                  ),
                  key: "inventory",
                },
              ],
            },
            {
              type: "group" as const,
              label: <Trans>Master Data</Trans>,
              key: "group-masterdata",
              children: [
                {
                  icon: <TeamOutlined />,
                  label: (
                    <Link to="/clients">
                      <Trans>Clients</Trans>
                    </Link>
                  ),
                  key: "clients",
                },
                {
                  icon: <AppstoreOutlined />,
                  label: (
                    <Link to="/products">
                      <Trans>Products</Trans>
                    </Link>
                  ),
                  key: "products",
                },
                {
                  icon: <ApartmentOutlined />,
                  label: (
                    <Link to="/organizations">
                      <Trans>Organizations</Trans>
                    </Link>
                  ),
                  key: "organizations",
                },
              ],
            },
            {
              icon: <SettingOutlined />,
              label: <Trans>Settings</Trans>,
              key: "settings",
              children: [
                {
                  icon: <FileOutlined />,
                  label: (
                    <Link to="/settings/invoice">
                      <Trans>Invoice</Trans>
                    </Link>
                  ),
                  key: "settings.invoice",
                },
                {
                  icon: <CalculatorOutlined />,
                  label: (
                    <Link to="/settings/tax-rates">
                      <Trans>Tax rates</Trans>
                    </Link>
                  ),
                  key: "settings.tax-rates",
                },
                {
                  icon: <DatabaseOutlined />,
                  label: (
                    <Link to="/settings/backup">
                      <Trans>Backup</Trans>
                    </Link>
                  ),
                  key: "settings.backup",
                },
                ...(isAdmin ? [{
                  icon: <UserOutlined />,
                  label: (
                    <Link to="/settings/users">
                      <Trans>Users</Trans>
                    </Link>
                  ),
                  key: "settings.users",
                }] : []),
              ],
            },
          ]}
        />
        <div style={{ position: "absolute", bottom: 0, width: "100%", padding: "16px" }}>
          <Button
            type="text"
            icon={<CommentOutlined />}
            onClick={(e) => {
              // Blur the button to remove focus after click
              e.currentTarget.blur();
              setFeedbackModalOpen(true);
            }}
            style={{
              width: "100%",
              color: "rgba(255, 255, 255, 0.65)",
              textAlign: "left",
            }}
          >
            {!siderCollapsed && <Trans>Feedback</Trans>}
          </Button>
        </div>
      </Sider>
      <Layout
        style={{ width: "100%", marginLeft: siderCollapsed ? 80 : 200, transition: "all 0.2s" }}
      >
        <Header
          style={{
            position: "sticky",
            top: 0,
            zIndex: 1,
            padding: 0,
            background: colorBgContainer,
            borderTop: "1px solid #f0f0f0",
          }}
        >
          <Row>
            <Col flex="auto">
              <Button
                type="text"
                icon={siderCollapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
                onClick={() => setSiderCollapsed(!siderCollapsed)}
                style={{
                  fontSize: "16px",
                  width: 64,
                  height: 64,
                }}
              />
            </Col>
            <Col>
              <Space>
                {!isEmpty(organizations) && (
                  <Select
                    showSearch={organizations.length > 5 ? true : false}
                    filterOption={(input, option) => {
                      if (!option) return false;
                      // Get the organization name from the option
                      const organizations = option as any;
                      const orgName = organizations?.children;
                      return orgName
                        ? String(orgName).toLowerCase().includes(input.toLowerCase())
                        : false;
                    }}
                    style={{ width: 200 }}
                    defaultValue={organization.id}
                    onSelect={(value) => {
                      setOrganizationId(value);
                      window.location.reload();
                    }}
                    popupRender={(menu) => (
                      <>
                        {menu}
                        <Divider style={{ margin: "8px 0" }} />
                        <Menu
                          onClick={() => navigate("/organizations/new")}
                          items={[
                            {
                              key: "new-org",
                              icon: <PlusOutlined />,
                              label: <Trans>New organization</Trans>,
                              style: { height: 32, lineHeight: "32px" },
                            },
                          ]}
                        />
                      </>
                    )}
                  >
                    {map(organizations, (organization: any) => (
                      <Option key={organization.id} value={organization.id}>
                        {organization.name}
                      </Option>
                    ))}
                  </Select>
                )}
                {currentUser && (
                  <Space size={4} style={{ marginRight: 4 }}>
                    <UserOutlined />
                    <span style={{ fontSize: 13 }}>{currentUser.displayName || currentUser.email}</span>
                    <Button
                      type="text"
                      icon={<LogoutOutlined />}
                      size="small"
                      onClick={handleLogout}
                      title={t`Sign out`}
                    />
                  </Space>
                )}
                <Select
                  variant="borderless"
                  style={{ marginRight: 24 }}
                  popupMatchSelectWidth={false}
                  onSelect={(value) => {
                    setLocale(value);
                    dynamicActivate(value);
                  }}
                  value={i18n.locale}
                  optionLabelProp="label"
                >
                  {map(locales, (locale) => {
                    const languageMap: Record<string, string> = {
                      en: "🇺🇸 English (US)",
                      "en-GB": "🇬🇧 English (UK)",
                      de: "🇩🇪 German",
                      et: "🇪🇪 Estonian",
                      fi: "🇫🇮 Finnish",
                      fr: "🇫🇷 French",
                      el: "🇬🇷 Greek",
                      nl: "🇳🇱 Dutch",
                      pt: "🇵🇹 Portuguese",
                      sv: "🇸🇪 Swedish",
                      uk: "🇺🇦 Ukrainian",
                    };
                    const languageText = languageMap[locale] || toUpper(locale);
                    const flagOnly = languageText.split(" ")[0];
                    return (
                      <Option value={locale} key={locale} label={flagOnly}>
                        {languageText}
                      </Option>
                    );
                  })}
                </Select>
              </Space>
            </Col>
          </Row>
        </Header>
        <Content
          style={{
            margin: "24px 16px",
            padding: 24,
            minHeight: 280,
            background: colorBgContainer,
            borderRadius: borderRadiusLG,
          }}
        >
          <Outlet />
          {/*<SignOut />*/}
        </Content>
        <div id="footer" />
      </Layout>
      <FeedbackModal open={feedbackModalOpen} onClose={() => setFeedbackModalOpen(false)} />
      {contextHolder}
    </Layout>
  );
}
