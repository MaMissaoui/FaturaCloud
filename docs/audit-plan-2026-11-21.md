# FaturaCloud Dependency and Supply-Chain Audit (2026-11-21)

**This is a dependency-currency and vulnerability audit of `main` at `941c28a`
(v3.74.3).** It covers Go modules and the Go toolchain, npm packages, the
Docker base images and the runtime image's OS packages, GitHub Actions, and
the repository's Dependabot setup. It does not re-audit application code
(the 2026-10-23 security audit and the 2026-10-31 review cover that).
Numbering continues at **F197**; no document under `docs/` uses F197 or later.

**How this document was produced.**

- `govulncheck -show verbose ./...` (v1.8.0 source mode) with the local
  go1.27.4, then again with `GOTOOLCHAIN=go1.27.3`, the toolchain the
  v3.74.3 image was built with.
- Go vulnerability database entries (`vuln.go.dev/ID/<id>.json`) for each
  reported id, to read the fixed versions.
- `pnpm audit` and `pnpm outdated` on the full tree, dev tooling included.
- `docker scout cves --only-fixed --only-severity critical,high` on the
  published `ghcr.io/mamissaoui/fatura-cloud:3.74.3` (linux/arm64, the
  Raspberry Pi target).
- The GitHub API for the repository's security settings, open Dependabot PRs
  and their check results.

## Severity overview

| Id   | Finding                                                        | Severity | Status |
| ---- | -------------------------------------------------------------- | -------- | ------ |
| F197 | The shipped binary is built with go1.27.3 (GO-2026-6859)       | High     | Fixed: #584 |
| F198 | CI scans `go.mod`'s toolchain, not the one that ships          | High     | Fixed: #587 + this PR |
| F199 | Dependabot alerts and security updates are disabled            | High     | Fixed: enabled 2026-11-21 |
| F200 | No scan runs between PRs                                       | Medium   | Fixed: this PR |
| F201 | Fixed Go advisories left in imported/required modules          | Medium   | Fixed: #578 + #587 |
| F202 | Base-image OS packages wait for a digest bump (zlib, 2 High)   | Medium   | Fixed: this PR |
| F203 | Dependabot PRs sat 1–3 weeks unmerged, one red                 | Medium   | Fixed: #588 |
| F204 | Indirect Go modules are never proposed by Dependabot           | Low      | Fixed: this PR |
| F205 | Dev-only npm advisories (glob, minimatch)                      | Low      | Fixed: #588 |

## F197: the shipped binary is built with go1.27.3 (High, Confirmed)

GO-2026-6859 (HTTP/1 server crash, a `MaxBytesReader` header race in
`net/http`) is fixed in go1.26.11 and go1.27.4. #582 raised `go.mod` to
1.26.11 and said "the Docker image already builds with 1.27.3", but 1.27.3
is in the vulnerable range. `GOTOOLCHAIN=go1.27.3 govulncheck ./...`
reports it as **called** by this code. Every image up to v3.74.3 carries it,
so an unauthenticated client can crash the server process.

**Fix:** #584 (Dependabot, golang 1.27.4-alpine plus the Debian digest),
merged 2026-11-21. **Prod is exposed until a release containing it is
deployed.**

## F198: CI scans the wrong toolchain (High, Confirmed)

`ci.yml`'s `go` job sets up Go from `go-version-file: go.mod` (1.26.11),
while the Dockerfile builds with its own `golang:` tag (1.27.x). The
govulncheck gate checked a standard library that never ships, so F197 was
green in CI.

**Fix:** `go.mod` now says `go 1.27.4`, the Dockerfile's version (#587). The
CI `docker` job now copies the built binary out of the `backend` target and
runs `govulncheck -mode=binary` on it, so the scan covers what ships
whatever `go.mod` says. CLAUDE.md requires the two versions to stay equal.

## F199: Dependabot alerts and security updates are disabled (High, Confirmed)

`GET /repos/MaMissaoui/FaturaCloud` reports
`dependabot_security_updates: disabled`, and the alerts API answers
"Dependabot alerts are disabled for this repository". The repository is
public. The only Dependabot activity is the weekly version-update schedule,
so a published advisory opened no PR and raised no alert.

**Fixed 2026-11-21:** both enabled by the owner. **Fix (owner, needs admin):** Settings → Code security → enable Dependabot
alerts and Dependabot security updates, or:

```bash
gh api -X PUT repos/MaMissaoui/FaturaCloud/vulnerability-alerts
gh api -X PUT repos/MaMissaoui/FaturaCloud/automated-security-fixes
```

These cover the Go modules, npm and Actions. They do not cover the Go
toolchain or the image's OS packages, which is why F200's daily scan exists.

## F200: no scan runs between PRs (Medium, Confirmed)

govulncheck and `pnpm audit` run only in `ci.yml`, on a PR or a push to
main. An advisory against unchanged code stays invisible until an unrelated
change happens to run CI. Nothing scans the runtime image's OS packages at
all.

**Fix:** `.github/workflows/security.yml`, daily at 05:41 UTC and on demand:
govulncheck in every section (called, imported and required), `pnpm audit
--audit-level moderate` on the whole tree, and Trivy (pinned action SHA and
binary version) on `ghcr.io/mamissaoui/fatura-cloud:latest` for fixed
CRITICAL/HIGH OS findings.

## F201: fixed Go advisories left in imported/required modules (Medium, Confirmed)

| Id            | Module                     | Fixed in                         | Reach (govulncheck) |
| ------------- | -------------------------- | -------------------------------- | ------------------- |
| GO-2026-6695  | `github.com/xuri/excelize/v2` | 2.11.1 (pivot-cache panic)    | imported |
| GO-2026-6860  | `golang.org/x/net`         | 0.62.0 (quadratic html coalescing) | imported |
| GO-2026-6866  | `golang.org/x/net`         | 0.62.0 (html raw-text rendering) | imported |
| GO-2026-5932  | `golang.org/x/crypto`      | 0.59.0 (openpgp)                 | required |

CI's gate fails only on *called* findings, so these stayed indefinitely.
"Not called" is static reachability. excelize parses workbooks that users
upload, so a panic there is one call-graph change away.

**Fix:** #578 (excelize v2.11.1), then #587 (`go get -u ./...`: every module
at its latest, x/net 0.63.0, x/crypto 0.59.0, plus go-jose 4.1.9, oauth2
0.38.0, text 0.44.0, modernc sqlite 1.62.0). govulncheck now reports
nothing in any section. #585 is superseded.

## F202: base-image OS packages wait for a digest bump (Medium, Confirmed)

Scout reports two fixed HIGH CVEs in v3.74.3's `zlib`: CVE-2026-85091 and
CVE-2026-76844, fixed in `1:1.3.dfsg+really1.3.1-1+deb13u1`. The runtime
stage runs `apt-get install` (unpinned, so it already takes today's
archive) but never `apt-get upgrade`, so the base's own packages stay frozen
at the digest's publish date.

**Fix:** `apt-get upgrade -y` before the install in the runtime stage. This
adds no non-reproducibility the unpinned install didn't already have.
security.yml's Trivy job catches a regression.

## F203: Dependabot PRs sat unmerged (Medium, Confirmed)

As of today, these were open: #576 (path-to-regexp 9, 3 weeks), #578
(excelize security fix, 2 weeks), #579 (actions), and #584/#585/#586 (1
week). #584 and #578 carried security fixes. #586 was red on a new oxlint
1.94 `set-state-in-effect` warning in `src/routes/settings/users.tsx`, so
the whole npm group was stuck.

**Fix:** #584 and #578 merged. #588 fixes the users page and brings every
npm package to its latest release: the minor/patch group, the exact pins
(antd 6.7.1, jotai 3.0.2, sass 1.108.2, @vitejs/plugin-react 6.3.0), and
the majors path-to-regexp 9 (`match("/*path")` is unchanged), jsdom 31 and
@testing-library/jest-dom 8. It supersedes #576 and #586. CLAUDE.md now
says the weekly groups are merged within the week.

## F204: indirect Go modules are never proposed (Low, Confirmed)

`golang.org/x/net` (indirect) stayed at 0.60.0 while 0.62.0 fixed two
advisories. The `gomod` entry in `dependabot.yml` had no `allow`, so its
weekly group proposed direct modules only.

**Fix:** `allow: [{dependency-type: all}]` on the gomod entry.

## F205: dev-only npm advisories (Low, Confirmed)

GHSA-3g3m-2fjr-6pw4 (glob) and GHSA-g3hv-75wg-m249 (minimatch), both
moderate, came through `@sentry/vite-plugin` 11.10 > `@sentry/bundler-plugins`.
They are build-time only and never ship. CI's dev audit is report-only at
`high`, so nobody saw them.

**Fix:** #588 (@sentry/vite-plugin 11.15). `pnpm audit` reports nothing.
security.yml audits the whole tree at `moderate`.

## Not findings

- Actions are SHA-pinned and the base images digest-pinned (F181). That
  stays. Currency comes from Dependabot proposing the bumps, not from
  floating tags. #579 (pnpm/action-setup 6.1.1, docker/login-action 4.8.0)
  is a routine update.
- Node stays on the 24 LTS line by documented decision (corepack).

## Verification

- `govulncheck -show verbose ./...` on #587's tree: "No vulnerabilities
  found" in every section.
- `go test ./...`, `pnpm lint`, `pnpm format:check`, `pnpm test` (154
  tests), `pnpm build` and `pnpm extract` pass on the PR branches.
- After the release: CI's binary scan is green, `security.yml` is green on
  dispatch, and Trivy shows no fixed HIGH/CRITICAL findings on the new image.
