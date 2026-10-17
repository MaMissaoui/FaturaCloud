# cmd/seed-demo — Claude notes

Loaded when working under `cmd/seed-demo/`. The flag reference and scenario descriptions are in `README.md` here.

- **It drives the real REST API** (a logged-in session, the CSRF header, real GL posting), never `db.Database`, so a run doubles as an API smoke test. Request bodies are the real `db.Create*Request` structs, because several of them mix JSON key conventions field by field (e.g. `house_number`).
- `scheduler.go` is a day-keyed task queue for multi-step chains (invoice → payment; PO → receive → bill → pay). `RunDue` drains until empty, including tasks scheduled for the same day from inside a running task, and `Run` calls it again at day end.
- `stock.go` mirrors on-hand stock locally so a delivery or sale never asks for more than was received. A server 409 from drift (for example on production completion) skips that step instead of failing the run.
- It never closes a fiscal year (irreversible) and keeps the never-paid share of documents small (~3–5%), so the aging tail stays realistic over 18 months.
- Moto scenario:
  - `production.go`: `setupBillsOfMaterials` gives every finished product a real BOM (a curated subset, so components can all be in stock), and `assembleBatch` reads it back and runs real Production Orders (create, then complete). This is what makes finished goods sellable and COGS appear.
  - `imports.go`: a monthly import with POs created against it (`maybeStartImport`, `maybeCreateImportLinkedPO`), which exercises `applyLandedCost`.
- `catalog.go`'s `localeFor` follows `--country`, so names, cities, phones and VAT ids match the organization's country.
- Retail scenario:
  - `setupCashRegisterAccount` wires the till (`defaultCashRegisterAccountId` → the `1010` Cash account), the one manual step a real organization does in its settings.
  - `retail_stock.go`: opening stock comes from purchase orders dated the first day (a count movement would post at the current time and leave Inventory negative in the past). Sales take only what's on the shelf. Restocking brings items at or below 2 back up to 5–10.
  - Coverage files (`retail_season.go`, `retail_b2b.go`, `retail_overheads.go`, `retail_extras.go`) exercise most of the app. Serialized products stay off the counter (`counterProducts`). `scheduleLoanCollection` re-reads the loan register after every payment.
- Every organization is created with `IsTest: true` (older servers ignore the field).
- **`--reset`** (F178): `resetTarget` (`masterdata.go`) deletes only a single organization named `--org-name` that is flagged `isTest`, and only when its id is passed in `--confirm-reset`; otherwise it stops and prints the id (`reset_test.go`).
