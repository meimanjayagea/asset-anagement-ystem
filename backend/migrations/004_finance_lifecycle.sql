ALTER TABLE categories ADD COLUMN depreciation_method text NOT NULL DEFAULT 'straight_line'
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
