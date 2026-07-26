-- Presence of a row = that ISO 3166-1 alpha-2 code is "active": offered in
-- the country picklist on organization/vendor/client forms. This is a
-- global, install-wide list (not per-organization) — the new-organization
-- form renders before any organization exists, so a per-org list couldn't
-- serve it. The full code list itself is static data on the frontend; this
-- table only ever needs to store which subset is active.
CREATE TABLE active_countries (
  code TEXT(2) PRIMARY KEY NOT NULL
);
