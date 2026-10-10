package app

import (
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
	"net/http"
)

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if e := decode(w, r, &in); e != nil {
		return e
	}
	if len(in.New) < 12 || len(in.New) > 72 || len(in.Current) > 72 {
		return fail(422, "Password baru 12–72 byte")
	}
	e := s.transaction(r, func(tx pgx.Tx) error {
		u := actor(r)
		var old string
		e := tx.QueryRow(r.Context(), `SELECT password_hash FROM users WHERE org_id=$1 AND id=$2 FOR UPDATE`, u.OrgID, u.ID).Scan(&old)
		if e != nil {
			return e
		}
		if bcrypt.CompareHashAndPassword([]byte(old), []byte(in.Current)) != nil {
			return fail(403, "Password lama salah")
		}
		h, e := bcrypt.GenerateFromPassword([]byte(in.New), 12)
		if e != nil {
			return e
		}
		_, e = tx.Exec(r.Context(), `UPDATE users SET password_hash=$1 WHERE org_id=$2 AND id=$3`, string(h), u.OrgID, u.ID)
		if e != nil {
			return e
		}
		_, e = tx.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, u.ID)
		if e != nil {
			return e
		}
		return logAudit(r.Context(), tx, u, "password_change", "user", u.ID, nil, map[string]bool{"sessions_revoked": true})
	})
	if e != nil {
		return e
	}
	http.SetCookie(w, &http.Cookie{Name: "assetflow_session", Value: "", Path: "/", HttpOnly: true, Secure: s.Secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	write(w, 200, map[string]bool{"ok": true})
	return nil
}
func (s *Server) updateUserAccess(w http.ResponseWriter, r *http.Request) error {
	n, e := id(r)
	if e != nil {
		return e
	}
	var in struct {
		Role     string  `json:"role"`
		Active   bool    `json:"active"`
		All      bool    `json:"all_branches"`
		Branches []int64 `json:"branch_ids"`
	}
	if e = decode(w, r, &in); e != nil {
		return e
	}
	if !validRole(in.Role) {
		return fail(422, "Role tidak valid")
	}
	if !canAssignRole(actor(r).Role, in.Role) {
		return fail(403, "Role tersebut tidak dapat diberikan oleh admin cabang")
	}
	if in.Role == "admin" {
		in.All = true
	}
	if actor(r).Role == "branch_admin" {
		if len(actor(r).BranchIDs) != 1 {
			return fail(403, "Admin cabang harus memiliki tepat satu cabang aktif")
		}
		in.All = false
		in.Branches = append([]int64(nil), actor(r).BranchIDs...)
	}
	if in.Role != "admin" && in.All {
		return fail(422, "Hanya admin pusat yang dapat memiliki akses seluruh cabang")
	}
	if in.Role == "branch_admin" && (in.All || uniqueCount(in.Branches) != 1) {
		return fail(422, "Admin cabang harus memiliki tepat satu cabang")
	}
	if n == actor(r).ID {
		return fail(403, "Perubahan akses sendiri tidak diizinkan")
	}
	e = s.transaction(r, func(tx pgx.Tx) error {
		u := actor(r)
		if _, e := tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock($1)`, u.OrgID); e != nil {
			return e
		}
		var admins int
		if e := tx.QueryRow(r.Context(), `SELECT count(*) FROM users WHERE org_id=$1 AND role='admin' AND active AND deleted_at IS NULL`, u.OrgID).Scan(&admins); e != nil {
			return e
		}
		var role string
		var active, all bool
		var oldBranches []int64
		e := tx.QueryRow(r.Context(), `SELECT role,active,all_branches,ARRAY(SELECT branch_id FROM user_branches WHERE user_id=users.id) FROM users WHERE org_id=$1 AND id=$2 AND deleted_at IS NULL FOR UPDATE`, u.OrgID, n).Scan(&role, &active, &all, &oldBranches)
		if e == pgx.ErrNoRows {
			return fail(404, "User tidak ditemukan")
		}
		if e != nil {
			return e
		}
		if u.Role == "branch_admin" {
			if role == "admin" || role == "branch_admin" {
				return fail(404, "User tidak ditemukan")
			}
			in.All = false
			in.Branches = append([]int64(nil), u.BranchIDs...)
			if !canAssignRole(u.Role, in.Role) {
				return fail(403, "Role tersebut tidak dapat diberikan oleh admin cabang")
			}
			shared := false
			for _, oldBranch := range oldBranches {
				for _, branch := range u.BranchIDs {
					if oldBranch == branch {
						shared = true
					}
				}
			}
			if !shared {
				return fail(404, "User tidak ditemukan")
			}
		}
		if role == "admin" && active && (in.Role != "admin" || !in.Active) && admins <= 1 {
			return fail(409, "Minimal satu admin aktif diperlukan")
		}
		_, e = tx.Exec(r.Context(), `UPDATE users SET role=$1,active=$2,all_branches=$5 WHERE org_id=$3 AND id=$4 AND deleted_at IS NULL`, in.Role, in.Active, u.OrgID, n, in.All)
		if e != nil {
			return e
		}
		if e := assignBranches(r, tx, n, in.All, in.Branches); e != nil {
			return e
		}
		_, e = tx.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, n)
		if e != nil {
			return e
		}
		return logAudit(r.Context(), tx, u, "access_change", "user", n, map[string]any{"role": role, "active": active, "all_branches": all, "branch_ids": oldBranches}, in)
	})
	if e != nil {
		return e
	}
	write(w, 200, map[string]bool{"ok": true})
	return nil
}
