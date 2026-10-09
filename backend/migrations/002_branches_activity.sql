CREATE TABLE branches(id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,org_id bigint NOT NULL REFERENCES organizations,code text NOT NULL CHECK(length(code) BETWEEN 1 AND 30),name text NOT NULL CHECK(length(name) BETWEEN 2 AND 150),address text NOT NULL DEFAULT '',UNIQUE(org_id,code),UNIQUE(org_id,id));
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
