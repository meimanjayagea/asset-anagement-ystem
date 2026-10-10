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
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	if command != "serve" && command != "migrate" && command != "bootstrap" {
		return fmt.Errorf("unknown command: %s", command)
	}
	if command == "migrate" || (command == "serve" && os.Getenv("AUTO_MIGRATE") == "true") {
		migrationDSN, err := migrationConnection(command, os.Getenv)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		err = migrateDatabase(ctx, migrationDSN)
		cancel()
		if err != nil {
			return err
		}
		if command == "migrate" {
			return nil
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		return e
	}
	defer db.Close()
	if e = db.Ping(ctx); e != nil {
		return e
	}
	server := &app.Server{DB: db}
	if e = server.CheckReadiness(ctx); e != nil {
		return e
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
		var orgCode string
		e = tx.QueryRow(ctx, `INSERT INTO organizations(name) VALUES($1) RETURNING id,code`, name).Scan(&org, &orgCode)
		if e != nil {
			return e
		}
		var employeeID string
		e = tx.QueryRow(ctx, `INSERT INTO users(org_id,email,name,password_hash,role,all_branches) VALUES($1,$2,'Administrator',$3,'admin',true) RETURNING employee_id`, org, email, string(h)).Scan(&employeeID)
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
		slog.Info("bootstrap complete", "org_id", org, "org_code", orgCode, "employee_id", employeeID)
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
	server.Origin = origin
	server.Secure = secure
	srv := &http.Server{Addr: ":" + port, Handler: server.Routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
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
