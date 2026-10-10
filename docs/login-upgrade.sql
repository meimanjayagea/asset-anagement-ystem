-- Generated from backend/migrations by validation/login-migration.mjs. Run as schema owner.

BEGIN;

SELECT pg_advisory_xact_lock(7842301);

CREATE TABLE IF NOT EXISTS schema_migrations(version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());

DO $upgrade$ BEGIN
IF EXISTS (SELECT 1 FROM schema_migrations WHERE version > 6) THEN
  RAISE EXCEPTION 'Database is newer than this migration bundle';
END IF;
END $upgrade$;

DO $upgrade$ BEGIN
IF NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version=1) THEN
  EXECUTE $migration_1$CREATE TABLE IF NOT EXISTS schema_migrations(version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());
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
$migration_1$;
END IF;
END $upgrade$;

DO $upgrade$ BEGIN
IF NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version=2) THEN
  EXECUTE $migration_2$CREATE TABLE branches(id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,org_id bigint NOT NULL REFERENCES organizations,code text NOT NULL CHECK(length(code) BETWEEN 1 AND 30),name text NOT NULL CHECK(length(name) BETWEEN 2 AND 150),address text NOT NULL DEFAULT '',UNIQUE(org_id,code),UNIQUE(org_id,id));
INSERT INTO branches(org_id,code,name) SELECT id,'HQ','Kantor Pusat' FROM organizations;
ALTER TABLE locations ADD COLUMN branch_id bigint;
UPDATE locations l SET branch_id=b.id FROM branches b WHERE b.org_id=l.org_id AND b.code='HQ';
ALTER TABLE locations ALTER COLUMN branch_id SET NOT NULL;
ALTER TABLE locations ADD FOREIGN KEY(org_id,branch_id) REFERENCES branches(org_id,id);
ALTER TABLE locations DROP CONSTRAINT locations_org_id_name_key;
ALTER TABLE locations ADD UNIQUE(org_id,branch_id,name);
CREATE INDEX locations_branch ON locations(org_id,branch_id);
ALTER TABLE users ADD COLUMN all_branches boolean NOT NULL DEFAULT true;
ALTER TABLE users ALTER COLUMN all_branches SET DEFAULT false;
ALTER TABLE users ADD CONSTRAINT admin_company_access CHECK(role<>'admin' OR all_branches);
CREATE TABLE user_branches(org_id bigint NOT NULL,user_id bigint NOT NULL,branch_id bigint NOT NULL,PRIMARY KEY(user_id,branch_id),FOREIGN KEY(org_id,user_id) REFERENCES users(org_id,id),FOREIGN KEY(org_id,branch_id) REFERENCES branches(org_id,id));
CREATE INDEX user_branches_scope ON user_branches(org_id,branch_id,user_id);
CREATE FUNCTION can_access_branch(company bigint,actor bigint,branch bigint) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(SELECT 1 FROM users u WHERE u.org_id=company AND u.id=actor AND u.active AND (u.all_branches OR EXISTS(SELECT 1 FROM user_branches ub WHERE ub.org_id=company AND ub.user_id=actor AND ub.branch_id=branch))) AND EXISTS(SELECT 1 FROM branches b WHERE b.org_id=company AND b.id=branch)
$$;
CREATE FUNCTION can_access_location(company bigint,actor bigint,location bigint) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(SELECT 1 FROM locations l WHERE l.org_id=company AND l.id=location AND can_access_branch(company,actor,l.branch_id))
$$;
ALTER TABLE categories ADD COLUMN maintenance_interval_days integer NOT NULL DEFAULT 0 CHECK(maintenance_interval_days BETWEEN 0 AND 3650);
ALTER TABLE categories ADD COLUMN maintenance_instructions text NOT NULL DEFAULT '' CHECK(length(maintenance_instructions)<=2000);
ALTER TABLE categories ADD COLUMN version integer NOT NULL DEFAULT 1;
ALTER TABLE assets ADD COLUMN next_maintenance_date date;
UPDATE assets a SET next_maintenance_date=a.purchase_date+c.maintenance_interval_days FROM categories c WHERE a.org_id=c.org_id AND a.category_id=c.id AND c.maintenance_interval_days>0 AND a.status<>'disposed';
CREATE INDEX assets_next_maintenance ON assets(org_id,next_maintenance_date) WHERE next_maintenance_date IS NOT NULL AND status<>'disposed';
ALTER TABLE maintenance ADD COLUMN branch_id bigint;
UPDATE maintenance m SET branch_id=l.branch_id FROM assets a JOIN locations l ON l.id=a.location_id WHERE a.id=m.asset_id;
ALTER TABLE maintenance ALTER COLUMN branch_id SET NOT NULL;
ALTER TABLE maintenance ADD FOREIGN KEY(org_id,branch_id) REFERENCES branches(org_id,id);
ALTER TABLE requests ADD COLUMN source_branch_id bigint;
ALTER TABLE requests ADD COLUMN target_branch_id bigint;
UPDATE requests r SET source_branch_id=l.branch_id FROM assets a JOIN locations l ON l.id=a.location_id WHERE a.id=r.asset_id;
UPDATE requests r SET target_branch_id=l.branch_id FROM locations l WHERE l.id=r.target_location_id;
ALTER TABLE requests ALTER COLUMN source_branch_id SET NOT NULL;
ALTER TABLE requests ADD FOREIGN KEY(org_id,source_branch_id) REFERENCES branches(org_id,id);
ALTER TABLE requests ADD FOREIGN KEY(org_id,target_branch_id) REFERENCES branches(org_id,id);
ALTER TABLE audit_logs ADD COLUMN branch_id bigint;
ALTER TABLE audit_logs ADD COLUMN related_branch_id bigint;
ALTER TABLE audit_logs ADD COLUMN request_id text NOT NULL DEFAULT '';
ALTER TABLE audit_logs ADD COLUMN peer_ip text NOT NULL DEFAULT '';
ALTER TABLE audit_logs ADD COLUMN user_agent text NOT NULL DEFAULT '';
-- Append-only audit rows intentionally remain unchanged; legacy branch metadata stays NULL.
CREATE TABLE user_activity_logs(
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,org_id bigint REFERENCES organizations,actor_id bigint REFERENCES users,actor_name text NOT NULL DEFAULT '',
 actor_branch_ids bigint[] NOT NULL DEFAULT '{}',all_branches boolean NOT NULL DEFAULT false,branch_id bigint,related_branch_id bigint,
 event text NOT NULL,method text NOT NULL,path text NOT NULL,status_code integer NOT NULL,duration_ms bigint NOT NULL,
 request_id text NOT NULL,peer_ip text NOT NULL,user_agent text NOT NULL,email_hash text NOT NULL DEFAULT '',attempted_email text NOT NULL DEFAULT '',created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX activity_org_listing ON user_activity_logs(org_id,id DESC);
CREATE INDEX activity_actor ON user_activity_logs(org_id,actor_id,id DESC);
CREATE INDEX activity_branches ON user_activity_logs USING gin(actor_branch_ids);
CREATE TRIGGER activity_no_change BEFORE UPDATE OR DELETE ON user_activity_logs FOR EACH ROW EXECUTE FUNCTION audit_immutable();
INSERT INTO schema_migrations(version) VALUES(2);
$migration_2$;
END IF;
END $upgrade$;

DO $upgrade$ BEGIN
IF NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version=3) THEN
  EXECUTE $migration_3$ALTER TABLE users DROP CONSTRAINT users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK(role IN (
 'admin','branch_admin','manager','operator','staff','employee',
 'finance','it_support','it_developer','auditor'
));
ALTER TABLE users ADD COLUMN deleted_at timestamptz;
ALTER TABLE branches ADD COLUMN deleted_at timestamptz;
ALTER TABLE categories ADD COLUMN branch_id bigint;
ALTER TABLE categories ADD CONSTRAINT categories_org_branch_fkey
 FOREIGN KEY(org_id,branch_id) REFERENCES branches(org_id,id);
ALTER TABLE categories DROP CONSTRAINT categories_org_id_name_key;
CREATE UNIQUE INDEX categories_global_name ON categories(org_id,name) WHERE branch_id IS NULL;
CREATE UNIQUE INDEX categories_branch_name ON categories(org_id,branch_id,name) WHERE branch_id IS NOT NULL;
CREATE INDEX categories_branch ON categories(org_id,branch_id);

INSERT INTO user_branches(org_id,user_id,branch_id)
 SELECT u.org_id,u.id,b.id FROM users u JOIN branches b ON b.org_id=u.org_id
 WHERE u.role<>'admin' AND u.all_branches AND b.deleted_at IS NULL
 ON CONFLICT DO NOTHING;
UPDATE users SET all_branches=(role='admin');
ALTER TABLE users ADD CONSTRAINT admin_company_wide_scope
 CHECK((role='admin' AND all_branches) OR (role<>'admin' AND NOT all_branches));

ALTER TABLE sessions ADD COLUMN active_branch_id bigint NOT NULL DEFAULT 0
 CHECK(active_branch_id>=0);

ALTER TABLE locations ADD COLUMN deleted_at timestamptz;
ALTER TABLE categories ADD COLUMN deleted_at timestamptz;
ALTER TABLE assets ADD COLUMN deleted_at timestamptz;

CREATE OR REPLACE FUNCTION can_access_branch(company bigint,actor bigint,branch bigint) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(
   SELECT 1 FROM users u
   WHERE u.org_id=company AND u.id=actor AND u.active AND u.deleted_at IS NULL
     AND (u.all_branches OR EXISTS(
       SELECT 1 FROM user_branches ub
       WHERE ub.org_id=company AND ub.user_id=actor AND ub.branch_id=branch
     ))
 ) AND EXISTS(
   SELECT 1 FROM branches b
   JOIN users u ON u.org_id=b.org_id AND u.id=actor
   WHERE b.org_id=company AND b.id=branch AND b.deleted_at IS NULL
     AND (u.role='admin' OR b.code<>'HQ')
 )
$$;

CREATE OR REPLACE FUNCTION can_access_location(company bigint,actor bigint,location bigint) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(
   SELECT 1 FROM locations l
   WHERE l.org_id=company AND l.id=location AND l.deleted_at IS NULL
     AND can_access_branch(company,actor,l.branch_id)
 )
$$;

CREATE OR REPLACE FUNCTION enforce_branch_admin_scope() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
 target_user bigint;
 target_role text;
 branch_count bigint;
 targets_hq boolean;
BEGIN
 IF TG_TABLE_NAME='users' THEN
   target_user := NEW.id;
 ELSIF TG_OP='DELETE' THEN
   target_user := OLD.user_id;
 ELSE
   target_user := NEW.user_id;
 END IF;
 SELECT role INTO target_role FROM users WHERE id=target_user;
 IF target_role='branch_admin' THEN
   SELECT count(*),COALESCE(bool_or(b.code='HQ'),false) INTO branch_count,targets_hq
   FROM user_branches ub JOIN branches b ON b.id=ub.branch_id
   WHERE ub.user_id=target_user;
   IF branch_count<>1 OR targets_hq THEN
     RAISE EXCEPTION 'branch_admin must have exactly one non-HQ branch' USING ERRCODE='23514';
   END IF;
 END IF;
 RETURN NULL;
END $$;

CREATE CONSTRAINT TRIGGER branch_admin_scope_users
 AFTER INSERT OR UPDATE ON users
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
 EXECUTE FUNCTION enforce_branch_admin_scope();
CREATE CONSTRAINT TRIGGER branch_admin_scope_assignments
 AFTER INSERT OR UPDATE OR DELETE ON user_branches
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
 EXECUTE FUNCTION enforce_branch_admin_scope();

CREATE INDEX assets_active_branch ON assets(org_id,location_id,id DESC) WHERE deleted_at IS NULL;
CREATE INDEX users_active_org ON users(org_id,id) WHERE deleted_at IS NULL;
INSERT INTO schema_migrations(version) VALUES(3);
$migration_3$;
END IF;
END $upgrade$;

DO $upgrade$ BEGIN
IF NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version=4) THEN
  EXECUTE $migration_4$ALTER TABLE categories ADD COLUMN depreciation_method text NOT NULL DEFAULT 'straight_line'
 CHECK(depreciation_method IN ('straight_line','declining_balance','non_depreciable'));
ALTER TABLE assets ADD COLUMN depreciation_method text NOT NULL DEFAULT 'straight_line'
 CHECK(depreciation_method IN ('straight_line','declining_balance','non_depreciable'));
ALTER TABLE assets ADD COLUMN depreciation_start_date date;
UPDATE assets SET depreciation_start_date=purchase_date WHERE depreciation_start_date IS NULL;
ALTER TABLE assets ALTER COLUMN depreciation_start_date SET NOT NULL;
ALTER TABLE assets ADD COLUMN supplier_name text NOT NULL DEFAULT '' CHECK(length(supplier_name)<=200);
ALTER TABLE assets ADD COLUMN acquisition_reference text NOT NULL DEFAULT '' CHECK(length(acquisition_reference)<=100);
ALTER TABLE assets ADD COLUMN disposed_at timestamptz;
ALTER TABLE assets ADD COLUMN disposal_proceeds bigint NOT NULL DEFAULT 0 CHECK(disposal_proceeds>=0);
ALTER TABLE assets ADD COLUMN disposal_book_value bigint CHECK(disposal_book_value>=0);
ALTER TABLE assets ADD COLUMN disposal_gross_value bigint CHECK(disposal_gross_value>=0);
ALTER TABLE assets ADD COLUMN disposal_accumulated_depreciation bigint CHECK(disposal_accumulated_depreciation>=0);
ALTER TABLE requests ADD COLUMN disposal_proceeds bigint NOT NULL DEFAULT 0 CHECK(disposal_proceeds>=0);

CREATE TABLE asset_movements(
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 org_id bigint NOT NULL,
 asset_id bigint NOT NULL,
 actor_id bigint NOT NULL,
 event text NOT NULL CHECK(event IN ('registered','assigned','returned','transferred','relocated','disposed')),
 from_location_id bigint,
 to_location_id bigint,
 from_branch_id bigint,
 to_branch_id bigint,
 from_custodian text NOT NULL DEFAULT '',
 to_custodian text NOT NULL DEFAULT '',
 note text NOT NULL DEFAULT '',
 occurred_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(org_id,asset_id) REFERENCES assets(org_id,id),
 FOREIGN KEY(org_id,actor_id) REFERENCES users(org_id,id),
 FOREIGN KEY(org_id,from_location_id) REFERENCES locations(org_id,id),
 FOREIGN KEY(org_id,to_location_id) REFERENCES locations(org_id,id),
 FOREIGN KEY(org_id,from_branch_id) REFERENCES branches(org_id,id),
 FOREIGN KEY(org_id,to_branch_id) REFERENCES branches(org_id,id)
);
CREATE INDEX asset_movements_timeline ON asset_movements(org_id,asset_id,id DESC);
CREATE FUNCTION immutable_asset_movement() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'asset movement history is append-only'; END $$;
CREATE TRIGGER asset_movement_no_change BEFORE UPDATE OR DELETE ON asset_movements
 FOR EACH ROW EXECUTE FUNCTION immutable_asset_movement();

CREATE TABLE asset_valuations(
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 org_id bigint NOT NULL,
 asset_id bigint NOT NULL,
 branch_id bigint NOT NULL,
 requested_by bigint NOT NULL,
 decided_by bigint,
 effective_date date NOT NULL,
 carrying_value_before bigint NOT NULL CHECK(carrying_value_before>=0),
 revalued_amount bigint NOT NULL CHECK(revalued_amount>=0),
 remaining_life_months integer NOT NULL CHECK(remaining_life_months BETWEEN 1 AND 1200),
 reason text NOT NULL CHECK(length(reason) BETWEEN 3 AND 1000),
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected')),
 decision_note text NOT NULL DEFAULT '',
 expected_version integer NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 decided_at timestamptz,
 FOREIGN KEY(org_id,asset_id) REFERENCES assets(org_id,id),
 FOREIGN KEY(org_id,branch_id) REFERENCES branches(org_id,id),
 FOREIGN KEY(org_id,requested_by) REFERENCES users(org_id,id),
 FOREIGN KEY(org_id,decided_by) REFERENCES users(org_id,id),
 CHECK(decided_by IS NULL OR decided_by<>requested_by)
);
CREATE UNIQUE INDEX one_pending_asset_valuation ON asset_valuations(org_id,asset_id) WHERE status='pending';
CREATE INDEX asset_valuations_listing ON asset_valuations(org_id,branch_id,id DESC);

CREATE TABLE service_contracts(
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 org_id bigint NOT NULL,
 branch_id bigint NOT NULL,
 asset_id bigint,
 name text NOT NULL CHECK(length(name) BETWEEN 2 AND 200),
 vendor text NOT NULL CHECK(length(vendor) BETWEEN 2 AND 200),
 contract_number text NOT NULL DEFAULT '' CHECK(length(contract_number)<=100),
 start_date date NOT NULL,
 end_date date NOT NULL,
 renewal_notice_days integer NOT NULL DEFAULT 30 CHECK(renewal_notice_days BETWEEN 0 AND 3650),
 annual_cost bigint NOT NULL DEFAULT 0 CHECK(annual_cost>=0),
 notes text NOT NULL DEFAULT '' CHECK(length(notes)<=2000),
 created_by bigint NOT NULL,
 deleted_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(org_id,branch_id) REFERENCES branches(org_id,id),
 FOREIGN KEY(org_id,asset_id) REFERENCES assets(org_id,id),
 FOREIGN KEY(org_id,created_by) REFERENCES users(org_id,id),
 CHECK(end_date>=start_date)
);
CREATE INDEX service_contracts_expiry ON service_contracts(org_id,branch_id,end_date) WHERE deleted_at IS NULL;

CREATE TABLE accounting_profiles(
 org_id bigint PRIMARY KEY REFERENCES organizations(id),
 asset_account text NOT NULL DEFAULT '1500' CHECK(length(asset_account) BETWEEN 1 AND 40),
 accumulated_depreciation_account text NOT NULL DEFAULT '1590' CHECK(length(accumulated_depreciation_account) BETWEEN 1 AND 40),
 depreciation_expense_account text NOT NULL DEFAULT '6000' CHECK(length(depreciation_expense_account) BETWEEN 1 AND 40),
 cash_account text NOT NULL DEFAULT '1100' CHECK(length(cash_account) BETWEEN 1 AND 40),
 disposal_gain_account text NOT NULL DEFAULT '7990' CHECK(length(disposal_gain_account) BETWEEN 1 AND 40),
 disposal_loss_account text NOT NULL DEFAULT '6990' CHECK(length(disposal_loss_account) BETWEEN 1 AND 40),
 revaluation_reserve_account text NOT NULL DEFAULT '3100' CHECK(length(revaluation_reserve_account) BETWEEN 1 AND 40),
 impairment_expense_account text NOT NULL DEFAULT '6900' CHECK(length(impairment_expense_account) BETWEEN 1 AND 40),
 updated_by bigint NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(org_id,updated_by) REFERENCES users(org_id,id)
);
INSERT INTO schema_migrations(version) VALUES(4);
$migration_4$;
END IF;
END $upgrade$;

DO $upgrade$ BEGIN
IF NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version=5) THEN
  EXECUTE $migration_5$ALTER TABLE organizations
  ADD COLUMN code text GENERATED ALWAYS AS ('ORG-' || lpad(id::text, 6, '0')) STORED;
CREATE UNIQUE INDEX organizations_code_unique ON organizations(code);
INSERT INTO schema_migrations(version) VALUES(5);
$migration_5$;
END IF;
END $upgrade$;

DO $upgrade$ BEGIN
IF NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version=6) THEN
  EXECUTE $migration_6$ALTER TABLE users
  ADD COLUMN employee_id text GENERATED ALWAYS AS ('EMP-' || id::text) STORED;
CREATE UNIQUE INDEX users_employee_id_unique ON users(org_id,employee_id);

ALTER TABLE user_activity_logs RENAME COLUMN email_hash TO identity_hash;
ALTER TABLE user_activity_logs ADD COLUMN attempted_employee_id text NOT NULL DEFAULT '';

INSERT INTO schema_migrations(version) VALUES(6);
$migration_6$;
END IF;
END $upgrade$;

COMMIT;
