package app

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

func (s *Server) archiveAsset(w http.ResponseWriter, r *http.Request) error {
	n, e := id(r)
	if e != nil {
		return e
	}
	e = s.transaction(r, func(tx pgx.Tx) error {
		u := actor(r)
		a, e := lockAsset(r.Context(), tx, u.OrgID, n)
		if e != nil {
			return e
		}
		var active bool
		e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM requests WHERE org_id=$1 AND asset_id=$2 AND status='pending') OR EXISTS(SELECT 1 FROM maintenance WHERE org_id=$1 AND asset_id=$2 AND status IN ('scheduled','in_progress')) OR EXISTS(SELECT 1 FROM asset_loans WHERE org_id=$1 AND asset_id=$2 AND status IN ('pending','checked_out')) OR EXISTS(SELECT 1 FROM stocktake_items i JOIN stocktakes st ON st.id=i.stocktake_id WHERE i.org_id=$1 AND i.asset_id=$2 AND st.status='open')`, u.OrgID, n).Scan(&active)
		if e != nil {
			return e
		}
		if active {
			return fail(409, "Selesaikan request, maintenance, atau stocktake aktif sebelum mengarsipkan aset")
		}
		if _, e = tx.Exec(r.Context(), `UPDATE assets SET deleted_at=now(),version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2 AND deleted_at IS NULL`, u.OrgID, n); e != nil {
			return e
		}
		return logAudit(r.Context(), tx, u, "archive", "asset", n, a, map[string]any{"deleted": true})
	})
	if e != nil {
		return e
	}
	write(w, http.StatusOK, map[string]bool{"ok": true})
	return nil
}

func (s *Server) restoreAsset(w http.ResponseWriter, r *http.Request) error {
	n, e := id(r)
	if e != nil {
		return e
	}
	e = s.transaction(r, func(tx pgx.Tx) error {
		u := actor(r)
		var before Asset
		e := tx.QueryRow(r.Context(), `SELECT a.id,a.tag,a.name,a.serial_number,a.category_id,a.location_id,a.custodian,a.status,a.purchase_date::text,a.purchase_cost,a.salvage_value,a.useful_life_months,a.warranty_until::text,a.version,l.branch_id,a.next_maintenance_date::text FROM assets a JOIN locations l ON l.id=a.location_id JOIN categories c ON c.id=a.category_id WHERE a.org_id=$1 AND a.id=$2 AND a.deleted_at IS NOT NULL AND l.deleted_at IS NULL AND c.deleted_at IS NULL AND can_access_branch(a.org_id,$3,l.branch_id) FOR UPDATE OF a`, u.OrgID, n, u.ID).Scan(&before.ID, &before.Tag, &before.Name, &before.Serial, &before.Category, &before.Location, &before.Custodian, &before.Status, &before.PurchaseDate, &before.Cost, &before.Salvage, &before.Life, &before.Warranty, &before.Version, &before.BranchID, &before.NextMaintenance)
		if errors.Is(e, pgx.ErrNoRows) {
			return fail(404, "Aset arsip tidak ditemukan")
		}
		if e != nil {
			return e
		}
		if _, e = tx.Exec(r.Context(), `UPDATE assets SET deleted_at=NULL,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2`, u.OrgID, n); e != nil {
			return e
		}
		return logAudit(r.Context(), tx, u, "restore", "asset", n, map[string]any{"deleted": true}, map[string]any{"deleted": false})
	})
	if e != nil {
		return e
	}
	write(w, http.StatusOK, map[string]bool{"ok": true})
	return nil
}

func (s *Server) archiveUser(w http.ResponseWriter, r *http.Request) error {
	n, e := id(r)
	if e != nil {
		return e
	}
	u := actor(r)
	if n == u.ID {
		return fail(403, "Akun yang sedang digunakan tidak dapat diarsipkan")
	}
	e = s.transaction(r, func(tx pgx.Tx) error {
		if _, e := tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock($1)`, u.OrgID); e != nil {
			return e
		}
		var role string
		var active, all bool
		var branches []int64
		e := tx.QueryRow(r.Context(), `SELECT role,active,all_branches,ARRAY(SELECT branch_id FROM user_branches WHERE user_id=users.id) FROM users WHERE org_id=$1 AND id=$2 AND deleted_at IS NULL FOR UPDATE`, u.OrgID, n).Scan(&role, &active, &all, &branches)
		if errors.Is(e, pgx.ErrNoRows) {
			return fail(404, "User tidak ditemukan")
		}
		if e != nil {
			return e
		}
		if u.Role == "branch_admin" {
			if role == "admin" || role == "branch_admin" || !sharesBranch(branches, u.BranchIDs) {
				return fail(404, "User tidak ditemukan")
			}
		}
		if role == "admin" && active {
			var count int
			if e = tx.QueryRow(r.Context(), `SELECT count(*) FROM users WHERE org_id=$1 AND role='admin' AND active AND deleted_at IS NULL`, u.OrgID).Scan(&count); e != nil {
				return e
			}
			if count <= 1 {
				return fail(409, "Minimal satu admin pusat aktif diperlukan")
			}
		}
		if _, e = tx.Exec(r.Context(), `UPDATE users SET active=false,deleted_at=now() WHERE org_id=$1 AND id=$2`, u.OrgID, n); e != nil {
			return e
		}
		if _, e = tx.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, n); e != nil {
			return e
		}
		if len(branches) == 1 {
			branch := branches[0]
			meta(r.Context()).Branch = &branch
		}
		return logAudit(r.Context(), tx, u, "archive", "user", n, map[string]any{"role": role, "active": active, "all_branches": all, "branch_ids": branches}, map[string]bool{"deleted": true})
	})
	if e != nil {
		return e
	}
	write(w, http.StatusOK, map[string]bool{"ok": true})
	return nil
}

func (s *Server) restoreUser(w http.ResponseWriter, r *http.Request) error {
	n, e := id(r)
	if e != nil {
		return e
	}
	u := actor(r)
	e = s.transaction(r, func(tx pgx.Tx) error {
		var role string
		var all bool
		var branches []int64
		e := tx.QueryRow(r.Context(), `SELECT role,all_branches,ARRAY(SELECT ub.branch_id FROM user_branches ub JOIN branches b ON b.id=ub.branch_id WHERE ub.user_id=users.id AND b.deleted_at IS NULL) FROM users WHERE org_id=$1 AND id=$2 AND deleted_at IS NOT NULL FOR UPDATE`, u.OrgID, n).Scan(&role, &all, &branches)
		if errors.Is(e, pgx.ErrNoRows) {
			return fail(404, "User arsip tidak ditemukan")
		}
		if e != nil {
			return e
		}
		if u.Role == "branch_admin" && (role == "admin" || role == "branch_admin" || !sharesBranch(branches, u.BranchIDs)) {
			return fail(404, "User arsip tidak ditemukan")
		}
		if !all && len(branches) == 0 {
			return fail(409, "User tidak memiliki cabang aktif; pulihkan cabangnya terlebih dahulu")
		}
		if u.Role == "branch_admin" {
			branches = append([]int64(nil), u.BranchIDs...)
			if e = assignBranches(r, tx, n, false, branches); e != nil {
				return e
			}
		}
		if _, e = tx.Exec(r.Context(), `UPDATE users SET active=true,deleted_at=NULL WHERE org_id=$1 AND id=$2`, u.OrgID, n); e != nil {
			return e
		}
		if _, e = tx.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, n); e != nil {
			return e
		}
		return logAudit(r.Context(), tx, u, "restore", "user", n, map[string]bool{"deleted": true}, map[string]bool{"deleted": false})
	})
	if e != nil {
		return e
	}
	write(w, http.StatusOK, map[string]bool{"ok": true})
	return nil
}

func (s *Server) archiveBranch(w http.ResponseWriter, r *http.Request) error {
	n, e := id(r)
	if e != nil {
		return e
	}
	u := actor(r)
	e = s.transaction(r, func(tx pgx.Tx) error {
		var code string
		e := tx.QueryRow(r.Context(), `SELECT code FROM branches WHERE org_id=$1 AND id=$2 AND deleted_at IS NULL FOR UPDATE`, u.OrgID, n).Scan(&code)
		if errors.Is(e, pgx.ErrNoRows) {
			return fail(404, "Cabang tidak ditemukan")
		}
		if e != nil {
			return e
		}
		if code == "HQ" {
			return fail(409, "Cabang pusat tidak dapat diarsipkan")
		}
		var hasAssets bool
		e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM assets a JOIN locations l ON l.id=a.location_id WHERE a.org_id=$1 AND l.branch_id=$2 AND a.deleted_at IS NULL)`, u.OrgID, n).Scan(&hasAssets)
		if e != nil {
			return e
		}
		if hasAssets {
			return fail(409, "Arsipkan aset aktif di cabang terlebih dahulu")
		}
		if _, e = tx.Exec(r.Context(), `UPDATE branches SET deleted_at=now() WHERE org_id=$1 AND id=$2`, u.OrgID, n); e != nil {
			return e
		}
		if _, e = tx.Exec(r.Context(), `UPDATE users u SET active=false WHERE u.org_id=$1 AND u.active AND NOT u.all_branches AND EXISTS(SELECT 1 FROM user_branches own WHERE own.user_id=u.id AND own.branch_id=$2) AND NOT EXISTS(SELECT 1 FROM user_branches other JOIN branches b ON b.id=other.branch_id WHERE other.user_id=u.id AND other.branch_id<>$2 AND b.deleted_at IS NULL)`, u.OrgID, n); e != nil {
			return e
		}
		if _, e = tx.Exec(r.Context(), `DELETE FROM sessions WHERE user_id IN (SELECT u.id FROM users u JOIN user_branches ub ON ub.user_id=u.id WHERE u.org_id=$1 AND ub.branch_id=$2 AND NOT u.active)`, u.OrgID, n); e != nil {
			return e
		}
		return logAudit(r.Context(), tx, u, "archive", "branch", n, map[string]string{"code": code}, map[string]bool{"deleted": true})
	})
	if e != nil {
		return e
	}
	write(w, http.StatusOK, map[string]bool{"ok": true})
	return nil
}

func (s *Server) restoreBranch(w http.ResponseWriter, r *http.Request) error {
	n, e := id(r)
	if e != nil {
		return e
	}
	u := actor(r)
	e = s.transaction(r, func(tx pgx.Tx) error {
		var code string
		e := tx.QueryRow(r.Context(), `UPDATE branches SET deleted_at=NULL WHERE org_id=$1 AND id=$2 AND deleted_at IS NOT NULL RETURNING code`, u.OrgID, n).Scan(&code)
		if errors.Is(e, pgx.ErrNoRows) {
			return fail(404, "Cabang arsip tidak ditemukan")
		}
		if e != nil {
			return e
		}
		return logAudit(r.Context(), tx, u, "restore", "branch", n, map[string]bool{"deleted": true}, map[string]string{"deleted": "false", "code": code})
	})
	if e != nil {
		return e
	}
	write(w, http.StatusOK, map[string]bool{"ok": true})
	return nil
}

func (s *Server) archiveLocation(w http.ResponseWriter, r *http.Request) error {
	n, e := id(r)
	if e != nil {
		return e
	}
	u := actor(r)
	e = s.transaction(r, func(tx pgx.Tx) error {
		var branch int64
		var name string
		e := tx.QueryRow(r.Context(), `SELECT branch_id,name FROM locations WHERE org_id=$1 AND id=$2 AND deleted_at IS NULL FOR UPDATE`, u.OrgID, n).Scan(&branch, &name)
		if errors.Is(e, pgx.ErrNoRows) {
			return fail(404, "Lokasi tidak ditemukan")
		}
		if e != nil {
			return e
		}
		if e = branchScope(r, tx, branch); e != nil {
			return e
		}
		var inUse bool
		e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM assets WHERE org_id=$1 AND location_id=$2 AND deleted_at IS NULL) OR EXISTS(SELECT 1 FROM stocktakes WHERE org_id=$1 AND location_id=$2 AND status='open')`, u.OrgID, n).Scan(&inUse)
		if e != nil {
			return e
		}
		if inUse {
			return fail(409, "Pindahkan atau arsipkan aset dan tutup stocktake aktif sebelum mengarsipkan lokasi")
		}
		if _, e = tx.Exec(r.Context(), `UPDATE locations SET deleted_at=now() WHERE org_id=$1 AND id=$2`, u.OrgID, n); e != nil {
			return e
		}
		branchID := branch
		meta(r.Context()).Branch = &branchID
		return logAudit(r.Context(), tx, u, "archive", "location", n, map[string]string{"name": name}, map[string]bool{"deleted": true})
	})
	if e != nil {
		return e
	}
	write(w, http.StatusOK, map[string]bool{"ok": true})
	return nil
}

func (s *Server) restoreLocation(w http.ResponseWriter, r *http.Request) error {
	n, e := id(r)
	if e != nil {
		return e
	}
	u := actor(r)
	e = s.transaction(r, func(tx pgx.Tx) error {
		var branch int64
		var name string
		e := tx.QueryRow(r.Context(), `UPDATE locations l SET deleted_at=NULL FROM branches b WHERE l.org_id=$1 AND l.id=$2 AND l.deleted_at IS NOT NULL AND b.id=l.branch_id AND b.deleted_at IS NULL AND can_access_branch(l.org_id,$3,l.branch_id) RETURNING l.branch_id,l.name`, u.OrgID, n, u.ID).Scan(&branch, &name)
		if errors.Is(e, pgx.ErrNoRows) {
			return fail(404, "Lokasi arsip tidak ditemukan")
		}
		if e != nil {
			return e
		}
		meta(r.Context()).Branch = &branch
		return logAudit(r.Context(), tx, u, "restore", "location", n, map[string]bool{"deleted": true}, map[string]any{"deleted": false, "name": name})
	})
	if e != nil {
		return e
	}
	write(w, http.StatusOK, map[string]bool{"ok": true})
	return nil
}

func (s *Server) archiveCategory(w http.ResponseWriter, r *http.Request) error {
	n, e := id(r)
	if e != nil {
		return e
	}
	u := actor(r)
	e = s.transaction(r, func(tx pgx.Tx) error {
		var name string
		var branchID int64
		var shared bool
		e := tx.QueryRow(r.Context(), `SELECT name,branch_id IS NULL,COALESCE(branch_id,0) FROM categories WHERE org_id=$1 AND id=$2 AND deleted_at IS NULL FOR UPDATE`, u.OrgID, n).Scan(&name, &shared, &branchID)
		if errors.Is(e, pgx.ErrNoRows) {
			return fail(404, "Kategori tidak ditemukan")
		}
		if e != nil {
			return e
		}
		if shared && u.Role != "admin" {
			return fail(404, "Kategori tidak ditemukan")
		}
		if branchID > 0 {
			if e = branchScope(r, tx, branchID); e != nil {
				return fail(404, "Kategori tidak ditemukan")
			}
		}
		var inUse bool
		e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM assets WHERE org_id=$1 AND category_id=$2 AND deleted_at IS NULL)`, u.OrgID, n).Scan(&inUse)
		if e != nil {
			return e
		}
		if inUse {
			return fail(409, "Kategori masih digunakan aset aktif")
		}
		if _, e = tx.Exec(r.Context(), `UPDATE categories SET deleted_at=now() WHERE org_id=$1 AND id=$2`, u.OrgID, n); e != nil {
			return e
		}
		return logAudit(r.Context(), tx, u, "archive", "category", n, map[string]string{"name": name}, map[string]bool{"deleted": true})
	})
	if e != nil {
		return e
	}
	write(w, http.StatusOK, map[string]bool{"ok": true})
	return nil
}

func (s *Server) restoreCategory(w http.ResponseWriter, r *http.Request) error {
	n, e := id(r)
	if e != nil {
		return e
	}
	u := actor(r)
	e = s.transaction(r, func(tx pgx.Tx) error {
		var name string
		e := tx.QueryRow(r.Context(), `UPDATE categories c SET deleted_at=NULL WHERE c.org_id=$1 AND c.id=$2 AND c.deleted_at IS NOT NULL AND ((c.branch_id IS NULL AND $3='admin') OR (c.branch_id IS NOT NULL AND can_access_branch(c.org_id,$4,c.branch_id))) RETURNING c.name`, u.OrgID, n, u.Role, u.ID).Scan(&name)
		if errors.Is(e, pgx.ErrNoRows) {
			return fail(404, "Kategori arsip tidak ditemukan")
		}
		if e != nil {
			return e
		}
		return logAudit(r.Context(), tx, u, "restore", "category", n, map[string]bool{"deleted": true}, map[string]any{"deleted": false, "name": name})
	})
	if e != nil {
		return e
	}
	write(w, http.StatusOK, map[string]bool{"ok": true})
	return nil
}

func sharesBranch(a, b []int64) bool {
	for _, left := range a {
		for _, right := range b {
			if left == right {
				return true
			}
		}
	}
	return false
}
