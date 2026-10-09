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
	Cost            int64   `json:"purchase_cost"`
	Salvage         int64   `json:"salvage_value"`
	Life            int     `json:"useful_life_months"`
	Warranty        *string `json:"warranty_until"`
	Version         int     `json:"version"`
	CategoryName    string  `json:"category_name"`
	LocationName    string  `json:"location_name"`
	BookValue       int64   `json:"book_value"`
	BranchID        int64   `json:"branch_id"`
	BranchName      string  `json:"branch_name"`
	NextMaintenance *string `json:"next_maintenance_date"`
}

func lockAsset(c context.Context, tx pgx.Tx, org, id int64) (Asset, error) {
	var a Asset
	e := tx.QueryRow(c, `SELECT a.id,a.tag,a.name,a.serial_number,a.category_id,a.location_id,a.custodian,a.status,a.purchase_date::text,a.purchase_cost,a.salvage_value,a.useful_life_months,a.warranty_until::text,a.version,l.branch_id,a.next_maintenance_date::text FROM assets a JOIN locations l ON l.id=a.location_id WHERE a.org_id=$1 AND a.id=$2 AND can_access_branch(a.org_id,$3,l.branch_id) FOR UPDATE OF a`, org, id, c.Value(ctxKey{}).(User).ID).Scan(&a.ID, &a.Tag, &a.Name, &a.Serial, &a.Category, &a.Location, &a.Custodian, &a.Status, &a.PurchaseDate, &a.Cost, &a.Salvage, &a.Life, &a.Warranty, &a.Version, &a.BranchID, &a.NextMaintenance)
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
	// Window count keeps page and count in the same PostgreSQL snapshot.
	rows, e := s.DB.Query(r.Context(), `SELECT a.id,a.tag,a.name,a.serial_number,a.category_id,a.location_id,a.custodian,a.status,a.purchase_date::text,a.purchase_cost,a.salvage_value,a.useful_life_months,a.warranty_until::text,a.version,c.name,l.name,l.branch_id,b.name,a.next_maintenance_date::text,count(*) OVER() FROM assets a JOIN categories c ON c.id=a.category_id AND c.org_id=a.org_id JOIN locations l ON l.id=a.location_id AND l.org_id=a.org_id JOIN branches b ON b.id=l.branch_id WHERE a.org_id=$1 AND can_access_branch(a.org_id,$6,l.branch_id) AND ($7::bigint=0 OR l.branch_id=$7) AND ($2='' OR a.name ILIKE '%'||$2||'%' OR a.tag ILIKE '%'||$2||'%' OR a.serial_number ILIKE '%'||$2||'%') AND ($3='' OR a.status=$3) ORDER BY a.id DESC LIMIT $4 OFFSET $5`, u.OrgID, search, status, size, (p-1)*size, u.ID, selectedBranch(r))
	if e != nil {
		return e
	}
	defer rows.Close()
	out := []Asset{}
	var total int64
	for rows.Next() {
		var a Asset
		e = rows.Scan(&a.ID, &a.Tag, &a.Name, &a.Serial, &a.Category, &a.Location, &a.Custodian, &a.Status, &a.PurchaseDate, &a.Cost, &a.Salvage, &a.Life, &a.Warranty, &a.Version, &a.CategoryName, &a.LocationName, &a.BranchID, &a.BranchName, &a.NextMaintenance, &total)
		if e != nil {
			return e
		}
		d, _ := time.Parse("2006-01-02", a.PurchaseDate)
		a.BookValue = bookValue(a.Cost, a.Salvage, a.Life, d, time.Now().In(time.FixedZone("WIB", 7*3600)))
		out = append(out, a)
	}
	if e = rows.Err(); e != nil {
		return e
	}
	if len(out) == 0 {
		e = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM assets a JOIN locations l ON l.id=a.location_id WHERE a.org_id=$1 AND can_access_branch(a.org_id,$4,l.branch_id) AND ($5::bigint=0 OR l.branch_id=$5) AND ($2='' OR a.name ILIKE '%'||$2||'%' OR tag ILIKE '%'||$2||'%' OR serial_number ILIKE '%'||$2||'%') AND ($3='' OR status=$3)`, u.OrgID, search, status, u.ID, selectedBranch(r)).Scan(&total)
		if e != nil {
			return e
		}
	}
	write(w, 200, map[string]any{"items": out, "total": total, "page": p, "size": size})
	return nil
}
func (s *Server) createAsset(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Tag      string  `json:"tag"`
		Name     string  `json:"name"`
		Serial   string  `json:"serial_number"`
		Category int64   `json:"category_id"`
		Location int64   `json:"location_id"`
		Date     string  `json:"purchase_date"`
		Cost     int64   `json:"purchase_cost"`
		Salvage  int64   `json:"salvage_value"`
		Life     int     `json:"useful_life_months"`
		Warranty *string `json:"warranty_until"`
	}
	if e := decode(w, r, &in); e != nil {
		return e
	}
	in.Tag = strings.ToUpper(strings.TrimSpace(in.Tag))
	in.Name = strings.TrimSpace(in.Name)
	in.Serial = strings.TrimSpace(in.Serial)
	d, e := time.Parse("2006-01-02", in.Date)
	if e != nil || d.After(time.Now().Add(24*time.Hour)) || len(in.Tag) < 1 || len(in.Tag) > 80 || len(in.Name) < 2 || len(in.Name) > 200 || len(in.Serial) > 200 {
		return fail(422, "Tag, nama, serial, atau tanggal tidak valid")
	}
	if e = validateMoney(in.Cost, in.Salvage, in.Life); e != nil {
		return fail(422, e.Error())
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
		e := tx.QueryRow(r.Context(), `INSERT INTO assets(org_id,tag,name,serial_number,category_id,location_id,purchase_date,purchase_cost,salvage_value,useful_life_months,warranty_until,next_maintenance_date) VALUES($1,$2,$3,$4,$5,$6,$7::text::date,$8,$9,$10,$11::text::date,(SELECT CASE WHEN maintenance_interval_days>0 THEN $7::text::date+maintenance_interval_days ELSE NULL END FROM categories WHERE org_id=$1 AND id=$5)) RETURNING id`, actor(r).OrgID, in.Tag, in.Name, in.Serial, in.Category, in.Location, in.Date, in.Cost, in.Salvage, in.Life, in.Warranty).Scan(&n)
		if e != nil {
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
		return logAudit(r.Context(), tx, actor(r), in.Action, "asset", n, a, map[string]any{"status": status, "custodian": custodian, "version": a.Version + 1})
	})
	if e != nil {
		return e
	}
	write(w, 200, map[string]bool{"ok": true})
	return nil
}
