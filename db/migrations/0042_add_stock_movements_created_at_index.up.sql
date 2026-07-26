CREATE INDEX idx_stockMovements_organizationId_createdAt
  ON stockMovements(organizationId, createdAt DESC);
