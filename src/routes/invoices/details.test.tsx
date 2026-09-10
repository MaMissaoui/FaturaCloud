import { describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import { createStore } from "jotai";
import { renderWithProviders } from "src/test-support/render-with-providers";
import InvoiceDetails from "./details";
import { organizationIdAtom, nextInvoiceNumberAtom } from "src/atoms/organization";
import type { Organization } from "src/types/models";
import type { Invoice } from "src/types/invoice";

// Component smoke tests for the invoice detail page (issue #175's own
// originally-suggested first slice) — deferred out of PR #203 specifically
// because this page needs the full harness (Jotai store, router params,
// ~6 atoms) plus the async-atom Suspense fix that #202 turned out to
// require (PR #204). Now that both exist, both routes below are
// tractable.
vi.mock("src/api", () => ({
  GetOrganization: vi.fn(),
  GetOrganizationLogoDataUri: vi.fn(),
  GetClients: vi.fn(),
  GetProducts: vi.fn(),
  GetTaxRates: vi.fn(),
  GetPaymentTerms: vi.fn(),
  GetInvoice: vi.fn(),
  GetInvoiceLineItems: vi.fn(),
  // Only needed for the existing-invoice route below: an existing (non-new)
  // invoice also renders PaymentPanel, whose mount effects call these
  // regardless of hasPostedEntry (see src/components/payments/payment-panel.tsx).
  GetAccounts: vi.fn(),
  GetInvoicePayments: vi.fn(),
}));

import {
  GetAccounts,
  GetClients,
  GetInvoice,
  GetInvoiceLineItems,
  GetInvoicePayments,
  GetOrganization,
  GetOrganizationLogoDataUri,
  GetPaymentTerms,
  GetProducts,
  GetTaxRates,
} from "src/api";

// Every field is nullable except `id` — this fills in only what the page
// actually reads (see getInitialValues/the invoice-number generator) and
// leaves the rest null, rather than fabricating an org-settings profile
// the test doesn't need.
function buildOrganization(overrides: Partial<Organization> = {}): Organization {
  return {
    id: "org_1",
    code: null,
    name: "Test Org",
    country: null,
    email: null,
    phone: null,
    website: null,
    registration_number: null,
    vatin: null,
    bank_name: null,
    iban: null,
    currency: "EUR",
    minimum_fraction_digits: 2,
    due_days: 14,
    overdueCharge: 0,
    customerNotes: null,
    createdAt: null,
    logo: null,
    invoiceNumberFormat: "INV-{number}",
    invoiceNumberCounter: 0,
    date_format: null,
    brandColor: null,
    match_price_tolerance_percent: null,
    match_quantity_tolerance_percent: null,
    bic: null,
    tax_number: null,
    street: null,
    house_number: null,
    postal_code: null,
    city: null,
    country_code: null,
    defaultArAccountId: null,
    defaultApAccountId: null,
    defaultRevenueAccountId: null,
    defaultExpenseAccountId: null,
    defaultCashAccountId: null,
    fxGainAccountId: null,
    fxLossAccountId: null,
    retainedEarningsAccountId: null,
    defaultInventoryAccountId: null,
    defaultGRNIAccountId: null,
    defaultCOGSAccountId: null,
    defaultInventoryAdjustmentAccountId: null,
    defaultImportCostsPayableAccountId: null,
    datevClearingAccountId: null,
    datev_consultant_number: null,
    datev_client_number: null,
    fiscalStampEnabled: 0,
    withholdingTaxEnabled: 0,
    defaultFiscalStampAmount: null,
    defaultStampDutyAccountId: null,
    invoiceLayout: null,
    ...overrides,
  };
}

// The four list-atom fetches every route below needs regardless of
// new-vs-existing (setClients/setProducts/setTaxRates/setPaymentTerms all
// fire from the same mount effect) plus the organization itself.
function mockCommonFetches() {
  vi.mocked(GetOrganization).mockResolvedValue(buildOrganization());
  vi.mocked(GetOrganizationLogoDataUri).mockResolvedValue(null);
  vi.mocked(GetClients).mockResolvedValue([]);
  vi.mocked(GetProducts).mockResolvedValue({ data: [], total: 0 });
  vi.mocked(GetTaxRates).mockResolvedValue([]);
  vi.mocked(GetPaymentTerms).mockResolvedValue([]);
}

function buildInvoice(overrides: Partial<Invoice> = {}): Invoice {
  return {
    id: "inv_1",
    organizationId: "org_1",
    number: "INV-EXISTING-1",
    state: "draft",
    clientId: "client_1",
    date: Date.now(),
    dueDate: null,
    currency: "EUR",
    exchangeRate: null,
    exchangeRateDate: null,
    customerNotes: null,
    overdueCharge: 0,
    total: 0,
    taxTotal: 0,
    subTotal: 0,
    createdAt: null,
    clientName: "Test Client",
    buyerReference: null,
    paymentTerms: null,
    fiscalStampAmount: 0,
    withholdingTaxRate: null,
    withholdingTaxAmount: null,
    ...overrides,
  };
}

describe("InvoiceDetails", () => {
  it("renders the new-invoice form without throwing, prefilled with the generated invoice number", async () => {
    mockCommonFetches();

    const store = createStore();
    // organizationAtom (and everything derived from it, like
    // nextInvoiceNumberAtom) reads organizationIdAtom — an atomWithStorage
    // defaulting to null, which InvoiceDetails treats as "no organization
    // selected" and renders nothing for (`if (!organization) return null`).
    // Set it directly on this test's own store before rendering.
    store.set(organizationIdAtom, "org_1");

    await renderWithProviders(<InvoiceDetails />, {
      route: "/invoices/new",
      path: "/invoices/:id",
      jotaiStore: store,
      flushAtoms: [nextInvoiceNumberAtom],
    });

    expect(await screen.findByText("Invoice details")).toBeInTheDocument();
    // The four list-atom effects (setClients/setProducts/setTaxRates/
    // setPaymentTerms) run after the render's own flush, so assert on
    // something they actually produced rather than just "didn't throw".
    expect(GetClients).toHaveBeenCalledWith("org_1");
    expect(GetProducts).toHaveBeenCalledWith("org_1");
    // nextInvoiceNumberAtom (INV-{number} format, counter 0+1) is what
    // seeds the new invoice's "number" field — proves organizationAtom AND
    // nextInvoiceNumberAtom both actually resolved through the harness's
    // Suspense fix, not just that the page didn't crash.
    expect(await screen.findByDisplayValue("INV-1")).toBeInTheDocument();
  });

  it("renders an existing invoice's saved data given a mocked invoice fetch", async () => {
    mockCommonFetches();
    vi.mocked(GetInvoice).mockResolvedValue(buildInvoice());
    vi.mocked(GetInvoiceLineItems).mockResolvedValue([]);
    vi.mocked(GetAccounts).mockResolvedValue([]);
    vi.mocked(GetInvoicePayments).mockResolvedValue([]);

    const store = createStore();
    store.set(organizationIdAtom, "org_1");

    await renderWithProviders(<InvoiceDetails />, {
      route: "/invoices/inv_1",
      path: "/invoices/:id",
      jotaiStore: store,
    });

    // invoiceAtom is wrapped in loadable() (src/routes/invoices/details.tsx's
    // loadableInvoiceAtom) specifically so it never suspends — it resolves
    // through Jotai's normal store.sub notification instead, the same path
    // every other plain atom update takes, not the act()-scoped Suspense
    // flush organizationAtom/nextInvoiceNumberAtom need. No flushAtoms entry
    // needed for it; findByDisplayValue's own polling is enough.
    expect(await screen.findByDisplayValue("INV-EXISTING-1")).toBeInTheDocument();
    expect(GetInvoice).toHaveBeenCalledWith("inv_1");
    expect(GetInvoiceLineItems).toHaveBeenCalledWith("inv_1");
  });
});
