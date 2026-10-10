ALTER TABLE organizations
  ADD COLUMN code text GENERATED ALWAYS AS ('ORG-' || lpad(id::text, 6, '0')) STORED;
CREATE UNIQUE INDEX organizations_code_unique ON organizations(code);
INSERT INTO schema_migrations(version) VALUES(5);
