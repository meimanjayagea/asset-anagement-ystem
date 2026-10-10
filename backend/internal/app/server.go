package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Server struct {
	DB       *pgxpool.Pool
	Secure   bool
	Origin   string
	mu       sync.Mutex
	attempts map[string]attempt
}
type attempt struct {
	Count int
	Reset time.Time
}
type User struct {
	ID             int64    `json:"id"`
	OrgID          int64    `json:"org_id"`
	Organization   string   `json:"organization_name"`
	Name           string   `json:"name"`
	Email          string   `json:"email"`
	Role           string   `json:"role"`
	AllBranches    bool     `json:"all_branches"`
	BranchIDs      []int64  `json:"branch_ids"`
	ActiveBranchID int64    `json:"active_branch_id"`
	Capabilities   []string `json:"capabilities"`
}
type ctxKey struct{}
type apiError struct {
	Code    int
	Message string
}

func (e apiError) Error() string    { return e.Message }
func fail(code int, s string) error { return apiError{code, s} }
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func report(w http.ResponseWriter, e error) {
	var a apiError
	if errors.As(e, &a) {
		write(w, a.Code, map[string]string{"error": a.Message})
		return
	}
	var p *pgconn.PgError
	if errors.As(e, &p) {
		switch p.Code {
		case "23505":
			write(w, 409, map[string]string{"error": "Data duplikat atau proses aktif sudah ada"})
			return
		case "23503", "23514", "22007", "22008", "22P02":
			write(w, 422, map[string]string{"error": "Referensi atau nilai data tidak valid"})
			return
		}
	}
	slog.Error("request failed", "error", e)
	write(w, 500, map[string]string{"error": "Internal server error"})
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return fail(400, "JSON tidak valid")
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return fail(400, "Hanya satu JSON object diizinkan")
	}
	return nil
}
func actor(r *http.Request) User { return r.Context().Value(ctxKey{}).(User) }
func id(r *http.Request) (int64, error) {
	v, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if e != nil || v < 1 {
		return 0, fail(400, "ID tidak valid")
	}
	return v, nil
}
func hash(s string) string { v := sha256.Sum256([]byte(s)); return hex.EncodeToString(v[:]) }
func token() (string, error) {
	b := make([]byte, 32)
	_, e := rand.Read(b)
	return hex.EncodeToString(b), e
}
func (s *Server) Routes() http.Handler {
	s.attempts = make(map[string]attempt)
	m := http.NewServeMux()
	m.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) { write(w, 200, map[string]string{"status": "ok"}) })
	m.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		c, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if e := s.DB.Ping(c); e != nil {
			write(w, 503, map[string]string{"status": "unavailable"})
			return
		}
		write(w, 200, map[string]string{"status": "ok"})
	})
	m.HandleFunc("GET /api/login/options", s.loginOptions)
	m.HandleFunc("POST /api/login", s.login)
	add := func(pattern, capability string, h func(http.ResponseWriter, *http.Request) error) {
		m.Handle(pattern, s.auth(capability, h))
	}
	add("GET /api/me", "", func(w http.ResponseWriter, r *http.Request) error { write(w, 200, actor(r)); return nil })
	add("POST /api/logout", "", s.logout)
	add("GET /api/dashboard", "dashboard.read", s.dashboard)
	add("GET /api/assets", "assets.read", s.listAssets)
	add("GET /api/assets/{id}/history", "assets.history", s.assetHistory)
	add("POST /api/assets", "assets.write", s.createAsset)
	add("POST /api/assets/{id}", "assets.write", s.updateAsset)
	add("POST /api/assets/{id}/action", "assets.operate", s.assetAction)
	add("DELETE /api/assets/{id}", "assets.archive", s.archiveAsset)
	add("POST /api/assets/{id}/restore", "assets.archive", s.restoreAsset)
	add("GET /api/locations", "locations.read", s.listLocations)
	add("POST /api/locations", "locations.manage", s.createLocation)
	add("DELETE /api/locations/{id}", "locations.archive", s.archiveLocation)
	add("POST /api/locations/{id}/restore", "locations.archive", s.restoreLocation)
	add("GET /api/categories", "categories.read", s.listCategories)
	add("POST /api/categories", "categories.manage", s.createCategory)
	add("DELETE /api/categories/{id}", "categories.archive", s.archiveCategory)
	add("POST /api/categories/{id}/restore", "categories.archive", s.restoreCategory)
	add("GET /api/requests", "requests.read", s.listRequests)
	add("POST /api/requests", "requests.create", s.createRequest)
	add("POST /api/requests/{id}/decision", "requests.decide", s.decideRequest)
	add("GET /api/maintenance", "maintenance.read", s.listMaintenance)
	add("POST /api/maintenance", "maintenance.manage", s.createMaintenance)
	add("POST /api/maintenance/{id}/action", "maintenance.manage", s.maintenanceAction)
	add("GET /api/audit", "audit.read", s.auditList)
	add("GET /api/users", "users.read", s.listUsers)
	add("POST /api/users", "users.manage", s.createUser)
	add("GET /api/user-roles", "users.manage", s.listAssignableRoles)
	add("POST /api/users/{id}/access", "users.manage", s.updateUserAccess)
	add("DELETE /api/users/{id}", "users.archive", s.archiveUser)
	add("POST /api/users/{id}/restore", "users.archive", s.restoreUser)
	add("GET /api/stocktakes", "stocktakes.read", s.listStocktakes)
	add("POST /api/stocktakes", "stocktakes.manage", s.createStocktake)
	add("GET /api/stocktakes/{id}/items", "stocktakes.read", s.stocktakeItems)
	add("POST /api/stocktakes/{id}/observe", "stocktakes.observe", s.observeStocktake)
	add("POST /api/stocktakes/{id}/close", "stocktakes.close", s.closeStocktake)
	add("POST /api/password", "", s.changePassword)
	add("GET /api/branches", "branches.read", s.listBranches)
	add("POST /api/branches", "branches.manage", s.createBranch)
	add("DELETE /api/branches/{id}", "branches.manage", s.archiveBranch)
	add("POST /api/branches/{id}/restore", "branches.manage", s.restoreBranch)
	add("POST /api/exports/assets", "assets.export", s.exportAssets)
	add("GET /api/activity", "activity.read", s.listActivity)
	add("POST /api/categories/{id}/policy", "categories.manage", s.categoryPolicy)
	add("GET /api/contracts", "contracts.read", s.listContracts)
	add("POST /api/contracts", "contracts.manage", s.createContract)
	add("DELETE /api/contracts/{id}", "contracts.manage", s.archiveContract)
	add("GET /api/finance/settings", "finance.read", s.accountingSettings)
	add("POST /api/finance/settings", "finance.manage", s.accountingSettings)
	add("GET /api/finance/valuations", "valuation.propose", s.valuations)
	add("POST /api/finance/valuations", "valuation.propose", s.valuations)
	add("POST /api/finance/valuations/{id}/decision", "valuation.decide", s.decideValuation)
	add("GET /api/finance/depreciation", "finance.read", s.depreciationReport)
	add("GET /api/finance/journals", "finance.read", s.journalPreview)
	add("POST /api/exports/journal", "reports.export", s.exportJournal)
	add("GET /api/reports/compliance", "reports.read", s.complianceReport)
	return s.middleware(m)
}
func (s *Server) auth(capability string, h func(http.ResponseWriter, *http.Request) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, e := r.Cookie("assetflow_session")
		if e != nil {
			report(w, fail(401, "Login diperlukan"))
			return
		}
		var u User
		e = s.DB.QueryRow(r.Context(), `SELECT u.id,u.org_id,o.name,u.name,u.email,u.role,u.all_branches,s.active_branch_id,ARRAY(SELECT ub.branch_id FROM user_branches ub JOIN branches b ON b.id=ub.branch_id WHERE ub.user_id=u.id AND b.deleted_at IS NULL AND (u.role='admin' OR b.code<>'HQ') ORDER BY ub.branch_id) FROM sessions s JOIN users u ON u.id=s.user_id JOIN organizations o ON o.id=u.org_id WHERE s.token_hash=$1 AND s.expires_at>now() AND u.active AND u.deleted_at IS NULL`, hash(c.Value)).Scan(&u.ID, &u.OrgID, &u.Organization, &u.Name, &u.Email, &u.Role, &u.AllBranches, &u.ActiveBranchID, &u.BranchIDs)
		if e != nil {
			if errors.Is(e, pgx.ErrNoRows) {
				report(w, fail(401, "Sesi berakhir"))
			} else {
				report(w, e)
			}
			return
		}
		u.Capabilities = capabilitiesForRole(u.Role)
		meta(r.Context()).User = &u
		if u.ActiveBranchID > 0 {
			meta(r.Context()).Branch = &u.ActiveBranchID
		}
		if raw := r.URL.Query().Get("branch_id"); raw != "" {
			branch, e := strconvBranch(raw)
			if e != nil || branch < 0 {
				report(w, fail(400, "branch_id tidak valid"))
				return
			}
			if branch > 0 {
				meta(r.Context()).Branch = &branch
				var ok bool
				e = s.DB.QueryRow(r.Context(), `SELECT can_access_branch($1,$2,$3)`, u.OrgID, u.ID, branch).Scan(&ok)
				if e != nil {
					report(w, e)
					return
				}
				if !ok {
					report(w, fail(403, "Cabang di luar akses"))
					return
				}
			}
		}
		if capability != "" && !hasCapability(u.Role, capability) {
			report(w, fail(403, "Akses ditolak"))
			return
		}
		if e = h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u))); e != nil {
			report(w, e)
		}
	})
}
func (s *Server) loginOptions(w http.ResponseWriter, r *http.Request) {
	rows, e := s.DB.Query(r.Context(), `SELECT o.id,o.name,b.id,b.code,b.name FROM organizations o JOIN branches b ON b.org_id=o.id AND b.deleted_at IS NULL ORDER BY o.id,b.code`)
	if e != nil {
		report(w, e)
		return
	}
	defer rows.Close()
	type branchOption struct {
		ID   int64  `json:"id"`
		Code string `json:"code"`
		Name string `json:"name"`
	}
	type organizationOption struct {
		ID       int64          `json:"id"`
		Name     string         `json:"name"`
		Branches []branchOption `json:"branches"`
	}
	orgs := []organizationOption{}
	indexes := map[int64]int{}
	for rows.Next() {
		var orgID, branchID int64
		var orgName, code, branchName string
		if e = rows.Scan(&orgID, &orgName, &branchID, &code, &branchName); e != nil {
			report(w, e)
			return
		}
		index, ok := indexes[orgID]
		if !ok {
			index = len(orgs)
			indexes[orgID] = index
			orgs = append(orgs, organizationOption{ID: orgID, Name: orgName, Branches: []branchOption{}})
		}
		orgs[index].Branches = append(orgs[index].Branches, branchOption{ID: branchID, Code: code, Name: branchName})
	}
	if e = rows.Err(); e != nil {
		report(w, e)
		return
	}
	write(w, 200, map[string]any{"organizations": orgs})
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		OrgID    int64  `json:"org_id"`
		BranchID int64  `json:"branch_id"`
	}
	if e := decode(w, r, &in); e != nil {
		report(w, e)
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	meta(r.Context()).LoginOrg = in.OrgID
	if in.BranchID > 0 {
		meta(r.Context()).Branch = &in.BranchID
	}
	meta(r.Context()).EmailHash = hash(in.Email)
	if len(in.Email) <= 254 {
		meta(r.Context()).AttemptedEmail = in.Email
	}
	if len(in.Email) < 3 || len(in.Email) > 254 || len(in.Password) < 1 || len(in.Password) > 72 || in.OrgID < 1 || in.BranchID < 0 {
		report(w, fail(422, "Kredensial tidak valid"))
		return
	}
	key := fmt.Sprintf("%d:%s", in.OrgID, in.Email)
	s.mu.Lock()
	now := time.Now()
	a := s.attempts[key]
	if now.After(a.Reset) {
		a = attempt{Reset: now.Add(15 * time.Minute)}
	}
	a.Count++
	if len(s.attempts) > 10000 {
		for k, v := range s.attempts {
			if now.After(v.Reset) {
				delete(s.attempts, k)
			}
		}
		if len(s.attempts) > 10000 {
			s.mu.Unlock()
			report(w, fail(429, "Coba lagi nanti"))
			return
		}
	}
	s.attempts[key] = a
	s.mu.Unlock()
	if a.Count > 10 {
		report(w, fail(429, "Terlalu banyak percobaan; tunggu 15 menit"))
		return
	}
	var u User
	var pw string
	e := s.DB.QueryRow(r.Context(), `SELECT u.id,u.org_id,o.name,u.name,u.email,u.role,u.password_hash,u.all_branches,ARRAY(SELECT ub.branch_id FROM user_branches ub JOIN branches b ON b.id=ub.branch_id WHERE ub.user_id=u.id AND b.deleted_at IS NULL AND (u.role='admin' OR b.code<>'HQ') ORDER BY ub.branch_id) FROM users u JOIN organizations o ON o.id=u.org_id WHERE u.org_id=$1 AND u.email=$2 AND u.active AND u.deleted_at IS NULL`, in.OrgID, in.Email).Scan(&u.ID, &u.OrgID, &u.Organization, &u.Name, &u.Email, &u.Role, &pw, &u.AllBranches, &u.BranchIDs)
	if e != nil || bcrypt.CompareHashAndPassword([]byte(pw), []byte(in.Password)) != nil {
		report(w, fail(401, "Kredensial tidak valid"))
		return
	}
	u.Capabilities = capabilitiesForRole(u.Role)
	u.ActiveBranchID = in.BranchID
	if u.ActiveBranchID > 0 {
		var allowed bool
		e = s.DB.QueryRow(r.Context(), `SELECT can_access_branch($1,$2,$3)`, u.OrgID, u.ID, u.ActiveBranchID).Scan(&allowed)
		if e != nil {
			report(w, e)
			return
		}
		if !allowed {
			u.ActiveBranchID = 0
		}
	}
	if !u.AllBranches && u.ActiveBranchID == 0 {
		e = s.DB.QueryRow(r.Context(), `SELECT COALESCE(min(b.id),0) FROM branches b JOIN user_branches ub ON ub.branch_id=b.id AND ub.org_id=b.org_id WHERE ub.org_id=$1 AND ub.user_id=$2 AND b.deleted_at IS NULL AND can_access_branch($1,$2,b.id)`, u.OrgID, u.ID).Scan(&u.ActiveBranchID)
		if e != nil {
			report(w, e)
			return
		}
		if u.ActiveBranchID == 0 {
			report(w, fail(401, "Kredensial tidak valid"))
			return
		}
	}
	meta(r.Context()).User = &u
	if u.ActiveBranchID > 0 {
		meta(r.Context()).Branch = &u.ActiveBranchID
	}
	t, e := token()
	if e != nil {
		report(w, e)
		return
	}
	tx, e := s.DB.Begin(r.Context())
	if e != nil {
		report(w, e)
		return
	}
	defer tx.Rollback(r.Context())
	_, e = tx.Exec(r.Context(), `DELETE FROM sessions WHERE expires_at<now()`)
	if e == nil {
		_, e = tx.Exec(r.Context(), `INSERT INTO sessions(token_hash,user_id,expires_at,active_branch_id) VALUES($1,$2,now()+interval '8 hours',$3)`, hash(t), u.ID, u.ActiveBranchID)
	}
	if e == nil {
		e = logAudit(r.Context(), tx, u, "login", "user", u.ID, nil, map[string]any{"email": u.Email})
	}
	if e == nil {
		e = tx.Commit(r.Context())
	}
	if e != nil {
		report(w, e)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "assetflow_session", Value: t, Path: "/", HttpOnly: true, Secure: s.Secure, SameSite: http.SameSiteStrictMode, MaxAge: 28800})
	write(w, 200, u)
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) error {
	c, _ := r.Cookie("assetflow_session")
	_, e := s.DB.Exec(r.Context(), `DELETE FROM sessions WHERE token_hash=$1`, hash(c.Value))
	if e != nil {
		return e
	}
	http.SetCookie(w, &http.Cookie{Name: "assetflow_session", Value: "", Path: "/", HttpOnly: true, Secure: s.Secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	write(w, 200, map[string]bool{"ok": true})
	return nil
}

func logAudit(c context.Context, tx pgx.Tx, u User, action, entity string, id int64, before, after any) error {
	b, _ := json.Marshal(before)
	a, _ := json.Marshal(after)
	m := meta(c)
	var branch, related *int64
	var query string
	switch entity {
	case "asset":
		query = `SELECT l.branch_id FROM assets a JOIN locations l ON l.id=a.location_id WHERE a.org_id=$1 AND a.id=$2`
	case "location":
		query = `SELECT branch_id FROM locations WHERE org_id=$1 AND id=$2`
	case "category":
		query = `SELECT COALESCE(branch_id,0) FROM categories WHERE org_id=$1 AND id=$2`
	case "request":
		query = `SELECT source_branch_id FROM requests WHERE org_id=$1 AND id=$2`
	case "maintenance":
		query = `SELECT branch_id FROM maintenance WHERE org_id=$1 AND id=$2`
	case "stocktake":
		query = `SELECT l.branch_id FROM stocktakes s JOIN locations l ON l.id=s.location_id WHERE s.org_id=$1 AND s.id=$2`
	case "asset_export":
		branch = m.Branch
	case "contract":
		query = `SELECT branch_id FROM service_contracts WHERE org_id=$1 AND id=$2`
	case "valuation":
		query = `SELECT branch_id FROM asset_valuations WHERE org_id=$1 AND id=$2`
	case "branch":
		branch = &id
	}
	if query != "" {
		var v int64
		if e := tx.QueryRow(c, query, u.OrgID, id).Scan(&v); e != nil {
			return e
		}
		if v > 0 {
			branch = &v
		}
	}
	if entity == "asset" && action == "transfer" {
		related = branch
		if old, ok := before.(Asset); ok {
			var v int64
			if e := tx.QueryRow(c, `SELECT branch_id FROM locations WHERE org_id=$1 AND id=$2`, u.OrgID, old.Location).Scan(&v); e != nil {
				return e
			}
			branch = &v
		}
	}
	if entity == "request" {
		if e := tx.QueryRow(c, `SELECT target_branch_id FROM requests WHERE org_id=$1 AND id=$2`, u.OrgID, id).Scan(&related); e != nil {
			return e
		}
	}
	if branch == nil {
		branch = m.Branch
	}
	if branch != nil {
		m.Branch = branch
		m.Related = related
	}
	_, e := tx.Exec(c, `INSERT INTO audit_logs(org_id,actor_id,action,entity,entity_id,before_data,after_data,branch_id,related_branch_id,request_id,peer_ip,user_agent) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, u.OrgID, u.ID, action, entity, id, b, a, branch, related, m.ID, m.IP, m.Agent)
	return e
}
func (s *Server) transaction(r *http.Request, f func(pgx.Tx) error) error {
	tx, e := s.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	if e = f(tx); e != nil {
		return e
	}
	return tx.Commit(r.Context())
}
func (s *Server) rows(w http.ResponseWriter, r *http.Request, q string, args ...any) error {
	rows, e := s.DB.Query(r.Context(), q, args...)
	if e != nil {
		return e
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		values, e := rows.Values()
		if e != nil {
			return e
		}
		m := map[string]any{}
		for i, f := range rows.FieldDescriptions() {
			m[f.Name] = values[i]
		}
		out = append(out, m)
	}
	if e = rows.Err(); e != nil {
		return e
	}
	write(w, 200, out)
	return nil
}
func page(r *http.Request) (int, int) {
	p, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if p < 1 {
		p = 1
	}
	if p > 100000 {
		p = 100000
	}
	if size < 1 {
		size = 25
	}
	if size > 100 {
		size = 100
	}
	return p, size
}

func ValidOrigin(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.Path == "" && u.RawQuery == "" && u.Fragment == "" && u.User == nil
}
