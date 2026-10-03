import dayjs from "dayjs";

import type { LoanStatusRow } from "src/api";
import type { Payment } from "src/types/models";

// A loan with an outstanding balance and no repayment for longer than this is
// flagged on the loan register. Counted from the last payment applied to one
// of the customer's loans, or from the latest sale when nothing was paid yet.
// Loans have no repayment schedule (a migrated loan is due on its sale date),
// so time since the last payment is the only honest signal of a stalled loan.
// The loan export applies the same rule server-side: change
// LoanStaleAfterDays in db/loan_register.go with it.
export const STALE_AFTER_DAYS = 60;

export type LoanTone = "open" | "stale" | "settled";
export type LoanRegisterFilter = "open" | "stale" | "settled";

export interface RegisterLoan {
  invoiceId: string;
  invoiceNumber: string;
  date: number;
  lines: LoanStatusRow[];
  amount: number;
  paid: number;
  outstanding: number;
}

export interface RegisterCustomer {
  clientId: string;
  clientName: string;
  // Newest loan first.
  loans: RegisterLoan[];
  amount: number;
  paid: number;
  outstanding: number;
  lastPaymentDate: number | null;
  lastSaleDate: number;
  // Whole calendar days since the last payment (or the latest sale).
  idleDays: number;
  tone: LoanTone;
}

const daysBetween = (from: number, to: number) =>
  dayjs(to).startOf("day").diff(dayjs(from).startOf("day"), "day");

// Groups the loan-status rows (one per invoice line) into one entry per
// customer with their loans, totals and the date of the last payment applied
// to any of those loans. A payment counts when it is an inbound, non-voided
// payment of that customer naming one of their loan invoices, so a later
// cash purchase doesn't make a stalled loan look active.
export const buildLoanRegister = (
  rows: LoanStatusRow[],
  payments: Payment[],
  now: number,
  staleAfterDays: number = STALE_AFTER_DAYS,
): RegisterCustomer[] => {
  const byClient = new Map<string, { name: string; loans: Map<string, RegisterLoan> }>();
  for (const row of rows) {
    let client = byClient.get(row.clientId);
    if (!client) {
      client = { name: row.clientName, loans: new Map() };
      byClient.set(row.clientId, client);
    }
    let loan = client.loans.get(row.invoiceId);
    if (!loan) {
      loan = {
        invoiceId: row.invoiceId,
        invoiceNumber: row.invoiceNumber,
        date: row.date,
        lines: [],
        amount: 0,
        paid: 0,
        outstanding: 0,
      };
      client.loans.set(row.invoiceId, loan);
    }
    loan.lines.push(row);
    loan.amount += row.amount;
    loan.paid += row.paid;
    loan.outstanding += row.outstanding;
  }

  const customers: RegisterCustomer[] = [];
  for (const [clientId, client] of byClient) {
    const loans = [...client.loans.values()].sort((a, b) => b.date - a.date);
    const loanNumbers = new Set(loans.map((l) => l.invoiceNumber).filter(Boolean));
    let lastPaymentDate: number | null = null;
    for (const p of payments) {
      if (p.clientId !== clientId || p.direction !== "inbound" || p.status === "voided") continue;
      if (!(p.invoiceNumbers ?? []).some((n) => loanNumbers.has(n))) continue;
      if (lastPaymentDate === null || p.date > lastPaymentDate) lastPaymentDate = p.date;
    }
    const lastSaleDate = Math.max(...loans.map((l) => l.date));
    const amount = loans.reduce((n, l) => n + l.amount, 0);
    const paid = loans.reduce((n, l) => n + l.paid, 0);
    const outstanding = loans.reduce((n, l) => n + l.outstanding, 0);
    const idleDays = Math.max(0, daysBetween(lastPaymentDate ?? lastSaleDate, now));
    const tone: LoanTone =
      outstanding <= 0 ? "settled" : idleDays > staleAfterDays ? "stale" : "open";
    customers.push({
      clientId,
      clientName: client.name,
      loans,
      amount,
      paid,
      outstanding,
      lastPaymentDate,
      lastSaleDate,
      idleDays,
      tone,
    });
  }
  return customers;
};

// The customers one filter tab shows: open = everyone who still owes
// (stalled ones included, listed first), stale = only the stalled ones,
// settled = fully repaid. Owing lists are ordered stalled first, then by the
// largest balance; the settled list by the most recent payment.
export const filterLoanRegister = (
  customers: RegisterCustomer[],
  filter: LoanRegisterFilter,
): RegisterCustomer[] => {
  if (filter === "settled") {
    return customers
      .filter((c) => c.tone === "settled")
      .sort((a, b) => (b.lastPaymentDate ?? 0) - (a.lastPaymentDate ?? 0));
  }
  return customers
    .filter((c) => (filter === "stale" ? c.tone === "stale" : c.tone !== "settled"))
    .sort(
      (a, b) =>
        Number(b.tone === "stale") - Number(a.tone === "stale") || b.outstanding - a.outstanding,
    );
};
