package app

import (
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
)

func (s *Server) listStocktakes(w http.ResponseWriter, r *http.Request) error {
	p, z := page(r)
	return s.rows(w, r, `SELECT s.id,s.title,s.status,l.name AS location_name,s.created_at,s.closed_at,l.branch_id,count(i.asset_id)::int AS expected,count(i.asset_id) FILTER(WHERE i.observed)::int AS observed,count(i.asset_id) FILTER(WHERE i.finding='missing')::int AS missing,count(i.asset_id) FILTER(WHERE i.finding='unverified')::int AS unverified,count(i.asset_id) FILTER(WHERE i.finding='damaged')::int AS damaged,count(i.asset_id) FILTER(WHERE i.finding='relocated')::int AS relocated FROM stocktakes s JOIN locations l ON l.id=s.location_id LEFT JOIN stocktake_items i ON i.stocktake_id=s.id WHERE s.org_id=$1 AND l.deleted_at IS NULL AND can_access_branch(s.org_id,$4,l.branch_id) AND ($5::bigint=0 OR l.branch_id=$5) GROUP BY s.id,l.name,l.branch_id ORDER BY s.id DESC LIMIT $2 OFFSET $3`, actor(r).OrgID, z, (p-1)*z, actor(r).ID, selectedBranch(r))
}
func (s *Server) createStocktake(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Title    string `json:"title"`
		Location int64  `json:"location_id"`
	}
	if e := decode(w, r, &in); e != nil {
		return e
	}
	in.Title = strings.TrimSpace(in.Title)
	if len(in.Title) < 3 || len(in.Title) > 200 {
		return fail(422, "Judul 3–200 karakter")
	}
	var n int64
	e := s.transaction(r, func(tx pgx.Tx) error {
		u := actor(r)
		if e := locationScope(r, tx, in.Location); e != nil {
			return e
		}
		e := tx.QueryRow(r.Context(), `INSERT INTO stocktakes(org_id,location_id,title,created_by) VALUES($1,$2,$3,$4) RETURNING id`, u.OrgID, in.Location, in.Title, u.ID).Scan(&n)
		if e != nil {
			return e
		}
		_, e = tx.Exec(r.Context(), `INSERT INTO stocktake_items(org_id,stocktake_id,asset_id,expected_version,expected_tag) SELECT a.org_id,$1,a.id,a.version,a.tag FROM assets a JOIN locations l ON l.id=a.location_id WHERE a.org_id=$2 AND a.location_id=$3 AND a.deleted_at IS NULL AND l.deleted_at IS NULL AND a.status<>'disposed'`, n, u.OrgID, in.Location)
		if e != nil {
			return e
		}
		return logAudit(r.Context(), tx, u, "open", "stocktake", n, nil, in)
	})
	if e != nil {
		return e
	}
	write(w, 201, map[string]int64{"id": n})
	return nil
}
func (s *Server) stocktakeItems(w http.ResponseWriter, r *http.Request) error {
	n, e := id(r)
	if e != nil {
		return e
	}
	p, z := page(r)
	return s.rows(w, r, `SELECT i.asset_id,i.expected_tag,a.name,(SELECT p.id FROM asset_photos p WHERE p.org_id=a.org_id AND p.asset_id=a.id AND p.purpose='catalog' ORDER BY p.id LIMIT 1) AS cover_photo_id,i.observed,i.notes,i.expected_version,a.version AS current_version,a.location_id AS current_location_id,a.status AS current_status,i.observed_at,i.finding,i.found_location_id,(SELECT jsonb_agg(p.photo_id) FROM asset_photo_links p JOIN asset_photos ap ON ap.id=p.photo_id AND ap.org_id=p.org_id WHERE p.org_id=i.org_id AND p.entity='stocktake_item' AND p.entity_id=i.stocktake_id AND ap.asset_id=i.asset_id) AS photo_ids FROM stocktake_items i JOIN assets a ON a.id=i.asset_id JOIN stocktakes st ON st.id=i.stocktake_id JOIN locations l ON l.id=st.location_id WHERE i.org_id=$1 AND i.stocktake_id=$2 AND a.deleted_at IS NULL AND l.deleted_at IS NULL AND can_access_branch(i.org_id,$5,l.branch_id) ORDER BY i.expected_tag LIMIT $3 OFFSET $4`, actor(r).OrgID, n, z, (p-1)*z, actor(r).ID)
}
func (s *Server) observeStocktake(w http.ResponseWriter, r *http.Request) error {
	n, e := id(r)
	if e != nil {
		return e
	}
	var in struct {
		Tag           string  `json:"tag"`
		Notes         string  `json:"notes"`
		Finding       string  `json:"finding"`
		Photos        []int64 `json:"photo_ids"`
		FoundLocation *int64  `json:"found_location_id"`
	}
	if e = decode(w, r, &in); e != nil {
		return e
	}
	in.Tag = strings.ToUpper(strings.TrimSpace(in.Tag))
	if in.Finding == "" {
		in.Finding = "present"
	}
	if in.Finding != "present" && in.Finding != "missing" && in.Finding != "damaged" && in.Finding != "relocated" {
		return fail(422, "Temuan opname tidak valid")
	}
	if in.Finding == "relocated" && in.FoundLocation == nil {
		return fail(422, "Lokasi temuan wajib")
	}
	if len(in.Tag) < 1 || len(in.Tag) > 80 || len(in.Notes) > 1000 {
		return fail(422, "Tag/catatan tidak valid")
	}
	e = s.transaction(r, func(tx pgx.Tx) error {
		u := actor(r)
		var status string
		e := tx.QueryRow(r.Context(), `SELECT st.status FROM stocktakes st JOIN locations l ON l.id=st.location_id WHERE st.org_id=$1 AND st.id=$2 AND can_access_branch(st.org_id,$3,l.branch_id) FOR UPDATE OF st`, u.OrgID, n, u.ID).Scan(&status)
		if errors.Is(e, pgx.ErrNoRows) {
			return fail(404, "Stocktake tidak ditemukan")
		}
		if e != nil {
			return e
		}
		if status != "open" {
			return fail(409, "Stocktake sudah ditutup")
		}
		var asset int64
		var observed bool
		var finding string
		e = tx.QueryRow(r.Context(), `SELECT asset_id,observed,finding FROM stocktake_items WHERE org_id=$1 AND stocktake_id=$2 AND expected_tag=$3`, u.OrgID, n, in.Tag).Scan(&asset, &observed, &finding)
		if errors.Is(e, pgx.ErrNoRows) {
			return fail(422, "Tag di luar snapshot lokasi; lakukan investigasi terpisah")
		}
		if e != nil {
			return e
		}
		if observed || finding != "unverified" {
			return fail(409, "Tag sudah diobservasi")
		}
		if in.FoundLocation != nil {
			if e = locationScope(r, tx, *in.FoundLocation); e != nil {
				return e
			}
		}
		if e = attachEvidence(r, tx, asset, in.Photos, "audit", "stocktake_item", n, in.Finding != "present"); e != nil {
			return e
		}
		_, e = tx.Exec(r.Context(), `UPDATE stocktake_items SET observed=$1,notes=$2,observed_by=$3,observed_at=now(),finding=$4,found_location_id=$5 WHERE org_id=$6 AND stocktake_id=$7 AND asset_id=$8`, in.Finding != "missing", in.Notes, u.ID, in.Finding, in.FoundLocation, u.OrgID, n, asset)
		if e != nil {
			return e
		}
		return logAudit(r.Context(), tx, u, "observe", "stocktake", n, nil, map[string]any{"asset_id": asset, "tag": in.Tag, "notes": in.Notes})
	})
	if e != nil {
		return e
	}
	write(w, 200, map[string]bool{"ok": true})
	return nil
}
func (s *Server) closeStocktake(w http.ResponseWriter, r *http.Request) error {
	n, e := id(r)
	if e != nil {
		return e
	}
	var in struct {
		Acknowledge bool `json:"acknowledge_discrepancies"`
	}
	if e = decode(w, r, &in); e != nil {
		return e
	}
	e = s.transaction(r, func(tx pgx.Tx) error {
		u := actor(r)
		var status string
		e := tx.QueryRow(r.Context(), `SELECT st.status FROM stocktakes st JOIN locations l ON l.id=st.location_id WHERE st.org_id=$1 AND st.id=$2 AND can_access_branch(st.org_id,$3,l.branch_id) FOR UPDATE OF st`, u.OrgID, n, u.ID).Scan(&status)
		if errors.Is(e, pgx.ErrNoRows) {
			return fail(404, "Stocktake tidak ditemukan")
		}
		if e != nil {
			return e
		}
		if status != "open" {
			return fail(409, "Stocktake sudah ditutup")
		}
		var missing, changed, unverified int
		e = tx.QueryRow(r.Context(), `SELECT count(*) FILTER(WHERE NOT i.observed),count(*) FILTER(WHERE i.expected_version<>a.version OR i.finding IN ('damaged','relocated')),count(*) FILTER(WHERE i.finding='unverified') FROM stocktake_items i JOIN assets a ON a.id=i.asset_id WHERE i.org_id=$1 AND i.stocktake_id=$2 AND a.deleted_at IS NULL`, u.OrgID, n).Scan(&missing, &changed, &unverified)
		if e != nil {
			return e
		}
		if unverified > 0 {
			return fail(409, "Verifikasi seluruh aset dan lampirkan bukti temuan sebelum menutup opname")
		}
		if (missing > 0 || changed > 0) && !in.Acknowledge {
			return fail(409, "Ada discrepancy: konfirmasi missing/changed sebelum close")
		}
		_, e = tx.Exec(r.Context(), `UPDATE stocktakes SET status='closed',closed_at=now() WHERE org_id=$1 AND id=$2`, u.OrgID, n)
		if e != nil {
			return e
		}
		return logAudit(r.Context(), tx, u, "close", "stocktake", n, nil, map[string]any{"missing": missing, "changed": changed, "acknowledged": in.Acknowledge})
	})
	if e != nil {
		return e
	}
	write(w, 200, map[string]bool{"ok": true})
	return nil
}
