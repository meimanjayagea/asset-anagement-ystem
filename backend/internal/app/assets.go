package app

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
)

type Asset struct {
	ID              int64   `json:"id"`
	Tag             string  `json:"tag"`
	Name            string  `json:"name"`
	Serial          string  `json:"serial_number"`
	Category        int64   `json:"category_id"`
	Location        int64   `json:"location_id"`
	Custodian       string  `json:"custodian"`
	Status          string  `json:"status"`
	PurchaseDate    string  `json:"purchase_date"`
	Supplier        string  `json:"supplier_name"`
	AcquisitionRef  string  `json:"acquisition_reference"`
	Cost            int64   `json:"purchase_cost,omitempty"`
	Salvage         int64   `json:"salvage_value,omitempty"`
	Life            int     `json:"useful_life_months"`
	Depreciation    string  `json:"depreciation_method"`
	DepStart        string  `json:"depreciation_start_date"`
	Warranty        *string `json:"warranty_until"`
	Version         int     `json:"version"`
	CategoryName    string  `json:"category_name"`
	LocationName    string  `json:"location_name"`
	BookValue       int64   `json:"book_value,omitempty"`
	BranchID        int64   `json:"branch_id"`
	BranchName      string  `json:"branch_name"`
	NextMaintenance *string `json:"next_maintenance_date"`
	DeletedAt       *string `json:"deleted_at,omitempty"`
}

func lockAsset(c context.Context, tx pgx.Tx, org, id int64) (Asset, error) {
	var a Asset
	e := tx.QueryRow(c, `SELECT a.id,a.tag,a.name,a.serial_number,a.category_id,a.location_id,a.custodian,a.status,a.purchase_date::text,a.purchase_cost,a.salvage_value,a.useful_life_months,a.warranty_until::text,a.version,l.branch_id,a.next_maintenance_date::text,a.supplier_name,a.acquisition_reference,a.depreciation_method,a.depreciation_start_date::text FROM assets a JOIN locations l ON l.id=a.location_id JOIN branches b ON b.id=l.branch_id JOIN categories c ON c.id=a.category_id WHERE a.org_id=$1 AND a.id=$2 AND a.deleted_at IS NULL AND l.deleted_at IS NULL AND b.deleted_at IS NULL AND c.deleted_at IS NULL AND can_access_branch(a.org_id,$3,l.branch_id) FOR UPDATE OF a`, org, id, c.Value(ctxKey{}).(User).ID).Scan(&a.ID, &a.Tag, &a.Name, &a.Serial, &a.Category, &a.Location, &a.Custodian, &a.Status, &a.PurchaseDate, &a.Cost, &a.Salvage, &a.Life, &a.Warranty, &a.Version, &a.BranchID, &a.NextMaintenance, &a.Supplier, &a.AcquisitionRef, &a.Depreciation, &a.DepStart)
	if errors.Is(e, pgx.ErrNoRows) {
		return a, fail(404, "Aset tidak ditemukan")
	}
	return a, e
}
func (s *Server) listAssets(w http.ResponseWriter, r *http.Request) error {
	p, size := page(r)
	search := r.URL.Query().Get("search")
	status := r.URL.Query().Get("status")
	if len(search) > 200 {
		return fail(400, "Pencarian terlalu panjang")
	}
	u := actor(r)
	archived := r.URL.Query().Get("archived") == "true" && hasCapability(u.Role, "assets.archive")
	// Window count keeps page and count in the same PostgreSQL snapshot.
	rows, e := s.DB.Query(r.Context(), `SELECT a.id,a.tag,a.name,a.serial_number,a.category_id,a.location_id,a.custodian,a.status,a.purchase_date::text,a.purchase_cost,a.salvage_value,a.useful_life_months,a.warranty_until::text,a.version,c.name,l.name,l.branch_id,b.name,a.next_maintenance_date::text,a.deleted_at::text,count(*) OVER(),a.supplier_name,a.acquisition_reference,a.depreciation_method,a.depreciation_start_date::text,COALESCE((SELECT v.revalued_amount FROM asset_valuations v WHERE v.org_id=a.org_id AND v.asset_id=a.id AND v.status='approved' AND v.effective_date<=$9::date ORDER BY v.effective_date DESC,v.id DESC LIMIT 1),a.purchase_cost),COALESCE((SELECT v.remaining_life_months FROM asset_valuations v WHERE v.org_id=a.org_id AND v.asset_id=a.id AND v.status='approved' AND v.effective_date<=$9::date ORDER BY v.effective_date DESC,v.id DESC LIMIT 1),a.useful_life_months),COALESCE((SELECT v.effective_date::text FROM asset_valuations v WHERE v.org_id=a.org_id AND v.asset_id=a.id AND v.status='approved' AND v.effective_date<=$9::date ORDER BY v.effective_date DESC,v.id DESC LIMIT 1),a.depreciation_start_date::text) FROM assets a JOIN categories c ON c.id=a.category_id AND c.org_id=a.org_id AND c.deleted_at IS NULL JOIN locations l ON l.id=a.location_id AND l.org_id=a.org_id AND l.deleted_at IS NULL JOIN branches b ON b.id=l.branch_id AND b.deleted_at IS NULL WHERE a.org_id=$1 AND (($8::boolean AND a.deleted_at IS NOT NULL) OR (NOT $8::boolean AND a.deleted_at IS NULL)) AND can_access_branch(a.org_id,$6,l.branch_id) AND ($7::bigint=0 OR l.branch_id=$7) AND ($2='' OR a.name ILIKE '%'||$2||'%' OR a.tag ILIKE '%'||$2||'%' OR a.serial_number ILIKE '%'||$2||'%') AND ($3='' OR a.status=$3) ORDER BY a.id DESC LIMIT $4 OFFSET $5`, u.OrgID, search, status, size, (p-1)*size, u.ID, selectedBranch(r), archived, time.Now().In(financeZone).Format("2006-01-02"))
	if e != nil {
		return e
	}
	defer rows.Close()
	out := []Asset{}
	var total int64
	for rows.Next() {
		var a Asset
		var basisCost int64
		var basisLife int
		var basisDate string
		e = rows.Scan(&a.ID, &a.Tag, &a.Name, &a.Serial, &a.Category, &a.Location, &a.Custodian, &a.Status, &a.PurchaseDate, &a.Cost, &a.Salvage, &a.Life, &a.Warranty, &a.Version, &a.CategoryName, &a.LocationName, &a.BranchID, &a.BranchName, &a.NextMaintenance, &a.DeletedAt, &total, &a.Supplier, &a.AcquisitionRef, &a.Depreciation, &a.DepStart, &basisCost, &basisLife, &basisDate)
		if e != nil {
			return e
		}
		start, _ := time.Parse("2006-01-02", basisDate)
		a.BookValue = depreciationBookValue(a.Depreciation, basisCost, a.Salvage, basisLife, start, time.Now().In(time.FixedZone("WIB", 7*3600)))
		if !hasCapability(u.Role, "assets.finance") {
			a.Cost, a.Salvage, a.BookValue = 0, 0, 0
		}
		out = append(out, a)
	}
	if e = rows.Err(); e != nil {
		return e
	}
	if len(out) == 0 {
		e = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM assets a JOIN locations l ON l.id=a.location_id AND l.deleted_at IS NULL JOIN branches b ON b.id=l.branch_id AND b.deleted_at IS NULL JOIN categories c ON c.id=a.category_id AND c.deleted_at IS NULL WHERE a.org_id=$1 AND (($6::boolean AND a.deleted_at IS NOT NULL) OR (NOT $6::boolean AND a.deleted_at IS NULL)) AND can_access_branch(a.org_id,$4,l.branch_id) AND ($5::bigint=0 OR l.branch_id=$5) AND ($2='' OR a.name ILIKE '%'||$2||'%' OR tag ILIKE '%'||$2||'%' OR serial_number ILIKE '%'||$2||'%') AND ($3='' OR status=$3)`, u.OrgID, search, status, u.ID, selectedBranch(r), archived).Scan(&total)
		if e != nil {
			return e
		}
	}
	write(w, 200, map[string]any{"items": out, "total": total, "page": p, "size": size})
	return nil
}
func (s *Server) createAsset(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Tag       string  `json:"tag"`
		Name      string  `json:"name"`
		Serial    string  `json:"serial_number"`
		Category  int64   `json:"category_id"`
		Location  int64   `json:"location_id"`
		Date      string  `json:"purchase_date"`
		Cost      int64   `json:"purchase_cost"`
		Salvage   int64   `json:"salvage_value"`
		Life      int     `json:"useful_life_months"`
		Method    string  `json:"depreciation_method"`
		DepStart  string  `json:"depreciation_start_date"`
		Supplier  string  `json:"supplier_name"`
		Reference string  `json:"acquisition_reference"`
		Warranty  *string `json:"warranty_until"`
	}
	if e := decode(w, r, &in); e != nil {
		return e
	}
	in.Tag = strings.ToUpper(strings.TrimSpace(in.Tag))
	in.Name = strings.TrimSpace(in.Name)
	in.Serial = strings.TrimSpace(in.Serial)
	in.Supplier = strings.TrimSpace(in.Supplier)
	in.Reference = strings.TrimSpace(in.Reference)
	useCategoryMethod := in.Method == ""
	d, e := time.Parse("2006-01-02", in.Date)
	if e != nil || d.After(time.Now().Add(24*time.Hour)) || len(in.Tag) < 1 || len(in.Tag) > 80 || len(in.Name) < 2 || len(in.Name) > 200 || len(in.Serial) > 200 {
		return fail(422, "Tag, nama, serial, atau tanggal tidak valid")
	}
	if in.DepStart == "" {
		in.DepStart = in.Date
	}
	if in.Method == "" {
		in.Method = "straight_line"
	}
	startDate, startErr := time.Parse("2006-01-02", in.DepStart)
	if startErr != nil || startDate.Before(d) || in.Method != "straight_line" && in.Method != "declining_balance" && in.Method != "non_depreciable" || len(in.Supplier) > 200 || len(in.Reference) > 100 {
		return fail(422, "Kebijakan depresiasi atau referensi akuisisi tidak valid")
	}
	if e = validateMoney(in.Cost, in.Salvage, in.Life); e != nil {
		return fail(422, e.Error())
	}
	if !hasCapability(actor(r).Role, "assets.finance") {
		in.Cost, in.Salvage, in.Method = 0, 0, "straight_line"
	}
	if in.Warranty != nil {
		wd, e := time.Parse("2006-01-02", *in.Warranty)
		if e != nil || wd.Before(d) {
			return fail(422, "Warranty harus >= tanggal pembelian")
		}
	}
	var n int64
	e = s.transaction(r, func(tx pgx.Tx) error {
		if e := locationScope(r, tx, in.Location); e != nil {
			return e
		}
		var categoryMethod string
		if e := tx.QueryRow(r.Context(), `SELECT c.depreciation_method FROM categories c JOIN locations l ON l.org_id=c.org_id WHERE c.org_id=$1 AND c.id=$2 AND c.deleted_at IS NULL AND l.id=$3 AND l.deleted_at IS NULL AND (c.branch_id IS NULL OR c.branch_id=l.branch_id)`, actor(r).OrgID, in.Category, in.Location).Scan(&categoryMethod); e != nil {
			if e == pgx.ErrNoRows {
				return fail(422, "Kategori tidak tersedia")
			}
			return e
		}
		if useCategoryMethod && hasCapability(actor(r).Role, "assets.finance") {
			in.Method = categoryMethod
		}
		e := tx.QueryRow(r.Context(), `INSERT INTO assets(org_id,tag,name,serial_number,category_id,location_id,purchase_date,purchase_cost,salvage_value,useful_life_months,warranty_until,next_maintenance_date,depreciation_method,depreciation_start_date,supplier_name,acquisition_reference) VALUES($1,$2,$3,$4,$5,$6,$7::text::date,$8,$9,$10,$11::text::date,(SELECT CASE WHEN maintenance_interval_days>0 THEN $7::text::date+maintenance_interval_days ELSE NULL END FROM categories WHERE org_id=$1 AND id=$5),$12,$13::text::date,$14,$15) RETURNING id`, actor(r).OrgID, in.Tag, in.Name, in.Serial, in.Category, in.Location, in.Date, in.Cost, in.Salvage, in.Life, in.Warranty, in.Method, in.DepStart, in.Supplier, in.Reference).Scan(&n)
		if e != nil {
			return e
		}
		if e = recordAssetMovement(r.Context(), tx, actor(r), n, "registered", nil, &in.Location, nil, nil, "", "", "Asset registered"); e != nil {
			return e
		}
		return logAudit(r.Context(), tx, actor(r), "create", "asset", n, nil, in)
	})
	if e != nil {
		return e
	}
	write(w, 201, map[string]int64{"id": n})
	return nil
}

func (s *Server) updateAsset(w http.ResponseWriter, r *http.Request) error {
	n, e := id(r)
	if e != nil {
		return e
	}
	var in struct {
		Tag       string  `json:"tag"`
		Name      string  `json:"name"`
		Serial    string  `json:"serial_number"`
		Category  int64   `json:"category_id"`
		Location  int64   `json:"location_id"`
		Date      string  `json:"purchase_date"`
		Supplier  string  `json:"supplier_name"`
		Reference string  `json:"acquisition_reference"`
		Cost      int64   `json:"purchase_cost"`
		Salvage   int64   `json:"salvage_value"`
		Life      int     `json:"useful_life_months"`
		Method    string  `json:"depreciation_method"`
		DepStart  string  `json:"depreciation_start_date"`
		Warranty  *string `json:"warranty_until"`
		Version   int     `json:"version"`
	}
	if e = decode(w, r, &in); e != nil {
		return e
	}
	in.Tag = strings.ToUpper(strings.TrimSpace(in.Tag))
	in.Name = strings.TrimSpace(in.Name)
	in.Serial = strings.TrimSpace(in.Serial)
	in.Supplier = strings.TrimSpace(in.Supplier)
	in.Reference = strings.TrimSpace(in.Reference)
	d, e := time.Parse("2006-01-02", in.Date)
	if e != nil || d.After(time.Now().Add(24*time.Hour)) || len(in.Tag) < 1 || len(in.Tag) > 80 || len(in.Name) < 2 || len(in.Name) > 200 || len(in.Serial) > 200 || in.Version < 1 {
		return fail(422, "Tag, nama, serial, tanggal, atau versi tidak valid")
	}
	if in.Warranty != nil {
		wd, e := time.Parse("2006-01-02", *in.Warranty)
		if e != nil || wd.Before(d) {
			return fail(422, "Warranty harus >= tanggal pembelian")
		}
	}
	u := actor(r)
	e = s.transaction(r, func(tx pgx.Tx) error {
		before, e := lockAsset(r.Context(), tx, u.OrgID, n)
		if e != nil {
			return e
		}
		if before.Version != in.Version {
			return fail(409, "Data berubah; refresh terlebih dahulu")
		}
		if !hasCapability(u.Role, "assets.finance") {
			in.Cost, in.Salvage, in.Life = before.Cost, before.Salvage, before.Life
			in.Method, in.DepStart = before.Depreciation, before.DepStart
			in.Supplier, in.Reference = before.Supplier, before.AcquisitionRef
		} else {
			if in.Method == "" {
				in.Method = before.Depreciation
			}
			if in.DepStart == "" {
				in.DepStart = before.DepStart
			}
		}
		startDate, startErr := time.Parse("2006-01-02", in.DepStart)
		if startErr != nil || startDate.Before(d) || in.Method != "straight_line" && in.Method != "declining_balance" && in.Method != "non_depreciable" || len(in.Supplier) > 200 || len(in.Reference) > 100 {
			return fail(422, "Kebijakan depresiasi atau referensi akuisisi tidak valid")
		}
		if e = validateMoney(in.Cost, in.Salvage, in.Life); e != nil {
			return fail(422, e.Error())
		}
		if e = locationScope(r, tx, in.Location); e != nil {
			return e
		}
		var targetBranch int64
		e = tx.QueryRow(r.Context(), `SELECT branch_id FROM locations WHERE org_id=$1 AND id=$2 AND deleted_at IS NULL`, u.OrgID, in.Location).Scan(&targetBranch)
		if e != nil {
			return e
		}
		if targetBranch != before.BranchID {
			return fail(409, "Perpindahan lintas cabang harus melalui request transfer")
		}
		var categoryAvailable bool
		if e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM categories c WHERE c.org_id=$1 AND c.id=$2 AND c.deleted_at IS NULL AND (c.branch_id IS NULL OR c.branch_id=$3))`, u.OrgID, in.Category, targetBranch).Scan(&categoryAvailable); e != nil {
			return e
		}
		if !categoryAvailable {
			return fail(422, "Kategori tidak tersedia untuk cabang ini")
		}
		structuralChange := in.Location != before.Location || in.Category != before.Category || in.Date != before.PurchaseDate || in.Method != before.Depreciation || in.DepStart != before.DepStart
		if structuralChange {
			var active bool
			e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM requests WHERE org_id=$1 AND asset_id=$2 AND status='pending') OR EXISTS(SELECT 1 FROM maintenance WHERE org_id=$1 AND asset_id=$2 AND status IN ('scheduled','in_progress')) OR EXISTS(SELECT 1 FROM stocktake_items i JOIN stocktakes st ON st.id=i.stocktake_id WHERE i.org_id=$1 AND i.asset_id=$2 AND st.status='open')`, u.OrgID, n).Scan(&active)
			if e != nil {
				return e
			}
			if active {
				return fail(409, "Selesaikan request, maintenance, atau stocktake aktif sebelum mengubah kategori, lokasi, atau tanggal")
			}
		}
		_, e = tx.Exec(r.Context(), `UPDATE assets SET tag=$1,name=$2,serial_number=$3,category_id=$4,location_id=$5,purchase_date=$6::text::date,purchase_cost=$7,salvage_value=$8,useful_life_months=$9,warranty_until=$10::text::date,supplier_name=$11,acquisition_reference=$12,depreciation_method=$13,depreciation_start_date=$14::text::date,next_maintenance_date=CASE WHEN category_id<>$4 OR purchase_date<>$6::text::date THEN (SELECT CASE WHEN maintenance_interval_days>0 THEN $6::text::date+maintenance_interval_days ELSE NULL END FROM categories WHERE org_id=$15 AND id=$4) ELSE next_maintenance_date END,version=version+1,updated_at=now() WHERE org_id=$15 AND id=$16 AND version=$17 AND deleted_at IS NULL`, in.Tag, in.Name, in.Serial, in.Category, in.Location, in.Date, in.Cost, in.Salvage, in.Life, in.Warranty, in.Supplier, in.Reference, in.Method, in.DepStart, u.OrgID, n, in.Version)
		if e != nil {
			return e
		}
		if in.Location != before.Location {
			from, to := before.Location, in.Location
			fromBranch, toBranch := before.BranchID, targetBranch
			if e = recordAssetMovement(r.Context(), tx, u, n, "relocated", &from, &to, &fromBranch, &toBranch, before.Custodian, before.Custodian, "Relocated within branch"); e != nil {
				return e
			}
		}
		after := map[string]any{"tag": in.Tag, "name": in.Name, "serial_number": in.Serial, "category_id": in.Category, "location_id": in.Location, "purchase_date": in.Date, "purchase_cost": in.Cost, "salvage_value": in.Salvage, "useful_life_months": in.Life, "depreciation_method": in.Method, "depreciation_start_date": in.DepStart, "supplier_name": in.Supplier, "acquisition_reference": in.Reference, "warranty_until": in.Warranty, "version": before.Version + 1}
		return logAudit(r.Context(), tx, u, "update", "asset", n, before, after)
	})
	if e != nil {
		return e
	}
	write(w, http.StatusOK, map[string]any{"ok": true, "version": in.Version + 1})
	return nil
}

func (s *Server) assetAction(w http.ResponseWriter, r *http.Request) error {
	n, e := id(r)
	if e != nil {
		return e
	}
	var in struct {
		Action    string `json:"action"`
		Custodian string `json:"custodian"`
		Version   int    `json:"version"`
	}
	if e = decode(w, r, &in); e != nil {
		return e
	}
	if in.Action != "assign" && in.Action != "return" {
		return fail(422, "Action tidak valid")
	}
	in.Custodian = strings.TrimSpace(in.Custodian)
	if in.Action == "assign" && (len(in.Custodian) < 2 || len(in.Custodian) > 200) {
		return fail(422, "Custodian wajib 2–200 karakter")
	}
	e = s.transaction(r, func(tx pgx.Tx) error {
		a, e := lockAsset(r.Context(), tx, actor(r).OrgID, n)
		if e != nil {
			return e
		}
		if a.Version != in.Version {
			return fail(409, "Data berubah; refresh terlebih dahulu")
		}
		if !allowed(a.Status, in.Action) {
			return fail(409, "Transisi status tidak diizinkan")
		}
		var pending bool
		e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM requests WHERE org_id=$1 AND asset_id=$2 AND status='pending') OR EXISTS(SELECT 1 FROM maintenance WHERE org_id=$1 AND asset_id=$2 AND status IN ('scheduled','in_progress'))`, actor(r).OrgID, n).Scan(&pending)
		if e != nil {
			return e
		}
		if pending {
			return fail(409, "Selesaikan request/maintenance aktif terlebih dahulu")
		}
		status, custodian := "available", ""
		if in.Action == "assign" {
			status = "assigned"
			custodian = in.Custodian
		}
		_, e = tx.Exec(r.Context(), `UPDATE assets SET status=$1,custodian=$2,version=version+1,updated_at=now() WHERE org_id=$3 AND id=$4`, status, custodian, actor(r).OrgID, n)
		if e != nil {
			return e
		}
		event := "assigned"
		if in.Action == "return" {
			event = "returned"
		}
		location, branch := a.Location, a.BranchID
		if e = recordAssetMovement(r.Context(), tx, actor(r), n, event, &location, &location, &branch, &branch, a.Custodian, custodian, "Custodian status changed"); e != nil {
			return e
		}
		return logAudit(r.Context(), tx, actor(r), in.Action, "asset", n, a, map[string]any{"status": status, "custodian": custodian, "version": a.Version + 1})
	})
	if e != nil {
		return e
	}
	write(w, 200, map[string]bool{"ok": true})
	return nil
}
