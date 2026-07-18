# FaturaCloud Re-Audit — Security, UI, Performance, Upgrades (2026-07-18)

Follow-up audit at commit `805f2a6` (v1.2.6). The 2026-07-12 plan (F1–F18) is fully
implemented and verified in code — this plan continues the numbering at F19 and
covers the ground the first audit deliberately skipped: frontend performance,
UI/UX defects, and dependency upgrades, plus residual security items.

**Instructions for the executing model:**
- Work phase by phase. One feature branch + PR per phase (never push to main
  directly). Conventional commits, no Claude attribution lines (CLAUDE.md
  "Committing").
- After every phase: `go vet ./... && go test -race ./...` and
  `pnpm lint && pnpm build` must pass.
- Update CLAUDE.md sections whose documented behavior a task changes.
- Tasks marked **[DECISION]** need user sign-off — skip them and list them at the
  end of the run instead of guessing.
- Baseline metrics to beat (from `pnpm build` at `805f2a6`):
  `dist/assets/index-*.js` = **3,714 kB (1,189 kB gzip)** single chunk, plus a
  1,370 kB pdf worker; 14 MB of `.map` files shipped inside the Docker image and
  served publicly.

---

## Severity overview

| # | Finding | Area | Severity | Phase |
|---|---------|------|----------|-------|
| F19 | `decodeJSON` has no request-body size cap on any JSON endpoint | Security | Medium | 1.1 |
| F20 | Invoice `state` is unvalidated free text; settable via PUT; `paid`-delete guard bypassable | Security | Medium | 1.2 |
| F21 | Source maps (14 MB) embedded in the binary and served publicly | Security/Perf | Medium | 1.3 |
| F22 | `jotai-devtools` pulls `jsondiffpatch` <0.7.2 (GHSA-33vc-wfww-vjfv, moderate XSS) | Security | Low | 1.4 |
| F23 | JWT lacks `iss`/`aud` claims; `lastLoginAt` update error ignored | Security | Low | 1.5 |
| F24 | No per-account login throttle (per-IP only) | Security | Low | 1.6 |
| F25 | HSTS header absent | Security | [DECISION] | 6.1 |
| F26 | JWT in localStorage (vs httpOnly cookie) | Security | [DECISION] | 6.2 |
| F27 | No code splitting — every route, react-pdf, pdfjs, dnd-kit in one 3.7 MB chunk | Performance | High | 2.1 |
| F28 | All 8 list tables render every row (`pagination={false}`) | Performance | Medium | 2.2 |
| F29 | Organizations list `SELECT *` ships every logo BLOB on every auth change | Performance | Medium | 2.3 |
| F30 | All 11 antd + dayjs locale bundles imported eagerly | Performance | Low | 2.4 |
| F31 | Invoice state vocabulary mismatch — raw untranslated "cancelled" tag, dead "Confirmed" filter, missing "Sent" filter | UI (bug) | Medium | 3.1 |
| F32 | Module-scope `` t`…` `` strings frozen at import — stale after locale switch | UI (bug) | Low | 3.2 |
| F33 | Overdue invoices visually indistinguishable | UI (UX) | Low | 3.3 |
| F34 | Clickable table rows not keyboard-accessible | UI (a11y) | Low | 3.4 |
| F35 | ~50 `any`-typed sites across atoms/routes | Maintainability | Low | 3.5 |
| F36 | Routine minor/patch dependency bumps (npm + Go) | Upgrades | — | 4.1 |
| F37 | react-pdf 9→10 + pdfjs-dist 4→6 (coupled) | Upgrades | — | 4.2 |
| F38 | react-router 7→8 | Upgrades | — | 4.3 |
| F39 | antd 5→6 | Upgrades | [DECISION] | 6.3 |
| F40 | TypeScript 5.9→7 (tsgo), @babel/core 7→8 | Upgrades | — | 4.4 |
| F41 | Add `govulncheck` + `pnpm audit` to CI | Upgrades/CI | — | 4.5 |

---

## Phase 1 — Security (P1)

### 1.1 Cap JSON request bodies (F19)

`api/helpers.go:decodeJSON` reads `r.Body` unbounded. The only size limit
anywhere is the 256 MB multipart cap on `POST /api/restore`
(`api/utility.go:156`); every other endpoint accepts arbitrarily large JSON,
bounded only by the 60 s ReadTimeout — an easy memory-exhaustion vector for any
authenticated user, and for the unauthenticated `POST /api/auth/login`.

Fix in `decodeJSON`: change the signature to take `w http.ResponseWriter`, wrap
with `r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)` before decoding.
**Sizing constraint:** organization create/update carries the logo as base64
inside JSON (BLOB column, CLAUDE.md "Database"), so the cap must comfortably fit
a multi-MB logo — use `10 << 20` (10 MB) globally rather than a tight 1 MB.
Detect `*http.MaxBytesError` in callers' error path (or centrally) and return
413 `"request body too large"` instead of the generic 400. Update every
`decodeJSON` call site (mechanical — grep `decodeJSON(r,`).

**Tests:** POST an 11 MB JSON body to a protected endpoint → 413; a 1 MB org
update with logo still succeeds.

### 1.2 Validate invoice state; strip it from PUT (F20)

Unlike orders/deliveries (fixed in F3), invoice state is still a free-text
write-anything field:
- `db/invoice.go:237` — `state = COALESCE(?, state)` in `UpdateInvoice`: PUT can
  set any state.
- `db/invoice.go:282 UpdateInvoiceState` — PATCH accepts any string, no
  validation.
- `db/invoice.go:298` — DeleteInvoice guards only `state == "paid"`; a client
  can PATCH `paid→draft` then DELETE, silently destroying a paid invoice.

Fix:
1. Define the canonical state set once in `db/invoice.go` (see F31 for which
   names — **do 3.1's decision first**, they must land together):
   `draft`, `sent`, `paid`, `cancelled`.
2. Validate on create (`CreateInvoiceRequest.State`, default empty → `"draft"`)
   and in `UpdateInvoiceState`; unknown value → `newValidationError` → 409 via
   `writeMutationError` (pattern from `db/order.go`).
3. Remove `State` from `UpdateInvoiceRequest` and drop the COALESCE — state
   changes go through PATCH only, matching the orders/deliveries convention.
   Verify the frontend never sends `state` in PUT bodies (`src/atoms/invoice.ts`
   builds `updateData` from the form — strip it there too; the details form
   holds state but must not submit it via save).
4. Invoices legitimately move freely between states (paid→sent for a bounced
   payment is real), so do **not** add a transition matrix — just set
   membership. The delete guard stays as is; with PATCH validated the
   paid→draft→delete path still exists but is now an explicit two-step user
   action, which is acceptable for this product (same trust level as editing).

**Tests (db layer):** create with `state:"garbage"` → error; PATCH to each
canonical state OK, to `"confirmed"` → validation error; PUT with `state` field
→ state unchanged.

### 1.3 Stop shipping source maps (F21)

`vite.config.ts` sets `sourcemap: true`; `pnpm build` writes ~14 MB of `.map`
files into `dist/`, the Dockerfile embeds all of `dist/` (`//go:embed all:dist`
via `COPY --from=frontend /app/dist`), so every deployment serves its full
original TypeScript source at `/assets/*.js.map` and the binary carries 14 MB of
dead weight.

Fix: keep generating maps (Sentry needs them at build time) but don't ship them:
- In `vite.config.ts` set `build.sourcemap: "hidden"` (drops the
  `//# sourceMappingURL` comment) **and** add
  `sourcemaps.filesToDeleteAfterUpload: ["./dist/**/*.map"]` to the
  `sentryVitePlugin` options — the plugin deletes them after upload, and also
  deletes them when no auth token is set (verify this; if the plugin skips
  deletion without a token, add an unconditional `rm` in the Dockerfile frontend
  stage after `pnpm build` instead).
- Confirm: after a local `pnpm build`, `ls dist/assets/*.map` is empty and the
  Go binary shrinks accordingly.
- CLAUDE.md Docker section: note that source maps exist only inside Sentry.

### 1.4 Bump jotai-devtools past the jsondiffpatch advisory (F22)

`pnpm audit --prod` reports one moderate: `jsondiffpatch <0.7.2` XSS via
`jotai-devtools 0.14.0`. The devtools are gated behind `import.meta.env.DEV`
(`src/app.tsx:78`) so production bundles never include it, but fix the hygiene:
bump `jotai-devtools` to a release depending on jsondiffpatch ≥0.7.2 (or add a
pnpm override), and move it (plus its style import) to `devDependencies` where
it belongs. Re-run `pnpm audit --prod` → clean.

### 1.5 JWT hygiene: iss/aud claims, lastLoginAt error (F23)

- `api/auth.go:issueTokenWithProvider` sets only `exp`/`iat`. Add
  `Issuer: "faturacloud"` and `Audience: ["faturacloud"]` to
  `RegisteredClaims`, and enforce with `jwt.WithIssuer(...)`,
  `jwt.WithAudience(...)` parse options in `api/middleware.go:37`. This
  invalidates outstanding tokens once — users re-login, acceptable; mention in
  the release notes/commit body.
- `api/auth.go:153` ignores the `UPDATE users SET lastLoginAt` error — log it
  (`log.Printf`), don't fail the login.

**Tests:** token without `iss`/`aud` (issue with the old claim set) → 401.

### 1.6 Per-account login throttle (F24)

`checkLoginRate` keys on IP only; a botnet rotating IPs gets unlimited attempts
against one account. Add a second bucket map keyed on the (lowercased) email
with the same 10/min window, checked after the IP bucket in `login`. Same sweep
goroutine. Skip when the email is empty. Do **not** change the response between
"rate-limited because IP" and "because account" (both 429 with the same
message) — no enumeration signal.

**Tests:** 11 attempts on one email from distinct IPs (spoof via
`httptest.NewRequest` RemoteAddr) → 11th gets 429.

---

## Phase 2 — Performance (P1)

### 2.1 Route-level code splitting (F27) — the big one

`src/app.tsx` imports every route eagerly, so the login page downloads
react-pdf, pdfjs-dist, @react-pdf/renderer, dnd-kit, and every settings page:
one 3,714 kB chunk (1,189 kB gzip).

Fix:
1. Convert all route imports in `src/app.tsx` to `React.lazy(() => import(...))`
   and wrap `<Routes>` in `<Suspense fallback={<Loading />}>` (the `Loading`
   component already exists). At minimum split: `invoices/details` (pulls the
   entire PDF stack — biggest win), `orders/details`, `deliveries/details`,
   all settings pages, `organizations/*`. Keep `login` and `auth-callback`
   eager (tiny, first paint).
2. `TaxRateForm` is imported eagerly in app.tsx for nested routes — lazy it too.
3. Check the pdf worker: `src/routes/invoices/details.tsx:64-67` configures
   `pdfjs.GlobalWorkerOptions` via `new URL(..., import.meta.url)` — after
   splitting, confirm the 1.37 MB worker is only fetched when the invoice
   details route loads (network tab), not on app boot.
4. Optionally add vendor chunking (Vite 8 = rolldown:
   `build.rolldownOptions.output.codeSplitting` or `advancedChunks`) to separate
   `antd` + `react` from app code for better caching — only if the lazy() split
   alone doesn't get the entry chunk under target.
5. **Acceptance:** entry JS ≤ ~1,500 kB raw / ~450 kB gzip; PDF stack in its own
   chunk loaded only on invoice details; `pnpm build` chunk-size warning gone or
   only on the lazy PDF chunk. Record before/after sizes in the PR description.
6. Watch out: lingui locale chunks and `dynamicActivate` already code-split
   per-locale — don't regress that.

### 2.2 Paginate the list tables (F28)

Every list page passes `pagination={false}` and renders all rows
(`src/routes/{invoices/index,orders,clients,deliveries,products,organizations/index,settings/tax-rates,settings/users}.tsx`).
With a few thousand invoices this is thousands of DOM rows with per-row
Dropdown/Tag components.

Fix: replace `pagination={false}` with
`pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}`
on the 8 list pages (keep `pagination={false}` on the two *details*-page inner
tables — line items are legitimately short). Client-side pagination over the
already-fetched array is fine at this product's scale; do **not** build
server-side pagination.

### 2.3 Keep logo BLOBs out of the organizations list (F29)

`db/organization.go:81` — `SELECT * FROM organizations` includes the `logo`
BLOB for every org; `[]byte` marshals as base64 into the JSON list response.
The list is re-fetched on every `currentUser` change (`src/app.tsx:165-169`),
so multi-MB logos are re-downloaded on each login/refresh for orgs the user
isn't even looking at.

Fix:
1. In `GetOrganizations`, select explicit columns excluding `logo` (keep
   `GetOrganization` (single) returning it — the invoice PDF and settings pages
   need it).
2. Audit frontend uses of the *list* logo: grep `organizationsAtom` /
   `organizations` consumers for `.logo` (org picker in `src/layouts/base.tsx`,
   organizations list page card/avatar). Anywhere the list rendered a logo,
   fetch the single org (already-existing `GET /api/organizations/{id}`) or drop
   the thumbnail. The active-org flow (`organizationAtom` keyed by
   `organizationIdAtom`) already fetches the single org — verify, since that's
   where the PDF logo comes from.
3. CLAUDE.md: note the list endpoint omits `logo`.

### 2.4 Lazy locale data (F30) — optional, do last

`src/app.tsx` statically imports 11 antd locales + 11 dayjs locales. They're
small (~5-10 kB each) but pure dead weight for 10 of 11 users. Convert the antd
locale switch to a `useEffect` + dynamic `import()` keyed on `locale` (mirroring
`dynamicActivate` for lingui), defaulting to `enUS` while loading. Skip if it
fights the lazy() work in 2.1 — this is a nice-to-have.

---

## Phase 3 — UI (P2)

### 3.1 Unify the invoice state vocabulary (F31) — visible bug

Three components disagree on what invoice states exist:
- Data/docs: `draft | sent | paid | cancelled` (CLAUDE.md; real rows contain
  `"cancelled"`, which rendered in the list as a raw lowercase
  untranslated "cancelled" tag).
- `src/components/invoices/state-select.tsx`: offers `draft/sent/paid/void`,
  colors keyed the same — `"cancelled"` falls through to an unstyled raw string.
- `src/routes/invoices/index.tsx:48 stateFilter`: offers
  `Draft/Confirmed/Paid/Void` — "Confirmed" matches nothing (order vocabulary
  leaked in), "Sent" is missing entirely, so the filter can't select sent
  invoices.

Fix (together with 1.2, same PR):
1. **[DECISION — resolved default]** canonical set is
   `draft | sent | paid | cancelled` (matches CLAUDE.md and existing data).
   "void" is renamed: add a migration
   `UPDATE invoices SET state='cancelled' WHERE state='void'`, plus one mapping
   any other legacy garbage to `draft`. If the user prefers "void" as the
   canonical name, invert the migration — ask only if they've expressed a
   preference; otherwise proceed with `cancelled`.
2. Single source of truth: `INVOICE_STATES` in `src/types/` (value + color),
   consumed by `state-select.tsx`, the list-page filter, and the details page.
   Labels translated with `<Trans>`/`t` **inside components** (see 3.2).
3. Filter list derives from `INVOICE_STATES` — Draft/Sent/Paid/Cancelled.
4. `stateColor` gains `cancelled: "volcano"`; remove the `@ts-expect-error` at
   `state-select.tsx:54` by typing the record properly.

### 3.2 Fix module-scope translation strings (F32)

`` t`…` `` evaluated at module scope resolves once at import with whatever
locale is active then — switching language leaves those strings stale.
Confirmed instance: `stateFilter` (`src/routes/invoices/index.tsx:48`). Sweep:
`grep -rn '^const.*t\`\|^  *text: t\`' src/` and audit every `` t`…` `` outside
a component/hook body (also check `src/routes/orders.tsx`, `deliveries.tsx`,
`products.tsx` for copied patterns). Move them into the component (the
`useLingui()` hook re-renders on locale change; plain `t` inside a component
body re-evaluates too since the page re-renders).

### 3.3 Overdue invoice highlighting (F33)

Invoices list shows due date but nothing distinguishes an overdue unpaid
invoice. In `src/routes/invoices/index.tsx` Due date column render: when
`dueDate < Date.now()` and `state` is `sent`, render the date in
`antd` danger color (`Typography.Text type="danger"`) with a tooltip
"Overdue". Cheap, high-value for an invoicing product.

### 3.4 Keyboard access for clickable rows (F34)

List rows navigate via `onRow onClick` only. The row's first cell already
contains a real `<Link>` (invoices) — replicate that pattern on the other list
pages that lack a link in any cell (check orders, deliveries, clients,
products), so keyboard users can tab to each record. Don't try to make `<tr>`
focusable — a link per row is the accessible-and-simple fix.

### 3.5 Type the atoms and routes (F35)

`src/atoms/*.ts` and list/detail routes use `any` heavily (~50 sites; worst:
`invoice.ts` 13, `order.ts` 12). `src/types/` already exists and `src/api`
functions are typed. Mechanical cleanup: give `invoicesAtom` et al. their real
element types, type the `(a, b)` sorter params and `record` in table columns.
No behavior change; `pnpm lint` (tsc) is the gate. Do this **last** in the
phase — it touches many files and merges worst.

---

## Phase 4 — Upgrades (P2)

Ordering matters: 4.1 first (small diffs, catches tooling breakage early), then
4.2/4.3 independently, 4.4 last. antd 6 is a [DECISION] (6.3). One PR each for
4.2/4.3; 4.1 can be a single PR.

### 4.1 Routine bumps (F36)

npm (all non-breaking per semver): `@ant-design/icons` 6.3.2, `jotai` 2.20.2,
`@lingui/*` 6.5.0 (all six packages together), `@sentry/react` 10.66,
`@sentry/vite-plugin` 5.4, `vite` 8.1.5, `@vitejs/plugin-react` 6.0.3,
`oxlint` 1.74, `oxfmt` 0.59 (re-run `pnpm format` after — formatter output may
shift), `tsx` 4.23, `nanoid` 6.0.0 (pure-ESM already; API unchanged — verify
`import { nanoid }` still resolves).

Go: `coreos/go-oidc/v3` v3.20.0, `golang.org/x/crypto` v0.54.0,
`modernc.org/sqlite` v1.54.0 (`go get -u <mod>` each, then `go mod tidy`).

Gate: `pnpm lint && pnpm build`, `go vet && go test -race ./...`, then boot
`go run .` + `pnpm dev` and click through login → invoices → PDF preview.

### 4.2 react-pdf 10 + pdfjs-dist 6 (F37)

`react-pdf` 9.2.1→10.x requires `pdfjs-dist` ≥5 — upgrade both together
(pdfjs-dist 4.8.69→6.1.x picks up upstream PDF.js security fixes). Read the
react-pdf v10 release notes first. Known touch points:
- `src/routes/invoices/details.tsx:60-67`: the CSS import paths
  (`react-pdf/dist/esm/...` may lose the `esm/` segment) and the
  `pdfjs.GlobalWorkerOptions.workerSrc = new URL("pdfjs-dist/build/pdf.worker.min.mjs", import.meta.url)`
  wiring — v10 may expect `.mjs` worker via `import.meta.url` differently.
- `vite.config.ts optimizeDeps.include: ["pdfjs-dist"]` — likely still needed.
- `@react-pdf/renderer` (4.5.1, the *generator*) is a separate package — leave
  it unless its latest minor bumps cleanly.
Verify with `/verify`-style manual check: invoice PDF **preview renders** and
**download works**, no console/CSP violations (the CSP in `main.go:131` was
tuned to the current worker behavior — wasm/data:/blob: — retest it).

### 4.3 react-router 8 (F38)

7.18→8. The app uses only `BrowserRouter/Routes/Route/Link/Navigate/useNavigate/
useLocation/useParams` (declarative mode, no loaders) — v8's breaking changes
concentrate in framework/data mode, so this should be near-mechanical. Read the
v8 upgrade guide, bump, fix types, click through all routes incl. the SPA
fallback paths (`/invoices/{id}/pdf` deep link, hard refresh on a nested route).

### 4.4 TypeScript 7 / Babel 8 (F40)

- `typescript` 5.9→7.0 (tsgo, the native port): try it — `pnpm lint` runs plain
  `tsc --noEmit` so the swap is low-risk. If `tsc` flags new strictness errors
  or tooling (oxlint type-aware rules, lingui) misbehaves, fall back to the
  latest 6.x, and only then to staying on 5.9. Whatever lands, pin it.
- `@babel/core` 7→8: **hold** unless `@lingui/babel-plugin-lingui-macro`,
  `babel-plugin-macros`, and `jotai-babel` all declare Babel 8 support — check
  their READMEs/releases; if any is missing, note it and skip (dev-only dep,
  zero runtime impact).

### 4.5 Vulnerability scanning in CI (F41)

`.github/workflows/ci.yml`: add `govulncheck ./...`
(`golang/govulncheck-action` or `go run golang.org/x/vuln/cmd/govulncheck@latest`)
to the Go job, and `pnpm audit --prod --audit-level high` to the frontend job
(`high` threshold so a moderate dev-chain advisory doesn't block releases;
F22 clears the current one anyway).

---

## Phase 6 — [DECISION] items — do not implement without user sign-off

### 6.1 HSTS (F25)

The app terminates HTTP; TLS comes from the user's reverse proxy (NPM per
`docs/oidc-sso.md`). Sending `Strict-Transport-Security` unconditionally would
break plain-HTTP LAN deployments. Options: (a) leave it to the proxy (current,
documented nowhere), (b) send it only when the request arrived via a trusted
proxy with `X-Forwarded-Proto: https`. Recommend (a) + a docs note in
CLAUDE.md/README ("set HSTS at your proxy"). Ask the user.

### 6.2 Cookie-based auth (F26)

JWT in localStorage is XSS-stealable; the CSP added in F14 is the mitigation.
Moving to an httpOnly SameSite=Strict cookie + CSRF header would close the
class but touches login, OIDC callback (currently delivers the token via URL
fragment), the fetch wrapper, and logout semantics. Real work, real payoff —
but not a quick fix. Present the trade-off; don't start it inside this plan.

### 6.3 antd 6 (F39)

antd 5.26→6.5 is the largest upgrade: theming (`ConfigProvider` token/algorithm
API changed), component deprecations, the custom dark-mode sider overrides in
`src/app.tsx:203-213`, and every page renders antd. v5 still receives
maintenance. Recommend: do it as its own dedicated effort **after** this plan
lands (the code-splitting and typing work in Phases 2–3 make the migration
diff smaller and safer), using the official v6 migration guide/codemod. Ask
the user whether to schedule it.

---

## Explicitly re-verified and OK (don't "fix")

All F1–F18 fixes are present and correct at `805f2a6`: `withDB`/`dbMu`
convention, restore validation+rollback, order/delivery transition tables,
per-request `isActive` check (`api/middleware.go:50`), login timing decoy +
rate limiter + trusted-proxy XFF, user CRUD validation incl. last-admin guards
(local and OIDC), CSP tuned for react-pdf, backup 0600/0700 perms, SPA fallback
JSON-404/no-listing, CI test+lint workflow, server-side invoice totals
validation (`db/invoice_totals.go`, exact rational arithmetic). OIDC flow
unchanged and previously reviewed sound. Monetary handling (integer cents +
decimal.js ROUND_HALF_UP) consistent throughout.

## Verification checklist (after all phases)

1. `go vet ./... && go test -race ./...` and `pnpm lint && pnpm build` — clean.
2. `pnpm build`: entry chunk ≤ ~450 kB gzip; no `.map` files in `dist/`; PDF
   worker/stack in a lazy chunk.
3. `pnpm audit --prod` — clean; `govulncheck ./...` — clean.
4. Manual sweep against `go run .` + built frontend: login → invoices list
   (filter by Sent works, cancelled renders as translated tag, overdue dates
   red) → invoice details → PDF preview + download (no CSP violations) →
   language switch updates filter labels → org with logo: list loads without
   logo payload, PDF still shows logo → 11 MB JSON body → 413 → PATCH invoice
   state `"garbage"` → 409 → PUT with `state` → ignored.
5. Deep-link refresh on `/invoices/{id}` and `/settings/tax-rates/new` still
   serves the SPA (router upgrade regression check).
