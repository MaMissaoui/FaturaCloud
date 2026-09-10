import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import { renderWithProviders } from "src/test-support/render-with-providers";
import PaymentPanel from "./payment-panel";
import type { Payment, PaymentApplication } from "src/types/models";

// PaymentPanel is the first component-level smoke test (issue #175): it
// takes only primitive props and fetches its own data via `src/api`
// functions directly — no Jotai atoms, no router — so it can prove the
// harness (Lingui macros, jsdom, antd's App/ConfigProvider) works without
// also needing to seed the dozen fetch-backed atoms the invoice detail page
// (the issue's own suggested target) would require. That page's test is
// deliberately deferred — see the PR description.
//
// Mocking `src/api` directly (vi.mock) rather than MSW: PaymentPanel calls
// named functions imported from `src/api`, so stubbing those functions is
// fewer moving parts than intercepting the underlying fetch calls, and
// matches how this codebase's atoms already isolate the API layer behind
// typed functions. MSW would earn its keep once a test needs to assert on
// request shape/headers, not for a smoke test.
//
// No mock needed for src/utils/date's useDatePickerFormat (which reads
// organizationAtom, an async Jotai atom) — renderWithProviders now flushes
// organizationAtom's Suspense correctly (see its ASYNC ATOM SUSPENSE FIX
// comment, issue #202).
vi.mock("src/api", () => ({
  GetInvoicePayments: vi.fn(),
  GetIncomingInvoicePayments: vi.fn(),
  GetPayment: vi.fn(),
  GetAccounts: vi.fn(),
  CreatePayment: vi.fn(),
  VoidPayment: vi.fn(),
}));

import { GetAccounts, GetInvoicePayments, GetPayment } from "src/api";

const payment: Payment = {
  id: "pay_1",
  organizationId: "org_1",
  direction: "inbound",
  clientId: "client_1",
  vendorId: null,
  bankAccountId: "acct_1",
  amount: 5000,
  currency: "EUR",
  exchangeRate: null,
  exchangeRateDate: null,
  date: Date.now(),
  method: "bank_transfer",
  reference: "REF-1",
  notes: null,
  status: "posted",
  journalEntryId: "je_1",
  voidingEntryId: null,
  createdAt: Date.now(),
};

const application: PaymentApplication = {
  id: "app_1",
  paymentId: "pay_1",
  documentType: "invoice",
  documentId: "inv_1",
  amount: 5000,
  createdAt: Date.now(),
};

describe("PaymentPanel", () => {
  // No `test.globals` in vitest.config.ts, so @testing-library/react's
  // auto-cleanup (which looks for a global `afterEach`) never registers —
  // without this, the second test's assertions can see the first test's
  // still-mounted DOM.
  afterEach(cleanup);

  it("renders without throwing given a mocked payment fetch, and shows the fetched payment", async () => {
    vi.mocked(GetInvoicePayments).mockResolvedValue([application]);
    vi.mocked(GetPayment).mockResolvedValue(payment);
    vi.mocked(GetAccounts).mockResolvedValue([]);

    await renderWithProviders(
      <PaymentPanel
        organizationId="org_1"
        documentType="invoice"
        documentId="inv_1"
        direction="inbound"
        clientId="client_1"
        currency="EUR"
        orgCurrency="EUR"
        total={10000}
        hasPostedEntry={true}
      />,
    );

    expect(await screen.findByText("Payments")).toBeInTheDocument();
    // Fetched row rendered: reference from the mocked payment, and the
    // "Record payment" button gated on hasPostedEntry + a nonzero balance.
    await waitFor(() => expect(screen.getByText("REF-1")).toBeInTheDocument());
    expect(screen.getByRole("button", { name: "Record payment" })).toBeInTheDocument();
  });

  it("renders nothing when there's no posted GL entry and no payment history", async () => {
    vi.mocked(GetInvoicePayments).mockResolvedValue([]);
    vi.mocked(GetAccounts).mockResolvedValue([]);

    await renderWithProviders(
      <PaymentPanel
        organizationId="org_1"
        documentType="invoice"
        documentId="inv_2"
        direction="inbound"
        clientId="client_1"
        currency="EUR"
        orgCurrency="EUR"
        total={10000}
        hasPostedEntry={false}
      />,
    );

    // The component genuinely returns null here — asserting on the absence
    // of its own content rather than "the container has zero children",
    // since the antd <App> wrapper from renderWithProviders always
    // contributes one (empty) DOM node regardless of what's rendered inside.
    await waitFor(() => expect(GetInvoicePayments).toHaveBeenCalled());
    expect(screen.queryByText("Payments")).not.toBeInTheDocument();
  });
});
