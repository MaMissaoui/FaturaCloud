# Migrating a paper loan register — plan

Status: built (2026-09-30) — phases 1–3 in #439, #440, #441; the French cutover guide is `docs/guide-reprise-registre-credits.md`. The owner's answers are recorded under
"Decisions". Motivated by counter businesses such as ELECTRO MISSAOUI that run their credit sales from a
paper register (a customer, what was sold, the price, the deposit, then installments written in
by hand) and want the Cash Book's Loan status to take over from it.

## Goal

At a chosen **cutover date**, every loan in the paper register — open ones and, for the customer's history, settled ones (decision A) — exists in the app, with
the following, so that from the next morning every collection is recorded at the counter:

- the right customer (with CIN, phones and guarantor);
- what was sold;
- the original price;
- what was already paid;
- the remaining balance.

The paper register is archived, not continued in parallel.

## What must NOT happen

An old loan is not a new sale. Recording it through `POST /api/cash-sales` would be wrong in four
ways:

1. **Stock.** A Cash Book sale takes its stock out (`movesStock`, `db/invoice_stock.go`). The goods
   left the shop long ago, and the cutover stock count (the stock upload) already reflects that.
   Moving it again would drive stock negative.
2. **Revenue and VAT.** The sale posts revenue and output VAT on today's date. Those sales were
   (or should have been) declared in their own periods. Re-posting them inflates this year's P&L
   and VAT.
3. **Cash.** Recording the deposit and past installments as payments would add money to today's
   register that isn't in the till.
4. **Dates.** GL posting needs an open fiscal period covering the entry date
   (`allocateAndFinalizeEntryTx`), so a loan from 2024 can't be posted on its own date anyway.

So a migrated loan must be a **receivable brought forward**, not a sale.

## Proposed approach: an opening-balance loan import

### The accounting (kept minimal, decision C)

Real accounting is done outside the app, so the app only needs its own books to stay coherent.
There's no new account, and no accountant step.

Each migrated loan becomes an invoice marked as migrated (a new column,
`invoices.origin = 'opening'`, migration `0097`), dated with its **original sale date**.

- **An open loan** posts **one** entry, dated the **cutover date**, for its **outstanding**
  balance: Dr Accounts Receivable, Cr the organization's existing **Retained Earnings** account
  (`organizations.retainedEarningsAccountId`, which every chart template already sets). Later
  Cash Book collections post as usual (Dr register, Cr AR), so AR runs down to zero as the loan
  is paid.
- **A settled loan** (paid to date = total) posts **nothing**: it's history only.
- **Paid to date** is one payment record, method `opening`, dated the cutover, applied to the
  invoice as a whole, with **no GL entry**, since the invoice's entry already carries only the
  outstanding balance. It never touches the register, so the Cash Book's daily cash stays true.
- **No revenue, VAT, COGS, stock or cash** anywhere.
- **Lines:** product, quantity and gross amount, with **no tax rate**.
- **Visibility:** the paid-to-date payment shows as **"Opening balance"** in the Cash Book's
  Payment history and on the invoice's PaymentPanel, not as "Cash". It stays out of the register's
  day totals, which are filtered by the register account.
- **Loan status lists every migrated loan,** settled ones included. Its filter hides an invoice
  cleared by exactly one whole-invoice payment (a pure cash sale), which is exactly what a
  settled migrated loan looks like, so `origin = 'opening'` always counts as a loan there.
- **A fiscal year must cover the cutover date,** the same requirement as any Cash Book sale.

### Reports

Every reader of `invoices` in `db/` was checked: `dashboard.go`, `sales_reports.go`, `client.go`,
`payment.go`, `payment_invoice_lines.go`, `cash_movement_details.go`, `stock.go` and
`exchange_rate.go`.

- **Excluded** (sales the app never made, which would inflate revenue):
  - `GetRevenueByMonth` (Revenue Trend and the Dashboard's revenue chart);
  - `GetSalesByClient`, `GetSalesByProduct`, `GetTaxSummary`;
  - the Dashboard's revenue, top-clients and top-products figures (`GetDashboardData`);
  - the invoice PDF/Excel export and the e-invoice (a document for a sale the app never issued).
- **Included** (receivables are real):
  - `GetLoanStatus`, `GetReceivableAging` and the Dashboard's receivables;
  - the customer's invoice history, Payment history and the payment product panel.

### Opening invoices are frozen

A migrated loan lives in the ordinary invoice tables, and the invoice screens are open to the
`general` and `sales` roles. Without a rule, the normal paths would undo the design. For example,
cancelling then re-sending would reverse the opening entry and post Dr AR / Cr **Revenue** on the
original sale date: a date with no fiscal year, or in a closed period, or booking the revenue after
all.

**Rule: an opening invoice changes only through Cash Book collections, and disappears only
through batch undo.** Every other path refuses with a clear 409 ("migrated loan: …"), each with
a test:

| Path                                    | Behaviour                                                                         |
| --------------------------------------- | --------------------------------------------------------------------------------- |
| `UpdateInvoiceState`                    | Refused, except the automatic `sent → paid` when a Cash Book collection clears it |
| `UpdateInvoice` (lines, totals, client) | Refused                                                                           |
| `DeleteInvoice`                         | Refused. Batch undo is the only removal                                           |
| `DuplicateInvoice`                      | Allowed, but the copy is an ordinary invoice: `origin` is never copied            |
| Invoice export / e-invoice              | Refused. A loan statement (the Loan status export) is the document for it         |
| `POST /api/payments` on it              | Allowed (e.g. a bank transfer settling the balance); posts like any payment       |
| Voiding the opening payment             | Refused. It isn't a real receipt; batch undo removes it                           |
| Cash Book line collections              | Allowed, unchanged                                                                |

The invoice detail page shows a "Migrated from the paper register (ref …)" banner, with the
editing controls disabled.

After the import, the Cash Book treats a migrated loan exactly like one of its own:

- it appears in Loan status;
- the cashier settles it line by line;
- it turns Paid when cleared.

### The tool: "Import loan register" (admin only)

The tool follows the same shape as the stock count upload (`db/mass_data_stock.go`): an Excel
template, then upload. It adds a **dry run**, because it creates financial records.

- **Download template.** One sheet, **one row per loan line**. Columns:
  - Loan ref*: the paper book's page/line, e.g. `B2-P045-3`.
  - Sale date*.
  - Customer name*, CIN, Phone, Phone 2, Address, Guarantor.
  - Product*: SKU, exact name, or free text.
  - Quantity.
  - Line amount*: gross, as written.
  - Paid to date: per loan, on its first row.
  - Last payment date.
  - Note.
  - A second sheet explains each column, with examples, in French.
- **Upload with dry run (the default).** Nothing is written. The report shows:
  - loans and lines found, and customers matched vs. to be created;
  - the **total outstanding**, grand total and per customer, for comparison with the paper book;
  - every problem, row by row: missing ref/date/amount, paid > total, a CIN on two different
    names, a customer name matching two clients, a product not found (the line is imported as
    free text, flagged not failed), and a date after the cutover.
- **Confirm import.** The same file is processed **in one transaction**: all or nothing, so a
  half-imported register can't happen.
  - Every created record carries an `importBatchId`.
  - The Loan ref is stored on the invoice (e.g. `buyerReference`). A re-upload skips loans whose
    ref already exists, so a corrected file can be uploaded again safely.
- **Undo a batch.** The safety net for a bad first attempt. It's refused once any of the batch's
  loans has received a payment in the app. Otherwise the loans and their paid-to-date records
  are deleted, and their opening entries are reversed (never edited). The customers the batch
  created are deleted only if nothing references them any more. A customer used by a new sale
  since, or named on a reversed opening entry's AR line, is kept, and a re-import matches it.
  (As built in phase 2: this keeps later work intact instead of refusing the undo.)
- **Customer matching,** in order:
  1. CIN;
  2. then exact name **and** phone together (a phone alone isn't enough, since families share
     one);
  3. then exact name alone, only when exactly one client has it;
  4. otherwise a new client with the Cash Book fields filled.

  An ambiguous match, or a phone that matches a client with a different name, fails the row in
  the dry run rather than guessing: the admin fixes the sheet or merges the clients first.

- **Invoice numbers:** the paper Loan ref itself (e.g. `B2-P045-3`) becomes the invoice number,
  so the migrated loans don't consume the organization's `FAC-` counter or leave gaps in it, and
  the cashier sees the same reference as in the book. (A separate `ANC-` series would need a
  second invoice counter, since invoice numbering lives on the organizations table.)

### The cutover procedure (the part that isn't code)

1. **Pick the cutover date,** e.g. the first of a month, a day the shop is closed.
2. **Customers:** optionally import them first with the existing Clients Excel import. The loan
   import can also create them.
3. **Transcribe** the loans into the template, open and settled. This is the real effort, and
   it can be split across people by book or page range. Open loans first: they're what the
   counter needs on day one, and settled ones can follow in a second batch.
4. **Dry run.** Compare the report's grand total and per-customer totals with the paper book.
   Fix, then repeat.
5. **Stock count:** the stock upload, the same day.
6. **Import** after the manager's review. Export Loan status to Excel and tick it against the
   paper book, customer by customer.
7. **Collections between transcription and import:** either update the sheet before the import,
   or record them in the Cash Book right after it.
8. **Freeze and archive the paper register,** noting the cutover date on its last page.

## Decisions (owner, 2026-09-30)

- **A. Scope: open and settled loans.** Settled loans give the customer's history ("this customer
  always pays"). They post no GL entry, and Loan status lists them as settled. The import can
  take them in the same file or a later batch.
- **B. Past installments: one "Paid to date" amount per loan.** It's the simplest to transcribe.
  - **Consequence for multi-item loans:** the amount is spread across the lines
    (`allocateInvoiceLines`), so every line shows a partial balance, and the counter settles one
    line per payment, capped at that line's outstanding.
  - Most paper loans are one item, so this matters little. If it bothers the cashiers, a "pay the
    whole loan" collection in the Cash Book is the follow-up, useful beyond the migration too.
- **C. Accounting: as simple as possible.** Real accounting is done elsewhere. See "The
  accounting" above: one entry per open loan against Retained Earnings, nothing for settled
  loans, and no new account.
- **D. Reports: exclude migrated loans** from the sales analytics, Tax Summary and the
  Dashboard's revenue figures.

## Out of scope, possibly later

- **Installment schedules:** agreed monthly amounts and due dates, overdue alerts. The app has no
  schedule today; a loan just has an outstanding balance. If the paper register has them, the
  import stores them in the Note, and a schedule becomes a separate feature.
- **Photographing the pages** and having a transcription assistant fill the template. It's
  possible, but it sends customer CINs and debts to an external service (a privacy decision),
  and handwriting needs human checking anyway. The dry run is the safeguard either way.

## Build estimate

Four phases, each its own PR:

1. **Migration and posting:** migration `0097` (`invoices.origin`, `importBatchId`), plus the
   posting path for opening invoices and payments (outstanding against Retained Earnings),
   the report exclusions, and the frozen-invoice guards. Go tests: GL balances, Loan status, each
   excluded report, and one test per path in the frozen-invoice table.
2. **Server-side import:** template export, dry run, all-or-nothing import, idempotency by Loan
   ref, and batch undo. Tests: matching, ambiguity, re-upload.
3. **Screen:** download, upload, dry-run report, confirm, and batch history with undo. Browser
   verification with a 50-loan sample sheet.
4. **Docs and a French user guide** for the cutover procedure: `docs/guide-reprise-registre-credits.md`.
