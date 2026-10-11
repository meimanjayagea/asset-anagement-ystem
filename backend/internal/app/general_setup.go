package app

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

var setupKey = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,39}$`)
var setupPrefix = regexp.MustCompile(`^[A-Z0-9]{1,8}$`)

func (s *Server) listGeneralCodes(w http.ResponseWriter, r *http.Request) error {
	return s.rows(w, r, `SELECT g.*, (SELECT count(*) FROM general_code_details d WHERE d.org_id=g.org_id AND d.general_code_id=g.id AND d.deleted_at IS NULL) AS detail_count FROM general_codes g WHERE g.org_id=$1 AND ((deleted_at IS NOT NULL)=$2) ORDER BY code LIMIT 1000`, actor(r).OrgID, r.URL.Query().Get("archived") == "true")
}
func (s *Server) listGeneralDetails(w http.ResponseWriter, r *http.Request) error {
	return s.rows(w, r, `SELECT d.*,g.code AS general_code,g.name AS general_name,g.company_prefix, g.company_prefix||'-'||d.prefix||'-'||lpad(d.next_number::text,d.sequence_width,'0') AS preview,(SELECT count(*) FROM issued_general_codes i WHERE i.org_id=d.org_id AND i.detail_id=d.id) AS issued_count FROM general_code_details d JOIN general_codes g ON g.org_id=d.org_id AND g.id=d.general_code_id WHERE d.org_id=$1 AND ((d.deleted_at IS NOT NULL)=$2) ORDER BY g.code,d.code LIMIT 1000`, actor(r).OrgID, r.URL.Query().Get("archived") == "true")
}
func (s *Server) listIssuedCodes(w http.ResponseWriter, r *http.Request) error {
	return s.rows(w, r, `SELECT i.*,d.name AS detail_name FROM issued_general_codes i JOIN general_code_details d ON d.org_id=i.org_id AND d.id=i.detail_id WHERE i.org_id=$1 ORDER BY i.id DESC LIMIT 1000`, actor(r).OrgID)
}

type generalInput struct {
	Code          string `json:"code"`
	Name          string `json:"name"`
	CompanyPrefix string `json:"company_prefix"`
	Description   string `json:"description"`
	Version       int    `json:"version"`
}

func (s *Server) saveGeneralCode(w http.ResponseWriter, r *http.Request) error {
	var in generalInput
	if err := decode(w, r, &in); err != nil {
		return err
	}
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	in.CompanyPrefix = strings.ToUpper(strings.TrimSpace(in.CompanyPrefix))
	in.Name = strings.TrimSpace(in.Name)
	if !setupKey.MatchString(in.Code) || !setupPrefix.MatchString(in.CompanyPrefix) || len(in.Name) < 2 || len(in.Name) > 150 || len(in.Description) > 1000 {
		return fail(422, "Kode/nama/prefix General Code tidak valid")
	}
	var n int64
	var err error
	if r.Method == "PUT" {
		n, err = id(r)
		if err != nil {
			return err
		}
	}
	err = s.transaction(r, func(tx pgx.Tx) error {
		if n == 0 {
			if err := tx.QueryRow(r.Context(), `INSERT INTO general_codes(org_id,code,name,company_prefix,description) VALUES($1,$2,$3,$4,$5) RETURNING id`, actor(r).OrgID, in.Code, in.Name, in.CompanyPrefix, in.Description).Scan(&n); err != nil {
				return err
			}
			return logAudit(r.Context(), tx, actor(r), "create", "general_code", n, nil, in)
		}
		var old generalInput
		if err := tx.QueryRow(r.Context(), `SELECT code,name,company_prefix,description,version FROM general_codes WHERE org_id=$1 AND id=$2 AND deleted_at IS NULL FOR UPDATE`, actor(r).OrgID, n).Scan(&old.Code, &old.Name, &old.CompanyPrefix, &old.Description, &old.Version); err == pgx.ErrNoRows {
			return fail(404, "General Code tidak ditemukan")
		} else if err != nil {
			return err
		}
		if old.Version != in.Version {
			return fail(409, "Versi berubah; muat ulang data")
		}
		if old.Code != in.Code {
			return fail(422, "Identitas kode tidak dapat diubah")
		}
		var issued bool
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM issued_general_codes i JOIN general_code_details d ON d.org_id=i.org_id AND d.id=i.detail_id WHERE d.org_id=$1 AND d.general_code_id=$2)`, actor(r).OrgID, n).Scan(&issued); err != nil {
			return err
		}
		if issued && old.CompanyPrefix != in.CompanyPrefix {
			return fail(409, "Prefix telah digunakan; buat aturan baru")
		}
		if _, err := tx.Exec(r.Context(), `UPDATE general_codes SET name=$1,company_prefix=$2,description=$3,version=version+1 WHERE org_id=$4 AND id=$5`, in.Name, in.CompanyPrefix, in.Description, actor(r).OrgID, n); err != nil {
			return err
		}
		return logAudit(r.Context(), tx, actor(r), "update", "general_code", n, old, in)
	})
	if err != nil {
		return err
	}
	status := 200
	if r.Method == "POST" {
		status = 201
	}
	write(w, status, map[string]any{"id": n})
	return nil
}

type detailInput struct {
	GeneralCodeID int64  `json:"general_code_id"`
	Code          string `json:"code"`
	Name          string `json:"name"`
	Prefix        string `json:"prefix"`
	EntityType    string `json:"entity_type"`
	Width         int    `json:"sequence_width"`
	Version       int    `json:"version"`
}

func (s *Server) saveGeneralDetail(w http.ResponseWriter, r *http.Request) error {
	var in detailInput
	if err := decode(w, r, &in); err != nil {
		return err
	}
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	in.Prefix = strings.ToUpper(strings.TrimSpace(in.Prefix))
	in.Name = strings.TrimSpace(in.Name)
	if !setupKey.MatchString(in.Code) || !setupPrefix.MatchString(in.Prefix) || len(in.Name) < 2 || len(in.Name) > 150 || in.Width < 3 || in.Width > 8 || (in.EntityType != "hq" && in.EntityType != "branch" && in.EntityType != "reference") {
		return fail(422, "Detail kode tidak valid")
	}
	var n int64
	var err error
	if r.Method == "PUT" {
		n, err = id(r)
		if err != nil {
			return err
		}
	}
	err = s.transaction(r, func(tx pgx.Tx) error {
		// All configuration/allocation paths lock parent first, then detail.
		var parent int64
		if err := tx.QueryRow(r.Context(), `SELECT id FROM general_codes WHERE org_id=$1 AND id=$2 AND deleted_at IS NULL FOR UPDATE`, actor(r).OrgID, in.GeneralCodeID).Scan(&parent); err == pgx.ErrNoRows {
			return fail(404, "General Code aktif tidak ditemukan")
		} else if err != nil {
			return err
		}
		if n == 0 {
			if err := tx.QueryRow(r.Context(), `INSERT INTO general_code_details(org_id,general_code_id,code,name,prefix,entity_type,sequence_width) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, actor(r).OrgID, in.GeneralCodeID, in.Code, in.Name, in.Prefix, in.EntityType, in.Width).Scan(&n); err != nil {
				return err
			}
			return logAudit(r.Context(), tx, actor(r), "create", "general_code_detail", n, nil, in)
		}
		var old detailInput
		var next int64
		if err := tx.QueryRow(r.Context(), `SELECT general_code_id,code,name,prefix,entity_type,sequence_width,version,next_number FROM general_code_details WHERE org_id=$1 AND id=$2 AND deleted_at IS NULL FOR UPDATE`, actor(r).OrgID, n).Scan(&old.GeneralCodeID, &old.Code, &old.Name, &old.Prefix, &old.EntityType, &old.Width, &old.Version, &next); err == pgx.ErrNoRows {
			return fail(404, "Detail kode tidak ditemukan")
		} else if err != nil {
			return err
		}
		if old.Version != in.Version {
			return fail(409, "Versi berubah; muat ulang")
		}
		if old.Code != in.Code || old.GeneralCodeID != in.GeneralCodeID {
			return fail(422, "Identitas dan kelompok detail tidak dapat diubah")
		}
		if next > 1 && (old.Prefix != in.Prefix || old.EntityType != in.EntityType || old.Width != in.Width) {
			return fail(409, "Format telah digunakan; buat detail baru")
		}
		if _, err := tx.Exec(r.Context(), `UPDATE general_code_details SET name=$1,prefix=$2,entity_type=$3,sequence_width=$4,version=version+1 WHERE org_id=$5 AND id=$6`, in.Name, in.Prefix, in.EntityType, in.Width, actor(r).OrgID, n); err != nil {
			return err
		}
		return logAudit(r.Context(), tx, actor(r), "update", "general_code_detail", n, old, in)
	})
	if err != nil {
		return err
	}
	status := 200
	if r.Method == "POST" {
		status = 201
	}
	write(w, status, map[string]any{"id": n})
	return nil
}
func (s *Server) archiveGeneralCode(w http.ResponseWriter, r *http.Request) error {
	n, err := id(r)
	if err != nil {
		return err
	}
	detail := strings.Contains(r.URL.Path, "general-code-details")
	restore := r.Method == "POST"
	err = s.transaction(r, func(tx pgx.Tx) error {
		parent := n
		if detail {
			if err := tx.QueryRow(r.Context(), `SELECT general_code_id FROM general_code_details WHERE org_id=$1 AND id=$2`, actor(r).OrgID, n).Scan(&parent); err == pgx.ErrNoRows {
				return fail(404, "Detail tidak ditemukan")
			} else if err != nil {
				return err
			}
		}
		var parentDeleted bool
		if err := tx.QueryRow(r.Context(), `SELECT deleted_at IS NOT NULL FROM general_codes WHERE org_id=$1 AND id=$2 FOR UPDATE`, actor(r).OrgID, parent).Scan(&parentDeleted); err == pgx.ErrNoRows {
			return fail(404, "General Code tidak ditemukan")
		} else if err != nil {
			return err
		}
		if detail && restore && parentDeleted {
			return fail(409, "Pulihkan General Code terlebih dahulu")
		}
		table := "general_codes"
		entity := "general_code"
		if detail {
			table = "general_code_details"
			entity = "general_code_detail"
		}
		if !detail && !restore {
			var active bool
			if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM general_code_details WHERE org_id=$1 AND general_code_id=$2 AND deleted_at IS NULL)`, actor(r).OrgID, n).Scan(&active); err != nil {
				return err
			}
			if active {
				return fail(409, "Arsipkan detail aktif terlebih dahulu")
			}
		}
		cmd, err := tx.Exec(r.Context(), fmt.Sprintf("UPDATE %s SET deleted_at=CASE WHEN $3 THEN NULL ELSE now() END,version=version+1 WHERE org_id=$1 AND id=$2 AND ((deleted_at IS NOT NULL)=$3)", table), actor(r).OrgID, n, restore)
		if err != nil {
			return err
		}
		if cmd.RowsAffected() != 1 {
			return fail(404, "Data tidak ditemukan")
		}
		action := "archive"
		if restore {
			action = "restore"
		}
		return logAudit(r.Context(), tx, actor(r), action, entity, n, map[string]bool{"deleted": restore}, map[string]bool{"deleted": !restore})
	})
	if err != nil {
		return err
	}
	write(w, 200, map[string]bool{"ok": true})
	return nil
}

func allocateGeneralCode(r *http.Request, tx pgx.Tx, detailID, entityID int64, entity string) (string, error) {
	org := actor(r).OrgID
	var parent int64
	if err := tx.QueryRow(r.Context(), `SELECT general_code_id FROM general_code_details WHERE org_id=$1 AND id=$2`, org, detailID).Scan(&parent); err == pgx.ErrNoRows {
		return "", fail(404, "Detail kode tidak ditemukan")
	} else if err != nil {
		return "", err
	}
	var company string
	if err := tx.QueryRow(r.Context(), `SELECT company_prefix FROM general_codes WHERE org_id=$1 AND id=$2 AND deleted_at IS NULL FOR UPDATE`, org, parent).Scan(&company); err == pgx.ErrNoRows {
		return "", fail(409, "General Code tidak aktif")
	} else if err != nil {
		return "", err
	}
	var prefix, kind string
	var width int
	var next int64
	if err := tx.QueryRow(r.Context(), `SELECT prefix,entity_type,sequence_width,next_number FROM general_code_details WHERE org_id=$1 AND id=$2 AND deleted_at IS NULL FOR UPDATE`, org, detailID).Scan(&prefix, &kind, &width, &next); err == pgx.ErrNoRows {
		return "", fail(409, "Detail kode tidak aktif")
	} else if err != nil {
		return "", err
	}
	if kind != entity {
		return "", fail(422, "Jenis detail tidak sesuai cabang/pusat")
	}
	number := fmt.Sprint(next)
	if len(number) > width {
		return "", fail(409, "Nomor urut habis; buat detail baru")
	}
	code := company + "-" + prefix + "-" + strings.Repeat("0", width-len(number)) + number
	if _, err := tx.Exec(r.Context(), `INSERT INTO issued_general_codes(org_id,detail_id,code,entity_type,entity_id) VALUES($1,$2,$3,$4,$5)`, org, detailID, code, entity, entityID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(r.Context(), `UPDATE general_code_details SET next_number=next_number+1,version=version+1 WHERE org_id=$1 AND id=$2`, org, detailID); err != nil {
		return "", err
	}
	return code, nil
}
