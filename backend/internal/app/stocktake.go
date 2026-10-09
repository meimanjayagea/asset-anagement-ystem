package app

import (
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
)

func (s *Server) listStocktakes(w http.ResponseWriter, r *http.Request) error {
	p, z := page(r)
	return s.rows(w, r, `SELECT s.id,s.title,s.status,l.name AS location_name,s.created_at,s.closed_at,l.branch_id,count(i.asset_id)::int AS expected,count(i.asset_id) FILTER(WHERE i.observed)::int AS observed,count(i.asset_id) FILTER(WHERE NOT i.observed)::int AS missing FROM stocktakes s JOIN locations l ON l.id=s.location_id LEFT JOIN stocktake_items i ON i.stocktake_id=s.id WHERE s.org_id=$1 AND can_access_branch(s.org_id,$4,l.branch_id) AND ($5::bigint=0 OR l.branch_id=$5) GROUP BY s.id,l.name,l.branch_id ORDER BY s.id DESC LIMIT $2 OFFSET $3`, actor(r).OrgID, z, (p-1)*z, actor(r).ID, selectedBranch(r))
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
		_, e = tx.Exec(r.Context(), `INSERT INTO stocktake_items(org_id,stocktake_id,asset_id,expected_version,expected_tag) SELECT org_id,$1,id,version,tag FROM assets WHERE org_id=$2 AND location_id=$3 AND status<>'disposed'`, n, u.OrgID, in.Location)
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
	return s.rows(w, r, `SELECT i.asset_id,i.expected_tag,a.name,i.observed,i.notes,i.expected_version,a.version AS current_version,a.location_id AS current_location_id,a.status AS current_status,i.observed_at FROM stocktake_items i JOIN assets a ON a.id=i.asset_id JOIN stocktakes st ON st.id=i.stocktake_id JOIN locations l ON l.id=st.location_id WHERE i.org_id=$1 AND i.stocktake_id=$2 AND can_access_branch(i.org_id,$5,l.branch_id) ORDER BY i.expected_tag LIMIT $3 OFFSET $4`, actor(r).OrgID, n, z, (p-1)*z, actor(r).ID)
}
func (s *Server) observeStocktake(w http.ResponseWriter, r *http.Request) error {
	n, e := id(r)
	if e != nil {
		return e
	}
	var in struct {
		Tag   string `json:"tag"`
		Notes string `json:"notes"`
	}
	if e = decode(w, r, &in); e != nil {
		return e
	}
	in.Tag = strings.ToUpper(strings.TrimSpace(in.Tag))
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
		e = tx.QueryRow(r.Context(), `SELECT asset_id,observed FROM stocktake_items WHERE org_id=$1 AND stocktake_id=$2 AND expected_tag=$3`, u.OrgID, n, in.Tag).Scan(&asset, &observed)
		if errors.Is(e, pgx.ErrNoRows) {
			return fail(422, "Tag di luar snapshot lokasi; lakukan investigasi terpisah")
		}
		if e != nil {
			return e
		}
		if observed {
			return fail(409, "Tag sudah diobservasi")
		}
		_, e = tx.Exec(r.Context(), `UPDATE stocktake_items SET observed=true,notes=$1,observed_by=$2,observed_at=now() WHERE org_id=$3 AND stocktake_id=$4 AND asset_id=$5`, in.Notes, u.ID, u.OrgID, n, asset)
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
		var missing, changed int
		e = tx.QueryRow(r.Context(), `SELECT count(*) FILTER(WHERE NOT i.observed),count(*) FILTER(WHERE i.expected_version<>a.version) FROM stocktake_items i JOIN assets a ON a.id=i.asset_id WHERE i.org_id=$1 AND i.stocktake_id=$2`, u.OrgID, n).Scan(&missing, &changed)
		if e != nil {
			return e
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
