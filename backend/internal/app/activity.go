package app

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

type requestKey struct{}
type requestMeta struct {
	Branch, Related                                               *int64
	ID, IP, Agent, IdentityHash, AttemptedEmail, AttemptedEmployeeID string
	User                                                          *User
	LoginOrg                                                      int64
}

func meta(c context.Context) *requestMeta {
	v, _ := c.Value(requestKey{}).(*requestMeta)
	if v == nil {
		return &requestMeta{}
	}
	return v
}

type bufferedResponse struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (b *bufferedResponse) Header() http.Header { return b.header }
func (b *bufferedResponse) WriteHeader(code int) {
	if b.status == 0 {
		b.status = code
	}
}
func (b *bufferedResponse) Write(p []byte) (int, error) {
	if b.status == 0 {
		b.status = 200
	}
	return b.body.Write(p)
}
func (s *Server) recordActivity(r *http.Request, status int, duration time.Duration) error {
	m := meta(r.Context())
	u := m.User
	var org, actorID any
	actorName := ""
	branches := []int64{}
	all := false
	// Authenticate denied requests before logging (CSRF/content-type/unknown route included).
	if u == nil {
		if cookie, e := r.Cookie("assetflow_session"); e == nil {
			var a User
			e = s.DB.QueryRow(r.Context(), `SELECT u.id,u.org_id,u.name,u.email,u.role,u.all_branches,ARRAY(SELECT ub.branch_id FROM user_branches ub JOIN branches b ON b.id=ub.branch_id WHERE ub.user_id=u.id AND b.deleted_at IS NULL AND (u.role='admin' OR b.code<>'HQ') ORDER BY ub.branch_id) FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now() AND u.active AND u.deleted_at IS NULL`, hash(cookie.Value)).Scan(&a.ID, &a.OrgID, &a.Name, &a.Email, &a.Role, &a.AllBranches, &a.BranchIDs)
			if e == nil {
				u = &a
			}
		}
	}
	if u != nil {
		org = u.OrgID
		actorID = u.ID
		actorName = u.Name
		branches = u.BranchIDs
		all = u.AllBranches
	} else if m.LoginOrg > 0 {
		var exists bool
		if e := s.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM organizations WHERE id=$1)`, m.LoginOrg).Scan(&exists); e != nil {
			return e
		}
		if exists {
			org = m.LoginOrg
		}
	}
	path := r.URL.Path
	if len(path) > 2048 {
		path = path[:2048]
	}
	path = strings.ToValidUTF8(path, "?")
	event := "action"
	if r.Method == "GET" {
		event = "read"
	}
	if status >= 400 {
		event = "denied"
	}
	if r.URL.Path == "/api/login" {
		event = "login_success"
		if status >= 400 {
			event = "login_failed"
		}
		if status == 429 {
			event = "login_throttled"
		}
	}
	if r.URL.Path == "/api/logout" && status < 400 {
		event = "logout"
	}
	_, e := s.DB.Exec(r.Context(), `INSERT INTO user_activity_logs(org_id,actor_id,actor_name,actor_branch_ids,all_branches,event,method,path,status_code,duration_ms,request_id,peer_ip,user_agent,identity_hash,branch_id,related_branch_id,attempted_email,attempted_employee_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`, org, actorID, actorName, branches, all, event, r.Method, path, status, duration.Milliseconds(), m.ID, m.IP, m.Agent, m.IdentityHash, m.Branch, m.Related, m.AttemptedEmail, m.AttemptedEmployeeID)
	return e
}
func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		rid, _ := token()
		ip, _, e := net.SplitHostPort(r.RemoteAddr)
		if e != nil {
			ip = r.RemoteAddr
		}
		agent := r.UserAgent()
		if len(agent) > 512 {
			agent = agent[:512]
		}
		agent = strings.ToValidUTF8(agent, "?")
		m := &requestMeta{ID: rid, IP: ip, Agent: agent}
		r = r.WithContext(context.WithValue(r.Context(), requestKey{}, m))
		b := &bufferedResponse{header: make(http.Header)}
		b.Header().Set("X-Request-ID", rid)
		b.Header().Set("X-Content-Type-Options", "nosniff")
		b.Header().Set("Cache-Control", "no-store")
		defer func() {
			if p := recover(); p != nil {
				slog.Error("panic", "request_id", rid, "panic", p)
				b.body.Reset()
				b.status = 0
				write(b, 500, map[string]string{"error": "Internal server error"})
			}
			if b.status == 0 {
				b.status = 200
			}
			if strings.HasPrefix(r.URL.Path, "/api/") {
				c, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 3*time.Second)
				e := s.recordActivity(r.WithContext(c), b.status, time.Since(started))
				cancel()
				if e != nil {
					slog.Error("activity persistence failed", "request_id", rid, "error", e)
					b.body.Reset()
					b.status = 0
					b.Header().Del("Set-Cookie")
					write(b, 503, map[string]string{"error": "Audit aktivitas tidak tersedia; refresh untuk memastikan hasil aksi"})
				}
			}
			for k, values := range b.Header() {
				for _, v := range values {
					w.Header().Add(k, v)
				}
			}
			w.WriteHeader(b.status)
			_, _ = w.Write(b.body.Bytes())
			slog.Info("http", "method", r.Method, "path", r.URL.Path, "status", b.status, "duration_ms", time.Since(started).Milliseconds(), "request_id", rid)
		}()
		if r.Method != "GET" && r.Method != "HEAD" {
			if r.Header.Get("X-Requested-With") != "AssetFlow" {
				write(b, 403, map[string]string{"error": "CSRF header required"})
				return
			}
			if r.Header.Get("Origin") != "" && r.Header.Get("Origin") != s.Origin {
				write(b, 403, map[string]string{"error": "Origin ditolak"})
				return
			}
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				write(b, 415, map[string]string{"error": "Gunakan application/json"})
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		next.ServeHTTP(b, r.WithContext(ctx))
	})
}
func (s *Server) listActivity(w http.ResponseWriter, r *http.Request) error {
	p, z := page(r)
	u := actor(r)
	return s.rows(w, r, `SELECT id,actor_id,actor_name,event,method,path,status_code,duration_ms,request_id,peer_ip,user_agent,created_at,branch_id,related_branch_id,attempted_email,attempted_employee_id FROM user_activity_logs WHERE org_id=$1 AND ($2 OR can_access_branch(org_id,$3,branch_id) OR can_access_branch(org_id,$3,related_branch_id)) AND ($4='' OR event=$4) AND ($7::bigint=0 OR branch_id=$7 OR related_branch_id=$7) ORDER BY id DESC LIMIT $5 OFFSET $6`, u.OrgID, u.AllBranches, u.ID, r.URL.Query().Get("event"), z, (p-1)*z, selectedBranch(r))
}
