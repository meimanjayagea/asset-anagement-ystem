package main

import (
	"assetflow/internal/app"
	"context"
	"embed"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

//go:embed migrations/*.sql
var migrations embed.FS

func run() error {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	timeout := 30 * time.Second
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL wajib")
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		return e
	}
	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour
	cfg.ConnConfig.RuntimeParams["timezone"] = "Asia/Jakarta"
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "10000"
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		cfg.ConnConfig.RuntimeParams["statement_timeout"] = "240000"
	}
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		return e
	}
	defer db.Close()
	if e = db.Ping(ctx); e != nil {
		return e
	}
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		conn, e := db.Acquire(ctx)
		if e != nil {
			return e
		}
		defer conn.Release()
		if _, e = conn.Exec(ctx, `SELECT pg_advisory_lock(7842301)`); e != nil {
			return e
		}
		defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(7842301)`)
		_, e = conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(version integer PRIMARY KEY,applied_at timestamptz NOT NULL DEFAULT now())`)
		if e != nil {
			return e
		}

		for _, migration := range []struct {
			Version int
			File    string
		}{{1, "001_init.sql"}, {2, "002_branches_activity.sql"}, {3, "003_roles_scope_archive.sql"}, {4, "004_finance_lifecycle.sql"}} {
			var exists bool
			if e = conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, migration.Version).Scan(&exists); e != nil {
				return e
			}
			if exists {
				continue
			}
			b, e := migrations.ReadFile("migrations/" + migration.File)
			if e != nil {
				return e
			}
			tx, e := conn.Begin(ctx)
			if e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, string(b)); e != nil {
				_ = tx.Rollback(ctx)
				return e
			}
			if e = tx.Commit(ctx); e != nil {
				return e
			}
			slog.Info("migration applied", "version", migration.Version)
		}
		return nil
	}
	if len(os.Args) > 1 && os.Args[1] == "bootstrap" {
		pw := os.Getenv("BOOTSTRAP_PASSWORD")
		email := strings.ToLower(strings.TrimSpace(os.Getenv("BOOTSTRAP_EMAIL")))
		name := os.Getenv("ORG_NAME")
		if len(pw) < 12 || len(pw) > 72 || !strings.Contains(email, "@") || name == "" {
			return fmt.Errorf("BOOTSTRAP_EMAIL, BOOTSTRAP_PASSWORD (12–72 byte), ORG_NAME wajib")
		}
		h, e := bcrypt.GenerateFromPassword([]byte(pw), 12)
		if e != nil {
			return e
		}
		tx, e := db.Begin(ctx)
		if e != nil {
			return e
		}
		defer tx.Rollback(ctx)
		if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7842302)`); e != nil {
			return e
		}
		var count int
		if e = tx.QueryRow(ctx, `SELECT count(*) FROM organizations`).Scan(&count); e != nil {
			return e
		}
		if count > 0 {
			return fmt.Errorf("bootstrap hanya untuk database kosong")
		}
		var org int64
		e = tx.QueryRow(ctx, `INSERT INTO organizations(name) VALUES($1) RETURNING id`, name).Scan(&org)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO users(org_id,email,name,password_hash,role,all_branches) VALUES($1,$2,'Administrator',$3,'admin',true)`, org, email, string(h))
		if e != nil {
			return e
		}
		var branch int64
		e = tx.QueryRow(ctx, `INSERT INTO branches(org_id,code,name) VALUES($1,'HQ','Kantor Pusat') RETURNING id`, org).Scan(&branch)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO locations(org_id,branch_id,name) VALUES($1,$2,'Kantor Pusat')`, org, branch)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO categories(org_id,name,useful_life_months) VALUES($1,'IT Equipment',48)`, org)
		if e != nil {
			return e
		}
		if e = tx.Commit(ctx); e != nil {
			return e
		}
		slog.Info("bootstrap complete", "org_id", org)
		return nil
	}
	origin := os.Getenv("APP_ORIGIN")
	if !app.ValidOrigin(origin) {
		return fmt.Errorf("APP_ORIGIN harus exact origin")
	}
	secure := os.Getenv("COOKIE_SECURE") != "false"
	parsed, _ := url.Parse(origin)
	if !secure && parsed.Hostname() != "localhost" && parsed.Hostname() != "127.0.0.1" {
		return fmt.Errorf("cookie insecure hanya untuk localhost")
	}
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{Addr: ":" + port, Handler: (&app.Server{DB: db, Origin: origin, Secure: secure}).Routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	done := make(chan error, 1)
	go func() { slog.Info("listening", "address", srv.Addr); done <- srv.ListenAndServe() }()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case e := <-done:
		return e
	case <-sig:
		c, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return srv.Shutdown(c)
	}
}
func main() {
	if e := run(); e != nil {
		slog.Error("fatal", "error", e)
		os.Exit(1)
	}
}
