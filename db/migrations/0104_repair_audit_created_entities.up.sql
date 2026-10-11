-- Audit F164: until setAuditEntity, the activity history took a created
-- document's id from the first "id" in the response. Two creates answer
-- with another object first, so their rows name the wrong document with
-- an empty label. This repairs the rows written before the fix.

-- A member add answered with the organization_users row: its id names the
-- membership. The history names a member by user (like the {userId}
-- routes), labelled with the email. A membership removed since can't be
-- traced back, so its row stays as it is. SET reads the row's old values,
-- so both subqueries see the membership id.
UPDATE audit_events
SET entityId = (SELECT ou.userId FROM organization_users ou WHERE ou.id = audit_events.entityId),
    entityLabel = COALESCE(
        (SELECT u.email FROM organization_users ou JOIN users u ON u.id = ou.userId
         WHERE ou.id = audit_events.entityId),
        entityLabel)
WHERE resource = 'members' AND method = 'POST'
  AND entityId IN (SELECT id FROM organization_users);

-- A cash sale answered client first: its row names the client. The sale is
-- that client's Cash Book invoice (movesStock = 1) in the same organization,
-- created in the seconds before the row (invoices.createdAt is UTC text to
-- the second, audit_events.createdAt milliseconds, written after the
-- sale's transaction). Repaired only where exactly one sale fits, so two
-- sales to one client within seconds stay as they are rather than risk
-- naming the wrong one.
UPDATE audit_events
SET entityId = (
        SELECT i.id FROM invoices i
        WHERE i.clientId = audit_events.entityId
          AND i.organizationId = audit_events.organizationId
          AND i.movesStock = 1
          AND CAST(strftime('%s', i.createdAt) AS INTEGER) * 1000
              BETWEEN audit_events.createdAt - 10000 AND audit_events.createdAt),
    entityLabel = (
        SELECT i.number FROM invoices i
        WHERE i.clientId = audit_events.entityId
          AND i.organizationId = audit_events.organizationId
          AND i.movesStock = 1
          AND CAST(strftime('%s', i.createdAt) AS INTEGER) * 1000
              BETWEEN audit_events.createdAt - 10000 AND audit_events.createdAt)
WHERE resource = 'cash-sales' AND method = 'POST' AND route = '/api/cash-sales'
  AND entityId IN (SELECT id FROM clients)
  AND (SELECT COUNT(*) FROM invoices i
       WHERE i.clientId = audit_events.entityId
         AND i.organizationId = audit_events.organizationId
         AND i.movesStock = 1
         AND CAST(strftime('%s', i.createdAt) AS INTEGER) * 1000
             BETWEEN audit_events.createdAt - 10000 AND audit_events.createdAt) = 1;
