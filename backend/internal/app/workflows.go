package app

import (
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
)

func (s *Server) listRequests(w http.ResponseWriter, r *http.Request) error {
	p, z := page(r)
	return s.rows(w, r, `SELECT r.id,r.asset_id,a.tag,a.name,r.kind,r.reason,r.status,r.expected_version,r.target_location_id,l.name AS target_location,r.requested_by,u.name AS requester,r.decision_note,r.created_at,r.source_branch_id,r.target_branch_id,CASE WHEN $6::boolean THEN r.disposal_proceeds ELSE NULL END AS disposal_proceeds FROM requests r JOIN assets a ON a.id=r.asset_id LEFT JOIN locations l ON l.id=r.target_location_id JOIN users u ON u.id=r.requested_by WHERE r.org_id=$1 AND a.deleted_at IS NULL AND can_access_branch(r.org_id,$4,r.source_branch_id) AND ($5::bigint=0 OR r.source_branch_id=$5) ORDER BY r.id DESC LIMIT $2 OFFSET $3`, actor(r).OrgID, z, (p-1)*z, actor(r).ID, selectedBranch(r), hasCapability(actor(r).Role, "assets.finance"))
}
func (s *Server) createRequest(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Asset    int64  `json:"asset_id"`
		Kind     string `json:"kind"`
		Target   *int64 `json:"target_location_id"`
		Reason   string `json:"reason"`
		Proceeds int64  `json:"disposal_proceeds"`
		Version  int    `json:"version"`
	}
	if e := decode(w, r, &in); e != nil {
		return e
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if len(in.Reason) < 3 || len(in.Reason) > 1000 || in.Proceeds < 0 || in.Proceeds > 1_000_000_000_000_000 || (in.Kind != "transfer" && in.Kind != "dispose") || (in.Kind == "transfer" && (in.Target == nil || in.Proceeds != 0)) || (in.Kind == "dispose" && in.Target != nil) {
		return fail(422, "Request tidak valid")
	}
	if in.Kind == "dispose" && in.Proceeds > 0 && !hasCapability(actor(r).Role, "assets.finance") {
		return fail(403, "Nilai hasil disposal hanya dapat dicatat oleh role finance")
	}
	var n int64
	e := s.transaction(r, func(tx pgx.Tx) error {
		a, e := lockAsset(r.Context(), tx, actor(r).OrgID, in.Asset)
		if e != nil {
			return e
		}
		if a.Version != in.Version {
			return fail(409, "Versi aset berubah")
		}
		if !allowed(a.Status, in.Kind) {
			return fail(409, "Aset harus available")
		}
		if in.Target != nil {
			if e := locationScope(r, tx, *in.Target); e != nil {
				return e
			}
		}
		if in.Target != nil && *in.Target == a.Location {
			return fail(422, "Lokasi tujuan sama")
		}
		var active bool
		e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM maintenance WHERE org_id=$1 AND asset_id=$2 AND status IN ('scheduled','in_progress'))`, actor(r).OrgID, in.Asset).Scan(&active)
		if e != nil {
			return e
		}
		if active {
			return fail(409, "Maintenance aktif")
		}
		e = tx.QueryRow(r.Context(), `INSERT INTO requests(org_id,asset_id,kind,target_location_id,reason,expected_version,requested_by,source_branch_id,target_branch_id,disposal_proceeds) VALUES($1,$2,$3,$4,$5,$6,$7,$8,(SELECT branch_id FROM locations WHERE org_id=$1 AND id=$4),$9) RETURNING id`, actor(r).OrgID, in.Asset, in.Kind, in.Target, in.Reason, in.Version, actor(r).ID, a.BranchID, in.Proceeds).Scan(&n)
		if e != nil {
			return e
		}
		return logAudit(r.Context(), tx, actor(r), "request", "request", n, nil, in)
	})
	if e != nil {
		return e
	}
	write(w, 201, map[string]int64{"id": n})
	return nil
}
func (s *Server) decideRequest(w http.ResponseWriter, r *http.Request) error {
	n, e := id(r)
	if e != nil {
		return e
	}
	var in struct {
		Approve bool   `json:"approve"`
		Note    string `json:"note"`
	}
	if e = decode(w, r, &in); e != nil {
		return e
	}
	if len(in.Note) > 1000 || (!in.Approve && len(strings.TrimSpace(in.Note)) < 3) {
		return fail(422, "Alasan penolakan wajib; maksimal 1000 karakter")
	}
	e = s.transaction(r, func(tx pgx.Tx) error {
		u := actor(r)
		var asset, requester int64
		var kind, status string
		var target *int64
		var version int
		var proceeds int64
		// Discover asset, then take asset lock before request lock in every write path.
		e := tx.QueryRow(r.Context(), `SELECT asset_id FROM requests WHERE org_id=$1 AND id=$2`, u.OrgID, n).Scan(&asset)
		if errors.Is(e, pgx.ErrNoRows) {
			return fail(404, "Request tidak ditemukan")
		}
		if e != nil {
			return e
		}
		a, e := lockAsset(r.Context(), tx, u.OrgID, asset)
		if e != nil {
			return e
		}
		e = tx.QueryRow(r.Context(), `SELECT kind,status,target_location_id,expected_version,requested_by,disposal_proceeds FROM requests WHERE org_id=$1 AND id=$2 FOR UPDATE`, u.OrgID, n).Scan(&kind, &status, &target, &version, &requester, &proceeds)
		if e != nil {
			return e
		}
		if status != "pending" {
			return fail(409, "Request sudah diputuskan")
		}
		if requester == u.ID {
			return fail(403, "Pembuat request tidak boleh menyetujui atau menolak sendiri")
		}
		if target != nil {
			if e := locationScope(r, tx, *target); e != nil {
				return e
			}
		}
		result := "rejected"
		if in.Approve {
			if a.Version != version || !allowed(a.Status, kind) {
				return fail(409, "Aset berubah; tolak request dan buat ulang")
			}
			result = "approved"
			if kind == "transfer" {
				_, e = tx.Exec(r.Context(), `UPDATE assets SET location_id=$1,version=version+1,updated_at=now() WHERE org_id=$2 AND id=$3`, target, u.OrgID, asset)
				if e == nil {
					var targetBranch int64
					if e = tx.QueryRow(r.Context(), `SELECT branch_id FROM locations WHERE org_id=$1 AND id=$2`, u.OrgID, *target).Scan(&targetBranch); e == nil {
						fromLocation, toLocation := a.Location, *target
						fromBranch, toBranch := a.BranchID, targetBranch
						e = recordAssetMovement(r.Context(), tx, u, asset, "transferred", &fromLocation, &toLocation, &fromBranch, &toBranch, a.Custodian, a.Custodian, in.Note)
					}
				}
			} else {
				today := time.Now().In(time.FixedZone("WIB", 7*3600))
				book, gross, accumulated, calcErr := assetBookValueAt(r.Context(), tx, u.OrgID, a, today)
				if calcErr != nil {
					return calcErr
				}
				_, e = tx.Exec(r.Context(), `UPDATE assets SET status='disposed',disposed_at=now(),disposal_proceeds=$1,disposal_book_value=$2,disposal_gross_value=$3,disposal_accumulated_depreciation=$4,next_maintenance_date=NULL,version=version+1,updated_at=now() WHERE org_id=$5 AND id=$6`, proceeds, book, gross, accumulated, u.OrgID, asset)
				if e == nil {
					fromLocation, fromBranch := a.Location, a.BranchID
					e = recordAssetMovement(r.Context(), tx, u, asset, "disposed", &fromLocation, nil, &fromBranch, nil, a.Custodian, "", in.Note)
				}
			}
			if e != nil {
				return e
			}
			if e = logAudit(r.Context(), tx, u, kind, "asset", asset, a, map[string]any{"request_id": n, "target_location_id": target, "version": a.Version + 1}); e != nil {
				return e
			}
		}
		_, e = tx.Exec(r.Context(), `UPDATE requests SET status=$1,decided_by=$2,decision_note=$3,decided_at=now() WHERE org_id=$4 AND id=$5`, result, u.ID, in.Note, u.OrgID, n)
		if e != nil {
			return e
		}
		return logAudit(r.Context(), tx, u, result, "request", n, map[string]string{"status": status}, in)
	})
	if e != nil {
		return e
	}
	write(w, 200, map[string]bool{"ok": true})
	return nil
}
func (s *Server) listMaintenance(w http.ResponseWriter, r *http.Request) error {
	p, z := page(r)
	u := actor(r)
	return s.rows(w, r, `SELECT m.id,m.asset_id,a.tag,a.name,m.title,m.due_date::text,m.status,CASE WHEN $6::boolean THEN m.cost ELSE NULL END AS cost,m.notes,m.completed_at,a.version AS asset_version,m.branch_id,b.name AS branch_name,c.maintenance_instructions FROM maintenance m JOIN assets a ON a.id=m.asset_id JOIN branches b ON b.id=m.branch_id JOIN categories c ON c.id=a.category_id WHERE m.org_id=$1 AND a.deleted_at IS NULL AND b.deleted_at IS NULL AND c.deleted_at IS NULL AND can_access_branch(m.org_id,$4,m.branch_id) AND ($5::bigint=0 OR m.branch_id=$5) ORDER BY m.id DESC LIMIT $2 OFFSET $3`, u.OrgID, z, (p-1)*z, u.ID, selectedBranch(r), hasCapability(u.Role, "assets.finance"))
}
func (s *Server) createMaintenance(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Asset   int64  `json:"asset_id"`
		Title   string `json:"title"`
		Due     string `json:"due_date"`
		Version int    `json:"version"`
	}
	if e := decode(w, r, &in); e != nil {
		return e
	}
	in.Title = strings.TrimSpace(in.Title)
	if _, e := time.Parse("2006-01-02", in.Due); e != nil || len(in.Title) < 3 || len(in.Title) > 200 {
		return fail(422, "Judul atau due date tidak valid")
	}
	var n int64
	e := s.transaction(r, func(tx pgx.Tx) error {
		a, e := lockAsset(r.Context(), tx, actor(r).OrgID, in.Asset)
		if e != nil {
			return e
		}
		if a.Version != in.Version || a.Status != "available" {
			return fail(409, "Versi berubah atau aset belum available")
		}
		var pending bool
		e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM requests WHERE org_id=$1 AND asset_id=$2 AND status='pending')`, actor(r).OrgID, in.Asset).Scan(&pending)
		if e != nil {
			return e
		}
		if pending {
			return fail(409, "Request pending")
		}
		e = tx.QueryRow(r.Context(), `INSERT INTO maintenance(org_id,asset_id,title,due_date,branch_id) VALUES($1,$2,$3,$4::text::date,$5) RETURNING id`, actor(r).OrgID, in.Asset, in.Title, in.Due, a.BranchID).Scan(&n)
		if e != nil {
			return e
		}
		return logAudit(r.Context(), tx, actor(r), "schedule", "maintenance", n, nil, in)
	})
	if e != nil {
		return e
	}
	write(w, 201, map[string]int64{"id": n})
	return nil
}
func (s *Server) maintenanceAction(w http.ResponseWriter, r *http.Request) error {
	n, e := id(r)
	if e != nil {
		return e
	}
	var in struct {
		Action  string `json:"action"`
		Cost    int64  `json:"cost"`
		Notes   string `json:"notes"`
		Version int    `json:"version"`
	}
	if e = decode(w, r, &in); e != nil {
		return e
	}
	if len(in.Notes) > 2000 || in.Cost < 0 || in.Cost > 1_000_000_000_000_000 {
		return fail(422, "Biaya atau catatan tidak valid")
	}
	e = s.transaction(r, func(tx pgx.Tx) error {
		u := actor(r)
		var asset int64
		var status string
		e := tx.QueryRow(r.Context(), `SELECT asset_id FROM maintenance WHERE org_id=$1 AND id=$2`, u.OrgID, n).Scan(&asset)
		if errors.Is(e, pgx.ErrNoRows) {
			return fail(404, "Maintenance tidak ditemukan")
		}
		if e != nil {
			return e
		}
		a, e := lockAsset(r.Context(), tx, u.OrgID, asset)
		if e != nil {
			return e
		}
		var jobBranch int64
		if e := tx.QueryRow(r.Context(), `SELECT branch_id FROM maintenance WHERE org_id=$1 AND id=$2`, u.OrgID, n).Scan(&jobBranch); e != nil {
			return e
		}
		if e := branchScope(r, tx, jobBranch); e != nil {
			return e
		}
		if a.Version != in.Version {
			return fail(409, "Versi berubah")
		}
		e = tx.QueryRow(r.Context(), `SELECT status FROM maintenance WHERE org_id=$1 AND id=$2 FOR UPDATE`, u.OrgID, n).Scan(&status)
		if e != nil {
			return e
		}
		next := ""
		assetStatus := a.Status
		switch in.Action {
		case "start":
			if status == "scheduled" && allowed(a.Status, "maintenance_start") {
				next = "in_progress"
				assetStatus = "maintenance"
			}
		case "complete":
			if status == "in_progress" && allowed(a.Status, "maintenance_complete") {
				next = "completed"
				assetStatus = "available"
			}
		case "cancel":
			if status == "scheduled" {
				next = "cancelled"
			}
		default:
			return fail(422, "Action tidak valid")
		}
		if next == "" {
			return fail(409, "Transisi maintenance tidak valid")
		}
		_, e = tx.Exec(r.Context(), `UPDATE maintenance SET status=$1,cost=$2,notes=$3,completed_at=CASE WHEN $1='completed' THEN now() ELSE NULL END WHERE org_id=$4 AND id=$5`, next, in.Cost, in.Notes, u.OrgID, n)
		if e != nil {
			return e
		}
		_, e = tx.Exec(r.Context(), `UPDATE assets SET status=$1,version=version+1,updated_at=now() WHERE org_id=$2 AND id=$3`, assetStatus, u.OrgID, asset)
		if e != nil {
			return e
		}
		if e = logAudit(r.Context(), tx, u, in.Action, "maintenance", n, map[string]string{"status": status}, in); e != nil {
			return e
		}
		if in.Action == "complete" {
			_, e = tx.Exec(r.Context(), `UPDATE assets a SET next_maintenance_date=CASE WHEN c.maintenance_interval_days>0 THEN CURRENT_DATE+c.maintenance_interval_days ELSE NULL END FROM categories c WHERE a.org_id=$1 AND a.id=$2 AND c.id=a.category_id`, u.OrgID, asset)
			if e != nil {
				return e
			}
		}
		after, e := lockAsset(r.Context(), tx, u.OrgID, asset)
		if e != nil {
			return e
		}
		return logAudit(r.Context(), tx, u, "maintenance_"+in.Action, "asset", asset, a, after)
	})
	if e != nil {
		return e
	}
	write(w, 200, map[string]bool{"ok": true})
	return nil
}

func (s *Server) auditList(w http.ResponseWriter, r *http.Request) error {
	p, z := page(r)
	u := actor(r)
	return s.rows(w, r, `SELECT a.id,a.actor_id,u.name AS actor,a.action,a.entity,a.entity_id,a.before_data,a.after_data,a.created_at,a.branch_id,a.related_branch_id,a.request_id,a.peer_ip,a.user_agent FROM audit_logs a JOIN users u ON u.id=a.actor_id WHERE a.org_id=$1 AND ($4 OR can_access_branch(a.org_id,$5,a.branch_id) OR can_access_branch(a.org_id,$5,a.related_branch_id)) AND ($6::bigint=0 OR a.branch_id=$6 OR a.related_branch_id=$6) ORDER BY a.id DESC LIMIT $2 OFFSET $3`, u.OrgID, z, (p-1)*z, u.AllBranches, u.ID, selectedBranch(r))
}
