-- Unknown legacy actor/timestamps remain NULL instead of inventing history.
CREATE FUNCTION stamp_record_metadata() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE actor bigint; fields jsonb;
BEGIN
 actor := NULLIF(current_setting('app.actor_id',true),'')::bigint;
 fields := to_jsonb(NEW);
 IF TG_OP='INSERT' THEN
   NEW.created_at := COALESCE(NEW.created_at,clock_timestamp());
   NEW.created_by := COALESCE(actor,NEW.created_by,
     NULLIF(fields->>'actor_id','')::bigint,
     NULLIF(fields->>'uploaded_by','')::bigint,
     NULLIF(fields->>'requested_by','')::bigint,
     NULLIF(fields->>'user_id','')::bigint);
   NEW.updated_at := COALESCE(NEW.updated_at,NEW.created_at);
   NEW.updated_by := COALESCE(actor,NEW.updated_by,NEW.created_by);
 ELSE
   NEW.created_at := OLD.created_at;
   NEW.created_by := OLD.created_by;
   NEW.updated_at := clock_timestamp();
   NEW.updated_by := actor;
 END IF;
 RETURN NEW;
END $$;

DO $$
DECLARE tbl text;
BEGIN
 FOREACH tbl IN ARRAY ARRAY['schema_migrations','organizations','users','sessions','locations','categories','assets','requests','maintenance','audit_logs','stocktakes','stocktake_items','branches','user_branches','user_activity_logs','asset_movements','asset_valuations','service_contracts','accounting_profiles','asset_photos','asset_photo_links','asset_loans','notification_receipts'] LOOP
   EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS created_at timestamptz',tbl);
   EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS created_by bigint',tbl);
   EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS updated_at timestamptz',tbl);
   EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS updated_by bigint',tbl);
   EXECUTE format('ALTER TABLE %I ALTER COLUMN created_at SET DEFAULT now()',tbl);
   EXECUTE format('ALTER TABLE %I ALTER COLUMN updated_at SET DEFAULT now()',tbl);
   EXECUTE format('CREATE TRIGGER record_metadata BEFORE INSERT OR UPDATE ON %I FOR EACH ROW EXECUTE FUNCTION stamp_record_metadata()',tbl);
 END LOOP;
END $$;
INSERT INTO schema_migrations(version) VALUES(8);
