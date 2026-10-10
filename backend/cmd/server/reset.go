package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func resetAccountPassword(ctx context.Context, db *pgxpool.Pool, orgCode, email, password, operationID string) error {
	orgCode = strings.ToUpper(strings.TrimSpace(orgCode))
	email = strings.ToLower(strings.TrimSpace(email))
	if orgCode == "" || !strings.Contains(email, "@") || len(password) < 12 || len(password) > 72 || len(operationID) < 16 || len(operationID) > 100 {
		return fmt.Errorf("account recovery requires organization code, email, 12-72 byte password and unique 16-100 byte operation ID")
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	var userID, orgID int64
	var employeeID string
	var active bool
	var deletedAt *time.Time
	rows, err := tx.Query(ctx, `SELECT u.id,u.org_id,u.employee_id,u.active,u.deleted_at
		FROM users u JOIN organizations o ON o.id=u.org_id
		WHERE o.code=$1 AND lower(u.email)=$2 FOR UPDATE OF u`, orgCode, email)
	if err != nil {
		return fmt.Errorf("recovery target unavailable: %w", err)
	}
	count := 0
	for rows.Next() {
		count++
		if err = rows.Scan(&userID, &orgID, &employeeID, &active, &deletedAt); err != nil {
			rows.Close()
			return err
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("recovery requires exactly one matching account; found %d", count)
	}
	if !active || deletedAt != nil {
		return fmt.Errorf("recovery target is inactive or archived; account access is not changed")
	}
	var applied bool
	// The account lock and immutable audit marker prevent repeated cold-start resets.
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_logs WHERE org_id=$1 AND entity='user' AND entity_id=$2 AND action='password_recovery' AND request_id=$3)`, orgID, userID, operationID).Scan(&applied); err != nil {
		return err
	}
	if !applied {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
		if err != nil {
			return fmt.Errorf("recovery password hashing failed")
		}
		if _, err = tx.Exec(ctx, `UPDATE users SET password_hash=$1 WHERE id=$2`, string(hash), userID); err != nil {
			return err
		}
		revoked, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1`, userID)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(org_id,actor_id,action,entity,entity_id,after_data,request_id,user_agent)
			VALUES($1,$2,'password_recovery','user',$2,jsonb_build_object('source','operator_recovery','sessions_revoked',$3::bigint),$4,'server/reset-password')`, orgID, userID, revoked.RowsAffected(), operationID); err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	slog.Info("account recovery complete", "org_code", orgCode, "employee_id", employeeID, "already_applied", applied)
	return nil
}
