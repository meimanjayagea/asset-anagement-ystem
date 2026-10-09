package app

import (
	"github.com/jackc/pgx/v5"
	"net/http"
)

func (s *Server) categoryPolicy(w http.ResponseWriter, r *http.Request) error {
	n, e := id(r)
	if e != nil {
		return e
	}
	var in struct {
		Interval     int    `json:"maintenance_interval_days"`
		Instructions string `json:"maintenance_instructions"`
		Version      int    `json:"version"`
		Apply        bool   `json:"apply_to_existing"`
	}
	if e = decode(w, r, &in); e != nil {
		return e
	}
	if in.Interval < 0 || in.Interval > 3650 || len(in.Instructions) > 2000 {
		return fail(422, "Interval 0–3650 hari, instruksi maksimal 2000 karakter")
	}
	var affected int64
	e = s.transaction(r, func(tx pgx.Tx) error {
		u := actor(r)
		var interval, version int
		var instructions string
		e := tx.QueryRow(r.Context(), `SELECT maintenance_interval_days,maintenance_instructions,version FROM categories WHERE org_id=$1 AND id=$2 FOR UPDATE`, u.OrgID, n).Scan(&interval, &instructions, &version)
		if e == pgx.ErrNoRows {
			return fail(404, "Kategori tidak ditemukan")
		}
		if e != nil {
			return e
		}
		if version != in.Version {
			return fail(409, "Versi kategori berubah")
		}
		_, e = tx.Exec(r.Context(), `UPDATE categories SET maintenance_interval_days=$1,maintenance_instructions=$2,version=version+1 WHERE org_id=$3 AND id=$4`, in.Interval, in.Instructions, u.OrgID, n)
		if e != nil {
			return e
		}
		if in.Apply {
			cmd, e := tx.Exec(r.Context(), `UPDATE assets a SET next_maintenance_date=CASE WHEN $1::integer=0 THEN NULL ELSE coalesce((SELECT max(completed_at)::date FROM maintenance m WHERE m.org_id=a.org_id AND m.asset_id=a.id AND m.status='completed'),a.purchase_date)+$1 END,version=version+1,updated_at=now() WHERE a.org_id=$2 AND a.category_id=$3 AND a.status<>'disposed' AND NOT EXISTS(SELECT 1 FROM requests r WHERE r.org_id=a.org_id AND r.asset_id=a.id AND r.status='pending') AND NOT EXISTS(SELECT 1 FROM maintenance m WHERE m.org_id=a.org_id AND m.asset_id=a.id AND m.status IN ('scheduled','in_progress'))`, in.Interval, u.OrgID, n)
			if e != nil {
				return e
			}
			affected = cmd.RowsAffected()
		}
		return logAudit(r.Context(), tx, u, "maintenance_policy", "category", n, map[string]any{"interval_days": interval, "instructions": instructions, "version": version}, map[string]any{"interval_days": in.Interval, "instructions": in.Instructions, "version": version + 1, "apply_to_existing": in.Apply, "assets_updated": affected})
	})
	if e != nil {
		return e
	}
	write(w, 200, map[string]any{"ok": true, "assets_updated": affected})
	return nil
}
