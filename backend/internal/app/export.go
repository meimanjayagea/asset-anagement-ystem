package app

import (
	"encoding/csv"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func safeCSV(v string) string {
	trimmed := strings.TrimLeft(v, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+@-\t\r", rune(trimmed[0])) {
		return "'" + v
	}
	return v
}
func (s *Server) exportAssets(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Page   int    `json:"page"`
		Size   int    `json:"size"`
		Search string `json:"search"`
		Status string `json:"status"`
		Branch int64  `json:"branch_id"`
	}
	if e := decode(w, r, &in); e != nil {
		return e
	}
	if in.Page < 1 || in.Page > 100000 || in.Size < 1 || in.Size > 100 || len(in.Search) > 200 || in.Branch < 0 {
		return fail(422, "Filter export tidak valid")
	}
	financial := hasCapability(actor(r).Role, "assets.finance")
	header := []string{"tag", "name", "serial_number", "status", "branch", "location", "custodian"}
	if financial {
		header = append(header, "purchase_cost", "book_value")
	}
	out := [][]string{header}
	e := s.transaction(r, func(tx pgx.Tx) error {
		u := actor(r)
		if in.Branch > 0 {
			if e := branchScope(r, tx, in.Branch); e != nil {
				return e
			}
			meta(r.Context()).Branch = &in.Branch
		}
		rows, e := tx.Query(r.Context(), `SELECT a.tag,a.name,a.serial_number,a.status,b.name,l.name,a.custodian,a.purchase_cost,a.salvage_value,a.useful_life_months,a.purchase_date,a.depreciation_method,a.depreciation_start_date,COALESCE((SELECT v.revalued_amount FROM asset_valuations v WHERE v.org_id=a.org_id AND v.asset_id=a.id AND v.status='approved' AND v.effective_date<=$8::date ORDER BY v.effective_date DESC,v.id DESC LIMIT 1),a.purchase_cost),COALESCE((SELECT v.remaining_life_months FROM asset_valuations v WHERE v.org_id=a.org_id AND v.asset_id=a.id AND v.status='approved' AND v.effective_date<=$8::date ORDER BY v.effective_date DESC,v.id DESC LIMIT 1),a.useful_life_months),COALESCE((SELECT v.effective_date FROM asset_valuations v WHERE v.org_id=a.org_id AND v.asset_id=a.id AND v.status='approved' AND v.effective_date<=$8::date ORDER BY v.effective_date DESC,v.id DESC LIMIT 1),a.depreciation_start_date) FROM assets a JOIN locations l ON l.id=a.location_id AND l.deleted_at IS NULL JOIN branches b ON b.id=l.branch_id AND b.deleted_at IS NULL JOIN categories c ON c.id=a.category_id AND c.deleted_at IS NULL WHERE a.org_id=$1 AND a.deleted_at IS NULL AND can_access_branch(a.org_id,$2,l.branch_id) AND ($3::bigint=0 OR l.branch_id=$3) AND ($4='' OR a.name ILIKE '%'||$4||'%' OR a.tag ILIKE '%'||$4||'%' OR a.serial_number ILIKE '%'||$4||'%') AND ($5='' OR a.status=$5) ORDER BY a.id DESC LIMIT $6 OFFSET $7`, u.OrgID, u.ID, in.Branch, in.Search, in.Status, in.Size, (in.Page-1)*in.Size, time.Now().In(financeZone).Format("2006-01-02"))
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var tag, name, serial, status, branch, location, custodian string
			var cost, salvage int64
			var life int
			var purchase, depStart, basisStart time.Time
			var method string
			var basisCost int64
			var basisLife int
			if e = rows.Scan(&tag, &name, &serial, &status, &branch, &location, &custodian, &cost, &salvage, &life, &purchase, &method, &depStart, &basisCost, &basisLife, &basisStart); e != nil {
				return e
			}
			value := depreciationBookValue(method, basisCost, salvage, basisLife, basisStart, time.Now().In(financeZone))
			values := []string{tag, name, serial, status, branch, location, custodian}
			if financial {
				values = append(values, strconv.FormatInt(cost, 10), strconv.FormatInt(value, 10))
			}
			for i, v := range values {
				values[i] = safeCSV(v)
			}
			out = append(out, values)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		rows.Close()
		return logAudit(r.Context(), tx, u, "export_csv", "asset_export", 0, nil, map[string]any{"page": in.Page, "size": in.Size, "branch_id": in.Branch, "status": in.Status, "rows": len(out) - 1})
	})
	if e != nil {
		return e
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="assets-current-page.csv"`)
	w.WriteHeader(200)
	_, _ = w.Write([]byte("\xef\xbb\xbf"))
	writer := csv.NewWriter(w)
	writer.UseCRLF = true
	writer.WriteAll(out)
	return writer.Error()
}
