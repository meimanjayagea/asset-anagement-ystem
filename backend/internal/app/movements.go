package app

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"
)

func recordAssetMovement(c context.Context, tx pgx.Tx, u User, assetID int64, event string, fromLocation, toLocation, fromBranch, toBranch *int64, fromCustodian, toCustodian, note string) error {
	if fromLocation != nil && fromBranch == nil {
		var branch int64
		if e := tx.QueryRow(c, `SELECT branch_id FROM locations WHERE org_id=$1 AND id=$2`, u.OrgID, *fromLocation).Scan(&branch); e != nil {
			return e
		}
		fromBranch = &branch
	}
	if toLocation != nil && toBranch == nil {
		var branch int64
		if e := tx.QueryRow(c, `SELECT branch_id FROM locations WHERE org_id=$1 AND id=$2`, u.OrgID, *toLocation).Scan(&branch); e != nil {
			return e
		}
		toBranch = &branch
	}
	_, e := tx.Exec(c, `INSERT INTO asset_movements(org_id,asset_id,actor_id,event,from_location_id,to_location_id,from_branch_id,to_branch_id,from_custodian,to_custodian,note) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, u.OrgID, assetID, u.ID, event, fromLocation, toLocation, fromBranch, toBranch, fromCustodian, toCustodian, note)
	return e
}

func (s *Server) assetHistory(w http.ResponseWriter, r *http.Request) error {
	id, e := id(r)
	if e != nil {
		return e
	}
	u := actor(r)
	var accessible bool
	e = s.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM assets a JOIN locations l ON l.org_id=a.org_id AND l.id=a.location_id WHERE a.org_id=$1 AND a.id=$2 AND a.deleted_at IS NULL AND can_access_branch(a.org_id,$3,l.branch_id))`, u.OrgID, id, u.ID).Scan(&accessible)
	if e != nil {
		return e
	}
	if !accessible {
		return fail(404, "Aset tidak ditemukan")
	}
	p, size := page(r)
	return s.rows(w, r, `SELECT m.id,m.event,CASE WHEN can_access_branch(m.org_id,$3,m.from_branch_id) THEN m.from_location_id END AS from_location_id,CASE WHEN can_access_branch(m.org_id,$3,m.from_branch_id) THEN fl.name END AS from_location,CASE WHEN can_access_branch(m.org_id,$3,m.to_branch_id) THEN m.to_location_id END AS to_location_id,CASE WHEN can_access_branch(m.org_id,$3,m.to_branch_id) THEN tl.name END AS to_location,CASE WHEN can_access_branch(m.org_id,$3,m.from_branch_id) THEN m.from_branch_id END AS from_branch_id,CASE WHEN can_access_branch(m.org_id,$3,m.from_branch_id) THEN fb.code END AS from_branch,CASE WHEN can_access_branch(m.org_id,$3,m.to_branch_id) THEN m.to_branch_id END AS to_branch_id,CASE WHEN can_access_branch(m.org_id,$3,m.to_branch_id) THEN tb.code END AS to_branch,CASE WHEN can_access_branch(m.org_id,$3,m.from_branch_id) THEN m.from_custodian END AS from_custodian,CASE WHEN can_access_branch(m.org_id,$3,m.to_branch_id) THEN m.to_custodian END AS to_custodian,m.note,m.occurred_at,u.name AS actor FROM asset_movements m JOIN users u ON u.org_id=m.org_id AND u.id=m.actor_id LEFT JOIN locations fl ON fl.org_id=m.org_id AND fl.id=m.from_location_id LEFT JOIN locations tl ON tl.org_id=m.org_id AND tl.id=m.to_location_id LEFT JOIN branches fb ON fb.org_id=m.org_id AND fb.id=m.from_branch_id LEFT JOIN branches tb ON tb.org_id=m.org_id AND tb.id=m.to_branch_id WHERE m.org_id=$1 AND m.asset_id=$2 AND (can_access_branch(m.org_id,$3,m.from_branch_id) OR can_access_branch(m.org_id,$3,m.to_branch_id)) AND ($4::bigint=0 OR m.from_branch_id=$4 OR m.to_branch_id=$4) ORDER BY m.id DESC LIMIT $5 OFFSET $6`, u.OrgID, id, u.ID, selectedBranch(r), size, (p-1)*size)
}
