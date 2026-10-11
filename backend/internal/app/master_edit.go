package app

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

// IDs, branch ownership and login identifiers stay stable when editing master data.
func (s *Server) updateMaster(w http.ResponseWriter, r *http.Request) error {
	n, err := id(r)
	if err != nil {
		return err
	}
	var in struct {
		Name     string `json:"name"`
		Address  string `json:"address"`
		Email    string `json:"email"`
		Building string `json:"building"`
		Floor    string `json:"floor"`
		Room     string `json:"room"`
		Life     int    `json:"useful_life_months"`
		Version  int    `json:"version"`
		DetailID int64  `json:"general_code_detail_id"`
	}
	if err = decode(w, r, &in); err != nil {
		return err
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	resource := strings.Split(r.URL.Path, "/")[2]
	nameLimit := 100
	if resource == "branches" {
		nameLimit = 150
	}
	if len(in.Name) < 2 || len(in.Name) > nameLimit || in.Version < 1 {
		return fail(422, "Nama/versi tidak valid")
	}
	if resource == "branches" && len(in.Address) > 1000 {
		return fail(422, "Alamat terlalu panjang")
	}
	if resource == "locations" && (len(in.Building) > 100 || len(in.Floor) > 40 || len(in.Room) > 100) {
		return fail(422, "Detail lokasi terlalu panjang")
	}
	if resource == "categories" && (in.Life < 1 || in.Life > 1200) {
		return fail(422, "Masa manfaat tidak valid")
	}
	if resource == "users" && (len(in.Email) > 254 || !strings.Contains(in.Email, "@")) {
		return fail(422, "Email tidak valid")
	}
	entity := map[string]string{"branches": "branch", "locations": "location", "categories": "category", "users": "user"}[resource]
	if entity == "" {
		return fail(404, "Data tidak ditemukan")
	}
	err = s.transaction(r, func(tx pgx.Tx) error {
		u := actor(r)
		var before []byte
		var version int
		query := "SELECT to_jsonb(t)-'password_hash',version FROM " + resource + " t WHERE org_id=$1 AND id=$2 AND deleted_at IS NULL FOR UPDATE"
		if err := tx.QueryRow(r.Context(), query, u.OrgID, n).Scan(&before, &version); err == pgx.ErrNoRows {
			return fail(404, "Data tidak ditemukan")
		} else if err != nil {
			return err
		}
		if version != in.Version {
			return fail(409, "Versi berubah; muat ulang")
		}
		var old struct {
			Branch       int64  `json:"branch_id"`
			Role         string `json:"role"`
			Code         string `json:"code"`
			BusinessCode string `json:"business_code"`
		}
		if err := json.Unmarshal(before, &old); err != nil {
			return err
		}
		switch resource {
		case "branches":
			if err := branchScope(r, tx, n); err != nil {
				return err
			}
			business := old.BusinessCode
			if in.DetailID > 0 {
				if business != "" {
					return fail(409, "Kode perusahaan sudah dialokasikan")
				}
				kind := "branch"
				if old.Code == "HQ" {
					kind = "hq"
				}
				business, err = allocateGeneralCode(r, tx, in.DetailID, n, kind)
				if err != nil {
					return err
				}
			}
			_, err = tx.Exec(r.Context(), `UPDATE branches SET name=$1,address=$2,business_code=$3 WHERE org_id=$4 AND id=$5`, in.Name, in.Address, business, u.OrgID, n)
		case "locations":
			if err := branchScope(r, tx, old.Branch); err != nil {
				return err
			}
			_, err = tx.Exec(r.Context(), `UPDATE locations SET name=$1,building=$2,floor=$3,room=$4 WHERE org_id=$5 AND id=$6`, in.Name, in.Building, in.Floor, in.Room, u.OrgID, n)
		case "categories":
			if old.Branch == 0 && u.Role != "admin" {
				return fail(404, "Kategori tidak ditemukan")
			}
			if old.Branch > 0 {
				if err := branchScope(r, tx, old.Branch); err != nil {
					return err
				}
			}
			_, err = tx.Exec(r.Context(), `UPDATE categories SET name=$1,useful_life_months=$2,version=version+1 WHERE org_id=$3 AND id=$4`, in.Name, in.Life, u.OrgID, n)
		case "users":
			if u.Role == "branch_admin" {
				var shared bool
				if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM user_branches own JOIN user_branches target USING(org_id,branch_id) WHERE own.org_id=$1 AND own.user_id=$2 AND target.user_id=$3)`, u.OrgID, u.ID, n).Scan(&shared); err != nil {
					return err
				}
				if !shared || old.Role == "admin" || old.Role == "branch_admin" {
					return fail(404, "User tidak ditemukan")
				}
			}
			_, err = tx.Exec(r.Context(), `UPDATE users SET name=$1,email=$2 WHERE org_id=$3 AND id=$4`, in.Name, in.Email, u.OrgID, n)
		}
		if err != nil {
			return err
		}
		var after []byte
		if err := tx.QueryRow(r.Context(), "SELECT to_jsonb(t)-'password_hash' FROM "+resource+" t WHERE org_id=$1 AND id=$2", u.OrgID, n).Scan(&after); err != nil {
			return err
		}
		return logAudit(r.Context(), tx, u, "update", entity, n, json.RawMessage(before), json.RawMessage(after))
	})
	if err != nil {
		return err
	}
	write(w, 200, map[string]bool{"ok": true})
	return nil
}
