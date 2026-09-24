// Focused sidebar per organization role (2026-09-22) — the frontend source of
// truth for which sections each role may see, extracted from src/layouts/base.tsx
// so it can be unit-tested (src/layouts/role-menu.test.ts). api/sections.go
// mirrors this allow-list server-side so a restricted role can't reach a hidden
// section by calling the endpoint directly.

// A missing top-level key hides that group/item entirely; a null value allows
// every child of the group, while an array allows only the listed child keys.
// admin, power_user and cashbook are deliberately absent: admin/power_user get
// the full menu, and cashbook has its own two-item menu (src/layouts/base.tsx).
// A role of "" (still loading, or no organization) is treated as unrestricted
// so the menu doesn't flash empty.
export const ROLE_MENU: Record<string, Record<string, string[] | null>> = {
  general: {
    dashboard: null,
    "cash-book": null,
    "group-sales": null,
    // Imports is purchasing-only; a general user keeps the rest of Purchasing.
    "group-purchasing": ["purchase-orders", "inbound-deliveries", "incoming-invoices"],
    // Production orders are a manufacturing concern.
    "group-inventory": ["inventory"],
    // Bill of Materials is a manufacturing concern.
    "group-masterdata": ["clients", "vendors", "products"],
    "group-reporting": null,
  },
  sales: {
    dashboard: null,
    "group-sales": null,
    "group-masterdata": ["clients", "products"],
    "group-reporting": ["revenue-trend", "sales-by-client", "sales-by-product"],
  },
  purchasing: {
    dashboard: null,
    "group-purchasing": null,
    "group-masterdata": ["vendors", "products"],
    "group-reporting": ["purchases-by-vendor"],
  },
  accounting: {
    dashboard: null,
    "group-accounting": null,
    "group-reporting": ["tax-summary"],
  },
};

// Every route prefix that belongs to a sidebar section, so a direct URL can be
// checked against the same allow-list the menu uses. Settings and organization
// routes are intentionally absent — they're not section-scoped here (GL Export
// keeps its own admin/accounting gate).
export const PATH_MENU_KEYS: Array<[prefix: string, topKey: string, childKey?: string]> = [
  ["/dashboard", "dashboard"],
  ["/cash-book", "cash-book"],
  ["/invoices", "group-sales", "invoices"],
  ["/deliveries", "group-sales", "deliveries"],
  ["/orders", "group-sales", "orders"],
  ["/imports", "group-purchasing", "imports"],
  ["/purchase-orders", "group-purchasing", "purchase-orders"],
  ["/inbound-deliveries", "group-purchasing", "inbound-deliveries"],
  ["/incoming-invoices", "group-purchasing", "incoming-invoices"],
  ["/inventory", "group-inventory", "inventory"],
  ["/production-orders", "group-inventory", "production-orders"],
  ["/clients", "group-masterdata", "clients"],
  ["/vendors", "group-masterdata", "vendors"],
  ["/products", "group-masterdata", "products"],
  ["/bill-of-materials", "group-masterdata", "bill-of-materials"],
  ["/accounting", "group-accounting"],
  // Reporting is the one group whose URL guard needs a child key: the
  // accounting role keeps only Tax Summary (group-reporting: ["tax-summary"]),
  // so each report path is listed individually rather than allowing the whole
  // "/reporting" prefix. An unknown "/reporting/*" path matches nothing and is
  // allowed, like every other path outside a known section.
  ["/reporting/revenue-trend", "group-reporting", "revenue-trend"],
  ["/reporting/sales-by-client", "group-reporting", "sales-by-client"],
  ["/reporting/sales-by-product", "group-reporting", "sales-by-product"],
  ["/reporting/purchases-by-vendor", "group-reporting", "purchases-by-vendor"],
  ["/reporting/tax-summary", "group-reporting", "tax-summary"],
];

export const pathMatches = (pathname: string, prefix: string) =>
  pathname === prefix || pathname.startsWith(prefix + "/");

// roleCanSeeMenuItem decides whether a role's allow-list covers one menu entry
// (a top-level item, or a child of a group). An unrestricted role ("",
// admin, power_user — anything absent from ROLE_MENU) sees everything.
export const roleCanSeeMenuItem = (role: string, topKey: string, childKey?: string): boolean => {
  const allow = ROLE_MENU[role];
  if (!allow) return true;
  if (!(topKey in allow)) return false;
  const childAllow = allow[topKey];
  return !(childKey && childAllow && !childAllow.includes(childKey));
};

// isRouteAllowedForRole decides whether a role may view a path, using the same
// ROLE_MENU allow-list as the sidebar. A path outside every known section
// (settings, organizations, ...) is allowed — this only scopes the sidebar's
// own sections. admin/power_user/cashbook and an unresolved role ("") are
// unrestricted here (cashbook has its own dedicated redirect in base.tsx).
export const isRouteAllowedForRole = (role: string, pathname: string): boolean => {
  const allow = ROLE_MENU[role];
  if (!allow) return true;
  const matched = PATH_MENU_KEYS.find(([prefix]) => pathMatches(pathname, prefix));
  if (!matched) return true;
  const [, topKey, childKey] = matched;
  return roleCanSeeMenuItem(role, topKey, childKey);
};

// filterMenuForRole applies ROLE_MENU to the full sidebar item list. The item
// objects are JSX-bearing literals, so this returns shallow copies with
// filtered children rather than mutating them.
export const filterMenuForRole = (role: string, items: any[]): any[] => {
  const allow = ROLE_MENU[role];
  if (!allow) return items;
  return items
    .filter((item) => item && item.key in allow)
    .map((item) => {
      const childAllow = allow[item.key];
      if (childAllow == null || !item.children) return item;
      return {
        ...item,
        children: item.children.filter((c: any) => c && childAllow.includes(c.key)),
      };
    });
};

// roleHomePath is the route a role is sent to when it lands on "/" or is
// bounced out of a section outside its scope (src/layouts/base.tsx,
// src/routes/index.tsx) — the focused view's "home". admin, power_user and
// general all default to the invoice list.
export const roleHomePath = (role: string): string => {
  switch (role) {
    case "cashbook":
      return "/cash-book";
    case "purchasing":
      return "/purchase-orders";
    case "accounting":
      return "/accounting/journal-entries";
    default:
      return "/invoices";
  }
};

// Which widgets a role sees (audit F147). The dashboard's data is shared at
// the API level (api/sections.go leaves the route membership-level), so this
// is presentation: each widget follows the same ROLE_MENU sections as the
// sidebar, and every row click / "View full report" link is shown only when
// the target page is one the role can open, instead of bouncing it.
//   - sales figures (revenue, top clients/products): the Sales section
//   - receivables (outstanding invoices): Sales or Accounting (AR aging)
//   - stock valuation: Inventory, Accounting (inventory valuation) or
//     Purchasing (who buys the stock)
// admin/power_user/general, and an unresolved role, see everything.
export const dashboardWidgetsForRole = (role: string) => {
  const sees = (key: string) => roleCanSeeMenuItem(role, key);
  const sales = sees("group-sales");
  return {
    sales,
    receivables: sales || sees("group-accounting"),
    stock: sees("group-inventory") || sees("group-accounting") || sees("group-purchasing"),
  };
};
