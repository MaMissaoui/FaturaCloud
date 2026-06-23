CREATE TABLE IF NOT EXISTS users (
  id           TEXT PRIMARY KEY,
  email        TEXT NOT NULL UNIQUE,
  passwordHash TEXT NOT NULL,
  displayName  TEXT NOT NULL DEFAULT '',
  role         TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('admin', 'user')),
  isActive     INTEGER NOT NULL DEFAULT 1,
  createdAt    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%S', 'now')),
  lastLoginAt  INTEGER
);
