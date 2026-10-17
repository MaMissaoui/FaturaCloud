-- Recreates the empty legacy tables exactly as 0012, 0014 and 0015 left them.
-- Create tags table
CREATE TABLE tags (
  id TEXT(21) PRIMARY KEY NOT NULL,
  organizationId TEXT NOT NULL,
  name TEXT NOT NULL,
  color TEXT NOT NULL,
  createdAt TEXT DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (organizationId) REFERENCES organizations(id) ON DELETE CASCADE
);
CREATE INDEX idx_tags_organizationId ON tags(organizationId);
CREATE INDEX idx_tags_name ON tags(name);

-- Create timeEntries table
CREATE TABLE timeEntries (
  id TEXT(21) PRIMARY KEY NOT NULL,
  organizationId TEXT NOT NULL,
  clientId TEXT,
  description TEXT,
  startTime INTEGER NOT NULL,
  endTime INTEGER,
  duration INTEGER DEFAULT 0,
  tags TEXT DEFAULT '[]',
  isBillable INTEGER DEFAULT 1,
  hourlyRate REAL,
  createdAt TEXT DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (organizationId) REFERENCES organizations(id) ON DELETE CASCADE,
  FOREIGN KEY (clientId) REFERENCES clients(id) ON DELETE SET NULL
);
CREATE INDEX idx_timeEntries_organizationId ON timeEntries(organizationId);
CREATE INDEX idx_timeEntries_clientId ON timeEntries(clientId);
CREATE INDEX idx_timeEntries_startTime ON timeEntries(startTime);
CREATE INDEX idx_timeEntries_endTime ON timeEntries(endTime);
-- Create projects table
CREATE TABLE projects (
  id TEXT(21) PRIMARY KEY NOT NULL,
  organizationId TEXT NOT NULL,
  name TEXT NOT NULL,
  clientId TEXT,
  startDate INTEGER,
  endDate INTEGER,
  archivedAt INTEGER,
  createdAt TEXT DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (organizationId) REFERENCES organizations(id) ON DELETE CASCADE,
  FOREIGN KEY (clientId) REFERENCES clients(id) ON DELETE SET NULL
);
CREATE INDEX idx_projects_organizationId ON projects(organizationId);
CREATE INDEX idx_projects_clientId ON projects(clientId);
CREATE INDEX idx_projects_name ON projects(name);
CREATE INDEX idx_projects_archivedAt ON projects(archivedAt);
-- Add projectId column to timeEntries table
ALTER TABLE timeEntries 
ADD COLUMN projectId TEXT;

-- Add foreign key constraint
CREATE INDEX idx_timeEntries_projectId ON timeEntries(projectId);

-- Note: Foreign key constraint will be handled at the application level
-- to avoid SQLite limitations with adding foreign keys to existing tables
