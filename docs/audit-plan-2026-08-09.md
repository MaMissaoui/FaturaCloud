# FaturaCloud Re-Audit — Security, Dependency Currency, i18n (2026-08-09)

Follow-up audit at commit `113ab83` (`v3.6.0`), scoped to security, up-to-date
components, and multi-language support only (no UI/perf/upgrade sweep this
time). The 2026-07-18 plan (F19–F41) is **fully implemented and verified in
code, including all three [DECISION] items** (F25 HSTS, F26 cookie auth, F39
antd 6) — see "Explicitly re-verified and OK" below. This plan continues the
numbering at F42 and covers what changed since: Phase 7 (inventory/COGS GL,
PR #77) and the Reporting menu (PR #80).

**Instructions for the executing model:**
- Work phase by phase. One feature branch + PR per phase (never push to main
  directly). Conventional commits, no Claude attribution lines.
- After every phase: `go vet ./... && go test -race ./...` and
  `pnpm lint && pnpm build` must pass.
- Update CLAUDE.md sections whose documented behavior a task changes.
- This document is an audit, not a remediation — no dependency bumps or
  fixes were made while producing it.

**Status:** F42, F43, F44, F46, F47 fixed the same day (branch
`fix/audit-2026-08-09-findings`, shipped in v3.6.1). **F45 resolved by
product decision**: accept English-only server error text, no translation
system built — see 3.1 and CLAUDE.md's Internationalization section.

---

## Severity overview

| # | Finding | Area | Severity | Phase | Status |
|---|---------|------|----------|-------|--------|
| F42 | `/reporting/*` endpoints accept no date range at all and run an unbounded aggregate over every document the org has ever had | Security/Perf | Medium | 1.1 | **Fixed** |
| F43 | GitHub Actions pinned to floating major-version tags, not commit SHAs | Security (supply chain) | Low | 1.2 | **Fixed** |
| F44 | `alpine:3.21` base image is 3 minor releases behind current stable (3.24.1) | Dependency currency | Low | 2.1 | **Fixed** |
| F45 | Server-side validation/409 error messages (~140 call sites) reach the UI in raw English with no translation path | i18n | Medium | 3.1 | **[DECISION] — resolved: accepted, won't fix** |
| F46 | Reporting chart tooltips show the raw field name (`revenue`/`spend`), untranslated | i18n | Low | 3.2 | **Fixed** |
| F47 | Revenue Trend's month column/axis renders raw `"YYYY-MM"` instead of a localized month name | i18n | Low | 3.3 | **Fixed** |

---

## Phase 1 — Security (P1)

### 1.1 Bound the Reporting endpoints' unbounded case (F42)

`api/reporting.go`'s `reportingEndDate` defaults a missing `endDate` to "now",
but a missing `startDate` stays `0` — `parseInt64Param`'s zero-value default,
which `db/sales_reports.go`'s `dateRangeFilter` treats as "unbounded on that
side" by design (that contract is correct and load-bearing for
`GetDashboardData`, which deliberately calls with `endDate=0`). The
consequence: `GET /api/organizations/{id}/reporting/tax-summary` with no
query params at all — not something the frontend ever sends, but nothing
stops a client that does — runs `getOutputTaxSummary`/`getInputTaxSummary`'s
nested per-`(document, taxRateId)` subquery over the organization's entire
invoice/incoming-invoice history, unbounded, unpaginated, on every call.

This is the same class of concern F19 (body-size cap) and F28 (table
pagination) already addressed elsewhere, and it's genuinely new here — none
of the five `/reporting/*` handlers existed before PR #80. `db.SetMaxOpenConns(1)`
serializes all writes through one connection; confirm whether this read path
also contends with `dbMu` such that one large aggregate stalls unrelated
requests, then size the actual impact against real data volume before
deciding severity.

**Fix direction (size before implementing):** default `startDate` at the API
boundary the same way `reportingEndDate` already defaults `endDate` — e.g.
`reportingStartDate` defaulting to some bounded lookback (a year? all of the
org's history capped at a row count?) when absent, mirroring the pattern
already established two lines away in the same file. Do **not** change
`dateRangeFilter`'s "0 = unbounded" contract — that would silently change
`GetDashboardData`'s behavior.

**Fixed:** `reportingStartDate` added to `api/reporting.go`, defaulting an
absent `startDate` to 5 years before `endDate` rather than staying `0`;
`dateRangeFilter`'s contract and `GetDashboardData`'s own unbounded calls are
untouched. Covered by `api/reporting_test.go`.

### 1.2 SHA-pin third-party GitHub Actions (F43)

`.github/workflows/ci.yml` and `docker.yml` reference `actions/checkout@v7`,
`docker/build-push-action@v7`, `docker/metadata-action@v6`, etc. — floating
major-version tags rather than pinned commit SHAs. A compromised action
publisher (or a hijacked tag) could push malicious code that gets picked up
silently on the next run, including the `docker.yml` workflow that has
`packages: write` and publishes the public image. First-party actions
(`actions/*`) are lower risk than third-party ones (`docker/*`); prioritize
pinning `docker/build-push-action`, `docker/login-action`,
`docker/metadata-action`, `docker/setup-buildx-action` if this is acted on.
Not urgent — no known compromise, this is a hardening item, not an active
exposure.

**Fixed:** all 8 action references in `ci.yml`/`docker.yml` pinned to the
commit SHA their tag currently resolves to, with the version kept as a
trailing comment.

---

## Phase 2 — Dependency currency

### 2.1 Alpine base image drift (F44)

`Dockerfile`'s runtime stage pins `alpine:3.21`. Current stable is **3.24.1**
(released 2026-06-13, confirmed via web search); 3.21 itself is still
receiving point releases (3.21.7 as of mid-2026) and is not EOL yet — Alpine
branches are typically supported ~2 years from release, and 3.21 shipped
~Dec 2024, so it has a few months of support left, not an immediate cliff.
This is currency drift, not a known exposure: no CVE surfaced against the
pinned image via this audit's tooling. Bump when convenient; re-verify the
Go binary's runtime deps (musl libc version skew) still link cleanly after.

**Fixed:** bumped to `alpine:3.24` in `Dockerfile` and `CLAUDE.md`. The Go
binary is `CGO_ENABLED=0` (statically linked), so there was no musl skew to
re-verify. Not exercised by CI (no PR-time Docker build step exists) — worth
a real `docker build` before or during the next release tag.

### Explicitly checked, not findings

- **`govulncheck ./...`** — 0 vulnerabilities in code or called dependencies
  (1 unreachable advisory in a required-but-uncalled module, not actionable).
- **`pnpm audit --prod`** — clean. Matches CI's gate
  (`pnpm audit --prod --audit-level high`).
- **`pnpm audit` (full, dev included)** — 11 advisories, all inside the
  Vite/PostCSS/Sass build toolchain or `jotai-devtools` (dev-gated behind
  `import.meta.env.DEV` since F22). Previously-accepted posture, unchanged;
  CI's `--prod` scope deliberately excludes them (see the workflow's own
  comment). Not re-listed as new findings.
- **`pnpm outdated`** — only routine patch/minor drift (antd 6.5.1→6.5.4,
  React 19.2.7→19.2.8, Sentry, Lingui, oxlint/oxfmt, etc.), no majors
  outstanding besides two already-known holds: `@babel/core` 7→8 (F40 already
  deferred this pending macro-plugin support) and `pdfjs-dist` 5→6, which is
  **not actually free to bump** — `node_modules/react-pdf/package.json` pins
  `pdfjs-dist` to `5.4.296` as a direct (non-peer) dependency, so this is
  blocked behind a future react-pdf major, not outstanding drift. No advisory
  against 5.4.296 today.
- **Go direct dependencies** (`go.mod`) — all current or one minor behind
  (`modernc.org/sqlite` 1.54.0→1.56.0), no majors, no CVEs.

---

## Phase 3 — i18n (P2)

### 3.1 Untranslated server-side error messages (F45) — structural, not a quick fix

`src/**/*.tsx` has 41 call sites of the pattern
`message.error(error instanceof Error ? error.message : t\`fallback\`)` (e.g.
`src/components/payments/payment-panel.tsx`, `src/routes/settings/gl-export.tsx`).
The `t\`fallback\`` half is translated; the `error.message` half — which is
what actually displays whenever the server returns a specific error — is
whatever English string the Go backend put in `{"error": "..."}`. The Go
side has roughly **142** distinct human-readable validation/409 messages
(`newValidationError(...)`, `errors.New(...)` call sites across `db/*.go`),
none of them translated, all reaching German/French users verbatim in
English. Examples: `"%q has no cost basis (no receiving movement found for
this unit, and no average cost to fall back to)"`, the multi-item FEC/DATEV
validation-problem lists, the 3-way-match-override reason requirement.

This is pre-existing architecture, not a regression, but Phase 7 (COGS/GRNI
cost-basis errors) and the GL export work substantially grew the count of
such messages, and no i18n extraction pass will ever surface this gap — it
lives entirely on the Go side, outside Lingui's reach. This is the highest-
value i18n item available; it needs a design decision (a Go-side message-key
system + frontend translation table, or accept English-only error text as a
product decision) rather than a mechanical fix — **[DECISION]**, don't
implement without sign-off.

**Resolved (2026-08-09): accept English-only server error text as a product
decision.** No Go-side message-key/translation system will be built. `error
instanceof Error ? error.message : t\`fallback\`` stays as-is: the generic
fallback is translated, the server's specific message displays in English
regardless of locale. Recorded in CLAUDE.md's Internationalization section so
this isn't re-flagged as a bug by a future audit or contributor.

### 3.2 Reporting chart tooltips show raw field names (F46)

`revenue-trend.tsx`, `sales-by-client.tsx`, `sales-by-product.tsx`,
`purchases-by-vendor.tsx` all configure
`tooltip={{ items: [{ field: "revenue", valueFormatter: ... }] }}` (or
`"spend"`) with no `name` override, so `@ant-design/plots` falls back to the
raw field string as the tooltip's row label — a German user hovering a bar
sees "revenue: 1.234 €", not "Umsatz: 1.234 €". Pre-existing in
`dashboard.tsx` (same pattern, same field), now present in 4 additional
pages. Fix: add `name: <translated label>` to each tooltip item — small,
mechanical, low risk; worth batching across all 5 pages including the
dashboard widget in one pass rather than fixing only the new ones.

**Fixed:** `name: t\`Revenue\`` / `t\`Spend\`` (using the existing
`@lingui/core/macro` `t` pattern already established in the codebase, e.g.
`payment-panel.tsx`) added to all 5 tooltip configs, including the dashboard
widget. Both strings already existed as translated msgids elsewhere in the
catalog, so `pnpm extract` produced no new missing entries.

### 3.3 Revenue Trend table shows raw `"YYYY-MM"` (F47)

`GetRevenueByMonth` returns `month` as SQLite's `strftime('%Y-%m', ...)`
output; both the chart x-axis (pre-existing, via `dashboard.tsx`) and the
new Revenue Trend page's table-view toggle (new — the dashboard widget never
had a table view) render it as-is: "2026-03" rather than a locale-formatted
"March 2026" / "März 2026" / "mars 2026". Low severity — the format is at
least unambiguous and sorts correctly — but worth a small fix (format via
`dayjs(month + "-01").format(...)` locale-aware in the render function,
matching `src/utils/date.ts`'s existing conventions) since the table view is
new surface area that didn't exist before this audit's scope.

**Fixed:** the table column now renders via
`dayjs(\`${month}-01\`).format("MMMM YYYY")`, which follows the app's global
`dayjs.locale(...)` switch (`src/utils/lingui.tsx`). The chart's x-axis is
unaffected by design — it's an AntV category axis, not a text render, so the
raw string there was never a user-visible localization gap.

### Explicitly checked, not findings

- `pnpm extract` produces **zero diff** against committed `.po` files — no
  source strings have drifted ahead of translation.
- `de.po`/`fr.po`: 832/832 messages translated in both locales, 0 missing, 0
  fuzzy entries.
- No module-scope `` t`...` `` strings (the F32 pattern) found anywhere in
  `src/`, including the 5 new reporting pages.
- The Reporting menu's 5 sidebar labels, page titles, column headers, and
  "Unrated"/"Top 20 by revenue"/"Top 20 by spend" strings are all
  `<Trans>`-wrapped and present with real (non-empty) `de`/`fr` translations.

---

## Explicitly re-verified and OK (don't "fix")

All F1–F41 fixes are present and correct at `113ab83`, **including the three
items the 2026-07-18 audit deliberately left as [DECISION]s**:
- **F25 (HSTS)**: sent conditionally, only when the request arrived over
  HTTPS (`requestIsHTTPS`, keyed off `X-Forwarded-Proto`) — the
  proxy-terminates-TLS-aware option the prior audit recommended, not sent
  unconditionally.
- **F26 (cookie auth)**: JWT now rides in an httpOnly `fc_token` cookie with
  `SameSite=Lax` + a custom `X-CSRF-Protection` header requirement
  (`csrfRequired` in `api/middleware.go`) on every state-changing request —
  the XSS-token-theft class F26 flagged is closed.
- **F39 (antd 6)**: `antd` is on 6.5.1, done as its own effort as recommended.

Also re-verified: JWT `iss`/`aud` binding (F23), per-account login throttle
(F24), route-level code splitting (F27), table pagination (F28), organizations
list omits the logo BLOB (F29), invoice state validated + unified vocabulary
(F20/F31/F32), `govulncheck`/`pnpm audit` wired into CI (F41), 10 MB JSON
body cap (F19), no source maps shipped (F21).

**New in this audit, also verified OK, not findings:**
- **No cross-org authorization check exists anywhere in the app** (confirmed:
  `users` has no `organizationId` column, `authMiddleware` checks only that
  the JWT's user is active) — any authenticated user, admin or not, can read
  any organization's data by ID, including all 5 new `/reporting/*`
  endpoints. This is a whole-app, single-tenant-per-deployment design
  property predating PR #80, not something the Reporting menu introduced or
  weakened — documented here so the operator is aware, not filed as a
  finding.
- The 5 new `/reporting/*` routes are `protected` (any authenticated user),
  consistent with every existing `/reports/*` (GL-derived) route — only the
  FEC/DATEV statutory *export* endpoints are `adminProtected`, a distinction
  that predates this audit and the Reporting menu doesn't disturb.
- SQL construction in `db/sales_reports.go` (`dateRangeFilter` and every
  query builder) — column names are always Go string literals from the
  caller, never request-derived; every value is bound via `?` placeholders;
  `limit` is hardcoded to `0` in all 5 `api/reporting.go` handlers, never
  read from a query parameter. No injection surface.
- `v3.6.0`'s Docker/release workflow (triggered by this session's tag push)
  completed successfully.

## Verification checklist (after any fix phase)

1. `go vet ./... && go test -race ./...` and `pnpm lint && pnpm build` — clean.
2. `govulncheck ./...` and `pnpm audit --prod --audit-level high` — clean.
3. `pnpm extract` — no diff; `de`/`fr` catalog stats — 0 missing, 0 fuzzy.
4. Manual: Reporting → Tax Summary with no date range narrowed (if F42's
   fix changes default behavior, confirm it) → each of the 5 reporting pages
   in German/French → hover a chart bar (F46) → toggle Revenue Trend's table
   view (F47).
