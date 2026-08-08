CREATE TABLE IF NOT EXISTS fiscal_years (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    startDate INTEGER NOT NULL,
    endDate INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'closed')),
    -- FEC's ValidDate: entries dated on/before this are locked even if the
    -- year isn't formally closed yet (a partial lock during audit prep).
    lockDate INTEGER,
    closedAt INTEGER,
    createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
    CHECK (endDate > startDate)
);
CREATE UNIQUE INDEX IF NOT EXISTS fiscal_years_organizationId_name ON fiscal_years(organizationId, name);
CREATE INDEX IF NOT EXISTS fiscal_years_organizationId_dates ON fiscal_years(organizationId, startDate, endDate);

CREATE TABLE IF NOT EXISTS fiscal_periods (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    fiscalYearId TEXT NOT NULL REFERENCES fiscal_years(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    startDate INTEGER NOT NULL,
    endDate INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'closed')),
    closedAt INTEGER,
    createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
    CHECK (endDate > startDate)
);
CREATE UNIQUE INDEX IF NOT EXISTS fiscal_periods_organizationId_name ON fiscal_periods(organizationId, name);
CREATE INDEX IF NOT EXISTS fiscal_periods_fiscalYearId ON fiscal_periods(fiscalYearId);
