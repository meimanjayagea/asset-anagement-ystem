package app

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Server) listLoans(w http.ResponseWriter, r *http.Request) error {
	p, z := page(r)
	u := actor(r)
	return s.rows(w, r, `SELECT x.id,x.asset_id,a.tag,a.name,x.borrower_id,u.name AS borrower,x.status,x.due_at,x.checked_out_at,x.returned_at,x.reason,x.condition_before,x.condition_after,x.return_condition,x.decision_note,a.version AS asset_version,x.created_at,(SELECT jsonb_agg(p.photo_id) FROM asset_photo_links p WHERE p.org_id=x.org_id AND p.entity='loan_before' AND p.entity_id=x.id) AS before_photo_ids,(SELECT jsonb_agg(p.photo_id) FROM asset_photo_links p WHERE p.org_id=x.org_id AND p.entity='loan_after' AND p.entity_id=x.id) AS after_photo_ids FROM asset_loans x JOIN assets a ON a.org_id=x.org_id AND a.id=x.asset_id JOIN users u ON u.org_id=x.org_id AND u.id=x.borrower_id WHERE x.org_id=$1 AND a.deleted_at IS NULL AND can_access_branch(x.org_id,$2,x.branch_id) AND ($3::bigint=0 OR x.branch_id=$3) AND ($4::boolean OR x.borrower_id=$2) ORDER BY x.id DESC LIMIT $5 OFFSET $6`, u.OrgID, u.ID, selectedBranch(r), hasCapability(u.Role, "loans.manage") || hasCapability(u.Role, "audit.read"), z, (p-1)*z)
}

func (s *Server) requestLoan(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Asset   int64  `json:"asset_id"`
		Due     string `json:"due_at"`
		Reason  string `json:"reason"`
		Version int    `json:"version"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	due, err := time.Parse(time.RFC3339, in.Due)
	in.Reason = strings.TrimSpace(in.Reason)
	if err != nil || !due.After(time.Now()) || due.After(time.Now().AddDate(1, 0, 0)) || len(in.Reason) < 3 || len(in.Reason) > 1000 {
		return fail(422, "Tanggal pengembalian harus di masa depan maksimal 1 tahun dan alasan wajib")
	}
	var n int64
	err = s.transaction(r, func(tx pgx.Tx) error {
		u := actor(r)
		a, err := lockAsset(r.Context(), tx, u.OrgID, in.Asset)
		if err != nil {
			return err
		}
		if a.Status != "available" || a.Version != in.Version {
			return fail(409, "Aset tidak tersedia atau versinya berubah")
		}
		var active bool
		if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM requests WHERE org_id=$1 AND asset_id=$2 AND status='pending') OR EXISTS(SELECT 1 FROM maintenance WHERE org_id=$1 AND asset_id=$2 AND status IN ('scheduled','in_progress'))`, u.OrgID, a.ID).Scan(&active); err != nil {
			return err
		}
		if active {
			return fail(409, "Selesaikan proses aktif terlebih dahulu")
		}
		if err = tx.QueryRow(r.Context(), `INSERT INTO asset_loans(org_id,asset_id,branch_id,borrower_id,due_at,reason,expected_version) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, u.OrgID, a.ID, a.BranchID, u.ID, due, in.Reason, a.Version).Scan(&n); err != nil {
			return err
		}
		return logAudit(r.Context(), tx, u, "loan_request", "asset", a.ID, nil, map[string]any{"loan_id": n, "due_at": in.Due, "reason": in.Reason})
	})
	if err != nil {
		return err
	}
	write(w, 201, map[string]int64{"id": n})
	return nil
}

func (s *Server) loanAction(w http.ResponseWriter, r *http.Request) error {
	n, err := id(r)
	if err != nil {
		return err
	}
	var in struct {
		Action    string  `json:"action"`
		Notes     string  `json:"notes"`
		Condition string  `json:"condition"`
		Photos    []int64 `json:"photo_ids"`
		Version   int     `json:"version"`
	}
	if err = decode(w, r, &in); err != nil {
		return err
	}
	in.Notes = strings.TrimSpace(in.Notes)
	if len(in.Notes) > 1000 {
		return fail(422, "Catatan terlalu panjang")
	}
	err = s.transaction(r, func(tx pgx.Tx) error {
		u := actor(r)
		var asset int64
		if err := tx.QueryRow(r.Context(), `SELECT asset_id FROM asset_loans WHERE org_id=$1 AND id=$2`, u.OrgID, n).Scan(&asset); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fail(404, "Peminjaman tidak ditemukan")
			}
			return err
		}
		a, err := lockAsset(r.Context(), tx, u.OrgID, asset)
		if err != nil {
			return err
		}
		var status string
		var borrower int64
		var expected int
		var due time.Time
		if err = tx.QueryRow(r.Context(), `SELECT status,borrower_id,expected_version,due_at FROM asset_loans WHERE org_id=$1 AND id=$2 FOR UPDATE`, u.OrgID, n).Scan(&status, &borrower, &expected, &due); err != nil {
			return err
		}
		manage := hasCapability(u.Role, "loans.manage")
		switch in.Action {
		case "cancel", "reject":
			if status != "pending" {
				return fail(409, "Peminjaman tidak pending")
			}
			if (in.Action == "cancel" && borrower != u.ID && !manage) || (in.Action == "reject" && !manage) {
				return fail(403, "Tidak diizinkan")
			}
			if len(in.Notes) < 3 {
				return fail(422, "Alasan wajib")
			}
			next := "cancelled"
			if in.Action == "reject" {
				next = "rejected"
			}
			if _, err = tx.Exec(r.Context(), `UPDATE asset_loans SET status=$1,decision_note=$2 WHERE org_id=$3 AND id=$4`, next, in.Notes, u.OrgID, n); err != nil {
				return err
			}
		case "checkout":
			if !manage || borrower == u.ID {
				return fail(403, "Penyerahan harus dilakukan petugas selain peminjam")
			}
			if status != "pending" || a.Status != "available" || a.Version != expected || a.Version != in.Version || !due.After(time.Now()) {
				return fail(409, "Aset atau batas waktu pinjaman berubah")
			}
			if len(in.Notes) < 3 {
				return fail(422, "Catatan kondisi awal wajib")
			}
			if err = attachEvidence(r, tx, asset, in.Photos, "checkout", "loan_before", n, true); err != nil {
				return err
			}
			var employee string
			if err = tx.QueryRow(r.Context(), `SELECT employee_id FROM users WHERE org_id=$1 AND id=$2 AND active AND deleted_at IS NULL AND can_access_branch(org_id,id,$3)`, u.OrgID, borrower, a.BranchID).Scan(&employee); err != nil {
				return fail(422, "Peminjam tidak aktif")
			}
			if _, err = tx.Exec(r.Context(), `UPDATE assets SET status='assigned',custodian=$1,version=version+1 WHERE org_id=$2 AND id=$3`, employee, u.OrgID, asset); err != nil {
				return err
			}
			if _, err = tx.Exec(r.Context(), `UPDATE asset_loans SET status='checked_out',checked_out_at=now(),handed_over_by=$1,condition_before=$2 WHERE org_id=$3 AND id=$4`, u.ID, in.Notes, u.OrgID, n); err != nil {
				return err
			}
			if err = recordAssetMovement(r.Context(), tx, u, asset, "assigned", &a.Location, &a.Location, &a.BranchID, &a.BranchID, a.Custodian, employee, in.Notes); err != nil {
				return err
			}
		case "return":
			if !manage || borrower == u.ID {
				return fail(403, "Penerimaan harus dilakukan petugas")
			}
			if status != "checked_out" || a.Status != "assigned" || a.Version != in.Version {
				return fail(409, "Pinjaman tidak aktif atau aset berubah")
			}
			if (in.Condition != "good" && in.Condition != "damaged") || len(in.Notes) < 3 {
				return fail(422, "Kondisi dan catatan akhir wajib")
			}
			if err = attachEvidence(r, tx, asset, in.Photos, "return", "loan_after", n, true); err != nil {
				return err
			}
			next := "available"
			if in.Condition == "damaged" {
				next = "maintenance"
			}
			if _, err = tx.Exec(r.Context(), `UPDATE assets SET status=$1,custodian='',version=version+1 WHERE org_id=$2 AND id=$3`, next, u.OrgID, asset); err != nil {
				return err
			}
			if _, err = tx.Exec(r.Context(), `UPDATE asset_loans SET status='returned',returned_at=now(),received_by=$1,condition_after=$2,return_condition=$3 WHERE org_id=$4 AND id=$5`, u.ID, in.Notes, in.Condition, u.OrgID, n); err != nil {
				return err
			}
			if err = recordAssetMovement(r.Context(), tx, u, asset, "returned", &a.Location, &a.Location, &a.BranchID, &a.BranchID, a.Custodian, "", in.Notes); err != nil {
				return err
			}
		default:
			return fail(422, "Aksi pinjaman tidak valid")
		}
		return logAudit(r.Context(), tx, u, "loan_"+in.Action, "asset", asset, map[string]any{"loan_id": n, "status": status}, map[string]any{"loan_id": n, "condition": in.Condition, "notes": in.Notes, "photo_ids": in.Photos})
	})
	if err != nil {
		return err
	}
	write(w, 200, map[string]bool{"ok": true})
	return nil
}
