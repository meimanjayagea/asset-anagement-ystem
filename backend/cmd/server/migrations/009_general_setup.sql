ALTER TABLE branches ADD COLUMN business_code text NOT NULL DEFAULT '' CHECK(length(business_code)<=30);
ALTER TABLE branches ADD COLUMN version integer NOT NULL DEFAULT 1;
ALTER TABLE locations ADD COLUMN version integer NOT NULL DEFAULT 1;
ALTER TABLE users ADD COLUMN version integer NOT NULL DEFAULT 1;

CREATE TABLE general_codes(
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 org_id bigint NOT NULL REFERENCES organizations,
 code text NOT NULL CHECK(code ~ '^[A-Z][A-Z0-9_]{1,39}$'),
 name text NOT NULL CHECK(length(name) BETWEEN 2 AND 150),
 company_prefix text NOT NULL CHECK(company_prefix ~ '^[A-Z0-9]{1,8}$'),
 description text NOT NULL DEFAULT '' CHECK(length(description)<=1000),
 version integer NOT NULL DEFAULT 1,
 deleted_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(), created_by bigint,
 updated_at timestamptz NOT NULL DEFAULT now(), updated_by bigint,
 UNIQUE(org_id,id), UNIQUE(org_id,code)
);
CREATE TABLE general_code_details(
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 org_id bigint NOT NULL,
 general_code_id bigint NOT NULL,
 code text NOT NULL CHECK(code ~ '^[A-Z][A-Z0-9_]{1,39}$'),
 name text NOT NULL CHECK(length(name) BETWEEN 2 AND 150),
 prefix text NOT NULL CHECK(prefix ~ '^[A-Z0-9]{1,8}$'),
 entity_type text NOT NULL CHECK(entity_type IN ('hq','branch','reference')),
 sequence_width integer NOT NULL DEFAULT 6 CHECK(sequence_width BETWEEN 3 AND 8),
 next_number bigint NOT NULL DEFAULT 1 CHECK(next_number BETWEEN 1 AND 100000000),
 version integer NOT NULL DEFAULT 1,
 deleted_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(), created_by bigint,
 updated_at timestamptz NOT NULL DEFAULT now(), updated_by bigint,
 UNIQUE(org_id,id), UNIQUE(org_id,general_code_id,code), UNIQUE(org_id,general_code_id,prefix),
 FOREIGN KEY(org_id,general_code_id) REFERENCES general_codes(org_id,id)
);
CREATE TABLE issued_general_codes(
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 org_id bigint NOT NULL,
 detail_id bigint NOT NULL,
 code text NOT NULL CHECK(length(code)<=30),
 entity_type text NOT NULL CHECK(entity_type IN ('hq','branch','reference')),
 entity_id bigint NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), created_by bigint,
 updated_at timestamptz NOT NULL DEFAULT now(), updated_by bigint,
 UNIQUE(org_id,code), UNIQUE(org_id,entity_type,entity_id),
 FOREIGN KEY(org_id,detail_id) REFERENCES general_code_details(org_id,id)
);
CREATE TRIGGER issued_code_no_change BEFORE UPDATE OR DELETE ON issued_general_codes FOR EACH ROW EXECUTE FUNCTION audit_immutable();
CREATE TRIGGER record_metadata BEFORE INSERT OR UPDATE ON general_codes FOR EACH ROW EXECUTE FUNCTION stamp_record_metadata();
CREATE TRIGGER record_metadata BEFORE INSERT OR UPDATE ON general_code_details FOR EACH ROW EXECUTE FUNCTION stamp_record_metadata();
CREATE TRIGGER record_metadata BEFORE INSERT OR UPDATE ON issued_general_codes FOR EACH ROW EXECUTE FUNCTION stamp_record_metadata();
CREATE FUNCTION bump_master_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN NEW.version := OLD.version+1; RETURN NEW; END $$;
CREATE TRIGGER bump_version BEFORE UPDATE ON branches FOR EACH ROW EXECUTE FUNCTION bump_master_version();
CREATE TRIGGER bump_version BEFORE UPDATE ON locations FOR EACH ROW EXECUTE FUNCTION bump_master_version();
CREATE TRIGGER bump_version BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION bump_master_version();
CREATE TABLE demo_seed_runs(
 org_id bigint NOT NULL REFERENCES organizations,
 seed_key text NOT NULL,
 summary jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), created_by bigint,
 updated_at timestamptz NOT NULL DEFAULT now(), updated_by bigint,
 PRIMARY KEY(org_id,seed_key)
);
CREATE TRIGGER record_metadata BEFORE INSERT OR UPDATE ON demo_seed_runs FOR EACH ROW EXECUTE FUNCTION stamp_record_metadata();
CREATE TRIGGER demo_seed_no_change BEFORE UPDATE OR DELETE ON demo_seed_runs FOR EACH ROW EXECUTE FUNCTION audit_immutable();
INSERT INTO schema_migrations(version) VALUES(9);

