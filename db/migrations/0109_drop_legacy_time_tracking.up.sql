-- tags, timeEntries and projects are leftovers from the desktop app this
-- project was forked from (migrations 0012, 0014, 0015). No code reads or
-- writes them, and production had no rows in any of them when this was
-- written (audit F188, checked 2026-10-17). Dropping a table drops its
-- indexes.
DROP TABLE IF EXISTS timeEntries;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS tags;
