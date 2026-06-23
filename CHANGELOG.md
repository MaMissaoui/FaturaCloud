# Changelog

All notable changes to Fatura will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.0.0-beta.28] - 2026-06-22

### Added
- JWT authentication — login page, Bearer token stored in `localStorage`, 24-hour expiry (HS256); configurable via `JWT_SECRET` env var
- User management — admin-only Users settings page (create, edit, deactivate, delete); roles: `admin` | `user`
- First admin auto-created on startup from `ADMIN_EMAIL` / `ADMIN_PASSWORD` env vars (defaults: `admin@fatura.cloud` / `admin`)
- Per-IP login rate limiting (10 attempts per minute) to prevent brute-force attacks
- Automatic backup scheduler — configurable hour and retention policy; stores named files in `{dbDir}/backups/`; runs as a background goroutine
- Backup history page — lists stored backup files with size, date, and one-click named restore
- Manual backup download and file-based restore via the Backup settings page
- Products entity — physical products and services with stock tracking
- Inventory management — stock movements (in / out / adjustment) with signed-delta storage; `products.stockQuantity` always recomputed as `SUM(quantity)`
- Orders entity — full order lifecycle (`draft` → `confirmed` → `shipped` → `delivered` / `cancelled`), line items, delivery-note PDF export

### Changed
- All settings pages (Organization, Invoice, Tax Rates, Backup, Users) now use Ant Design Card-based layout with `maxWidth: 720` containers
- Tax Rates list UI matches the Products page (full-width, sortable table, inline search, drawer form)
- All entity create/edit forms migrated from modal dialogs to right-side drawers for a consistent UX
- All API routes are now JWT-protected; public routes limited to `GET /api/version`, `POST /api/auth/login`, `POST /api/auth/logout`
- `Users` sidebar item is admin-only; header shows logged-in user name and a logout button

### Removed
- Time tracking, projects, and tags entities (removed to focus on core invoicing workflow)

## [2.0.0-beta.27] - 2026-06-21

### Added
- CLAUDE.md with codebase guidance for Claude Code

### Changed
- Migrated from Wails/desktop app to a pure Go HTTP server serving an embedded React frontend over the network
- Switched from `MemoryRouter` to `BrowserRouter`; Go server now has a SPA fallback handler that returns `index.html` for any path not found in the embedded `dist/`, enabling deep-links and browser navigation
- Dockerfile updated for pnpm 11 (added `allowBuilds` for native deps) and Go 1.26

### Fixed
- Null `currency` crash on the invoice page — strip `undefined` values from the new-organization form before persisting so SQLite never receives `NULL` for currency
- All `Intl.NumberFormat` calls now fall back to `"EUR"` when currency is absent
- PDF export: `SaveFile` now accepts `Blob | Uint8Array` directly; the invoice PDF path no longer converts the blob to a `number[]`

## [2.0.0-beta.26] - 2026-05-27

### Changed
- Go module updated to Go 1.26 (was 1.22)
- React upgraded from 18 to 19.2.6
- Vite upgraded from 6 to 8 (now uses rolldown bundler internally)
- LinguiJS upgraded from 5 to 6.1.0 (required for Vite 8 / rolldown compatibility)
- `@vitejs/plugin-react` upgraded from 4 to 6.0.2
- `@sentry/vite-plugin` upgraded from 4 to 5.3.0
- `modernc.org/sqlite`: 1.29.10 → 1.50.1
- `golang-migrate/migrate`: 4.17.0 → 4.19.1
- `jmoiron/sqlx`: 1.3.5 → 1.4.0
- `jotai`: 2.17.1 → 2.20.0
- `react-router`: 7.13 → 7.15.1
- Various other minor dependency bumps

### Fixed
- TypeScript error in invoice PDF export: React 19 changed `ReactElement` default generic from `<any>` to `<unknown>`, breaking the `@react-pdf/renderer` `pdf()` call

### Removed
- Unused `@lingui/macro` package (macros are now part of `@lingui/core` in v6)

## [2.0.0-beta.25] - 2026-05-27

### Changed
- Replaced Tauri/Rust backend with Wails v2/Go — faster build times, CGO-free SQLite, and type-safe auto-generated JS bindings
- All 38 backend commands ported to public Go methods on `App` struct (same SQL, same behaviour)
- SQLite accessed via `jmoiron/sqlx` + `modernc.org/sqlite` (no CGO required)
- Migrations moved to `db/migrations/` using `golang-migrate` format and applied automatically on startup
- Frontend IPC switched from Tauri `invoke()` string calls to Wails auto-generated async functions
- PDF save dialog now uses Wails `runtime.SaveFileDialog` instead of Tauri plugins
- Backup/restore dialogs use Wails file dialogs instead of Tauri plugins
- Router switched from `BrowserRouter` to `MemoryRouter` for compatibility with the `wails://` WebView URL scheme

### Removed
- Tauri/Rust backend (`src-tauri/`)
- Auto-updater feature (to be reimplemented in a future release)
- `@tauri-apps/*` npm dependencies

### Fixed
- `projectId` field missing from `TimeEntry` Go struct caused all time entry queries to fail after migration 0015

## [2.0.0-beta.24] - 2026-05-18

### Fixed
- AppImage crash on Linux distros shipping libglvnd 1.7+ (Ubuntu 26.04, Nobara, recent Manjaro) — bundled WebKitGTK no longer aborts with `EGL_BAD_PARAMETER` (#180, #211)

### Removed
- AI assistant feature

### Changed
- Updated Rust crate dependencies

## [2.0.0-beta.23] - 2025-09-20

### Added
- Drag and drop functionality for reordering invoice line items using the MoreOutlined (⋯) icon

### Changed
- Enhanced invoice line items table with better column width optimization (Qty column reduced to 80px)
- Improved input field consistency with full-width styling across all numeric columns

## [2.0.0-beta.22] - 2025-09-14

### Added
- Greek (Ελληνικά) locale support with complete translation and proper date formatting
- In-place PDF preview functionality for invoice editing workflow

### Changed
- Improved accessibility with better color contrast for invoice status badges and download links
- Optimized Largest Contentful Paint (LCP) performance by fixing fetchpriority attribute

### Fixed
- React Suspense error in tax rates form that could occur during synchronous input
- Promise.withResolvers compatibility issues in older JavaScript environments

## [2.0.0-beta.21] - 2025-08-18

### Added
- UK English locale (en-GB) with proper regional formatting - calendar weeks start on Monday for European users
- Country field in organization settings to specify company location
- Date format customization per organization with multiple format options (DD/MM/YYYY, MM/DD/YYYY, YYYY-MM-DD, etc.)
- Version tracking in Sentry error reports for better debugging

### Changed
- Language selector now displays both US (🇺🇸) and UK (🇬🇧) English options
- Date formatting throughout the application now respects organization preferences

### Fixed
- Replaced deprecated Ant Design dropdownRender with popupRender API
- Invoice line item calculations no longer error with undefined decimal values

## [2.0.0-beta.20] - 2025-08-06

### Added
- User feedback modal integrated with Sentry for collecting bug reports and feature requests
- Feedback button in the sidebar navigation for easy access to submit feedback
- Complete translations for feedback modal across all 8 supported languages (German, Estonian, Finnish, French, Dutch, Portuguese, Swedish, Ukrainian)

### Changed
- Updated all language translation files with 17 new strings for feedback and AI features
- Enhanced Sentry integration to support user feedback collection

## [2.0.0-beta.19] - 2025-08-06

### Added
- New command to update invoice state independently without modifying other fields
- Ant Design locale configuration for proper date picker and component translations
- Loading spinner indicator when client modal is fetching data
- Improved invoice state management with dedicated state update function

### Changed
- Client form now uses React's startTransition for smoother state updates
- Invoice state dropdown now uses optimized state-only update instead of full invoice update
- Overdue charge input in settings now displays percentage symbol with proper formatting
- Enhanced client form with better loading states and error handling

### Fixed
- Invoice overdue charge field now properly saves when creating or updating invoices (backend fix)
- Client modal data loading race conditions resolved with proper loading states
- Invoice state changes now more efficient and reliable

## [2.0.0-beta.18] - 2025-08-06

### Added
- Daily overdue charge percentage field to invoice details form with help text
- Support for configuring daily overdue charges on invoices

### Fixed
- PDF generation now correctly handles client email data in both JSON string and array formats
- Organization selector dropdown rendering issue in the navigation bar

### Changed
- Invoice form layout adjusted with more compact date fields (reduced column spans)
- Build workflow updated to support RELEASE_TOKEN for GitHub Actions

## [2.0.0-beta.17] - 2025-08-06

### Added
- Overdue charge functionality for invoices with configurable percentage rate per day
- Complete translations for all 7 supported languages (German, Finnish, French, Dutch, Portuguese, Swedish, Ukrainian)
- Improved translation workflow with `yarn extract` integration for better status tracking

### Fixed
- Invoice counter not updating when creating new invoices
- Logo upload validation now properly shows error messages for unsupported file types
- Tax calculations in invoice PDFs now correctly group items by tax rate
- Missing translation strings for tax percentage headers and overdue charges

### Changed
- Enhanced invoice PDF generation with better tax grouping and subtotal calculations
- Translation workflow documentation updated to include extraction step first

### Removed
- Windows platform support temporarily disabled in GitHub Actions build workflow

## [2.0.0-beta.16] - 2025-08-05

### Fixed
- Updated translation extraction to include all new AI-related strings
- Fixed missing translations across all supported languages (21 strings per language)

## [2.0.0-beta.15] - 2025-08-05

### Added
- AI-powered invoice assistant with Claude integration for intelligent invoice creation
- Invoice creation through AI conversation with automatic form pre-filling
- AI settings page for configuring Anthropic API key
- Support for attaching images and documents to AI conversations for context
- Real-time invoice form updates based on AI suggestions
- Windows builds now available through GitHub Actions CI/CD pipeline

### Changed
- AI invoice creation workflow refactored to use form pre-filling for better user control
- Enhanced AI assistant with file attachment capabilities for improved context understanding

### Fixed
- TypeScript and ESLint errors in AI assistant implementation
- AI client selection validation to prevent setting invalid client IDs in invoice forms

## [2.0.0-beta.14] - 2025-07-29

### Fixed
- Endless redirect loop when navigating to organization creation page without an existing organization
- Organization search functionality in the header selector now properly filters by organization name
- Organization deletion now automatically activates the next available organization

## [2.0.0-beta.13] - 2025-07-29

### Added
- Website version now fetched dynamically and rebuilds on releases for accurate version display
- Default values for organization creation to streamline setup process

### Changed
- Updated axios dependency from 1.10.0 to 1.11.0 for improved security and performance
- Enhanced organization creation workflow with better validation and redirect logic
- Improved form submission UX by hiding cancel button during submission to prevent accidental cancellations
- Invoice creation now navigates to the new invoice URL after successful creation

### Fixed
- Organization creation redirect now properly navigates to invoices page when organization already exists
- React suspension error when creating new organizations resolved with improved state management
- TypeScript lint error from unused useAtom import cleaned up

## [2.0.0-beta.12] - 2025-07-22

### Fixed
- Invoice number generation error when no format is configured (prevents "null is not an object" error)
- Sentry sourcemap uploads in CI/CD builds by adding authentication token to GitHub Actions workflow

### Changed
- Enhanced Sentry integration with release tracking and explicit sourcemap configuration

## [2.0.0-beta.11] - 2025-07-18

### Changed
- Invoice numbers now display without zero-padding (e.g., "1" instead of "00001")
- Cleaned up invoice settings UI by removing obsolete prefix/suffix fields in favor of flexible format templates

## [2.0.0-beta.10] - 2025-07-17

### Fixed
- Tax rate creation now properly converts percentage strings to numbers before database storage
- Default tax rate selection automatically applies to new invoices and line items
- Only one tax rate can be marked as default per organization through transaction support

### Changed
- Improved tax rate field handling in invoice forms for better data consistency

## [2.0.0-beta.9] - 2025-07-14

### Added
- Complete project management system with create, edit, archive, and search functionality
- Project association to time tracking entries with intelligent client/project selection
- Time tracking reports page with filtering, grouping, and export capabilities
- Project status management (Active/Archived) with visual indicators
- Complete translations for all missing strings (400+ new translations across 8 languages)
- Comprehensive SEO improvements for the new Astro-based website
- Platform-specific download links and sitemap integration for better discoverability

### Changed
- Refactored database.rs into modular structure using modern Rust 2018+ patterns
- Improved time tracking reports with enhanced filtering and user interface
- Optimized website performance by fixing render blocking requests and LCP image loading
- Enhanced navigation with dedicated reports section
- Updated default window height for better user experience

### Fixed
- Projects page client display now properly fetches and shows client names using SQL JOIN
- TimeEntryForm suspension error when stopping timer resolved
- Form conflicts in TimeRangeCell component eliminated
- Memory allocation issues in development builds
- Invoice settings component cleanup and optimization

## [2.0.0-beta.8] - 2025-07-08

### Added
- Complete translations for all supported languages achieving 100% coverage (184/184 strings)

### Changed
- Disabled Sentry telemetry for enhanced privacy
- Cleaned up obsolete translation entries across all locale files for better maintainability

### Fixed
- JavaScript heap out of memory errors in GitHub Actions macOS builds by increasing Node.js memory allocation to 4GB
- Invoice state translations now display correctly in invoice list view using proper Trans components

## [2.0.0-beta.7] - 2025-07-05

### Added
- Ability to clear/unset tax rates on invoice line items with new clear button
- Precise decimal arithmetic using decimal.js library for all financial calculations

### Fixed
- Floating-point precision errors in invoice calculations (e.g. 0.1234 × 1000 now correctly shows 123.4 instead of 123.39999999999999)
- Tax calculations now use precise percentage calculations instead of floating-point arithmetic
- Invoice total calculations (subtotal + tax) now use precise decimal addition
- Form field calculations for quantity, price, and total now maintain precision during updates
- Added safety checks for missing tax rate percentages to prevent calculation errors

## [2.0.0-beta.6] - 2025-07-02

### Added
- Complete time tracking feature with start/stop timer functionality
- Global timer widget in header that persists across app navigation
- Inline editing for time entry descriptions and client assignments
- Time range editing with popover containing time pickers and date selector
- Persistent running timer that restores automatically on app restart
- Invoice duplication feature for quickly creating similar invoices
- Complete translations for 7 additional languages: German, Finnish, French, Dutch, Swedish, Ukrainian, and Portuguese
- Enhanced language dropdown with flag emojis for better visual identification
- Improved language selector UX with compact flag-only display in header
- Client code field for invoice numbering and invoice organization
- Jotai DevTools integration for better state debugging in development

### Changed
- Time tracking interface with streamlined table columns (tags visible, billable temporarily hidden)
- Time entry form simplified with core fields (description, client, time range, duration, tags)
- Language dropdown now auto-sizes to fit content properly without truncation
- Organization switching preserves current page to eliminate navigation flicker
- Updated README.md to reflect all 9 supported languages (English, German, Estonian, Finnish, French, Dutch, Portuguese, Swedish, Ukrainian)
- Refactored atoms into organized directory structure for better code maintainability
- Enhanced Sentry integration with frontend error tracking

### Fixed
- Time entry edit button in actions dropdown now works without page refresh
- Empty time entry descriptions now display placeholder text for better clickability
- Organization switching no longer causes UI flicker with improved loading states
- Language dropdown width issues resolved with proper Ant Design props
- Removed invalid yarn add-locale command from documentation
- App loading issues after atom refactoring resolved
- Language preference persistence now works correctly across app restarts

## [2.0.0-beta.5] - 2025-06-29

### Added
- Automatic invoice numbering with configurable options
- Auto-updater functionality with Tauri plugin
- Backup and restore functionality for data safety
- Sentry error tracking for better crash diagnostics
- Client delete functionality
- Migrated from tauri-plugin-sql to pure SQLx for all database operations

### Fixed
- Production build migration loading issue
- macOS crash issues with Sentry error tracking
- TypeScript errors in backup error handling
- React Hook useEffect dependency warnings
- Ant Design Descriptions deprecated props
- Invoice delete functionality
- Invoice creation and legacy column cleanup
- TypeScript build errors in organization atom
- Tauri v2 permissions for devtools and core functionality

### Changed
- Migrated from tauri-plugin-sql to pure SQLx for all database operations
- Upgraded from Tauri v1 to v2
- Improved backup page UI layout and responsiveness
- Updated dependencies including:
  - Vite from v5.4.19 to v6.3.5
  - ESLint from v8.57.1 to v9.29.0
  - TypeScript ESLint from v6.14.0 to v8.34.1
  - React Router from v6.30.1 to v7.6.2
  - react-pdf from v8 to v9.2.1
  - LinguiJS from v4 to v5
  - @react-pdf/renderer from 3.4.5 to 4.3.0
  - @ant-design/icons from 5.6.1 to 6.0.0
- Limited publish workflow to tauri.conf.json changes
- Updated README for beta release
- Fixed GitHub Actions badge URL
