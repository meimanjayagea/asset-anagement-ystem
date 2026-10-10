ALTER TABLE assets ADD COLUMN brand text NOT NULL DEFAULT '' CHECK(length(brand)<=100);
ALTER TABLE assets ADD COLUMN model text NOT NULL DEFAULT '' CHECK(length(model)<=100);
ALTER TABLE assets ADD COLUMN specifications text NOT NULL DEFAULT '' CHECK(length(specifications)<=4000);
ALTER TABLE assets ADD COLUMN rfid_tag text NOT NULL DEFAULT '' CHECK(length(rfid_tag)<=128);
CREATE UNIQUE INDEX assets_rfid_unique ON assets(org_id,rfid_tag) WHERE rfid_tag<>'' AND deleted_at IS NULL;
ALTER TABLE locations ADD COLUMN building text NOT NULL DEFAULT '' CHECK(length(building)<=100);
ALTER TABLE locations ADD COLUMN floor text NOT NULL DEFAULT '' CHECK(length(floor)<=40);
ALTER TABLE locations ADD COLUMN room text NOT NULL DEFAULT '' CHECK(length(room)<=100);

CREATE TABLE asset_photos(
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 org_id bigint NOT NULL,
 asset_id bigint NOT NULL,
 purpose text NOT NULL CHECK(purpose IN ('catalog','damage','repair','disposal','checkout','return','audit')),
 caption text NOT NULL DEFAULT '' CHECK(length(caption)<=200),
 content bytea NOT NULL CHECK(octet_length(content) BETWEEN 1 AND 524288),
 mime_type text NOT NULL DEFAULT 'image/jpeg' CHECK(mime_type='image/jpeg'),
 width integer NOT NULL CHECK(width BETWEEN 1 AND 2048),
 height integer NOT NULL CHECK(height BETWEEN 1 AND 2048),
 uploaded_by bigint NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(org_id,id),
 FOREIGN KEY(org_id,asset_id) REFERENCES assets(org_id,id),
 FOREIGN KEY(org_id,uploaded_by) REFERENCES users(org_id,id)
);
CREATE INDEX asset_photos_listing ON asset_photos(org_id,asset_id,id);
CREATE TABLE asset_photo_links(
 org_id bigint NOT NULL,
 photo_id bigint NOT NULL,
 entity text NOT NULL CHECK(entity IN ('request','maintenance_before','maintenance_after','stocktake_item','loan_before','loan_after')),
 entity_id bigint NOT NULL,
 PRIMARY KEY(photo_id,entity,entity_id),
 UNIQUE(photo_id),
 FOREIGN KEY(org_id,photo_id) REFERENCES asset_photos(org_id,id)
);
CREATE TRIGGER photo_no_change BEFORE UPDATE OR DELETE ON asset_photos FOR EACH ROW EXECUTE FUNCTION audit_immutable();
CREATE TRIGGER photo_links_no_change BEFORE UPDATE OR DELETE ON asset_photo_links FOR EACH ROW EXECUTE FUNCTION audit_immutable();

CREATE TABLE asset_loans(
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 org_id bigint NOT NULL,
 asset_id bigint NOT NULL,
 branch_id bigint NOT NULL,
 borrower_id bigint NOT NULL,
 due_at timestamptz NOT NULL,
 reason text NOT NULL CHECK(length(reason) BETWEEN 3 AND 1000),
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','checked_out','returned','rejected','cancelled')),
 expected_version integer NOT NULL,
 checked_out_at timestamptz,
 returned_at timestamptz,
 handed_over_by bigint,
 received_by bigint,
 condition_before text NOT NULL DEFAULT '',
 condition_after text NOT NULL DEFAULT '',
 return_condition text CHECK(return_condition IN ('good','damaged')),
 decision_note text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(org_id,asset_id) REFERENCES assets(org_id,id),
 FOREIGN KEY(org_id,branch_id) REFERENCES branches(org_id,id),
 FOREIGN KEY(org_id,borrower_id) REFERENCES users(org_id,id),
 FOREIGN KEY(org_id,handed_over_by) REFERENCES users(org_id,id),
 FOREIGN KEY(org_id,received_by) REFERENCES users(org_id,id)
);
CREATE UNIQUE INDEX one_active_asset_loan ON asset_loans(org_id,asset_id) WHERE status IN ('pending','checked_out');
CREATE INDEX asset_loans_due ON asset_loans(org_id,branch_id,due_at) WHERE status='checked_out';

ALTER TABLE maintenance ADD COLUMN kind text NOT NULL DEFAULT 'preventive' CHECK(kind IN ('preventive','repair'));
ALTER TABLE maintenance ADD COLUMN requested_by bigint;
ALTER TABLE maintenance ADD COLUMN assigned_to bigint;
ALTER TABLE maintenance ADD COLUMN checklist jsonb NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(checklist)='array');
ALTER TABLE maintenance ADD COLUMN parts jsonb NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(parts)='array');
ALTER TABLE maintenance ADD COLUMN evidence_required boolean NOT NULL DEFAULT false;
ALTER TABLE maintenance ALTER COLUMN evidence_required SET DEFAULT true;
ALTER TABLE maintenance ADD FOREIGN KEY(org_id,requested_by) REFERENCES users(org_id,id);
ALTER TABLE maintenance ADD FOREIGN KEY(org_id,assigned_to) REFERENCES users(org_id,id);

ALTER TABLE requests ADD COLUMN disposal_method text NOT NULL DEFAULT 'write_off' CHECK(disposal_method IN ('write_off','sale','abandonment'));
ALTER TABLE requests ADD COLUMN evidence_required boolean NOT NULL DEFAULT false;
ALTER TABLE requests ALTER COLUMN evidence_required SET DEFAULT true;
ALTER TABLE stocktake_items ADD COLUMN finding text NOT NULL DEFAULT 'unverified' CHECK(finding IN ('unverified','present','missing','damaged','relocated'));
ALTER TABLE stocktake_items ADD COLUMN found_location_id bigint;
ALTER TABLE stocktake_items ADD FOREIGN KEY(org_id,found_location_id) REFERENCES locations(org_id,id);
UPDATE stocktake_items SET finding='present' WHERE observed;
-- Preserve historical discrepancies; evidence requirements apply to new observations.
UPDATE stocktake_items i SET finding='missing' FROM stocktakes s
 WHERE s.org_id=i.org_id AND s.id=i.stocktake_id AND s.status='closed' AND NOT i.observed;

CREATE TABLE notification_receipts(
 org_id bigint NOT NULL,
 user_id bigint NOT NULL,
 notification_key text NOT NULL CHECK(length(notification_key)<=200),
 read_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(user_id,notification_key),
 FOREIGN KEY(org_id,user_id) REFERENCES users(org_id,id)
);
INSERT INTO schema_migrations(version) VALUES(7);
