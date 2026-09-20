-- Per-user JWT revocation counter (F109). A session token carries the
-- tokenVersion it was issued with (see api/middleware.go's Claims and
-- api/auth.go's issueTokenWithProvider); authMiddleware rejects a token whose
-- embedded value no longer matches the stored one. Logout and password change
-- both bump this column, so an old cookie stops working immediately instead
-- of surviving up to the JWT's full 24h lifetime. DEFAULT 0 keeps every
-- pre-existing user — and any token already in flight, which was necessarily
-- issued with version 0 — valid across this migration.
ALTER TABLE users ADD COLUMN tokenVersion INTEGER NOT NULL DEFAULT 0
    CHECK (tokenVersion >= 0);
