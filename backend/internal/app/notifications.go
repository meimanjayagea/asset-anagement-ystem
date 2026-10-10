package app

import (
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Server) notifications(w http.ResponseWriter, r *http.Request) error {
	u := actor(r)
	return s.rows(w, r, `WITH reminders AS (
 SELECT 'maintenance:'||a.id||':'||a.next_maintenance_date AS key,'maintenance' AS kind,a.id AS asset_id,a.tag,a.name,a.next_maintenance_date::timestamptz AS due_at,l.branch_id FROM assets a JOIN locations l ON l.org_id=a.org_id AND l.id=a.location_id WHERE a.org_id=$1 AND a.deleted_at IS NULL AND a.status<>'disposed' AND a.next_maintenance_date<=CURRENT_DATE+7 AND can_access_branch(a.org_id,$2,l.branch_id)
 UNION ALL
 SELECT 'loan:'||x.id||':'||x.due_at::text,'loan',a.id,a.tag,a.name,x.due_at,x.branch_id FROM asset_loans x JOIN assets a ON a.org_id=x.org_id AND a.id=x.asset_id WHERE x.org_id=$1 AND x.status='checked_out' AND x.due_at<=now()+interval '3 days' AND can_access_branch(x.org_id,$2,x.branch_id) AND ($4::boolean OR x.borrower_id=$2)
 UNION ALL
 SELECT 'work:'||m.id||':'||m.due_date::text,'work_order',a.id,a.tag,a.name,m.due_date::timestamptz,m.branch_id FROM maintenance m JOIN assets a ON a.org_id=m.org_id AND a.id=m.asset_id WHERE m.org_id=$1 AND m.status IN ('scheduled','in_progress') AND m.due_date<=CURRENT_DATE+7 AND can_access_branch(m.org_id,$2,m.branch_id)
 ) SELECT x.*,x.due_at<now() AS overdue,(n.notification_key IS NOT NULL) AS read FROM reminders x LEFT JOIN notification_receipts n ON n.org_id=$1 AND n.user_id=$2 AND n.notification_key=x.key WHERE ($3::bigint=0 OR x.branch_id=$3) ORDER BY x.due_at LIMIT 100`, u.OrgID, u.ID, selectedBranch(r), hasCapability(u.Role, "loans.manage"))
}

func (s *Server) readNotification(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Key string `json:"key"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if len(in.Key) < 1 || len(in.Key) > 200 || (!strings.HasPrefix(in.Key, "loan:") && !strings.HasPrefix(in.Key, "maintenance:") && !strings.HasPrefix(in.Key, "work:")) {
		return fail(422, "Notifikasi tidak valid")
	}
	err := s.transaction(r, func(tx pgx.Tx) error {
		_, err := tx.Exec(r.Context(), `INSERT INTO notification_receipts(org_id,user_id,notification_key) VALUES($1,$2,$3) ON CONFLICT(user_id,notification_key) DO UPDATE SET read_at=now()`, actor(r).OrgID, actor(r).ID, in.Key)
		return err
	})
	if err != nil {
		return err
	}
	write(w, 200, map[string]bool{"ok": true})
	return nil
}

func (s *Server) lifecycleReport(w http.ResponseWriter, r *http.Request) error {
	u := actor(r)
	rows, err := s.DB.Query(r.Context(), `SELECT a.id,a.tag,a.name,a.brand,a.model,a.status,l.name AS location,l.building,l.floor,l.room,a.custodian,a.purchase_date,a.warranty_until,CASE WHEN $4::boolean THEN a.purchase_cost END AS purchase_cost,CASE WHEN $4::boolean THEN COALESCE((SELECT sum(cost) FROM maintenance WHERE org_id=a.org_id AND asset_id=a.id AND status='completed'),0) END AS maintenance_cost,(SELECT count(*) FROM maintenance WHERE org_id=a.org_id AND asset_id=a.id AND status='completed') AS services,(SELECT count(*) FROM asset_photos WHERE org_id=a.org_id AND asset_id=a.id) AS photos,(SELECT count(*) FROM asset_loans WHERE org_id=a.org_id AND asset_id=a.id AND status='checked_out') AS active_loans,a.created_at,a.created_by,a.updated_at,a.updated_by FROM assets a JOIN locations l ON l.org_id=a.org_id AND l.id=a.location_id WHERE a.org_id=$1 AND a.deleted_at IS NULL AND l.deleted_at IS NULL AND can_access_branch(a.org_id,$2,l.branch_id) AND ($3::bigint=0 OR l.branch_id=$3) ORDER BY a.tag LIMIT 5001`, u.OrgID, u.ID, selectedBranch(r), hasCapability(u.Role, "assets.finance"))
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		if len(out) >= 5000 {
			return fail(413, "Laporan melebihi 5000 aset; persempit cabang")
		}
		values, err := rows.Values()
		if err != nil {
			return err
		}
		record := map[string]any{}
		for index, field := range rows.FieldDescriptions() {
			record[field.Name] = values[index]
		}
		out = append(out, record)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	write(w, 200, out)
	return nil
}
