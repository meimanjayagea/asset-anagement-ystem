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
	ID          int64   `json:"id"`
	OrgID       int64   `json:"org_id"`
	Name        string  `json:"name"`
	Email       string  `json:"email"`
	Role        string  `json:"role"`
	AllBranches bool    `json:"all_branches"`
	BranchIDs   []int64 `json:"branch_ids"`
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
	m.HandleFunc("POST /api/login", s.login)
	add := func(pattern, roles string, h func(http.ResponseWriter, *http.Request) error) {
		m.Handle(pattern, s.auth(roles, h))
	}
	add("GET /api/me", "", func(w http.ResponseWriter, r *http.Request) error { write(w, 200, actor(r)); return nil })
	add("POST /api/logout", "", s.logout)
	add("GET /api/dashboard", "", s.dashboard)
	add("GET /api/assets", "", s.listAssets)
	add("POST /api/assets", "admin,manager,operator", s.createAsset)
	add("POST /api/assets/{id}/action", "admin,manager,operator", s.assetAction)
	add("GET /api/locations", "", s.listLocations)
	add("POST /api/locations", "admin,manager", s.createLocation)
	add("GET /api/categories", "", s.listCategories)
	add("POST /api/categories", "admin,manager", s.createCategory)
	add("GET /api/requests", "", s.listRequests)
	add("POST /api/requests", "admin,manager,operator", s.createRequest)
	add("POST /api/requests/{id}/decision", "admin,manager", s.decideRequest)
	add("GET /api/maintenance", "", s.listMaintenance)
	add("POST /api/maintenance", "admin,manager,operator", s.createMaintenance)
	add("POST /api/maintenance/{id}/action", "admin,manager,operator", s.maintenanceAction)
	add("GET /api/audit", "admin,manager,auditor", s.auditList)
	add("GET /api/users", "admin", s.listUsers)
	add("POST /api/users", "admin", s.createUser)
	add("GET /api/stocktakes", "", s.listStocktakes)
	add("POST /api/stocktakes", "admin,manager,operator", s.createStocktake)
	add("GET /api/stocktakes/{id}/items", "", s.stocktakeItems)
	add("POST /api/stocktakes/{id}/observe", "admin,manager,operator", s.observeStocktake)
	add("POST /api/stocktakes/{id}/close", "admin,manager", s.closeStocktake)
	add("POST /api/password", "", s.changePassword)
	add("POST /api/users/{id}/access", "admin", s.updateUserAccess)
	add("GET /api/branches", "", s.listBranches)
	add("POST /api/branches", "admin", s.createBranch)
	add("POST /api/exports/assets", "", s.exportAssets)
	add("GET /api/activity", "admin,manager,auditor", s.listActivity)
	add("POST /api/categories/{id}/policy", "admin", s.categoryPolicy)
	return s.middleware(m)
}
func (s *Server) auth(roles string, h func(http.ResponseWriter, *http.Request) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, e := r.Cookie("assetflow_session")
		if e != nil {
			report(w, fail(401, "Login diperlukan"))
			return
		}
		var u User
		e = s.DB.QueryRow(r.Context(), `SELECT u.id,u.org_id,u.name,u.email,u.role,u.all_branches,ARRAY(SELECT ub.branch_id FROM user_branches ub WHERE ub.user_id=u.id ORDER BY ub.branch_id) FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now() AND u.active`, hash(c.Value)).Scan(&u.ID, &u.OrgID, &u.Name, &u.Email, &u.Role, &u.AllBranches, &u.BranchIDs)
		if e != nil {
			if errors.Is(e, pgx.ErrNoRows) {
				report(w, fail(401, "Sesi berakhir"))
			} else {
				report(w, e)
			}
			return
		}
		meta(r.Context()).User = &u
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
		if roles != "" && !strings.Contains(","+roles+",", ","+u.Role+",") {
			report(w, fail(403, "Akses ditolak"))
			return
		}
		if e = h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u))); e != nil {
			report(w, e)
		}
	})
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		OrgID    int64  `json:"org_id"`
	}
	if e := decode(w, r, &in); e != nil {
		report(w, e)
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	meta(r.Context()).LoginOrg = in.OrgID
	meta(r.Context()).EmailHash = hash(in.Email)
	if len(in.Email) <= 254 {
		meta(r.Context()).AttemptedEmail = in.Email
	}
	if len(in.Email) < 3 || len(in.Email) > 254 || len(in.Password) < 1 || len(in.Password) > 72 || in.OrgID < 1 {
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
	e := s.DB.QueryRow(r.Context(), `SELECT id,org_id,name,email,role,password_hash,all_branches,ARRAY(SELECT ub.branch_id FROM user_branches ub WHERE ub.user_id=users.id ORDER BY ub.branch_id) FROM users WHERE org_id=$1 AND email=$2 AND active`, in.OrgID, in.Email).Scan(&u.ID, &u.OrgID, &u.Name, &u.Email, &u.Role, &pw, &u.AllBranches, &u.BranchIDs)
	if e != nil || bcrypt.CompareHashAndPassword([]byte(pw), []byte(in.Password)) != nil {
		report(w, fail(401, "Kredensial tidak valid"))
		return
	}
	meta(r.Context()).User = &u
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
		_, e = tx.Exec(r.Context(), `INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '8 hours')`, hash(t), u.ID)
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
	case "request":
		query = `SELECT source_branch_id FROM requests WHERE org_id=$1 AND id=$2`
	case "maintenance":
		query = `SELECT branch_id FROM maintenance WHERE org_id=$1 AND id=$2`
	case "stocktake":
		query = `SELECT l.branch_id FROM stocktakes s JOIN locations l ON l.id=s.location_id WHERE s.org_id=$1 AND s.id=$2`
	case "asset_export":
		branch = m.Branch
	case "branch":
		branch = &id
	}
	if query != "" {
		var v int64
		if e := tx.QueryRow(c, query, u.OrgID, id).Scan(&v); e != nil {
			return e
		}
		branch = &v
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
