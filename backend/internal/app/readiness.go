package app

import (
	"context"
	"fmt"
)

// CheckReadiness validates the schema and permissions needed to complete login.
func (s *Server) CheckReadiness(ctx context.Context) error {
	var versions int
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM schema_migrations WHERE version IN (1,2,3,4,5,6)`).Scan(&versions); err != nil {
		return fmt.Errorf("schema unavailable; run migrate: %w", err)
	}
	if versions != 6 {
		return fmt.Errorf("schema incomplete (%d/6); run migrate", versions)
	}
	for _, query := range []string{
		`SELECT code FROM organizations LIMIT 0`,
		`SELECT employee_id,password_hash,active,deleted_at FROM users LIMIT 0`,
		`SELECT token_hash,user_id,expires_at,active_branch_id FROM sessions LIMIT 0`,
		`SELECT branch_id FROM user_branches LIMIT 0`,
		`SELECT code,deleted_at FROM branches LIMIT 0`,
		`SELECT org_id,actor_id,branch_id,related_branch_id,request_id,peer_ip,user_agent FROM audit_logs LIMIT 0`,
		`SELECT identity_hash,attempted_employee_id,attempted_email FROM user_activity_logs LIMIT 0`,
	} {
		rows, err := s.DB.Query(ctx, query)
		if err != nil {
			return fmt.Errorf("login schema unavailable: %w", err)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("login schema unavailable: %w", err)
		}
	}
	var allowed bool
	err := s.DB.QueryRow(ctx, `SELECT
		has_table_privilege(current_user,'sessions','SELECT') AND
		has_table_privilege(current_user,'sessions','INSERT') AND
		has_table_privilege(current_user,'sessions','DELETE') AND
		has_table_privilege(current_user,'audit_logs','INSERT') AND
		has_table_privilege(current_user,'user_activity_logs','INSERT') AND
		has_sequence_privilege(current_user,pg_get_serial_sequence('audit_logs','id'),'USAGE') AND
		has_sequence_privilege(current_user,pg_get_serial_sequence('user_activity_logs','id'),'USAGE')`).Scan(&allowed)
	if err != nil {
		return fmt.Errorf("login permissions unavailable: %w", err)
	}
	if !allowed {
		return fmt.Errorf("runtime role lacks login/session/audit permissions; apply runtime grants")
	}
	return nil
}
