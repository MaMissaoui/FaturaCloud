import { describe, expect, it, vi } from "vitest";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import { createStore } from "jotai";
import { renderWithProviders } from "src/test-support/render-with-providers";
import InvoiceDetails from "./details";
import { organizationIdAtom, nextInvoiceNumberAtom } from "src/atoms/organization";
import { currentUserAtom } from "src/atoms/auth";
import type { CurrentUser } from "src/api";
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
    amountInWordsEnabled: 0,
    defaultFiscalStampAmount: null,
    defaultStampDutyAccountId: null,
    invoiceLayout: null,
    defaultCashRegisterAccountId: null,
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
    discountAmount: 0,
    ...overrides,
  };
}

// organizationAtom only loads for a signed-in user (it used to fetch while
// logged out on the login page, cache a 401 as null and never recover), so
// every test that selects an organization also signs a user in, as the real
// app's GetMe/login does before any page renders.
const testUser: CurrentUser = {
  id: "user_1",
  email: "user@example.test",
  displayName: "Test User",
  role: "admin",
  isPlatformAdmin: 0,
  isActive: 1,
  authProvider: "local",
};

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
    store.set(currentUserAtom, testUser);

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
    store.set(currentUserAtom, testUser);

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

  // Regression test for the field.key/field.name divergence flagged while
  // migrating the line-items table to the shared shell: the Qty/Price/Total
  // back-compute handlers read and wrote through `field.key` (the Form.List
  // row's stable identity) while the Form.Items themselves bind through
  // `field.name` (the row's current positional index). Those two are equal
  // until a row above is removed — after that the surviving rows keep their
  // original keys but are re-indexed, so a handler keyed on field.key writes
  // to a row that no longer exists (or a newly-created phantom one) instead of
  // the row the user is editing. Deleting the first of two rows is the
  // smallest repro of exactly that divergence (the survivor ends up key 1,
  // name 0), and keeps the test cheap enough for CI.
  it("back-computes the visible row's total, not a stale key's, after a line item is deleted", async () => {
    mockCommonFetches();

    const store = createStore();
    store.set(organizationIdAtom, "org_1");
    store.set(currentUserAtom, testUser);

    const { container } = await renderWithProviders(<InvoiceDetails />, {
      route: "/invoices/new",
      path: "/invoices/:id",
      jotaiStore: store,
      flushAtoms: [nextInvoiceNumberAtom],
    });
    await screen.findByText("Invoice details");

    // A new invoice seeds one line item; add a second so one can be removed.
    fireEvent.click(screen.getByRole("button", { name: /add line item/i }));

    const table = container.querySelector(".ant-table") as HTMLElement;
    const numbers = () =>
      Array.from(table.querySelectorAll<HTMLInputElement>(".ant-input-number-input"));

    // Each row renders Qty, Price and Total as number inputs, in that order.
    await waitFor(() => expect(numbers()).toHaveLength(6));

    // Remove the first row — the survivor keeps Form.List key 1 but becomes
    // positional name 0.
    fireEvent.click(screen.getAllByRole("button", { name: /remove line item/i })[0]);
    await waitFor(() => expect(numbers()).toHaveLength(3));

    // Quantity still defaults to 1, so typing a price must back-compute the
    // row's total (1 × 20 = 20). Keyed on the stale field.key of 1 that set
    // lands on a non-existent row and the total stays empty.
    fireEvent.change(numbers()[1], { target: { value: "20" } });
    await waitFor(() => expect(numbers()[2].value).toBe("20"));
    // Rendering the whole invoice page in jsdom (antd Form.List/Table) is a
    // few seconds on its own, over the 5s default timeout on slower CI.
  }, 30000);

  // A `noStyle` Form.Item renders no error text, so before this the required
  // rules on the line-item cells (product/description/price/total) blocked
  // form.submit() completely silently: no inline error, no toast, no request —
  // Save just did nothing. "Description required" is unique to a line-item
  // description cell, so it only renders once that feedback is wired up.
  it("shows a line item's validation error instead of failing Save silently", async () => {
    mockCommonFetches();

    const store = createStore();
    store.set(organizationIdAtom, "org_1");
    store.set(currentUserAtom, testUser);

    await renderWithProviders(<InvoiceDetails />, {
      route: "/invoices/new",
      path: "/invoices/:id",
      jotaiStore: store,
      flushAtoms: [nextInvoiceNumberAtom],
    });
    await screen.findByText("Invoice details");

    // The seeded line item has no description, so Save must surface the
    // missing field rather than quietly doing nothing.
    fireEvent.click(screen.getByText("Save"));
    expect(await screen.findByText("Description required")).toBeInTheDocument();
  }, 30000);
});
