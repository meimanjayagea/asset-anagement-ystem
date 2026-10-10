package app

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Server) catalogDetail(w http.ResponseWriter, r *http.Request) error {
	n, err := id(r)
	if err != nil {
		return err
	}
	return s.rows(w, r, `SELECT a.id,a.brand,a.model,a.specifications,a.rfid_tag,a.created_at,a.created_by,a.updated_at,a.updated_by,l.building,l.floor,l.room,(SELECT COALESCE(sum(m.cost),0) FROM maintenance m WHERE m.org_id=a.org_id AND m.asset_id=a.id AND m.status='completed' AND $4::boolean) AS maintenance_total FROM assets a JOIN locations l ON l.org_id=a.org_id AND l.id=a.location_id WHERE a.org_id=$1 AND a.id=$2 AND a.deleted_at IS NULL AND can_access_branch(a.org_id,$3,l.branch_id)`, actor(r).OrgID, n, actor(r).ID, hasCapability(actor(r).Role, "assets.finance"))
}

func (s *Server) updateCatalog(w http.ResponseWriter, r *http.Request) error {
	n, err := id(r)
	if err != nil {
		return err
	}
	var in struct {
		Brand          string `json:"brand"`
		Model          string `json:"model"`
		Specifications string `json:"specifications"`
		RFID           string `json:"rfid_tag"`
		Version        int    `json:"version"`
	}
	if err = decode(w, r, &in); err != nil {
		return err
	}
	in.Brand = strings.TrimSpace(in.Brand)
	in.Model = strings.TrimSpace(in.Model)
	in.RFID = strings.ToUpper(strings.TrimSpace(in.RFID))
	if len(in.Brand) > 100 || len(in.Model) > 100 || len(in.Specifications) > 4000 || len(in.RFID) > 128 {
		return fail(422, "Spesifikasi terlalu panjang")
	}
	err = s.transaction(r, func(tx pgx.Tx) error {
		a, err := lockAsset(r.Context(), tx, actor(r).OrgID, n)
		if err != nil {
			return err
		}
		if a.Version != in.Version {
			return fail(409, "Versi aset berubah; refresh")
		}
		if _, err = tx.Exec(r.Context(), `UPDATE assets SET brand=$1,model=$2,specifications=$3,rfid_tag=$4,version=version+1 WHERE org_id=$5 AND id=$6`, in.Brand, in.Model, in.Specifications, in.RFID, actor(r).OrgID, n); err != nil {
			return err
		}
		return logAudit(r.Context(), tx, actor(r), "specifications", "asset", n, nil, in)
	})
	if err != nil {
		return err
	}
	write(w, 200, map[string]bool{"ok": true})
	return nil
}

func (s *Server) lookupAsset(w http.ResponseWriter, r *http.Request) error {
	code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
	if len(code) < 1 || len(code) > 128 {
		return fail(422, "Kode aset tidak valid")
	}
	var n int64
	var tag string
	err := s.DB.QueryRow(r.Context(), `SELECT a.id,a.tag FROM assets a JOIN locations l ON l.org_id=a.org_id AND l.id=a.location_id WHERE a.org_id=$1 AND (upper(a.tag)=$2 OR a.rfid_tag=$2) AND a.deleted_at IS NULL AND can_access_branch(a.org_id,$3,l.branch_id)`, actor(r).OrgID, code, actor(r).ID).Scan(&n, &tag)
	if errors.Is(err, pgx.ErrNoRows) {
		return fail(404, "Kode aset tidak ditemukan")
	}
	if err != nil {
		return err
	}
	write(w, 200, map[string]any{"id": n, "tag": tag})
	return nil
}

func (s *Server) locationDetails(w http.ResponseWriter, r *http.Request) error {
	n, err := id(r)
	if err != nil {
		return err
	}
	var in struct {
		Building string `json:"building"`
		Floor    string `json:"floor"`
		Room     string `json:"room"`
	}
	if err = decode(w, r, &in); err != nil {
		return err
	}
	if len(in.Building) > 100 || len(in.Floor) > 40 || len(in.Room) > 100 {
		return fail(422, "Detail lokasi terlalu panjang")
	}
	err = s.transaction(r, func(tx pgx.Tx) error {
		if err := locationScope(r, tx, n); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `UPDATE locations SET building=$1,floor=$2,room=$3 WHERE org_id=$4 AND id=$5`, strings.TrimSpace(in.Building), strings.TrimSpace(in.Floor), strings.TrimSpace(in.Room), actor(r).OrgID, n); err != nil {
			return err
		}
		return logAudit(r.Context(), tx, actor(r), "location_details", "location", n, nil, in)
	})
	if err != nil {
		return err
	}
	write(w, 200, map[string]bool{"ok": true})
	return nil
}
