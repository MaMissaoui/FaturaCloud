# FaturaCloud Audit — Findings & Execution Plan (2026-07-12)

Audit of the full repo (Go backend, db layer, frontend auth surface, Docker/CI) at
commit `5afd52a`. `go vet ./...` clean, `go test ./...` passing before any changes.

**Instructions for the executing model:**
- Work phase by phase, in order. Each numbered task = one focused change; commit per
  task or per phase using conventional commits (see CLAUDE.md "Committing" — no
  Claude attribution lines).
- After every phase: `go vet ./... && go test ./...` and `pnpm type-check` must pass.
- Add/extend Go tests where a task says so — `db/db_test.go` and `api/oidc_test.go`
  show the existing test style (in-memory/temp-file SQLite, table-driven).
- Update CLAUDE.md sections whose documented behavior a task changes.
- Tasks marked **[DECISION]** need user sign-off before implementing — skip them and
  list them at the end of the run instead of guessing.

---

## Severity overview

| # | Finding | Severity | Phase |
|---|---------|----------|-------|
| F1 | `h.db` read without `dbMu` in most handlers — data race with restore's DB swap | High | 1.1 |
| F2 | Failed restore leaves `h.db = nil` → app bricked, scheduler goroutine panic kills process | High | 1.2 |
| F3 | Delivery/order status changeable via PUT, bypassing stock logic; no server-side transition validation | High | 1.3 |
| F4 | Line items of shipped/delivered deliveries editable → inventory desync | High | 1.4 |
| F5 | Deactivated/deleted users keep API access until 24h JWT expiry | High | 2.1 |
| F6 | `updateUser`: role change silently ignored without displayName; role unvalidated; admin can lock themselves out | Medium | 2.2 |
| F7 | Login timing side-channel enables user enumeration; no input validation on user create | Medium | 2.3 |
| F8 | `CreateDelivery`/`replaceDeliveryLineItems` not transactional | Medium | 3.1 |
| F9 | `NextDeliveryNumber` = COUNT+1 → duplicate numbers after deletions | Medium | 3.2 |
| F10 | Swallowed DB errors in `users.go`; all insert errors mapped to "email already exists" | Medium | 3.3 |
| F11 | `decodeJSON` errors echoed verbatim to clients in newer handlers | Low | 3.4 |
| F12 | `0022_add_orders.down.sql` missing (every other migration has a down) | Low | 3.5 |
| F13 | No CI running tests/lint — only the tag-triggered Docker publish exists | Medium | 4.1 |
| F14 | No Content-Security-Policy header (JWT lives in localStorage → XSS is the main theft vector) | Medium | 5.1 |
| F15 | Backups/config written 0644 in 0755 dir — full financial DB world-readable on the host | Medium | 5.2 |
| F16 | Unmatched `/api/*` paths return index.html with 200; embedded-FS directory paths may render listings | Low | 5.3 |
| F17 | Any non-admin user can cascade-delete an organization or edit users' shared data | [DECISION] | 6.1 |
| F18 | Invoice/order totals are client-computed and stored untrusted | [DECISION] | 6.2 |

---

## Phase 1 — Data integrity & concurrency (P0)

### 1.1 Fix the `h.db` data race (F1)

`api/utility.go:swapDatabase` closes the DB and reassigns/nils `h.db` under
`h.dbMu.Lock()`, but only `auth.go`, `users.go`, and `utility.go` ever take the
lock. Every other handler file (`clients.go`, `invoices.go`, `orders.go`,
`deliveries.go`, `products.go`, `stock.go`, `tax_rates.go`, `organizations.go`)
reads `h.db` unsynchronized — a Go memory-model race, and during a restore a
request can hit a closed pool or nil pointer.

**Approach:** add a route-level middleware in `api/router.go` that holds
`h.dbMu.RLock()` for the duration of the request, applied inside the existing
`protected`/`adminProtected` wrappers:

```go
withDB := func(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        h.dbMu.RLock()
        defer h.dbMu.RUnlock()
        next.ServeHTTP(w, r)
    })
}
```

**Critical exclusions — RWMutex is not reentrant, wrapping these deadlocks:**
- `POST /api/backups/{name}/restore` and `POST /api/restore` call `swapDatabase`,
  which takes the **write** lock. Register these two routes *without* `withDB`.
- Handlers that already take `dbMu` internally (`login`, `me`, all of `users.go`,
  `triggerBackup`, `provisionOrSyncUser` via the public OIDC callback) must have
  their internal locking **removed** if the route gains `withDB` — or keep their
  internal locking and skip the middleware. Pick one convention: prefer
  middleware-only, delete the now-redundant `h.dbMu` calls from `auth.go`,
  `users.go`, `utility.go` handler bodies. `provisionOrSyncUser` (called from the
  *public* `oidcCallback`, no middleware) and `runScheduler` keep their own locks;
  `provisionOrSyncUser` currently takes the **write** lock — RLock is not enough
  there only if you rely on it for its check-then-insert atomicity, which it does
  (the SQL `ON CONFLICT` already handles races, so demote it to `RLock` when the
  middleware convention lands, or leave `Lock` — just never nest it under `withDB`).
- The login rate limiter and JWT parse must stay *outside* the lock (they don't
  touch the DB) — that's already true if `withDB` wraps only the innermost handler.

**Tests:** add an `api` test that runs a restore concurrently with a burst of GET
requests and passes under `go test -race ./api/`.

### 1.2 Make restore failure non-fatal (F2)

`api/utility.go:swapDatabase`: if `db.NewDatabase` fails after the copy (garbage
upload, failed migration), the handler returns 500 with `h.db = nil` and the
safety backup still on disk. Every later request panics on nil deref; worse,
`runScheduler` (a bare goroutine) will nil-deref and **kill the whole process**.

Fix, in `swapDatabase`:
1. **Validate before swapping:** open the candidate file read-only with
   `db.NewDatabase` (or a lighter `PRAGMA integrity_check` helper on a temp copy)
   *before* closing the live DB. Reject with 400 ("not a valid FaturaCloud
   database") if it fails.
2. **Roll back on reopen failure:** in the branch where `db.NewDatabase(h.dbPath)`
   fails after the copy, restore `safetyPath` over `h.dbPath` and reopen, exactly
   like the copy-failure branch above it already does. Only if *that* also fails
   may the handler give up — and then it should `log.Fatalf` rather than limp with
   `h.db = nil`.
3. Defensively nil-check `h.db` in `runScheduler` under its RLock.

**Tests:** upload a non-SQLite file via `POST /api/restore` in a handler test →
expect 400, and the pre-existing data still readable afterwards.

### 1.3 Enforce status transitions server-side; strip status from PUT (F3)

Stock movements are only generated in `db.UpdateDeliveryStatus`
(`db/delivery.go:220`). But:
- `db.UpdateDelivery` (`db/delivery.go:161`) COALESCEs a client-supplied
  `Status` straight into the row — `PUT /api/deliveries/{id}` with
  `{"status":"shipped"}` ships without touching stock, and `shipped→draft` via PUT
  then re-shipping via PATCH double-decrements stock.
- `db.UpdateOrderStatus` and `db.UpdateDeliveryStatus` accept *any* transition
  string; the legal transitions live only in the two frontend `STATUS_TRANSITIONS`
  maps (`src/routes/deliveries/details.tsx:49`, `src/routes/orders/details.tsx:61`,
  plus each page's separate Cancel button).

Fix:
1. Remove `Status` from `db.UpdateDeliveryRequest` and stop COALESCE-ing `status`
   in `UpdateDelivery`; same for `db.UpdateOrderRequest`/`UpdateOrder`
   (`db/order.go:57,176`). Status changes go through the PATCH endpoints only.
   Check `src/api/index.ts` / atoms / details pages don't send `status` in PUT
   bodies (they use the PATCH functions today — verify, then delete the field from
   the TS types too).
2. Add transition tables in the db layer and return `newValidationError` (→ 409
   via `writeMutationError`) for anything else:
   - deliveries: `draft→{shipped,cancelled}`, `shipped→{delivered,cancelled}`;
     `delivered`/`cancelled` terminal. (Matches UI: cancel offered for any
     non-cancelled, non-delivered status; note UI currently also allows
     `draft→cancelled`, keep that.) `shipped→cancelled` keeps its stock-restore
     branch; `delivered→cancelled` is now *rejected* — today the switch in
     `UpdateDeliveryStatus` would silently cancel without restoring stock.
   - orders: `draft→{confirmed,cancelled}`, `confirmed→{shipped,cancelled}`,
     `shipped→{delivered,cancelled}`; `delivered`/`cancelled` terminal — mirror of
     the UI map. Same-status no-ops may return the row unchanged.
3. `api/orders.go:updateOrderStatus` must switch from `writeInternalError` to
   `writeMutationError` if it doesn't already, so the 409 surfaces.
4. Also validate `status`/`state` on *create* paths (`CreateOrderRequest.Status`,
   invoice `state`) against the known value sets; default empty → `"draft"`.

**Tests (db layer):** table-driven transition matrix for both entities; assert
stock is decremented exactly once across `draft→shipped→cancelled→(reject re-ship)`.

### 1.4 Freeze line items once a delivery has shipped (F4)

`db.UpdateDelivery` replaces line items regardless of status — editing quantities
on a `shipped` delivery desyncs inventory from the movements already written, and
cancelling afterwards restores the *new* quantities.

Fix: in `UpdateDelivery`, when `req.LineItems != nil` and the current status is
`shipped` or `delivered`, return `newValidationError("cannot edit line items of a
%s delivery")`; route the handler error through `writeMutationError`
(`api/deliveries.go:updateDelivery` currently uses `writeInternalError` — change it).
Header fields (tracking number, notes) stay editable. The frontend details page
already disables most editing post-draft — verify and align if not.

---

## Phase 2 — AuthN/AuthZ hardening (P1)

### 2.1 Check `isActive` (and existence) on every request (F5)

`authMiddleware` (`api/middleware.go:28`) validates only the JWT signature.
Deactivating (`isActive=0`) or deleting a user leaves their token working for up
to 24h on every endpoint except `/api/auth/me`. For a self-hosted financial app
this is the wrong trade.

Fix: after parsing claims, do one indexed lookup
(`SELECT isActive FROM users WHERE id = ?`); missing row or `0` → 401. With
`SetMaxOpenConns(1)` and SQLite this is microseconds. Keep it inside the Phase 1.1
`withDB` RLock (i.e. do the check in a small middleware layered *inside* `withDB`,
or fold `withDB` + auth-user-check into one middleware — simplest is one combined
`protected` middleware: RLock → parse JWT → check isActive → next).

**Tests:** deactivate a user, assert an old-but-valid token now gets 401.

### 2.2 Fix `updateUser` (F6)

`api/users.go:114`:
- Role is only written when `DisplayName != ""` (they share one UPDATE) — a
  role-only change is silently dropped. Split into independent updates.
- Validate `role ∈ {"user","admin"}` on create and update; reject otherwise (400).
- Guard self-lockout: reject an admin demoting **or deactivating themselves**
  (mirror of the existing self-delete guard in `deleteUser`), and reject
  demoting/deactivating/deleting the **last active admin** (count active admins
  first).
- Return 404 when the target user doesn't exist (currently returns 200 with a
  zero-value user because the Get error is ignored).

**Tests:** role-only update persists; self-demotion 400; last-admin demotion 400.

### 2.3 Login/user-create input hardening (F7)

- `api/auth.go:login`: when the email lookup misses, run
  `bcrypt.CompareHashAndPassword` against a fixed dummy hash before returning 401,
  so response time doesn't reveal whether the account exists.
- `api/users.go:createUser`: enforce minimal validation — email contains `@` (or
  `net/mail.ParseAddress`), password ≥ 8 chars (constant, mention in the error),
  role validated per 2.2. Apply the same password rule to `updateUser` password
  changes. Do **not** add complexity rules beyond length.

---

## Phase 3 — Robustness & correctness (P1–P2)

### 3.1 Make delivery create/line-item replacement transactional (F8)

`db/delivery.go`: `CreateDelivery` inserts the header then calls
`replaceDeliveryLineItems`, which runs `DELETE` + N `INSERT`s + per-item lookups
directly on `d.DB`. A mid-loop failure loses line items with no rollback.
Refactor `replaceDeliveryLineItems` to take the `sqlExecer`-style tx (pattern
already used by `insertStockMovementTx` in `db/stock.go:12` — but it also needs
`Get`, so accept `*sqlx.Tx` or add a `Get` to a small interface), and wrap
`CreateDelivery` / the line-item branch of `UpdateDelivery` in transactions like
`CreateInvoice` does.

### 3.2 Delivery number generation (F9)

`db.NextDeliveryNumber` returns `COUNT(*)+1` → deleting a draft delivery makes the
next number collide with the newest one. Use
`MAX(CAST(SUBSTR(deliveryNumber, 5) AS INTEGER))+1` over rows matching the
`DEL-%` pattern (keep the `DEL-%04d` format), falling back to 1. There's no
UNIQUE constraint on `deliveryNumber` — that's tolerable; this fix just stops
manufacturing collisions.

### 3.3 Stop swallowing DB errors in `users.go` (F10)

Every `h.db.DB.Exec`/`Get` in `listUsers`, `createUser`, `updateUser`,
`deleteUser`, and `EnsureFirstAdmin` ignores its error. Handle them with
`writeInternalError`. In `createUser`, only map the insert error to 409 when it's
actually the UNIQUE(email) violation (match `sqlite.Error` code or
`strings.Contains(err.Error(), "UNIQUE constraint failed: users.email")` — the
modernc driver's error string); otherwise 500. `EnsureFirstAdmin` should
`log.Printf` failures.

### 3.4 Stop echoing decode errors (F11)

Newer handlers (`organizations.go`, `products.go`, `deliveries.go`, `orders.go`,
`stock.go`, `tax_rates.go`, `clients.go` — grep `err.Error()` after `decodeJSON`)
return raw Go JSON-decoder messages. Replace with the fixed string
`"invalid request body"` like `auth.go`/`utility.go` do.

### 3.5 Add `0022_add_orders.down.sql` (F12)

Only migration without a down file. Add one dropping `orderLineItems` then
`orders` (mirror `0025`'s style).

---

## Phase 4 — CI (P1)

### 4.1 Add a test/lint workflow (F13)

The only workflow (`.github/workflows/docker.yml`) builds images on version tags.
Add `.github/workflows/ci.yml` on `push` to `main` + `pull_request`:
- Go job: setup-go (1.26, cache on go.sum) → `go vet ./...` → `go test -race ./...`
- Frontend job: pnpm/action-setup + setup-node 22 (cache pnpm) →
  `pnpm install --frozen-lockfile` → `pnpm lint` (runs tsc + oxlint) → `pnpm build`
  (build needed because `main.go` embeds `dist/`; alternatively have the Go job
  `mkdir -p dist && touch dist/index.html` before vet/test — the embed just needs
  the dir. Prefer the real `pnpm build` in the frontend job and the stub in the Go
  job so the jobs stay parallel.)

---

## Phase 5 — Security hardening (P2)

### 5.1 Content-Security-Policy (F14)

The JWT lives in localStorage, so XSS = token theft. In `main.go:securityHeaders`
add a CSP for the SPA responses. Everything is same-origin by design (no CDNs);
Ant Design and @react-pdf need inline styles, and workers need blob: for PDF
rendering. Starting policy:

```
default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline';
img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self' *.sentry.io;
worker-src 'self' blob:; frame-ancestors 'none'; base-uri 'self'; object-src 'none'
```

**Must verify in the running app** (invoice PDF preview/download, logo upload,
Sentry if configured) — react-pdf renders via blob workers and data URIs; adjust
until the console shows no CSP violations. Only send the header on HTML/document
responses if it interferes with API responses (it won't, but keep it simple: set
it globally). Drop `*.sentry.io` from connect-src if VITE_SENTRY_DSN handling
makes it unnecessary for unconfigured builds — acceptable to keep unconditionally.

### 5.2 Tighten backup file permissions (F15)

- `main.go:41`: backup dir `os.MkdirAll(backupDir, 0700)`.
- `api/utility.go:writeBackupConfig`: `0600`.
- `db.Backup` uses `VACUUM INTO` (created with SQLite's default 0644): after a
  successful vacuum, `os.Chmod(destPath, 0600)`. Same for the safety copy and
  `copyFile` output (`os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)`).

### 5.3 SPA fallback hygiene (F16)

In `main.go:spaHandler`:
- Return JSON 404 (`{"error":"not found"}`) for any unmatched path starting with
  `/api/` instead of serving index.html with 200.
- When the opened path is a directory (`f.Stat().IsDir()`), fall through to the
  index.html fallback instead of letting `http.FileServerFS` render a directory
  listing for e.g. `/assets/`.

---

## Phase 6 — [DECISION] items — do not implement without user sign-off

### 6.1 Authorization model breadth (F17)

Today every authenticated user (role `user` included) can create/edit/delete
*everything* except users and backups — including `DELETE /api/organizations/{id}`,
which cascade-deletes all clients/invoices/orders/deliveries of that org. If the
intended model is "admins manage, users work", destructive org operations
(`DELETE /api/organizations/{id}`, maybe `PUT` of `invoice_number_counter`) should
move to `adminProtected`. This changes product behavior — ask the user.

### 6.2 Server-side validation of financial totals (F18)

Invoice `total/taxTotal/subTotal` and all line-item math are computed client-side
(decimal.js) and stored verbatim. A buggy/hostile client can store totals that
don't match line items. Recomputing server-side (integer cents) would be
defense-in-depth but duplicates the tax logic. Ask the user whether to add a
consistency check (reject if |client total − recomputed| > 0) or leave as-is.

---

## Explicitly reviewed and OK (don't "fix")

- OIDC flow (`api/oidc.go`): state/nonce/PKCE in an HMAC-signed, single-use,
  HttpOnly cookie; ID-token signature/audience verified; token delivered via URL
  fragment. Sound. `isHTTPS` trusting `X-Forwarded-Proto` only affects the state
  cookie's Secure flag — not worth coupling to TRUSTED_PROXIES.
- JWT alg confinement (`WithValidMethods(["HS256"])`), prod startup guards for
  JWT_SECRET/ADMIN_PASSWORD, login rate limiter + trusted-proxy XFF handling,
  body-size limits, `writeDBError`/`writeMutationError` error taxonomy,
  tax-rate delete guard, stock recompute-in-transaction pattern, Dockerfile
  (non-root, cross-compile, BuildKit secret for Sentry token), backup-name
  traversal guard (`filepath.Base`).
- Client-supplied record IDs (nanoids) on create — harmless; UNIQUE PKs reject dupes.

## Verification checklist (run after all phases)

1. `go vet ./... && go test -race ./...` — clean.
2. `pnpm lint && pnpm build` — clean.
3. Manual (or scripted with curl against `go run .`):
   - restore a garbage file → 400, app still serves data; restore a real backup → OK.
   - `PUT /api/deliveries/{id}` with `"status":"shipped"` → status unchanged.
   - `PATCH .../status` `shipped→draft` → 409; `draft→shipped` decrements stock once.
   - deactivate a user → their existing token 401s immediately.
   - role-only user update persists; demoting the last admin → 400.
   - unmatched `/api/nope` → JSON 404; `/assets/` → no directory listing.
   - invoice PDF preview + logo upload still work with CSP enabled (no console
     violations).
