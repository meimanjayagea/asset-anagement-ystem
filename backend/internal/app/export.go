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
	out := [][]string{{"tag", "name", "serial_number", "status", "branch", "location", "custodian", "purchase_cost", "book_value"}}
	e := s.transaction(r, func(tx pgx.Tx) error {
		u := actor(r)
		if in.Branch > 0 {
			if e := branchScope(r, tx, in.Branch); e != nil {
				return e
			}
			meta(r.Context()).Branch = &in.Branch
		}
		rows, e := tx.Query(r.Context(), `SELECT a.tag,a.name,a.serial_number,a.status,b.name,l.name,a.custodian,a.purchase_cost,a.salvage_value,a.useful_life_months,a.purchase_date FROM assets a JOIN locations l ON l.id=a.location_id JOIN branches b ON b.id=l.branch_id WHERE a.org_id=$1 AND can_access_branch(a.org_id,$2,l.branch_id) AND ($3::bigint=0 OR l.branch_id=$3) AND ($4='' OR a.name ILIKE '%'||$4||'%' OR a.tag ILIKE '%'||$4||'%' OR a.serial_number ILIKE '%'||$4||'%') AND ($5='' OR a.status=$5) ORDER BY a.id DESC LIMIT $6 OFFSET $7`, u.OrgID, u.ID, in.Branch, in.Search, in.Status, in.Size, (in.Page-1)*in.Size)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var tag, name, serial, status, branch, location, custodian string
			var cost, salvage int64
			var life int
			var purchase time.Time
			if e = rows.Scan(&tag, &name, &serial, &status, &branch, &location, &custodian, &cost, &salvage, &life, &purchase); e != nil {
				return e
			}
			value := bookValue(cost, salvage, life, purchase, time.Now().In(time.FixedZone("WIB", 7*3600)))
			values := []string{tag, name, serial, status, branch, location, custodian, strconv.FormatInt(cost, 10), strconv.FormatInt(value, 10)}
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
