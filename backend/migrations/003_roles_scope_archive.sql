ALTER TABLE users DROP CONSTRAINT users_role_check;
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
