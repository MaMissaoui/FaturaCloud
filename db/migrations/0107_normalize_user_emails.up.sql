-- Emails are stored trimmed and in lower case (ASCII letters, the same
-- folding as db.NormalizeEmail), the form every sign-in and lookup now uses
-- (audit F174): before, "Ali@Shop.tn" and "ali@shop.tn" were different
-- accounts, and signing in with a capital letter failed.
--
-- An account that would collide with another once normalized is left as it
-- is: the two need merging by hand first. Production had none when this was
-- written (audit Phase 0, 2026-10-17).
UPDATE users SET email = lower(trim(email))
WHERE email <> lower(trim(email))
  AND NOT EXISTS (
    SELECT 1 FROM users other
    WHERE other.id <> users.id AND lower(trim(other.email)) = lower(trim(users.email))
  );
