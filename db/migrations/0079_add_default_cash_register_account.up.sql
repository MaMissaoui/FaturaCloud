-- The 16th column in the default*AccountId family (db/account.go's
-- accountDefaults / db/organization.go's accountDefaults Go-emitted SET
-- block). Deliberately separate from defaultCashAccountId: every shipped
-- chart-of-accounts template (generic, SKR04, PCG) wires defaultCashAccountId
-- to the Bank account, not the literal Cash/Kasse/Caisse till account — there
-- was no existing signal for "this is the physical cash register" a Cash
-- Book balance/withdrawal feature could resolve against. Nullable, same
-- convention as every other default account: unset until an admin picks one,
-- and the Cash Book screen/report refuses with a 409 (not a 500) until then.
ALTER TABLE organizations ADD COLUMN defaultCashRegisterAccountId TEXT REFERENCES accounts(id);
