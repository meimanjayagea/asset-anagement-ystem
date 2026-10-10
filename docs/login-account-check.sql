-- Read-only. Run after login-upgrade.sql in the application's database.
SELECT version, applied_at FROM schema_migrations ORDER BY version;

SELECT o.code AS org_code, o.name AS organization, u.employee_id,
       u.email, u.active, u.deleted_at, u.role, u.all_branches,
       ARRAY(SELECT b.code FROM user_branches ub JOIN branches b ON b.id=ub.branch_id
             WHERE ub.user_id=u.id AND b.deleted_at IS NULL
               AND can_access_branch(u.org_id,u.id,b.id) ORDER BY b.code) AS accessible_branches,
       CASE WHEN NOT u.active OR u.deleted_at IS NOT NULL THEN 'account inactive or archived'
            WHEN NOT u.all_branches AND NOT EXISTS (
              SELECT 1 FROM user_branches ub JOIN branches b ON b.id=ub.branch_id
              WHERE ub.user_id=u.id AND b.deleted_at IS NULL AND can_access_branch(u.org_id,u.id,b.id)
            ) THEN 'no active branch access'
            ELSE 'use org_code + employee_id + existing password' END AS login_status
FROM users u JOIN organizations o ON o.id=u.org_id
WHERE lower(u.email)='meiman@example.test';

SELECT column_name FROM information_schema.columns
WHERE table_schema=current_schema() AND table_name='user_activity_logs'
  AND column_name IN ('identity_hash','attempted_employee_id','attempted_email');

SELECT has_table_privilege(current_user,'user_activity_logs','INSERT') AS audit_insert,
       has_sequence_privilege(current_user,pg_get_serial_sequence('user_activity_logs','id'),'USAGE') AS audit_sequence;
