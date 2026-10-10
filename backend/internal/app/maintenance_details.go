package app

import "net/http"

func (s *Server) plannedMaintenance(w http.ResponseWriter, r *http.Request) error {
	p, z := page(r)
	u := actor(r)
	return s.rows(w, r, `SELECT a.id AS asset_id,'planned-'||a.id AS id,a.tag,a.name,c.maintenance_instructions AS title,'planned' AS status,'preventive' AS kind,a.next_maintenance_date::text AS due_date,a.version AS asset_version FROM assets a JOIN locations l ON l.org_id=a.org_id AND l.id=a.location_id JOIN categories c ON c.org_id=a.org_id AND c.id=a.category_id WHERE a.org_id=$1 AND a.deleted_at IS NULL AND a.status<>'disposed' AND a.next_maintenance_date IS NOT NULL AND can_access_branch(a.org_id,$2,l.branch_id) AND ($3::bigint=0 OR l.branch_id=$3) AND NOT EXISTS(SELECT 1 FROM maintenance m WHERE m.org_id=a.org_id AND m.asset_id=a.id AND m.status IN ('scheduled','in_progress')) ORDER BY a.next_maintenance_date,a.id LIMIT $4 OFFSET $5`, u.OrgID, u.ID, selectedBranch(r), z, (p-1)*z)
}

func (s *Server) maintenanceAssignees(w http.ResponseWriter, r *http.Request) error {
	u := actor(r)
	return s.rows(w, r, `SELECT u.id,u.name,u.employee_id FROM users u WHERE u.org_id=$1 AND u.active AND u.deleted_at IS NULL AND u.role IN ('admin','branch_admin','manager','operator','it_support') AND EXISTS(SELECT 1 FROM branches b WHERE b.org_id=u.org_id AND b.deleted_at IS NULL AND can_access_branch(u.org_id,u.id,b.id) AND can_access_branch(u.org_id,$2,b.id) AND ($3::bigint=0 OR b.id=$3)) ORDER BY u.name LIMIT 1000`, u.OrgID, u.ID, selectedBranch(r))
}

type checkItem struct {
	Label string `json:"label"`
	Done  bool   `json:"done"`
}
type sparePart struct {
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
	Cost     int64  `json:"cost"`
}

func validateChecklist(items []checkItem, complete bool) error {
	if len(items) > 50 {
		return fail(422, "Checklist maksimal 50 item")
	}
	seen := map[string]bool{}
	for _, item := range items {
		if len(item.Label) < 1 || len(item.Label) > 200 || seen[item.Label] || (complete && !item.Done) {
			return fail(422, "Checklist tidak valid atau belum lengkap")
		}
		seen[item.Label] = true
	}
	return nil
}
func validateParts(items []sparePart) error {
	if len(items) > 50 {
		return fail(422, "Suku cadang maksimal 50 item")
	}
	for _, item := range items {
		if len(item.Name) < 1 || len(item.Name) > 200 || item.Quantity < 1 || item.Quantity > 10000 || item.Cost < 0 || item.Cost > 1000000000000 {
			return fail(422, "Suku cadang tidak valid")
		}
	}
	return nil
}
