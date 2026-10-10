package app

import (
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"strconv"
	"strings"
)

func selectedBranch(r *http.Request) int64 {
	if raw := r.URL.Query().Get("branch_id"); raw != "" {
		v, _ := strconvBranch(raw)
		return v
	}
	if u, ok := r.Context().Value(ctxKey{}).(User); ok {
		return u.ActiveBranchID
	}
	return 0
}
func strconvBranch(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.ParseInt(s, 10, 64)
}
func locationScope(r *http.Request, tx pgx.Tx, location int64) error {
	var ok bool
	e := tx.QueryRow(r.Context(), `SELECT can_access_location($1,$2,$3)`, actor(r).OrgID, actor(r).ID, location).Scan(&ok)
	if e != nil {
		return e
	}
	if !ok {
		return fail(403, "Lokasi/cabang di luar akses Anda")
	}
	return nil
}
func branchScope(r *http.Request, tx pgx.Tx, branch int64) error {
	var ok bool
	e := tx.QueryRow(r.Context(), `SELECT can_access_branch($1,$2,$3)`, actor(r).OrgID, actor(r).ID, branch).Scan(&ok)
	if e != nil {
		return e
	}
	if !ok {
		return fail(403, "Cabang di luar akses Anda")
	}
	return nil
}
func (s *Server) listBranches(w http.ResponseWriter, r *http.Request) error {
	u := actor(r)
	archived := r.URL.Query().Get("archived") == "true" && hasCapability(u.Role, "branches.manage")
	return s.rows(w, r, `SELECT id,code,name,address,deleted_at FROM branches WHERE org_id=$1 AND (($3::boolean AND deleted_at IS NOT NULL) OR (NOT $3::boolean AND deleted_at IS NULL)) AND (can_access_branch(org_id,$2,id) OR ($3::boolean AND $4::boolean)) ORDER BY code LIMIT 1000`, u.OrgID, u.ID, archived, u.AllBranches)
}
func (s *Server) createBranch(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Code    string `json:"code"`
		Name    string `json:"name"`
		Address string `json:"address"`
	}
	if e := decode(w, r, &in); e != nil {
		return e
	}
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	in.Name = strings.TrimSpace(in.Name)
	if len(in.Code) < 1 || len(in.Code) > 30 || len(in.Name) < 2 || len(in.Name) > 150 || len(in.Address) > 1000 {
		return fail(422, "Kode/nama/alamat cabang tidak valid")
	}
	var n int64
	e := s.transaction(r, func(tx pgx.Tx) error {
		e := tx.QueryRow(r.Context(), `INSERT INTO branches(org_id,code,name,address) VALUES($1,$2,$3,$4) RETURNING id`, actor(r).OrgID, in.Code, in.Name, in.Address).Scan(&n)
		if e != nil {
			return e
		}
		return logAudit(r.Context(), tx, actor(r), "create", "branch", n, nil, in)
	})
	if e != nil {
		return e
	}
	write(w, 201, map[string]int64{"id": n})
	return nil
}
func (s *Server) listLocations(w http.ResponseWriter, r *http.Request) error {
	u := actor(r)
	archived := r.URL.Query().Get("archived") == "true" && hasCapability(u.Role, "locations.archive")
	return s.rows(w, r, `SELECT l.id,l.name,l.branch_id,b.name AS branch_name,b.code AS branch_code,l.deleted_at FROM locations l JOIN branches b ON b.id=l.branch_id WHERE l.org_id=$1 AND (($4::boolean AND l.deleted_at IS NOT NULL) OR (NOT $4::boolean AND l.deleted_at IS NULL)) AND b.deleted_at IS NULL AND can_access_branch(l.org_id,$2,l.branch_id) AND ($3::bigint=0 OR l.branch_id=$3) ORDER BY b.code,l.name LIMIT 1000`, u.OrgID, u.ID, selectedBranch(r), archived)
}
func (s *Server) createLocation(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Name   string `json:"name"`
		Branch int64  `json:"branch_id"`
	}
	if e := decode(w, r, &in); e != nil {
		return e
	}
	in.Name = strings.TrimSpace(in.Name)
	if len(in.Name) < 2 || len(in.Name) > 100 {
		return fail(422, "Nama lokasi 2–100 karakter")
	}
	var n int64
	e := s.transaction(r, func(tx pgx.Tx) error {
		if e := branchScope(r, tx, in.Branch); e != nil {
			return e
		}
		e := tx.QueryRow(r.Context(), `INSERT INTO locations(org_id,branch_id,name) VALUES($1,$2,$3) RETURNING id`, actor(r).OrgID, in.Branch, in.Name).Scan(&n)
		if e != nil {
			return e
		}
		return logAudit(r.Context(), tx, actor(r), "create", "location", n, nil, in)
	})
	if e != nil {
		return e
	}
	write(w, 201, map[string]int64{"id": n})
	return nil
}
func (s *Server) listCategories(w http.ResponseWriter, r *http.Request) error {
	u := actor(r)
	archived := r.URL.Query().Get("archived") == "true" && hasCapability(u.Role, "categories.archive")
	return s.rows(w, r, `SELECT c.id,c.name,c.useful_life_months,c.maintenance_interval_days,c.maintenance_instructions,c.version,c.branch_id,b.name AS branch_name,c.deleted_at,c.depreciation_method FROM categories c LEFT JOIN branches b ON b.org_id=c.org_id AND b.id=c.branch_id WHERE c.org_id=$1 AND (($2::boolean AND c.deleted_at IS NOT NULL) OR (NOT $2::boolean AND c.deleted_at IS NULL)) AND (c.branch_id IS NULL OR can_access_branch(c.org_id,$3,c.branch_id)) AND ($4::bigint=0 OR c.branch_id IS NULL OR c.branch_id=$4) ORDER BY c.name LIMIT 1000`, u.OrgID, archived, u.ID, selectedBranch(r))
}
func (s *Server) createCategory(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Name         string `json:"name"`
		Life         int    `json:"useful_life_months"`
		Interval     int    `json:"maintenance_interval_days"`
		Instructions string `json:"maintenance_instructions"`
		Depreciation string `json:"depreciation_method"`
		Branch       int64  `json:"branch_id"`
	}
	if e := decode(w, r, &in); e != nil {
		return e
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Depreciation == "" {
		in.Depreciation = "straight_line"
	}
	if in.Depreciation != "straight_line" && in.Depreciation != "declining_balance" && in.Depreciation != "non_depreciable" {
		return fail(422, "Metode depresiasi kategori tidak valid")
	}
	if !hasCapability(actor(r).Role, "assets.finance") {
		in.Depreciation = "straight_line"
	}
	if len(in.Name) < 2 || len(in.Name) > 100 || in.Life < 1 || in.Life > 1200 || in.Interval < 0 || in.Interval > 3650 || len(in.Instructions) > 2000 {
		return fail(422, "Kategori/policy maintenance tidak valid")
	}
	var n int64
	e := s.transaction(r, func(tx pgx.Tx) error {
		u := actor(r)
		if u.Role == "branch_admin" {
			in.Branch = selectedBranch(r)
		}
		if in.Branch > 0 {
			if e := branchScope(r, tx, in.Branch); e != nil {
				return e
			}
		} else if u.Role != "admin" {
			return fail(403, "Admin cabang hanya dapat membuat kategori untuk cabangnya")
		}
		e := tx.QueryRow(r.Context(), `INSERT INTO categories(org_id,name,useful_life_months,maintenance_interval_days,maintenance_instructions,branch_id,depreciation_method) VALUES($1,$2,$3,$4,$5,NULLIF($6,0),$7) RETURNING id`, u.OrgID, in.Name, in.Life, in.Interval, in.Instructions, in.Branch, in.Depreciation).Scan(&n)
		if e != nil {
			return e
		}
		return logAudit(r.Context(), tx, actor(r), "create", "category", n, nil, in)
	})
	if e != nil {
		return e
	}
	write(w, 201, map[string]int64{"id": n})
	return nil
}
func (s *Server) listAssignableRoles(w http.ResponseWriter, r *http.Request) error {
	write(w, http.StatusOK, assignableRoles(actor(r).Role))
	return nil
}
func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) error {
	u := actor(r)
	archived := r.URL.Query().Get("archived") == "true" && hasCapability(u.Role, "users.archive")
	return s.rows(w, r, `SELECT u.id,u.name,u.email,u.employee_id,u.role,u.active,u.all_branches,ARRAY(SELECT ub.branch_id FROM user_branches ub JOIN branches b ON b.id=ub.branch_id WHERE ub.user_id=u.id AND b.deleted_at IS NULL AND (u.role='admin' OR b.code<>'HQ') ORDER BY ub.branch_id) AS branch_ids,u.deleted_at FROM users u WHERE u.org_id=$1 AND (($4::boolean AND u.deleted_at IS NOT NULL) OR (NOT $4::boolean AND u.deleted_at IS NULL)) AND ($2 OR EXISTS(SELECT 1 FROM user_branches target JOIN user_branches own ON own.org_id=target.org_id AND own.branch_id=target.branch_id WHERE target.org_id=u.org_id AND target.user_id=u.id AND own.user_id=$3)) ORDER BY u.id LIMIT 1000`, u.OrgID, u.AllBranches, u.ID, archived)
}
func assignBranches(r *http.Request, tx pgx.Tx, user int64, all bool, branches []int64) error {
	if !all && len(branches) == 0 {
		return fail(422, "User terbatas wajib memiliki akses cabang")
	}
	if len(branches) > 1000 {
		return fail(422, "Maksimal 1000 cabang/user")
	}
	var role string
	if e := tx.QueryRow(r.Context(), `SELECT role FROM users WHERE org_id=$1 AND id=$2`, actor(r).OrgID, user).Scan(&role); e != nil {
		return e
	}
	if role != "admin" && all {
		return fail(422, "Hanya admin pusat yang dapat memiliki akses seluruh cabang")
	}
	_, e := tx.Exec(r.Context(), `DELETE FROM user_branches WHERE org_id=$1 AND user_id=$2`, actor(r).OrgID, user)
	if e != nil {
		return e
	}
	seen := map[int64]bool{}
	for _, branch := range branches {
		if seen[branch] {
			continue
		}
		seen[branch] = true
		var exists bool
		e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM branches WHERE org_id=$1 AND id=$2 AND deleted_at IS NULL AND ($3='admin' OR code<>'HQ'))`, actor(r).OrgID, branch, role).Scan(&exists)
		if e != nil {
			return e
		}
		if !exists {
			return fail(422, "Cabang pusat hanya dapat diberikan kepada admin pusat")
		}
		if _, e = tx.Exec(r.Context(), `INSERT INTO user_branches(org_id,user_id,branch_id) VALUES($1,$2,$3)`, actor(r).OrgID, user, branch); e != nil {
			return e
		}
	}
	return nil
}
func (s *Server) createUser(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Name     string  `json:"name"`
		Email    string  `json:"email"`
		Password string  `json:"password"`
		Role     string  `json:"role"`
		All      bool    `json:"all_branches"`
		Branches []int64 `json:"branch_ids"`
	}
	if e := decode(w, r, &in); e != nil {
		return e
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if actor(r).Role == "branch_admin" {
		if len(actor(r).BranchIDs) != 1 {
			return fail(403, "Admin cabang harus memiliki tepat satu cabang aktif")
		}
		in.All = false
		in.Branches = append([]int64(nil), actor(r).BranchIDs...)
	}
	if in.Role == "branch_admin" && (in.All || uniqueCount(in.Branches) != 1) {
		return fail(422, "Admin cabang harus memiliki tepat satu cabang")
	}
	if in.Role == "admin" {
		in.All = true
	} else if in.All {
		return fail(422, "Hanya admin pusat yang dapat memiliki akses seluruh cabang")
	}
	if !validRole(in.Role) {
		return fail(422, "Role tidak valid")
	}
	if !canAssignRole(actor(r).Role, in.Role) {
		return fail(403, "Role tersebut tidak dapat diberikan")
	}
	if len(in.Name) < 2 || len(in.Name) > 100 || len(in.Password) < 12 || len(in.Password) > 72 || len(in.Email) > 254 || !strings.Contains(in.Email, "@") {
		return fail(422, "Nama/email/role/password tidak valid")
	}
	pw, e := bcrypt.GenerateFromPassword([]byte(in.Password), 12)
	if e != nil {
		return e
	}
	var n int64
	var employeeID string
	e = s.transaction(r, func(tx pgx.Tx) error {
		e := tx.QueryRow(r.Context(), `INSERT INTO users(org_id,name,email,password_hash,role,all_branches) VALUES($1,$2,$3,$4,$5,$6) RETURNING id,employee_id`, actor(r).OrgID, in.Name, in.Email, string(pw), in.Role, in.All).Scan(&n, &employeeID)
		if e != nil {
			return e
		}
		if e = assignBranches(r, tx, n, in.All, in.Branches); e != nil {
			return e
		}
		return logAudit(r.Context(), tx, actor(r), "create", "user", n, nil, map[string]any{"name": in.Name, "email": in.Email, "role": in.Role, "all_branches": in.All, "branch_ids": in.Branches})
	})
	if e != nil {
		return e
	}
	write(w, 201, map[string]any{"id": n, "employee_id": employeeID})
	return nil
}
func uniqueCount(ids []int64) int {
	seen := map[int64]struct{}{}
	for _, id := range ids {
		seen[id] = struct{}{}
	}
	return len(seen)
}
func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) error {
	u := actor(r)
	branch := selectedBranch(r)
	var total, available, assigned, maint, disposed, cost, due int64
	e := s.DB.QueryRow(r.Context(), `SELECT count(*),count(*) FILTER(WHERE a.status='available'),count(*) FILTER(WHERE a.status='assigned'),count(*) FILTER(WHERE a.status='maintenance'),count(*) FILTER(WHERE a.status='disposed'),coalesce(sum(a.purchase_cost) FILTER(WHERE a.status<>'disposed'),0)::bigint,count(*) FILTER(WHERE a.next_maintenance_date<=CURRENT_DATE AND a.status<>'disposed') FROM assets a JOIN locations l ON l.id=a.location_id WHERE a.org_id=$1 AND a.deleted_at IS NULL AND l.deleted_at IS NULL AND can_access_branch(a.org_id,$2,l.branch_id) AND ($3::bigint=0 OR l.branch_id=$3)`, u.OrgID, u.ID, branch).Scan(&total, &available, &assigned, &maint, &disposed, &cost, &due)
	if e != nil {
		return e
	}
	var pending, overdue int64
	e = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM requests WHERE org_id=$1 AND status='pending' AND can_access_branch(org_id,$2,source_branch_id) AND ($3::bigint=0 OR source_branch_id=$3)`, u.OrgID, u.ID, branch).Scan(&pending)
	if e != nil {
		return e
	}
	e = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM maintenance WHERE org_id=$1 AND status IN ('scheduled','in_progress') AND due_date<CURRENT_DATE AND can_access_branch(org_id,$2,branch_id) AND ($3::bigint=0 OR branch_id=$3)`, u.OrgID, u.ID, branch).Scan(&overdue)
	if e != nil {
		return e
	}
	result := map[string]any{"total": total, "available": available, "assigned": assigned, "maintenance": maint, "disposed": disposed, "pending_requests": pending, "overdue_maintenance": overdue, "maintenance_due_assets": due}
	if hasCapability(u.Role, "assets.finance") {
		result["purchase_value"] = cost
	}
	write(w, 200, result)
	return nil
}
