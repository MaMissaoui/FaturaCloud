import { describe, expect, it } from "vitest";

import {
  filterMenuForRole,
  isRouteAllowedForRole,
  roleCanSeeMenuItem,
  roleHomePath,
} from "./role-menu";

describe("roleHomePath", () => {
  it("sends each focused role to its own section", () => {
    expect(roleHomePath("cashbook")).toBe("/cash-book");
    expect(roleHomePath("purchasing")).toBe("/purchase-orders");
    expect(roleHomePath("accounting")).toBe("/accounting/journal-entries");
  });

  it("defaults to the invoice list for everyone else", () => {
    for (const role of ["", "admin", "power_user", "general", "sales"]) {
      expect(roleHomePath(role)).toBe("/invoices");
    }
  });
});

describe("isRouteAllowedForRole", () => {
  const allowed = (role: string, paths: string[]) => {
    for (const path of paths) {
      expect(isRouteAllowedForRole(role, path), `${role} should reach ${path}`).toBe(true);
    }
  };
  const denied = (role: string, paths: string[]) => {
    for (const path of paths) {
      expect(isRouteAllowedForRole(role, path), `${role} should NOT reach ${path}`).toBe(false);
    }
  };

  it("leaves admin, power_user, cashbook and an unresolved role unrestricted", () => {
    const paths = [
      "/accounting/journal-entries",
      "/imports",
      "/production-orders",
      "/bill-of-materials",
    ];
    for (const role of ["", "admin", "power_user", "cashbook"]) {
      allowed(role, paths);
    }
  });

  it("lets general keep everything except Accounting, Imports, Bill of Materials and Production Orders", () => {
    allowed("general", [
      "/dashboard",
      "/cash-book",
      "/invoices",
      "/invoices/abc",
      "/orders",
      "/deliveries",
      "/purchase-orders",
      "/inbound-deliveries",
      "/incoming-invoices",
      "/inventory",
      "/clients",
      "/vendors",
      "/products",
      "/reporting/revenue-trend",
      "/reporting/purchases-by-vendor",
      "/reporting/tax-summary",
    ]);
    denied("general", [
      "/accounting/journal-entries",
      "/accounting/trial-balance",
      "/imports",
      "/production-orders",
      "/bill-of-materials",
    ]);
  });

  it("scopes sales to Sales, Clients/Products and the sales reports", () => {
    allowed("sales", [
      "/dashboard",
      "/invoices",
      "/deliveries",
      "/orders",
      "/clients",
      "/products",
      "/reporting/revenue-trend",
      "/reporting/sales-by-client",
      "/reporting/sales-by-product",
    ]);
    denied("sales", [
      "/purchase-orders",
      "/inbound-deliveries",
      "/incoming-invoices",
      "/imports",
      "/inventory",
      "/production-orders",
      "/vendors",
      "/bill-of-materials",
      "/accounting/journal-entries",
      "/reporting/purchases-by-vendor",
      "/reporting/tax-summary",
    ]);
  });

  it("scopes purchasing to Purchasing/Imports, Vendors/Products and Purchases by Vendor", () => {
    allowed("purchasing", [
      "/dashboard",
      "/purchase-orders",
      "/inbound-deliveries",
      "/incoming-invoices",
      "/imports",
      "/vendors",
      "/products",
      "/reporting/purchases-by-vendor",
    ]);
    denied("purchasing", [
      "/invoices",
      "/orders",
      "/deliveries",
      "/clients",
      "/inventory",
      "/production-orders",
      "/bill-of-materials",
      "/accounting/journal-entries",
      "/reporting/revenue-trend",
      "/reporting/tax-summary",
    ]);
  });

  it("scopes accounting to Accounting and Tax Summary", () => {
    allowed("accounting", [
      "/dashboard",
      "/accounting/journal-entries",
      "/accounting/trial-balance",
      "/reporting/tax-summary",
    ]);
    denied("accounting", [
      "/invoices",
      "/purchase-orders",
      "/imports",
      "/inventory",
      "/production-orders",
      "/bill-of-materials",
      "/clients",
      "/vendors",
      "/products",
      "/reporting/revenue-trend",
      "/reporting/purchases-by-vendor",
    ]);
  });

  it("does not scope settings or organization routes for any role", () => {
    const paths = ["/settings/invoice", "/organizations", "/organizations/new"];
    for (const role of ["general", "sales", "purchasing", "accounting", "cashbook"]) {
      allowed(role, paths);
    }
  });
});

describe("roleCanSeeMenuItem", () => {
  it("reports the Imports and Bill of Materials gates used by the forms", () => {
    expect(roleCanSeeMenuItem("general", "group-purchasing", "imports")).toBe(false);
    expect(roleCanSeeMenuItem("purchasing", "group-purchasing", "imports")).toBe(true);
    expect(roleCanSeeMenuItem("admin", "group-purchasing", "imports")).toBe(true);

    expect(roleCanSeeMenuItem("general", "group-masterdata", "bill-of-materials")).toBe(false);
    expect(roleCanSeeMenuItem("sales", "group-masterdata", "bill-of-materials")).toBe(false);
    expect(roleCanSeeMenuItem("admin", "group-masterdata", "bill-of-materials")).toBe(true);
    expect(roleCanSeeMenuItem("power_user", "group-masterdata", "bill-of-materials")).toBe(true);
  });

  it("treats a null group value as every child allowed", () => {
    expect(roleCanSeeMenuItem("sales", "group-sales", "deliveries")).toBe(true);
    expect(roleCanSeeMenuItem("accounting", "group-accounting", "anything")).toBe(true);
  });
});

describe("filterMenuForRole", () => {
  const items = [
    { key: "dashboard" },
    {
      key: "group-sales",
      children: [{ key: "invoices" }, { key: "orders" }, { key: "deliveries" }],
    },
    {
      key: "group-purchasing",
      children: [{ key: "imports" }, { key: "purchase-orders" }, { key: "inbound-deliveries" }],
    },
    {
      key: "group-masterdata",
      children: [
        { key: "clients" },
        { key: "vendors" },
        { key: "products" },
        { key: "bill-of-materials" },
      ],
    },
    { key: "group-reporting", children: [{ key: "revenue-trend" }, { key: "tax-summary" }] },
  ];

  it("returns the list untouched for unrestricted roles", () => {
    for (const role of ["", "admin", "power_user", "cashbook"]) {
      expect(filterMenuForRole(role, items)).toBe(items);
    }
  });

  it("drops groups outside the allow-list and prunes children within them", () => {
    const filtered = filterMenuForRole("general", items);
    expect(filtered.map((i: any) => i.key)).toEqual([
      "dashboard",
      "group-sales",
      "group-purchasing",
      "group-masterdata",
      "group-reporting",
    ]);
    expect(
      filtered.find((i: any) => i.key === "group-purchasing").children.map((c: any) => c.key),
    ).toEqual(["purchase-orders", "inbound-deliveries"]);
    expect(
      filtered.find((i: any) => i.key === "group-masterdata").children.map((c: any) => c.key),
    ).toEqual(["clients", "vendors", "products"]);
    // A null group value keeps every child.
    expect(filtered.find((i: any) => i.key === "group-sales").children).toHaveLength(3);
  });

  it("keeps only the domain group and shared master data for a domain role", () => {
    const filtered = filterMenuForRole("sales", items);
    expect(filtered.map((i: any) => i.key)).toEqual([
      "dashboard",
      "group-sales",
      "group-masterdata",
      "group-reporting",
    ]);
    expect(
      filtered.find((i: any) => i.key === "group-masterdata").children.map((c: any) => c.key),
    ).toEqual(["clients", "products"]);
    expect(
      filtered.find((i: any) => i.key === "group-reporting").children.map((c: any) => c.key),
    ).toEqual(["revenue-trend"]);
  });
});
