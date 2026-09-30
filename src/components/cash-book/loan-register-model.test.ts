import { describe, expect, it } from "vitest";

import type { LoanStatusRow } from "src/api";
import type { Payment } from "src/types/models";

import { buildLoanRegister, filterLoanRegister, STALE_AFTER_DAYS } from "./loan-register-model";

const day = (iso: string) => new Date(`${iso}T10:00:00`).getTime();
const NOW = day("2026-09-30");

const row = (over: Partial<LoanStatusRow>): LoanStatusRow => ({
  lineId: "l1",
  invoiceId: "i1",
  invoiceNumber: "FAC-1",
  clientId: "c1",
  clientName: "Hédi Trabelsi",
  date: day("2026-03-12"),
  productName: "Climatiseur",
  sku: "",
  quantity: 1,
  amount: 139000,
  paid: 75000,
  outstanding: 64000,
  ...over,
});

const payment = (over: Partial<Payment>): Payment => ({
  id: "p1",
  organizationId: "o1",
  direction: "inbound",
  clientId: "c1",
  vendorId: null,
  bankAccountId: "a1",
  amount: 15000,
  currency: "TND",
  exchangeRate: null,
  exchangeRateDate: null,
  date: day("2026-09-21"),
  method: "cash",
  reference: null,
  notes: null,
  status: "posted",
  journalEntryId: null,
  voidingEntryId: null,
  createdAt: 0,
  invoiceNumbers: ["FAC-1"],
  ...over,
});

describe("buildLoanRegister", () => {
  it("groups lines into loans and loans into customers, with totals", () => {
    const [c] = buildLoanRegister(
      [
        row({ lineId: "a" }),
        row({
          lineId: "b",
          invoiceId: "i2",
          invoiceNumber: "FAC-2",
          date: day("2026-08-04"),
          amount: 229000,
          paid: 50000,
          outstanding: 179000,
        }),
        row({
          lineId: "c",
          invoiceId: "i2",
          invoiceNumber: "FAC-2",
          date: day("2026-08-04"),
          amount: 26000,
          paid: 0,
          outstanding: 26000,
        }),
      ],
      [payment({})],
      NOW,
    );
    expect(c.loans.map((l) => l.invoiceNumber)).toEqual(["FAC-2", "FAC-1"]);
    expect(c.loans[0].lines).toHaveLength(2);
    expect(c.loans[0].outstanding).toBe(205000);
    expect(c.amount).toBe(394000);
    expect(c.paid).toBe(125000);
    expect(c.outstanding).toBe(269000);
    expect(c.lastPaymentDate).toBe(day("2026-09-21"));
    expect(c.idleDays).toBe(9);
    expect(c.tone).toBe("open");
  });

  it("flags a balance with no payment for more than the threshold as stale", () => {
    const [c] = buildLoanRegister([row({})], [payment({ date: day("2026-07-18") })], NOW);
    expect(c.idleDays).toBe(74);
    expect(c.tone).toBe("stale");
  });

  it("counts from the latest sale when nothing was paid", () => {
    const [c] = buildLoanRegister([row({ date: day("2026-09-27"), paid: 0 })], [], NOW);
    expect(c.lastPaymentDate).toBeNull();
    expect(c.idleDays).toBe(3);
    expect(c.tone).toBe("open");
  });

  it("stays open exactly at the threshold and turns stale the day after", () => {
    const at = day("2026-09-30") - STALE_AFTER_DAYS * 86_400_000;
    expect(buildLoanRegister([row({})], [payment({ date: at })], NOW)[0].tone).toBe("open");
    expect(buildLoanRegister([row({})], [payment({ date: at - 86_400_000 })], NOW)[0].tone).toBe(
      "stale",
    );
  });

  it("ignores voided payments, outbound ones and payments for other invoices", () => {
    const [c] = buildLoanRegister(
      [row({})],
      [
        payment({ id: "v", status: "voided", date: day("2026-09-29") }),
        payment({ id: "o", direction: "outbound", date: day("2026-09-29") }),
        payment({ id: "x", invoiceNumbers: ["FAC-99"], date: day("2026-09-29") }),
        payment({ id: "y", clientId: "c2", date: day("2026-09-29") }),
        payment({ id: "ok", date: day("2026-06-01") }),
      ],
      NOW,
    );
    expect(c.lastPaymentDate).toBe(day("2026-06-01"));
    expect(c.tone).toBe("stale");
  });

  it("marks a fully repaid customer settled however long ago they paid", () => {
    const [c] = buildLoanRegister(
      [row({ paid: 139000, outstanding: 0 })],
      [payment({ date: day("2026-01-15") })],
      NOW,
    );
    expect(c.tone).toBe("settled");
  });
});

describe("filterLoanRegister", () => {
  const customers = buildLoanRegister(
    [
      row({ lineId: "1", clientId: "small", clientName: "Small", outstanding: 10000 }),
      row({
        lineId: "2",
        clientId: "big",
        clientName: "Big",
        invoiceId: "i2",
        invoiceNumber: "FAC-2",
        outstanding: 90000,
      }),
      row({
        lineId: "3",
        clientId: "late",
        clientName: "Late",
        invoiceId: "i3",
        invoiceNumber: "FAC-3",
        outstanding: 5000,
      }),
      row({
        lineId: "4",
        clientId: "done",
        clientName: "Done",
        invoiceId: "i4",
        invoiceNumber: "FAC-4",
        paid: 139000,
        outstanding: 0,
      }),
    ],
    [
      payment({ id: "a", clientId: "small", date: day("2026-09-20") }),
      payment({ id: "b", clientId: "big", invoiceNumbers: ["FAC-2"], date: day("2026-09-20") }),
      payment({ id: "c", clientId: "late", invoiceNumbers: ["FAC-3"], date: day("2026-05-01") }),
      payment({ id: "d", clientId: "done", invoiceNumbers: ["FAC-4"], date: day("2026-04-01") }),
    ],
    NOW,
  );

  it("lists everyone who owes, stalled first, then by balance", () => {
    expect(filterLoanRegister(customers, "open").map((c) => c.clientName)).toEqual([
      "Late",
      "Big",
      "Small",
    ]);
  });

  it("lists only stalled customers under stale, and repaid ones under settled", () => {
    expect(filterLoanRegister(customers, "stale").map((c) => c.clientName)).toEqual(["Late"]);
    expect(filterLoanRegister(customers, "settled").map((c) => c.clientName)).toEqual(["Done"]);
  });
});
