package app

import (
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"strconv"
	"strings"
)

func selectedBranch(r *http.Request) int64 {
	v, _ := strconvBranch(r.URL.Query().Get("branch_id"))
	return v
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
	return s.rows(w, r, `SELECT id,code,name,address FROM branches WHERE org_id=$1 AND can_access_branch(org_id,$2,id) ORDER BY code LIMIT 1000`, u.OrgID, u.ID)
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
	return s.rows(w, r, `SELECT l.id,l.name,l.branch_id,b.name AS branch_name,b.code AS branch_code FROM locations l JOIN branches b ON b.id=l.branch_id WHERE l.org_id=$1 AND can_access_branch(l.org_id,$2,l.branch_id) AND ($3::bigint=0 OR l.branch_id=$3) ORDER BY b.code,l.name LIMIT 1000`, u.OrgID, u.ID, selectedBranch(r))
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
	return s.rows(w, r, `SELECT id,name,useful_life_months,maintenance_interval_days,maintenance_instructions,version FROM categories WHERE org_id=$1 ORDER BY name LIMIT 1000`, actor(r).OrgID)
}
func (s *Server) createCategory(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Name         string `json:"name"`
		Life         int    `json:"useful_life_months"`
		Interval     int    `json:"maintenance_interval_days"`
		Instructions string `json:"maintenance_instructions"`
	}
	if e := decode(w, r, &in); e != nil {
		return e
	}
	in.Name = strings.TrimSpace(in.Name)
	if len(in.Name) < 2 || len(in.Name) > 100 || in.Life < 1 || in.Life > 1200 || in.Interval < 0 || in.Interval > 3650 || len(in.Instructions) > 2000 {
		return fail(422, "Kategori/policy maintenance tidak valid")
	}
	var n int64
	e := s.transaction(r, func(tx pgx.Tx) error {
		e := tx.QueryRow(r.Context(), `INSERT INTO categories(org_id,name,useful_life_months,maintenance_interval_days,maintenance_instructions) VALUES($1,$2,$3,$4,$5) RETURNING id`, actor(r).OrgID, in.Name, in.Life, in.Interval, in.Instructions).Scan(&n)
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
func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) error {
	return s.rows(w, r, `SELECT id,name,email,role,active,all_branches,ARRAY(SELECT ub.branch_id FROM user_branches ub WHERE ub.user_id=u.id ORDER BY ub.branch_id) AS branch_ids FROM users u WHERE org_id=$1 ORDER BY id LIMIT 1000`, actor(r).OrgID)
}
func assignBranches(r *http.Request, tx pgx.Tx, user int64, all bool, branches []int64) error {
	if !all && len(branches) == 0 {
		return fail(422, "User terbatas wajib memiliki akses cabang")
	}
	if len(branches) > 1000 {
		return fail(422, "Maksimal 1000 cabang/user")
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
		e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM branches WHERE org_id=$1 AND id=$2)`, actor(r).OrgID, branch).Scan(&exists)
		if e != nil {
			return e
		}
		if !exists {
			return fail(422, "Cabang bukan milik perusahaan")
		}
		if _, e = tx.Exec(r.Context(), `INSERT INTO user_branches(org_id,user_id,branch_id) VALUES($1,$2,$3)`, actor(r).OrgID, user, branch); e != nil {
			return e
		}
	}
	return nil
}
func validRole(v string) bool {
	switch v {
	case "admin", "manager", "operator", "auditor":
		return true
	}
	return false
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
	if in.Role == "admin" {
		in.All = true
	}
	if len(in.Name) < 2 || len(in.Name) > 100 || len(in.Password) < 12 || len(in.Password) > 72 || len(in.Email) > 254 || !strings.Contains(in.Email, "@") || !validRole(in.Role) {
		return fail(422, "Nama/email/role/password tidak valid")
	}
	pw, e := bcrypt.GenerateFromPassword([]byte(in.Password), 12)
	if e != nil {
		return e
	}
	var n int64
	e = s.transaction(r, func(tx pgx.Tx) error {
		e := tx.QueryRow(r.Context(), `INSERT INTO users(org_id,name,email,password_hash,role,all_branches) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, actor(r).OrgID, in.Name, in.Email, string(pw), in.Role, in.All).Scan(&n)
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
	write(w, 201, map[string]int64{"id": n})
	return nil
}
func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) error {
	u := actor(r)
	branch := selectedBranch(r)
	var total, available, assigned, maint, disposed, cost, due int64
	e := s.DB.QueryRow(r.Context(), `SELECT count(*),count(*) FILTER(WHERE a.status='available'),count(*) FILTER(WHERE a.status='assigned'),count(*) FILTER(WHERE a.status='maintenance'),count(*) FILTER(WHERE a.status='disposed'),coalesce(sum(a.purchase_cost) FILTER(WHERE a.status<>'disposed'),0)::bigint,count(*) FILTER(WHERE a.next_maintenance_date<=CURRENT_DATE AND a.status<>'disposed') FROM assets a JOIN locations l ON l.id=a.location_id WHERE a.org_id=$1 AND can_access_branch(a.org_id,$2,l.branch_id) AND ($3::bigint=0 OR l.branch_id=$3)`, u.OrgID, u.ID, branch).Scan(&total, &available, &assigned, &maint, &disposed, &cost, &due)
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
	write(w, 200, map[string]any{"total": total, "available": available, "assigned": assigned, "maintenance": maint, "disposed": disposed, "purchase_value": cost, "pending_requests": pending, "overdue_maintenance": overdue, "maintenance_due_assets": due})
	return nil
}
