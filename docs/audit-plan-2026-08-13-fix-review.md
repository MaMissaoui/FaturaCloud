# FaturaCloud Audit — Fix-Review of F48–F65 (2026-08-13)

Follow-up audit at commit `7a44363` — the F48–F65 fixes (#96, #97, #98, #99),
merged into `main` today, unreleased (no version bump past `v3.7.3` yet).
`git diff --stat 8106e27 HEAD` is 45 files / 2294 insertions, all of it that
one round of fixes and nothing else — no other commits landed on `main` in
between. That isolates the scope: this is not a calendar-driven rotation
back to security/i18n/UI-UX (07-18, 08-09 already covered that ground days
ago with no new code since to re-examine). It is a review of whether each
F48–F65 fix actually reached every place its own class of bug applies, since
that code has had no second pair of eyes on it yet.

**Method:** for every fix in the last round, grep/read every other call site
that fits the same pattern (same DB layer shape, same frontend Drawer/Modal
shape) and confirm the fix's own stated scope is complete. No new areas were
swept — see "Deliberately not re-examined" below for what this explicitly
skips and why.

**Instructions for the executing model:**
- This document is an audit, not a remediation — no code was changed while
  producing it.
- Continues the numbering at **F66**.
- When remediation is authorized: one feature branch + PR, conventional
  commits, no Claude attribution lines. `go vet ./... && go test -race ./...`
  and `pnpm lint && pnpm build` must pass. Update CLAUDE.md sections whose
  documented behavior a task changes.

**Status:** Open — 4 findings, none fixed yet. Awaiting go-ahead to open a
remediation branch/PR.

---

## Severity overview

| # | Finding | Area | Severity | Status |
|---|---------|------|----------|--------|
| F66 | `UpdateOrderStatus`/`UpdatePurchaseOrderStatus` never got F48's re-check pattern — bare pre-check-then-`UPDATE`, no transaction, no `RowsAffected` guard | Concurrency | Medium | Open |
| F67 | `client.ts`/`vendor.ts` (+ their Drawer forms) never got F56's rethrow-after-toast — a failed create/update still closes the drawer and discards the user's input | Frontend | Medium | Open |
| F68 | Four more `Create*` DB functions never got F61's empty-`req.ID` guard: `CreateDelivery`, `CreateOrder`, `CreateOrganization`, `CreateTaxRate` | Data integrity | Low | Open |
| F69 | Three of F48's four re-read-under-`tx` guards (`UpdateIncomingInvoiceState`, `UpdateInboundDeliveryStatus`, `UpdateDeliveryStatus`) have zero concurrent-test coverage — `go test -race` passing proves nothing about them | Test coverage | Low | Open |

---

## F66 — `UpdateOrderStatus`/`UpdatePurchaseOrderStatus` lack F48's TOCTOU guard

**Severity: Medium**

F48 closed the double-post/double-move race in every status-transition path
that touches GL or stock — `UpdateInvoiceState`, `UpdateIncomingInvoiceState`,
`UpdateInboundDeliveryStatus`, `UpdateDeliveryStatus` — by re-reading the
row's status **inside** the transaction, immediately after `Beginx()`, and
aborting if a concurrent request has already moved it out from under the
first request's stale pre-tx read. `reverseEntryTx`/`allocateAndFinalizeEntryTx`
carry the equivalent `RowsAffected`-checked `WHERE status = ...` guard on
their own `UPDATE`s.

`UpdateOrderStatus` (`db/order.go:300-314`) and `UpdatePurchaseOrderStatus`
(`db/purchase_order.go:312-329`) do neither. Both:

```go
current, err := d.GetOrder(orderID)          // pre-tx read
...
if status != current.Status && !orderStatusTransitions[current.Status][status] {
    return nil, newValidationError(...)      // decision made against stale read
}
_, err = d.DB.Exec(`UPDATE orders SET status = ? WHERE id = ?`, status, orderID)
                                              // no transaction, no WHERE status = ?,
                                              // no RowsAffected check
```

confirmed via `api/orders.go:86` / `api/purchase_orders.go:79` — neither
handler wraps the call in anything that would add the missing guard either.

Two concurrent `PATCH .../status` requests against the same order/purchase
order can both pass the transition check against the same stale `current`
value, and both writes succeed with no serialization between them — the
transition matrix (`orderStatusTransitions`/`purchaseOrderStatusTransitions`)
is bypassed for whichever request loses the race, silently. Unlike the four
paths F48 fixed, `orders.status`/`purchase_orders.status` alone don't post GL
or move stock directly (that happens on the linked delivery/receipt, which
already has its own F48 guard) — so this doesn't corrupt the ledger by
itself. But it can leave an order/PO in a status inconsistent with what
downstream code assumes (e.g. a PO that raced past `cancelled` into
`received` is exactly the state Finding B in the Phase 7 design note worried
about for receipts, just one level up the chain), and it's the same bug
class left un-swept on the two status-transition tables that weren't in
F48's own list.

**Suggested fix** (mirrors F48's existing shape exactly): begin a
transaction, re-`SELECT status FROM orders/purchase_orders WHERE id = ?`
under it, re-run the transition check against that value, `UPDATE ... WHERE
id = ? AND status = ?` with a `RowsAffected` check, commit.

---

## F67 — `client.ts`/`vendor.ts` Drawer forms don't rethrow on save failure

**Severity: Medium**

F56 fixed `product.ts`/`tax-rate.ts`: their save atoms now rethrow after
toasting, and `products/form.tsx`/`tax-rates/form.tsx`'s `handleSubmit`
catches that and only calls `handleClose()` on success — so a failed save
(duplicate SKU, FK violation, server 500) keeps the drawer open with the
user's input intact. `account.ts`/`journal.ts` already had this shape before
F56 (cited in CLAUDE.md as the precedent F56 was matching).

`client.ts` and `vendor.ts` are two more Drawer-form entities (per
CLAUDE.md: "vendors list with a `Drawer` form on the same page — no detail
route," and clients follow the identical pattern) that never got the same
treatment. Both setter atoms swallow the error instead of rethrowing:

```ts
// src/atoms/client.ts (and the identical shape in vendor.ts)
} catch (error) {
  console.error("Client operation failed:", error);
  if (!clientId) {
    message.error(t`Client creation failed`);
  } else {
    message.error(t`Client update failed`);
  }
}
```

and `src/components/clients/form.tsx:84-91` (`vendors/form.tsx` is the same
shape) has no try/catch around the call:

```ts
const handleSubmit = async (values: any) => {
  setSubmitting(true);
  await setClient(values);       // never throws, even on a failed save
  setClientId(null);
  navigate(location.pathname, { state: { clientModal: false } });
  form.resetFields();            // ...so this always runs
  setSubmitting(false);
};
```

A failed client/vendor create or update (e.g. a duplicate that the backend
rejects with a 409) shows the error toast, then unconditionally closes the
drawer and resets the form anyway — the user's typed input is gone and they
have to start over, with only a toast (which may have already scrolled away)
as a clue why. This is the exact bug class F56 fixed, just not reached for
these two entities.

**Suggested fix**: add `throw error;` after the `message.error(...)` calls in
both atoms' catch blocks (matching `account.ts`/`journal.ts`/`product.ts`),
and wrap `handleSubmit` in both forms in try/catch so only the success path
navigates/closes/resets — same shape as `products/form.tsx`.

---

## F68 — Four more `Create*` functions lack F61's empty-`req.ID` guard

**Severity: Low**

F61 added `if req.ID == "" { req.ID, _ = gonanoid.New() }` to `CreateInvoice`,
which had been missing it while every other `Create*` in `db/` already had
it. A full sweep of every `func (d *Database) Create*` in `db/*.go` against
that guard shows four more that still don't have it:

- `CreateDelivery` (`db/delivery.go:145`)
- `CreateOrder` (`db/order.go:168`)
- `CreateOrganization` (`db/organization.go:231`)
- `CreateTaxRate` (`db/tax_rate.go:111`)

None of the corresponding `api/*.go` handlers (`api/deliveries.go`,
`api/orders.go`, `api/organizations.go`, `api/tax_rates.go`) assign or guard
`req.ID` either — confirmed by grepping all four for `req.ID`/`gonanoid`,
zero hits. The frontend always generates a client-side nanoid before calling
these (`nanoid()` in the corresponding atom), so this isn't reachable through
the app's own UI today — same as F61's own framing, this is defense-in-depth
against any client that skips it (a scripted request, a future frontend bug,
a different client of the API), where `{"id": ""}` (or an omitted `id` field)
would otherwise insert an empty-string primary key.

**Suggested fix**: same one-line guard as F61, added to all four.

---

## F69 — Three of F48's four guards have no concurrent-test coverage

**Severity: Low (process/coverage gap, not a live bug)**

`db/concurrency_test.go` has five tests exercising the double-post/
double-reverse race class with actual concurrent goroutines:
`TestConcurrentUpdateInvoiceStateDoesNotDoublePost`,
`TestConcurrentVoidPaymentDoesNotDoubleReverse`,
`TestConcurrentCreatePaymentDoesNotOverpay`,
`TestConcurrentCloseFiscalYearPostsExactlyOneClosingEntry`,
`TestConcurrentDeleteJournalEntryDoesNotDeleteAPostedEntry`.

F48's own description names four paths that got the re-read-under-`tx`
guard: `UpdateInvoiceState`, `UpdateIncomingInvoiceState`,
`UpdateInboundDeliveryStatus`, `UpdateDeliveryStatus`. Only the first has a
concurrent test. Grepping every `*_test.go` in `db/` for
`UpdateIncomingInvoiceState`, `UpdateInboundDeliveryStatus`, and
`UpdateDeliveryStatus` finds only sequential (non-goroutine) call sites — no
`go func`/`sync.WaitGroup` anywhere near them.

`go test -race ./...` passing is not evidence these three guards are
correct; it's evidence nothing in the suite exercises the race they're
supposed to close. A regression that silently dropped one of these three
`tx`-scoped re-reads (e.g. during an unrelated refactor) would not be caught
by CI today.

**Suggested fix**: add `TestConcurrentUpdateIncomingInvoiceStateDoesNotDoublePost`,
`TestConcurrentUpdateInboundDeliveryStatusDoesNotDoubleMoveStock`, and
`TestConcurrentUpdateDeliveryStatusDoesNotDoubleMoveStock`
to `db/concurrency_test.go`, mirroring the existing invoice test's shape
(two goroutines racing the same transition, assert exactly one GL
entry/stock movement fan-out resulted). [[CLAUDE.md's own CI race-test
timeout budget note]] applies if these are added — check the 20-minute
`-race` budget still holds.

---

## Deliberately not re-examined

These were covered within the last audit cycle with no code changes since
to re-open them — re-litigating them here would be padding, not a finding:

- General security/dependency-currency/i18n sweep (2026-08-09, F42-F47) —
  no commits since besides the F48-F65 fixes themselves, which this document
  reviews directly.
- Frontend performance/UI-UX general sweep (2026-07-18, F19-F41) and the
  UI consistency plan (`docs/ui-consistency-plan.md`, all tiers 0-4 done) —
  same reasoning.
- F45 (English-only server error text), F63 (restore auto-migration), Peppol
  BIS non-support, lettrage/`reconciliation_groups` deferral, `users.tsx`'s
  module-level `searchAtom` — all standing, documented product decisions.
  Not re-flagged.
