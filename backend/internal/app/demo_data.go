package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

const demoSeedKey = "lifecycle-demo-v1"

func (s *Server) seedDemoEndpoint(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Confirmation string `json:"confirmation"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if in.Confirmation != "SEED_DEMO_KEEP_EXISTING" {
		return fail(422, "Konfirmasi pengisian DEMO diperlukan")
	}
	var code string
	if err := s.DB.QueryRow(r.Context(), `SELECT code FROM organizations WHERE id=$1`, actor(r).OrgID).Scan(&code); err != nil {
		return err
	}
	result, err := s.SeedDemo(r.Context(), code, actor(r).ID)
	if err != nil {
		return err
	}
	write(w, 200, result)
	return nil
}

// Explicit, transactional, additive seed. Replays return the original manifest.
func (s *Server) SeedDemo(ctx context.Context, orgCode string, actorID ...int64) (map[string]any, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var org, admin int64
	var requested int64
	if len(actorID) > 0 {
		requested = actorID[0]
	}
	if err = tx.QueryRow(ctx, `SELECT o.id,u.id FROM organizations o JOIN users u ON u.org_id=o.id WHERE o.code=$1 AND u.role='admin' AND u.active AND u.deleted_at IS NULL AND ($2::bigint=0 OR u.id=$2) ORDER BY u.id LIMIT 1`, orgCode, requested).Scan(&org, &admin); err != nil {
		return nil, fmt.Errorf("DEMO requires existing organization and active central admin")
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, org); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `SELECT set_config('app.actor_id',$1,true)`, fmt.Sprint(admin)); err != nil {
		return nil, err
	}
	var previous []byte
	err = tx.QueryRow(ctx, `SELECT summary FROM demo_seed_runs WHERE org_id=$1 AND seed_key=$2`, org, demoSeedKey).Scan(&previous)
	if err == nil {
		var result map[string]any
		if err = json.Unmarshal(previous, &result); err != nil {
			return nil, err
		}
		result["already_seeded"] = true
		return result, nil
	}
	if err != pgx.ErrNoRows {
		return nil, err
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM branches WHERE org_id=$1 AND code='DEMO')`, org).Scan(&exists); err != nil {
		return nil, err
	}
	if exists {
		return nil, fail(409, "Cabang DEMO sudah ada tanpa manifest; data lama tidak ditimpa")
	}
	// No changes outside this new branch, its scoped categories and fictional users.
	insertID := func(query string, args ...any) (int64, error) {
		var n int64
		e := tx.QueryRow(ctx, query, args...).Scan(&n)
		return n, e
	}
	branch, err := insertID(`INSERT INTO branches(org_id,code,name,address) VALUES($1,'DEMO','DEMO - Simulasi AssetFlow','Data fiktif, bukan data operasional') RETURNING id`, org)
	if err != nil {
		return nil, err
	}
	rooms := []int64{}
	for _, name := range []string{"DEMO - Gudang", "DEMO - Operasional", "DEMO - Arsip"} {
		n, e := insertID(`INSERT INTO locations(org_id,branch_id,name,building,floor,room) VALUES($1,$2,$3,'Gedung DEMO','1',$3) RETURNING id`, org, branch, name)
		if e != nil {
			return nil, e
		}
		rooms = append(rooms, n)
	}
	categoryIDs := []int64{}
	for _, method := range []string{"straight_line", "declining_balance", "non_depreciable"} {
		n, e := insertID(`INSERT INTO categories(org_id,branch_id,name,useful_life_months,maintenance_interval_days,maintenance_instructions,depreciation_method) VALUES($1,$2,$3,48,90,'DEMO: periksa kabel, kondisi fisik dan fungsi',$4) RETURNING id`, org, branch, "DEMO - "+method, method)
		if e != nil {
			return nil, e
		}
		categoryIDs = append(categoryIDs, n)
	}
	people := map[string]int64{}
	for _, role := range []string{"branch_admin", "manager", "operator", "staff", "employee", "finance", "it_support", "it_developer", "auditor"} {
		n, e := insertID(`INSERT INTO users(org_id,email,name,password_hash,role,active,all_branches) VALUES($1,$2,$3,'!DEMO-NO-LOGIN',$4,$4='staff',false) RETURNING id`, org, "assetflow-demo+"+role+"@example.test", "DEMO - "+role, role)
		if e != nil {
			return nil, e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO user_branches(org_id,user_id,branch_id) VALUES($1,$2,$3)`, org, n, branch); e != nil {
			return nil, e
		}
		people[role] = n
	}
	names := []string{"Katalog laptop", "Kendaraan penanggung jawab", "Servis terjadwal", "Perbaikan berjalan", "Servis selesai", "Servis dibatalkan", "Pinjaman menunggu", "Pinjaman jatuh tempo", "Pinjaman kembali baik", "Pinjaman kembali rusak", "Pinjaman ditolak", "Pinjaman dibatalkan", "Mutasi menunggu", "Mutasi disetujui", "Mutasi ditolak", "Afkir", "Penjualan", "Ditinggalkan", "Opname hilang", "Opname rusak", "Opname pindah", "Arsip aset", "Revaluasi menunggu", "Revaluasi disetujui", "Revaluasi ditolak", "Pelepasan menunggu"}
	assets := []int64{}
	var dateText string
	if err = tx.QueryRow(ctx, `SELECT CURRENT_DATE::text`).Scan(&dateText); err != nil {
		return nil, err
	}
	today, err := time.Parse("2006-01-02", dateText)
	if err != nil {
		return nil, err
	}
	purchaseDate := today.AddDate(0, 0, -365)
	for i, name := range names {
		status := "available"
		custodian := ""
		version := 1
		location := rooms[0]
		if i == 1 || i == 7 {
			status = "assigned"
			custodian = "DEMO - Staff"
			version = 2
		}
		if i == 3 {
			status = "maintenance"
			version = 2
		}
		if i == 9 {
			status = "maintenance"
			version = 3
		}
		if i == 4 || i == 8 {
			version = 3
		}
		if i == 5 || i == 23 {
			version = 2
		}
		if i == 7 {
			custodian = fmt.Sprintf("EMP-%d", people["staff"])
		}
		if i == 13 {
			location = rooms[1]
			version = 2
		}
		if i >= 15 && i <= 17 {
			status = "disposed"
			version = 2
		}
		n, e := insertID(`INSERT INTO assets(org_id,tag,name,serial_number,category_id,location_id,custodian,status,purchase_date,depRECIATION_start_date,purchase_cost,salvage_value,useful_life_months,warranty_until,depreciation_method,brand,model,specifications,supplier_name,acquisition_reference,rfid_tag,next_maintenance_date,version,deleted_at,disposed_at,disposal_proceeds,disposal_book_value,disposal_gross_value,disposal_accumulated_depreciation) VALUES($1,$2,$3,$4,$5,$6,$7,$8,CURRENT_DATE-365,CURRENT_DATE-365,12000000,1200000,48,CURRENT_DATE+365,$9,'DEMO','Model simulasi','DEMO: data spesifikasi fiktif','Vendor DEMO','DEMO-PO-001',$10,CASE WHEN $8='disposed' THEN NULL ELSE CURRENT_DATE-2 END,$11,CASE WHEN $12 THEN now() ELSE NULL END,CASE WHEN $8='disposed' THEN now()-interval '2 days' ELSE NULL END,$13,CASE WHEN $8='disposed' THEN 9300000 ELSE NULL END,CASE WHEN $8='disposed' THEN 12000000 ELSE NULL END,CASE WHEN $8='disposed' THEN 2700000 ELSE NULL END) RETURNING id`, org, fmt.Sprintf("DEMO-AST-%03d", i+1), "DEMO - "+name, fmt.Sprintf("DEMO-SN-%03d", i+1), categoryIDs[i%3], location, custodian, status, []string{"straight_line", "declining_balance", "non_depreciable"}[i%3], fmt.Sprintf("DEMO-RFID-%03d", i+1), version, i == 21, func() int {
			if i == 16 {
				return 3000000
			}
			return 0
		}())
		if e != nil {
			return nil, e
		}
		assets = append(assets, n)
		if status == "disposed" {
			method := []string{"straight_line", "declining_balance", "non_depreciable"}[i%3]
			book := depreciationBookValue(method, 12000000, 1200000, 48, purchaseDate, today.AddDate(0, 0, -2))
			if _, e = tx.Exec(ctx, `UPDATE assets SET disposal_book_value=$1::bigint,disposal_accumulated_depreciation=12000000-$1::bigint WHERE org_id=$2 AND id=$3`, book, org, n); e != nil {
				return nil, e
			}
		}
		if _, e = tx.Exec(ctx, `INSERT INTO asset_movements(org_id,asset_id,actor_id,event,to_location_id,to_branch_id,note) VALUES($1,$2,$3,'registered',$4,$5,'DEMO - Registrasi simulasi')`, org, n, admin, rooms[0], branch); e != nil {
			return nil, e
		}
	}
	// Small deterministic raster illustrations are explicitly not operational photographs.
	illustration := func(damage bool) []byte {
		img := image.NewRGBA(image.Rect(0, 0, 400, 260))
		draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{230, 235, 238, 255}}, image.Point{}, draw.Src)
		draw.Draw(img, image.Rect(80, 40, 320, 190), &image.Uniform{color.RGBA{42, 47, 51, 255}}, image.Point{}, draw.Src)
		draw.Draw(img, image.Rect(90, 50, 310, 177), &image.Uniform{color.RGBA{28, 130, 138, 255}}, image.Point{}, draw.Src)
		draw.Draw(img, image.Rect(55, 190, 345, 215), &image.Uniform{color.RGBA{120, 130, 140, 255}}, image.Point{}, draw.Src)
		if damage {
			draw.Draw(img, image.Rect(225, 80, 260, 160), &image.Uniform{color.RGBA{200, 55, 65, 255}}, image.Point{}, draw.Src)
		}
		var b bytes.Buffer
		_ = jpeg.Encode(&b, img, &jpeg.Options{Quality: 80})
		return b.Bytes()
	}
	proof := func(asset int64, purpose, entity string, entityID int64) (int64, error) {
		n, e := insertID(`INSERT INTO asset_photos(org_id,asset_id,purpose,caption,content,width,height,uploaded_by) VALUES($1,$2,$3,$4,$5,400,260,$6) RETURNING id`, org, asset, purpose, "DEMO - ilustrasi "+purpose+", bukan foto operasional", illustration(purpose == "damage" || purpose == "disposal" || purpose == "audit"), admin)
		if e != nil {
			return 0, e
		}
		if entity != "" {
			_, e = tx.Exec(ctx, `INSERT INTO asset_photo_links(org_id,photo_id,entity,entity_id) VALUES($1,$2,$3,$4)`, org, n, entity, entityID)
		}
		return n, e
	}
	for i, a := range assets {
		if _, e := proof(a, "catalog", "", 0); e != nil {
			return nil, e
		}
		if i == 0 {
			if _, e := proof(a, "catalog", "", 0); e != nil {
				return nil, e
			}
		}
	}
	for i, status := range []string{"scheduled", "in_progress", "completed", "cancelled"} {
		a := assets[2+i]
		kind := "preventive"
		if i == 1 || i == 2 {
			kind = "repair"
		}
		n, e := insertID(`INSERT INTO maintenance(org_id,asset_id,branch_id,title,due_date,status,cost,notes,completed_at,kind,requested_by,assigned_to,checklist,parts) VALUES($1,$2,$3,$4,CURRENT_DATE-1,$5,$6,'DEMO - Catatan servis',CASE WHEN $5='completed' THEN now()-interval '1 day' ELSE NULL END,$7,$8,$9,$10::jsonb,$11::jsonb) RETURNING id`, org, a, branch, "DEMO - "+status, status, func() int {
			if i == 2 {
				return 350000
			}
			return 0
		}(), kind, people["staff"], people["it_support"], fmt.Sprintf(`[{"label":"Periksa fungsi","done":%t}]`, i == 2), `[{"name":"Kabel DEMO","quantity":1,"cost":50000}]`)
		if e != nil {
			return nil, e
		}
		if kind == "repair" {
			if _, e = proof(a, "damage", "maintenance_before", n); e != nil {
				return nil, e
			}
		}
		if i == 2 {
			if _, e = proof(a, "repair", "maintenance_after", n); e != nil {
				return nil, e
			}
			if _, e = tx.Exec(ctx, `UPDATE assets SET next_maintenance_date=CURRENT_DATE+89 WHERE org_id=$1 AND id=$2`, org, a); e != nil {
				return nil, e
			}
		}
	}
	for i, status := range []string{"pending", "checked_out", "returned", "returned", "rejected", "cancelled"} {
		a := assets[6+i]
		n, e := insertID(`INSERT INTO asset_loans(org_id,asset_id,branch_id,borrower_id,due_at,reason,status,expected_version,checked_out_at,returned_at,handed_over_by,received_by,condition_before,condition_after,return_condition,decision_note) VALUES($1,$2,$3,$4,now()+CASE WHEN $5='checked_out' THEN interval '-1 day' ELSE interval '2 days' END,'DEMO - Pinjam inspeksi',$5,1,CASE WHEN $6 THEN now()-interval '3 days' ELSE NULL END,CASE WHEN $5='returned' THEN now()-interval '1 day' ELSE NULL END,CASE WHEN $6 THEN $7::bigint ELSE NULL END,CASE WHEN $5='returned' THEN $7::bigint ELSE NULL END,CASE WHEN $6 THEN 'DEMO - Kondisi awal baik' ELSE '' END,CASE WHEN $5='returned' THEN 'DEMO - Hasil pemeriksaan' ELSE '' END,CASE WHEN $5='returned' THEN $8::text ELSE NULL END,'DEMO - Simulasi') RETURNING id`, org, a, branch, people["staff"], status, i >= 1 && i <= 3, people["operator"], func() string {
			if i == 3 {
				return "damaged"
			}
			return "good"
		}())
		if e != nil {
			return nil, e
		}
		if i >= 1 && i <= 3 {
			if _, e = proof(a, "checkout", "loan_before", n); e != nil {
				return nil, e
			}
		}
		if status == "returned" {
			if _, e = proof(a, "return", "loan_after", n); e != nil {
				return nil, e
			}
		}
	}
	for _, sample := range []struct {
		Index                int
		Kind, Status, Method string
	}{{12, "transfer", "pending", "write_off"}, {13, "transfer", "approved", "write_off"}, {14, "transfer", "rejected", "write_off"}, {15, "dispose", "approved", "write_off"}, {16, "dispose", "approved", "sale"}, {17, "dispose", "approved", "abandonment"}, {25, "dispose", "pending", "write_off"}} {
		var target, branchTarget *int64
		if sample.Kind == "transfer" {
			target = &rooms[1]
			branchTarget = &branch
		}
		n, e := insertID(`INSERT INTO requests(org_id,asset_id,kind,target_location_id,reason,expected_version,status,requested_by,decided_by,decision_note,decided_at,source_branch_id,target_branch_id,disposal_method,disposal_proceeds) VALUES($1,$2,$3,$4,'DEMO - Simulasi persetujuan',1,$5,$6,CASE WHEN $5<>'pending' THEN $7::bigint ELSE NULL END,'DEMO - Persetujuan terpisah',CASE WHEN $5<>'pending' THEN now()-interval '2 days' ELSE NULL END,$8,$9,$10,$11) RETURNING id`, org, assets[sample.Index], sample.Kind, target, sample.Status, people["operator"], people["manager"], branch, branchTarget, sample.Method, func() int {
			if sample.Method == "sale" {
				return 3000000
			}
			return 0
		}())
		if e != nil {
			return nil, e
		}
		if sample.Kind == "dispose" {
			if _, e = proof(assets[sample.Index], "disposal", "request", n); e != nil {
				return nil, e
			}
		}
		if sample.Status == "approved" {
			event := "disposed"
			var to *int64
			if sample.Kind == "transfer" {
				event = "transferred"
				to = target
			}
			if _, e = tx.Exec(ctx, `INSERT INTO asset_movements(org_id,asset_id,actor_id,event,from_location_id,to_location_id,from_branch_id,to_branch_id,note) VALUES($1,$2,$3,$4,$5,$6,$7,$7,'DEMO - Riwayat keputusan')`, org, assets[sample.Index], people["manager"], event, rooms[0], to, branch); e != nil {
				return nil, e
			}
		}
	}
	for _, status := range []string{"closed", "open"} {
		n, e := insertID(`INSERT INTO stocktakes(org_id,location_id,title,status,created_by,closed_at) VALUES($1,$2,$3,$4,$5,CASE WHEN $4='closed' THEN now() ELSE NULL END) RETURNING id`, org, rooms[0], "DEMO - Opname "+status, status, admin)
		if e != nil {
			return nil, e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO stocktake_items(org_id,stocktake_id,asset_id,expected_version,expected_tag,observed,notes,observed_by,observed_at,finding,found_location_id) SELECT $1,$2,a.id,a.version,a.tag,($3::boolean AND a.id<>$5),'DEMO - Temuan lapangan',CASE WHEN $3 THEN $4::bigint ELSE NULL END,CASE WHEN $3 THEN now() ELSE NULL END,CASE WHEN NOT $3 THEN 'unverified' WHEN a.id=$5 THEN 'missing' WHEN a.id=$6 THEN 'damaged' WHEN a.id=$7 THEN 'relocated' ELSE 'present' END,CASE WHEN $3 AND a.id=$7 THEN $8::bigint ELSE NULL END FROM assets a WHERE a.org_id=$1 AND a.location_id=$9 AND a.status<>'disposed' AND a.deleted_at IS NULL`, org, n, status == "closed", admin, assets[18], assets[19], assets[20], rooms[1], rooms[0]); e != nil {
			return nil, e
		}
		if status == "closed" {
			for _, index := range []int{18, 19, 20} {
				if _, e = proof(assets[index], "audit", "stocktake_item", n); e != nil {
					return nil, e
				}
			}
		}
	}
	for i, status := range []string{"pending", "approved", "rejected"} {
		method := []string{"straight_line", "declining_balance", "non_depreciable"}[(22+i)%3]
		book := depreciationBookValue(method, 12000000, 1200000, 48, purchaseDate, today.AddDate(0, 0, -1))
		_, e := insertID(`INSERT INTO asset_valuations(org_id,asset_id,branch_id,requested_by,decided_by,effective_date,carrying_value_before,revalued_amount,remaining_life_months,reason,status,decision_note,expected_version,decided_at) VALUES($1,$2,$3,$4,CASE WHEN $5<>'pending' THEN $6::bigint ELSE NULL END,CURRENT_DATE-1,9300000,9000000,36,'DEMO - Evaluasi nilai',$5,'DEMO - Keputusan',1,CASE WHEN $5<>'pending' THEN now() ELSE NULL END) RETURNING id`, org, assets[22+i], branch, people["finance"], status, admin)
		if e != nil {
			return nil, e
		}
		if _, e = tx.Exec(ctx, `UPDATE asset_valuations SET carrying_value_before=$1 WHERE org_id=$2 AND asset_id=$3`, book, org, assets[22+i]); e != nil {
			return nil, e
		}
	}
	for i, days := range []int{15, -10, 365} {
		if _, err = tx.Exec(ctx, `INSERT INTO service_contracts(org_id,branch_id,asset_id,name,vendor,contract_number,start_date,end_date,annual_cost,notes,created_by,deleted_at) VALUES($1,$2,$3,$4,'Vendor DEMO',$5,CURRENT_DATE-400,CURRENT_DATE+$6::integer,1200000,'DEMO - Kontrak simulasi',$7,CASE WHEN $8 THEN now() ELSE NULL END)`, org, branch, assets[i], "DEMO - Kontrak "+[]string{"akan berakhir", "kedaluwarsa", "arsip"}[i], fmt.Sprintf("DEMO-CTR-%03d", i+1), days, admin, i == 2); err != nil {
			return nil, err
		}
	}
	// General Setup examples are scoped and never rewrite the real HQ identifier.
	master, err := insertID(`INSERT INTO general_codes(org_id,code,name,company_prefix,description) VALUES($1,'DEMO_COMPANY','DEMO - Kode perusahaan','DEMO','Konfigurasi fiktif cabang/pusat') RETURNING id`, org)
	if err != nil {
		return nil, err
	}
	for _, d := range []struct{ Code, Name, Prefix, Kind string }{{"HEAD_OFFICE", "DEMO - Pusat", "PST", "hq"}, {"BRANCH", "DEMO - Cabang", "CBG", "branch"}, {"COST_CENTER", "DEMO - Pusat biaya", "CC", "reference"}} {
		detailID, e := insertID(`INSERT INTO general_code_details(org_id,general_code_id,code,name,prefix,entity_type) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, org, master, d.Code, d.Name, d.Prefix, d.Kind)
		if e != nil {
			return nil, e
		}
		if d.Kind == "branch" {
			if _, e = tx.Exec(ctx, `INSERT INTO issued_general_codes(org_id,detail_id,code,entity_type,entity_id) VALUES($1,$2,'DEMO-CBG-000001','branch',$3)`, org, detailID, branch); e != nil {
				return nil, e
			}
			if _, e = tx.Exec(ctx, `UPDATE general_code_details SET next_number=2,version=version+1 WHERE org_id=$1 AND id=$2`, org, detailID); e != nil {
				return nil, e
			}
			if _, e = tx.Exec(ctx, `UPDATE branches SET business_code='DEMO-CBG-000001' WHERE org_id=$1 AND id=$2`, org, branch); e != nil {
				return nil, e
			}
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE locations SET deleted_at=now() WHERE org_id=$1 AND id=$2`, org, rooms[2]); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `UPDATE users SET deleted_at=now() WHERE org_id=$1 AND id=$2`, org, people["it_developer"]); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO categories(org_id,branch_id,name,useful_life_months,deleted_at) VALUES($1,$2,'DEMO - Kategori arsip',48,now())`, org, branch); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO general_code_details(org_id,general_code_id,code,name,prefix,entity_type,deleted_at) VALUES($1,$2,'LEGACY','DEMO - Detail arsip','OLD','reference',now())`, org, master); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO general_codes(org_id,code,name,company_prefix,description,deleted_at) VALUES($1,'DEMO_ARCHIVE','DEMO - Kelompok arsip','OLDDEMO','Data fiktif',now())`, org); err != nil {
		return nil, err
	}
	summary := map[string]any{"seed_key": demoSeedKey, "branch_id": branch, "branch_code": "DEMO", "assets": len(assets), "users": len(people), "maintenance": 4, "loans": 6, "requests": 7, "stocktakes": 2, "valuations": 3, "contracts": 3, "general_codes": 2, "general_code_details": 4, "locations": 3, "categories": 4, "photos": 42, "issued_codes": 1}
	encoded, err := json.Marshal(summary)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO demo_seed_runs(org_id,seed_key,summary) VALUES($1,$2,$3::jsonb)`, org, demoSeedKey, encoded); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(org_id,actor_id,action,entity,entity_id,branch_id,after_data) VALUES($1,$2,'seed_demo','branch',$3,$3,$4::jsonb)`, org, admin, branch, encoded); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return summary, nil
}
