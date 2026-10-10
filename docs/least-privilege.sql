-- Run as schema owner after migrations. Provision login credentials out of band.
-- CREATE ROLE assetflow_runtime LOGIN PASSWORD 'secret-from-secure-provisioning';
GRANT CONNECT ON DATABASE assetflow TO assetflow_runtime;
GRANT USAGE ON SCHEMA public TO assetflow_runtime;
GRANT SELECT ON organizations,schema_migrations TO assetflow_runtime;
GRANT SELECT,INSERT,UPDATE ON users,locations,categories,assets,requests,maintenance,stocktakes,stocktake_items TO assetflow_runtime;
GRANT SELECT,INSERT,DELETE ON sessions TO assetflow_runtime;
GRANT SELECT,INSERT,UPDATE ON branches TO assetflow_runtime;
GRANT SELECT,INSERT,DELETE ON user_branches TO assetflow_runtime;
GRANT SELECT,INSERT ON user_activity_logs TO assetflow_runtime;
GRANT SELECT,INSERT ON audit_logs TO assetflow_runtime;
GRANT SELECT,INSERT ON asset_movements TO assetflow_runtime;
GRANT SELECT,INSERT,UPDATE ON asset_valuations,service_contracts,accounting_profiles TO assetflow_runtime;
GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA public TO assetflow_runtime;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
-- Runtime has no table ownership, DDL, superuser or audit UPDATE/DELETE/TRUNCATE.
-- Future migrations must grant only specific new objects; no blanket ALL privileges.
