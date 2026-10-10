ALTER TABLE users
  ADD COLUMN employee_id text GENERATED ALWAYS AS ('EMP-' || id::text) STORED;
CREATE UNIQUE INDEX users_employee_id_unique ON users(org_id,employee_id);

ALTER TABLE user_activity_logs RENAME COLUMN email_hash TO identity_hash;
ALTER TABLE user_activity_logs ADD COLUMN attempted_employee_id text NOT NULL DEFAULT '';

INSERT INTO schema_migrations(version) VALUES(6);
