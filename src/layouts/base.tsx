import { useEffect, useState } from "react";
import { Outlet, Link, useLocation, useNavigate } from "react-router";
import {
  Button,
  Divider,
  Dropdown,
  Layout,
  Menu,
  Select,
  Space,
  Row,
  Col,
  theme,
  Typography,
} from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import {
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  DashboardOutlined,
  WalletOutlined,
  FileTextOutlined,
  TeamOutlined,
  SolutionOutlined,
  ShoppingCartOutlined,
  ImportOutlined,
  AuditOutlined,
  SettingOutlined,
  FileOutlined,
  CalculatorOutlined,
  ScheduleOutlined,
  ColumnWidthOutlined,
  PlusOutlined,
  DatabaseOutlined,
  AppstoreOutlined,
  InboxOutlined,
  DeploymentUnitOutlined,
  ShoppingOutlined,
  ShopOutlined,
  FolderOutlined,
  SendOutlined,
  CommentOutlined,
  LogoutOutlined,
  UserOutlined,
  ApartmentOutlined,
  BuildOutlined,
  SunOutlined,
  MoonOutlined,
  GlobalOutlined,
  BankOutlined,
  BookOutlined,
  CalendarOutlined,
  UnorderedListOutlined,
  TableOutlined,
  LineChartOutlined,
  FundOutlined,
  ClockCircleOutlined,
  GoldOutlined,
  FieldTimeOutlined,
  ExportOutlined,
  BarChartOutlined,
  FileExcelOutlined,
  ContainerOutlined,
  OrderedListOutlined,
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

import { siderAtom, localeAtom, themeAtom } from "src/atoms/generic";
import {
  organizationsAtom,
  organizationsLoadedAtom,
  organizationIdAtom,
  organizationAtom,
  isOrgAdminOrAccountingAtom,
  isCashbookAtom,
  myOrgRoleAtom,
} from "src/atoms/organization";
import { currentUserAtom, isPlatformAdminAtom } from "src/atoms/auth";
import { GetVersion, Logout } from "src/api";
import { filterMenuForRole, isRouteAllowedForRole, roleHomePath } from "src/layouts/role-menu";
import FeedbackModal from "src/components/feedback-modal";
import Wordmark from "src/components/wordmark";
import { dynamicActivate, locales } from "src/utils/lingui";

const { Content, Header, Sider } = Layout;
const { Option } = Select;

// The focused per-role menu allow-list, URL guard, menu filter and role home
// live in src/layouts/role-menu.ts so they can be unit-tested; api/sections.go
// mirrors the same allow-list server-side.

export default function BaseLayout() {
  const { i18n } = useLingui();
  const location = useLocation();
  const navigate = useNavigate();

  const {
    token: { colorBgContainer, borderRadiusLG, colorBorderSecondary, colorPrimary },
  } = theme.useToken();

  // Feedback modal state
  const [feedbackModalOpen, setFeedbackModalOpen] = useState(false);

  // App version, shown next to the "Send feedback" icon in the header
  const [version, setVersion] = useState<string | null>(null);
  useEffect(() => {
    GetVersion()
      .then(setVersion)
      .catch(() => setVersion(null));
  }, []);

  // Below this width the sidebar becomes an overlay drawer instead of
  // pushing content — it's shown/hidden via mobileMenuOpen (not persisted,
  // unlike the desktop siderCollapsed preference below).
  const [isMobile, setIsMobile] = useState(
    () => typeof window !== "undefined" && window.innerWidth < 768,
  );
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  useEffect(() => {
    const mql = window.matchMedia("(max-width: 767px)");
    const onChange = () => setIsMobile(mql.matches);
    onChange();
    mql.addEventListener("change", onChange);
    return () => mql.removeEventListener("change", onChange);
  }, []);

  // Organizations
  const organizations = useAtomValue(organizationsAtom);
  const organizationsLoaded = useAtomValue(organizationsLoadedAtom);

  // Organization
  const organizationId = useAtomValue(organizationIdAtom);
  const setOrganizationId = useSetAtom(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);

  // Locale
  const setLocale = useSetAtom(localeAtom);

  // Sider
  const [siderCollapsed, setSiderCollapsed] = useAtom(siderAtom);

  // Color theme (light/dark)
  const [themeMode, setThemeMode] = useAtom(themeAtom);

  // Auth
  const currentUser = useAtomValue(currentUserAtom);
  const isPlatformAdmin = useAtomValue(isPlatformAdminAtom);
  const canAccessGLExport = useAtomValue(isOrgAdminOrAccountingAtom);
  const isCashbook = useAtomValue(isCashbookAtom);
  const orgRole = useAtomValue(myOrgRoleAtom);

  // The cashbook (counter/till) role gets a deliberately reduced view: only
  // Cash Book and Clients are reachable. Enforced here rather than only by
  // hiding menu entries, so a typed URL bounces back to the counter screen.
  // Purely a UI restriction — reads stay membership-level server-side, so
  // this is not an authorization boundary.
  useEffect(() => {
    if (!isCashbook) return;
    const allowed =
      location.pathname === "/cash-book" ||
      location.pathname.startsWith("/cash-book/") ||
      location.pathname === "/clients" ||
      location.pathname.startsWith("/clients/");
    if (!allowed) navigate("/cash-book", { replace: true });
  }, [isCashbook, location.pathname, navigate]);

  // The focused per-role views (general, sales, purchasing, accounting) hide
  // the sections outside their scope in the menu below and bounce a typed URL
  // back to the role's home here. Same UI-only caveat as cashbook above.
  useEffect(() => {
    if (!isRouteAllowedForRole(orgRole, location.pathname)) {
      navigate(roleHomePath(orgRole), { replace: true });
    }
  }, [orgRole, location.pathname, navigate]);

  const handleLogout = () => {
    Logout();
    navigate("/login");
  };

  // If no organizationId is set, redirect to index page
  if (!organizationId) {
    navigate("/");
    return null;
  }

  // If organizationId exists but organization is null, clear the invalid ID and redirect —
  // but only once organizationsAtom (a separate fetch) confirms the id truly isn't one of the
  // caller's organizations. organizationAtom's getter also returns null on a transient fetch
  // error (see src/atoms/organization.ts), not just on a genuinely nonexistent id; treating
  // every null the same used to silently drop the user onto a different (alphabetically-first)
  // organization after switching, right when a fetch blip made the newly-selected one fail to
  // load — indistinguishable from the switch itself being broken. Until organizations has
  // loaded, or if it still lists this id, fall through to the "Loading..." state below instead.
  if (
    organizationId &&
    organization === null &&
    organizationsLoaded &&
    !organizations.some((org) => org.id === organizationId)
  ) {
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
    const section = pathArray[0];
    const salesSections = ["invoices", "deliveries", "orders"];
    const purchasingSections = [
      "imports",
      "purchase-orders",
      "inbound-deliveries",
      "incoming-invoices",
    ];
    const masterDataSections = ["clients", "vendors", "products", "bill-of-materials"];
    if (salesSections.includes(section)) {
      openKeys = ["group-sales"];
    } else if (purchasingSections.includes(section)) {
      openKeys = ["group-purchasing"];
    } else if (section === "inventory" || section === "production-orders") {
      openKeys = ["group-inventory"];
    } else if (masterDataSections.includes(section)) {
      openKeys = ["group-masterdata"];
    } else if (section === "accounting") {
      openKeys = ["group-accounting"];
    } else if (section === "reporting") {
      openKeys = ["group-reporting"];
    }
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

  const mobileSiderOpen = isMobile && mobileMenuOpen;
  const siderIsCollapsed = isMobile ? !mobileMenuOpen : siderCollapsed;
  const closeMobileMenu = () => {
    if (isMobile) setMobileMenuOpen(false);
  };

  // #183: Settings moved from a sidebar group into this header icon —
  // isSettingsRoute just tints the icon while any /settings/* route is
  // active, since it no longer has a sidebar entry to show as selected.
  // /organizations is included because it now lives in this menu too.
  const isSettingsRoute =
    location.pathname.startsWith("/settings") || location.pathname.startsWith("/organizations");
  const settingsMenuItems = [
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
      icon: <ScheduleOutlined />,
      label: (
        <Link to="/settings/payment-terms">
          <Trans>Payment terms</Trans>
        </Link>
      ),
      key: "settings.payment-terms",
    },
    {
      icon: <ColumnWidthOutlined />,
      label: (
        <Link to="/settings/units-of-measure">
          <Trans>Units of measure</Trans>
        </Link>
      ),
      key: "settings.units-of-measure",
    },
    {
      icon: <FileExcelOutlined />,
      label: (
        <Link to="/settings/document-templates">
          <Trans>Document Templates</Trans>
        </Link>
      ),
      key: "settings.document-templates",
    },
    {
      icon: <OrderedListOutlined />,
      label: (
        <Link to="/settings/document-numbering">
          <Trans>Document Numbering</Trans>
        </Link>
      ),
      key: "settings.document-numbering",
    },
    // Organizations moved here from the sidebar's Master Data group (it's
    // organization configuration, not day-to-day master data). The route
    // itself is still the standalone /organizations page.
    {
      icon: <ApartmentOutlined />,
      label: (
        <Link to="/organizations">
          <Trans>Organizations</Trans>
        </Link>
      ),
      key: "settings.organizations",
    },
    ...(isPlatformAdmin
      ? [
          {
            icon: <DatabaseOutlined />,
            label: (
              <Link to="/settings/backup">
                <Trans>Backup</Trans>
              </Link>
            ),
            key: "settings.backup",
          },
          {
            icon: <UserOutlined />,
            label: (
              <Link to="/settings/users">
                <Trans>Users</Trans>
              </Link>
            ),
            key: "settings.users",
          },
          {
            icon: <GlobalOutlined />,
            label: (
              <Link to="/settings/countries">
                <Trans>Countries</Trans>
              </Link>
            ),
            key: "settings.countries",
          },
        ]
      : []),
    // GL Export is an org-scoped action, not a platform-wide one — an org
    // admin or accounting-role member sees it here even without being a
    // platform admin, and a platform admin who isn't a member of the
    // currently selected organization at either role doesn't.
    ...(canAccessGLExport
      ? [
          {
            icon: <ExportOutlined />,
            label: (
              <Link to="/settings/gl-export">
                <Trans>GL Export</Trans>
              </Link>
            ),
            key: "settings.gl-export",
          },
        ]
      : []),
  ];

  // The two entries the restricted cashbook role keeps. Declared once and
  // reused both in the full menu below and in the cashbook-only menu, so
  // the two can never drift.
  const cashBookMenuItem = {
    icon: <WalletOutlined />,
    label: (
      <Link to="/cash-book">
        <Trans>Cash Book</Trans>
      </Link>
    ),
    key: "cash-book",
  };
  const clientsMenuItem = {
    icon: <TeamOutlined />,
    label: (
      <Link to="/clients">
        <Trans>Clients</Trans>
      </Link>
    ),
    key: "clients",
  };

  return (
    <Layout hasSider style={{ minHeight: "100vh", width: "100%" }}>
      <a
        href="#main-content"
        className="skip-link"
        style={{
          background: colorBgContainer,
          color: colorPrimary,
          border: `1px solid ${colorPrimary}`,
        }}
      >
        <Trans>Skip to main content</Trans>
      </a>
      {isMobile && (
        <div
          onClick={closeMobileMenu}
          style={{
            display: mobileSiderOpen ? "block" : "none",
            position: "fixed",
            inset: 0,
            background: "rgba(0, 0, 0, 0.45)",
            zIndex: 999,
          }}
        />
      )}
      <Sider
        theme={themeMode}
        trigger={null}
        collapsible
        collapsed={siderIsCollapsed}
        collapsedWidth={isMobile ? 0 : 80}
        style={{
          overflow: "auto",
          height: "100vh",
          position: "fixed",
          left: 0,
          top: 0,
          bottom: 0,
          zIndex: isMobile ? 1000 : undefined,
          borderRight: `1px solid ${colorBorderSecondary}`,
        }}
      >
        <div
          className="logo"
          style={{
            padding: siderIsCollapsed ? "16px 8px 12px" : "18px 12px 14px 16px",
            display: "flex",
            alignItems: "center",
            justifyContent: siderIsCollapsed ? "center" : "space-between",
          }}
        >
          <Link
            to={roleHomePath(orgRole)}
            style={{
              display: "flex",
              alignItems: "center",
              justifyContent: siderIsCollapsed ? "center" : "flex-start",
              gap: 8,
              minWidth: 0,
            }}
          >
            <img
              src="/logo-minimal.png"
              alt="FaturaCloud"
              style={{ width: siderIsCollapsed ? 44 : 34, height: "auto", flexShrink: 0 }}
            />
            {!siderIsCollapsed && <Wordmark fontSize={17} />}
          </Link>
        </div>
        {/* A collapsed inline menu renders its open groups as floating
        popups, so opening the current route's group while collapsed left
        that submenu stuck open over the page — on every grouped route on a
        phone, where the sider is always collapsed until the menu button is
        tapped. Collapsed starts with nothing open; the key remounts the
        (uncontrolled) menu on toggle so the default re-applies, and hover
        popups on the collapsed desktop sider still work. */}
        <Menu
          key={siderIsCollapsed ? "collapsed" : "expanded"}
          theme={themeMode}
          mode="inline"
          defaultOpenKeys={siderIsCollapsed ? [] : openKeys}
          defaultSelectedKeys={selectedKeys}
          onClick={closeMobileMenu}
          items={
            isCashbook
              ? [cashBookMenuItem, clientsMenuItem]
              : filterMenuForRole(orgRole, [
                  {
                    icon: <DashboardOutlined />,
                    label: (
                      <Link to="/dashboard">
                        <Trans>Dashboard</Trans>
                      </Link>
                    ),
                    key: "dashboard",
                  },
                  cashBookMenuItem,
                  {
                    icon: <ShopOutlined />,
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
                    icon: <ShoppingCartOutlined />,
                    label: <Trans>Purchasing</Trans>,
                    key: "group-purchasing",
                    children: [
                      {
                        icon: <ContainerOutlined />,
                        label: (
                          <Link to="/imports">
                            <Trans>Imports</Trans>
                          </Link>
                        ),
                        key: "imports",
                      },
                      {
                        icon: <ShoppingCartOutlined />,
                        label: (
                          <Link to="/purchase-orders">
                            <Trans>Purchase Orders</Trans>
                          </Link>
                        ),
                        key: "purchase-orders",
                      },
                      {
                        icon: <ImportOutlined />,
                        label: (
                          <Link to="/inbound-deliveries">
                            <Trans>Goods Receipts</Trans>
                          </Link>
                        ),
                        key: "inbound-deliveries",
                      },
                      {
                        icon: <AuditOutlined />,
                        label: (
                          <Link to="/incoming-invoices">
                            <Trans>Incoming Invoices</Trans>
                          </Link>
                        ),
                        key: "incoming-invoices",
                      },
                    ],
                  },
                  {
                    icon: <InboxOutlined />,
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
                      {
                        icon: <DeploymentUnitOutlined />,
                        label: (
                          <Link to="/production-orders">
                            <Trans>Production Orders</Trans>
                          </Link>
                        ),
                        key: "production-orders",
                      },
                    ],
                  },
                  {
                    icon: <FolderOutlined />,
                    label: <Trans>Master Data</Trans>,
                    key: "group-masterdata",
                    children: [
                      clientsMenuItem,
                      {
                        icon: <SolutionOutlined />,
                        label: (
                          <Link to="/vendors">
                            <Trans>Vendors</Trans>
                          </Link>
                        ),
                        key: "vendors",
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
                        icon: <BuildOutlined />,
                        label: (
                          <Link to="/bill-of-materials">
                            <Trans>Bill of Materials</Trans>
                          </Link>
                        ),
                        key: "bill-of-materials",
                      },
                    ],
                  },
                  {
                    icon: <BankOutlined />,
                    label: <Trans>Accounting</Trans>,
                    key: "group-accounting",
                    children: [
                      {
                        icon: <BankOutlined />,
                        label: (
                          <Link to="/accounting/chart-of-accounts">
                            <Trans>Chart of Accounts</Trans>
                          </Link>
                        ),
                        key: "accounting.chart-of-accounts",
                      },
                      {
                        icon: <BookOutlined />,
                        label: (
                          <Link to="/accounting/journals">
                            <Trans>Journals</Trans>
                          </Link>
                        ),
                        key: "accounting.journals",
                      },
                      {
                        icon: <CalendarOutlined />,
                        label: (
                          <Link to="/accounting/fiscal-periods">
                            <Trans>Fiscal Periods</Trans>
                          </Link>
                        ),
                        key: "accounting.fiscal-periods",
                      },
                      {
                        icon: <UnorderedListOutlined />,
                        label: (
                          <Link to="/accounting/journal-entries">
                            <Trans>Journal Entries</Trans>
                          </Link>
                        ),
                        key: "accounting.journal-entries",
                      },
                      {
                        icon: <TableOutlined />,
                        label: (
                          <Link to="/accounting/trial-balance">
                            <Trans>Trial Balance</Trans>
                          </Link>
                        ),
                        key: "accounting.trial-balance",
                      },
                      {
                        icon: <LineChartOutlined />,
                        label: (
                          <Link to="/accounting/profit-and-loss">
                            <Trans>Profit &amp; Loss</Trans>
                          </Link>
                        ),
                        key: "accounting.profit-and-loss",
                      },
                      {
                        icon: <FundOutlined />,
                        label: (
                          <Link to="/accounting/balance-sheet">
                            <Trans>Balance Sheet</Trans>
                          </Link>
                        ),
                        key: "accounting.balance-sheet",
                      },
                      {
                        icon: <ClockCircleOutlined />,
                        label: (
                          <Link to="/accounting/ar-aging">
                            <Trans>AR Aging</Trans>
                          </Link>
                        ),
                        key: "accounting.ar-aging",
                      },
                      {
                        icon: <FieldTimeOutlined />,
                        label: (
                          <Link to="/accounting/ap-aging">
                            <Trans>AP Aging</Trans>
                          </Link>
                        ),
                        key: "accounting.ap-aging",
                      },
                      {
                        icon: <GoldOutlined />,
                        label: (
                          <Link to="/accounting/inventory-valuation">
                            <Trans>Inventory Valuation</Trans>
                          </Link>
                        ),
                        key: "accounting.inventory-valuation",
                      },
                      {
                        icon: <WalletOutlined />,
                        label: (
                          <Link to="/accounting/daily-cash-movements">
                            <Trans>Daily Cash Movements</Trans>
                          </Link>
                        ),
                        key: "accounting.daily-cash-movements",
                      },
                    ],
                  },
                  {
                    icon: <BarChartOutlined />,
                    label: <Trans>Reporting</Trans>,
                    key: "group-reporting",
                    children: [
                      {
                        icon: <LineChartOutlined />,
                        label: (
                          <Link to="/reporting/revenue-trend">
                            <Trans>Revenue Trend</Trans>
                          </Link>
                        ),
                        key: "reporting.revenue-trend",
                      },
                      {
                        icon: <TeamOutlined />,
                        label: (
                          <Link to="/reporting/sales-by-client">
                            <Trans>Sales by Client</Trans>
                          </Link>
                        ),
                        key: "reporting.sales-by-client",
                      },
                      {
                        icon: <AppstoreOutlined />,
                        label: (
                          <Link to="/reporting/sales-by-product">
                            <Trans>Sales by Product</Trans>
                          </Link>
                        ),
                        key: "reporting.sales-by-product",
                      },
                      {
                        icon: <ShoppingCartOutlined />,
                        label: (
                          <Link to="/reporting/purchases-by-vendor">
                            <Trans>Purchases by Vendor</Trans>
                          </Link>
                        ),
                        key: "reporting.purchases-by-vendor",
                      },
                      {
                        icon: <CalculatorOutlined />,
                        label: (
                          <Link to="/reporting/tax-summary">
                            <Trans>Tax Summary</Trans>
                          </Link>
                        ),
                        key: "reporting.tax-summary",
                      },
                    ],
                  },
                ])
          }
        />
      </Sider>
      <Layout
        style={{
          width: "100%",
          marginLeft: isMobile ? 0 : siderCollapsed ? 80 : 200,
          // The Sider is position:fixed, so this inner Layout is offset by a
          // marginLeft instead of being laid out beside it. With the default
          // min-width:auto it can't shrink below its widest child's min-content
          // — the Cash Book's `scroll={{ x: "max-content" }}` loan-status table
          // — so that page alone pushed the whole layout ~200px past the
          // viewport (hiding the table's right columns and the search rows'
          // money rail) instead of scrolling inside the table. min-width:0 lets
          // the flex item shrink and the inner scroll container do its job.
          minWidth: 0,
          transition: "all 0.2s",
        }}
      >
        <Header
          style={{
            position: "sticky",
            top: 0,
            zIndex: 1,
            padding: 0,
            background: colorBgContainer,
            borderBottom: `1px solid ${colorBorderSecondary}`,
          }}
        >
          {/* wrap={false}: the header is a fixed 64px tall, so a wrapped second
          line (the right-hand icons on a phone) spilled over the page instead
          of fitting. On mobile the org select narrows and the version and
          user name are hidden so one line fits a 390px screen. */}
          <Row wrap={false}>
            <Col flex="auto" style={{ minWidth: 0 }}>
              <Space align="center">
                <Button
                  type="text"
                  icon={siderIsCollapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
                  onClick={() =>
                    isMobile
                      ? setMobileMenuOpen(!mobileMenuOpen)
                      : setSiderCollapsed(!siderCollapsed)
                  }
                  aria-label={siderIsCollapsed ? t`Expand sidebar` : t`Collapse sidebar`}
                  style={{
                    fontSize: "16px",
                    width: isMobile ? 48 : 64,
                    height: 64,
                  }}
                />
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
                    style={{ width: isMobile ? 140 : 200 }}
                    defaultValue={organization.id}
                    onSelect={(value) => {
                      setOrganizationId(value);
                      window.location.reload();
                    }}
                    popupRender={(menu) => (
                      <>
                        {menu}
                        {/* Creating an organization is out of scope for the
                        restricted cashbook role. */}
                        {!isCashbook && (
                          <>
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
              </Space>
            </Col>
            <Col flex="none">
              <Space size={isMobile ? 0 : "small"}>
                {version && !isMobile && (
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    {version}
                  </Typography.Text>
                )}
                <Button
                  type="text"
                  icon={<CommentOutlined />}
                  onClick={(e) => {
                    // Blur the button to remove focus after click
                    e.currentTarget.blur();
                    setFeedbackModalOpen(true);
                  }}
                  title={t`Send feedback`}
                  aria-label={t`Send feedback`}
                />
                <Button
                  type="text"
                  icon={themeMode === "dark" ? <SunOutlined /> : <MoonOutlined />}
                  onClick={() => setThemeMode(themeMode === "dark" ? "light" : "dark")}
                  title={themeMode === "dark" ? t`Switch to light mode` : t`Switch to dark mode`}
                  aria-label={
                    themeMode === "dark" ? t`Switch to light mode` : t`Switch to dark mode`
                  }
                />
                {/* The restricted cashbook role has no settings screens —
                every item in this menu is out of its scope. */}
                {!isCashbook && (
                  <Dropdown
                    menu={{ items: settingsMenuItems }}
                    trigger={["click"]}
                    placement="bottomRight"
                  >
                    <Button
                      type="text"
                      icon={<SettingOutlined />}
                      style={isSettingsRoute ? { color: colorPrimary } : undefined}
                      title={t`Settings`}
                      aria-label={t`Settings`}
                    />
                  </Dropdown>
                )}
                <Select
                  variant="borderless"
                  popupMatchSelectWidth={false}
                  aria-label={t`Language`}
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
                      de: "🇩🇪 German",
                      fr: "🇫🇷 French",
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
                {currentUser && (
                  <Space size={4} style={{ marginRight: isMobile ? 8 : 24 }}>
                    {!isMobile && (
                      <>
                        <UserOutlined />
                        <span style={{ fontSize: 13 }}>
                          {currentUser.displayName || currentUser.email}
                        </span>
                      </>
                    )}
                    <Button
                      type="text"
                      icon={<LogoutOutlined />}
                      size="small"
                      onClick={handleLogout}
                      title={t`Sign out`}
                      aria-label={t`Sign out`}
                    />
                  </Space>
                )}
              </Space>
            </Col>
          </Row>
        </Header>
        <Content
          id="main-content"
          tabIndex={-1}
          style={{
            // Tighter on a phone, where 16px margin + 24px padding on each
            // side left under 310px of a 390px screen for the page itself.
            margin: isMobile ? "12px 8px" : "24px 16px",
            padding: isMobile ? 12 : 24,
            minHeight: 280,
            // Content is a flex item in the outer Layout (next to the Sider).
            // Its default min-width:auto lets a wide child — the Cash Book's
            // `scroll={{ x: "max-content" }}` loan-status table — expand the
            // whole column past the viewport instead of scrolling inside its
            // own container, pushing the right-hand columns (and the search
            // rows' money rail) off-screen. min-width:0 lets it shrink so the
            // inner scroll container does its job.
            minWidth: 0,
            background: colorBgContainer,
            borderRadius: borderRadiusLG,
          }}
        >
          <Outlet />
          {/*<SignOut />*/}
        </Content>
        {/* The sticky element has to be this wrapper, not the Footer that
        ResponsiveFooter portals into it: sticky only moves within its parent,
        and this div is exactly the Footer's size, so a sticky Footer never
        stuck and every detail page's Save/status actions sat at the very end
        of a long document. This div's parent is the full-height column. */}
        <div id="footer" style={{ position: "sticky", bottom: 0, zIndex: 1 }} />
      </Layout>
      <FeedbackModal open={feedbackModalOpen} onClose={() => setFeedbackModalOpen(false)} />
    </Layout>
  );
}
