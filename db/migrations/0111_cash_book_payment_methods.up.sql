-- Lets an organization accept card and transfer payments at the Cash Book
-- (cash_and_bank), not only cash. The default keeps every existing
-- organization cash-only, so money received at the counter always lands in
-- the physical register until the owner explicitly opts in.
ALTER TABLE organizations ADD COLUMN cashBookPaymentMethods TEXT NOT NULL DEFAULT 'cash_only';
