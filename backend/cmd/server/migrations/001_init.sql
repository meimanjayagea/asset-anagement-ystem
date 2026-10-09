CREATE TABLE IF NOT EXISTS schema_migrations(version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE organizations(id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, name text NOT NULL);
CREATE TABLE users(id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, org_id bigint NOT NULL REFERENCES organizations, email text NOT NULL, name text NOT NULL, password_hash text NOT NULL, role text NOT NULL CHECK(role IN ('admin','manager','operator','auditor')), active boolean NOT NULL DEFAULT true, UNIQUE(org_id,email), UNIQUE(org_id,id));
CREATE TABLE sessions(token_hash text PRIMARY KEY, user_id bigint NOT NULL REFERENCES users, expires_at timestamptz NOT NULL);
CREATE INDEX sessions_expiry ON sessions(expires_at);
CREATE TABLE locations(id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, org_id bigint NOT NULL REFERENCES organizations, name text NOT NULL, UNIQUE(org_id,name), UNIQUE(org_id,id));
CREATE TABLE categories(id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, org_id bigint NOT NULL REFERENCES organizations, name text NOT NULL, useful_life_months integer NOT NULL CHECK(useful_life_months BETWEEN 1 AND 1200), UNIQUE(org_id,name), UNIQUE(org_id,id));
CREATE TABLE assets(
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, org_id bigint NOT NULL REFERENCES organizations,
 tag text NOT NULL CHECK(length(tag) BETWEEN 1 AND 80), name text NOT NULL CHECK(length(name) BETWEEN 1 AND 200), serial_number text NOT NULL DEFAULT '',
 category_id bigint NOT NULL, location_id bigint NOT NULL,
 custodian text NOT NULL DEFAULT '', status text NOT NULL DEFAULT 'available' CHECK(status IN ('available','assigned','maintenance','disposed')),
 purchase_date date NOT NULL, purchase_cost bigint NOT NULL CHECK(purchase_cost>=0), salvage_value bigint NOT NULL DEFAULT 0 CHECK(salvage_value>=0 AND salvage_value<=purchase_cost),
 useful_life_months integer NOT NULL CHECK(useful_life_months BETWEEN 1 AND 1200), warranty_until date,
 version integer NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(org_id,tag), UNIQUE(org_id,id), FOREIGN KEY(org_id,category_id) REFERENCES categories(org_id,id), FOREIGN KEY(org_id,location_id) REFERENCES locations(org_id,id),
 CHECK((status='assigned' AND length(trim(custodian))>0) OR (status<>'assigned' AND custodian=''))
);
CREATE UNIQUE INDEX assets_serial_unique ON assets(org_id,serial_number) WHERE serial_number<>'';
CREATE INDEX assets_listing ON assets(org_id,id DESC);
CREATE INDEX assets_status ON assets(org_id,status);
CREATE INDEX assets_location ON assets(org_id,location_id);
CREATE TABLE requests(
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, org_id bigint NOT NULL REFERENCES organizations, asset_id bigint NOT NULL,
 kind text NOT NULL CHECK(kind IN ('transfer','dispose')), target_location_id bigint, reason text NOT NULL CHECK(length(reason) BETWEEN 3 AND 1000),
 expected_version integer NOT NULL, status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected')),
 requested_by bigint NOT NULL, decided_by bigint, decision_note text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(), decided_at timestamptz,
 FOREIGN KEY(org_id,asset_id) REFERENCES assets(org_id,id), FOREIGN KEY(org_id,target_location_id) REFERENCES locations(org_id,id),
 FOREIGN KEY(org_id,requested_by) REFERENCES users(org_id,id), FOREIGN KEY(org_id,decided_by) REFERENCES users(org_id,id),
 CHECK((kind='transfer' AND target_location_id IS NOT NULL) OR (kind='dispose' AND target_location_id IS NULL))
);
CREATE UNIQUE INDEX one_pending_request ON requests(org_id,asset_id) WHERE status='pending';
CREATE INDEX requests_listing ON requests(org_id,status,id DESC);
CREATE TABLE maintenance(
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, org_id bigint NOT NULL REFERENCES organizations, asset_id bigint NOT NULL, title text NOT NULL,
 due_date date NOT NULL, status text NOT NULL DEFAULT 'scheduled' CHECK(status IN ('scheduled','in_progress','completed','cancelled')),
 cost bigint NOT NULL DEFAULT 0 CHECK(cost>=0), notes text NOT NULL DEFAULT '', completed_at timestamptz,
 FOREIGN KEY(org_id,asset_id) REFERENCES assets(org_id,id)
);
CREATE UNIQUE INDEX one_active_maintenance ON maintenance(org_id,asset_id) WHERE status IN ('scheduled','in_progress');
CREATE INDEX maintenance_due ON maintenance(org_id,status,due_date);
CREATE TABLE audit_logs(id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, org_id bigint NOT NULL REFERENCES organizations, actor_id bigint NOT NULL, action text NOT NULL, entity text NOT NULL, entity_id bigint NOT NULL, before_data jsonb, after_data jsonb, created_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX audit_listing ON audit_logs(org_id,id DESC);
CREATE FUNCTION audit_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit log is append-only'; END $$;
CREATE TRIGGER audit_no_change BEFORE UPDATE OR DELETE ON audit_logs FOR EACH ROW EXECUTE FUNCTION audit_immutable();
CREATE TABLE stocktakes(id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,org_id bigint NOT NULL REFERENCES organizations,location_id bigint NOT NULL,title text NOT NULL,status text NOT NULL DEFAULT 'open' CHECK(status IN ('open','closed')),created_by bigint NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),closed_at timestamptz,FOREIGN KEY(org_id,location_id) REFERENCES locations(org_id,id),FOREIGN KEY(org_id,created_by) REFERENCES users(org_id,id),UNIQUE(org_id,id));
CREATE UNIQUE INDEX one_open_stocktake ON stocktakes(org_id,location_id) WHERE status='open';
CREATE TABLE stocktake_items(org_id bigint NOT NULL,stocktake_id bigint NOT NULL,asset_id bigint NOT NULL,expected_version integer NOT NULL,expected_tag text NOT NULL,observed boolean NOT NULL DEFAULT false,notes text NOT NULL DEFAULT '',observed_by bigint,observed_at timestamptz,PRIMARY KEY(stocktake_id,asset_id),FOREIGN KEY(org_id,stocktake_id) REFERENCES stocktakes(org_id,id),FOREIGN KEY(org_id,asset_id) REFERENCES assets(org_id,id),FOREIGN KEY(org_id,observed_by) REFERENCES users(org_id,id));
INSERT INTO schema_migrations(version) VALUES(1);
