import fs from "node:fs";
import { fileURLToPath } from "node:url";
import { PGlite } from "@electric-sql/pglite";

const root = fileURLToPath(new URL("../", import.meta.url));
const files = fs.readdirSync(root + "backend/migrations").filter((name) => /^\d{3}_.*\.sql$/.test(name)).sort();
const migrations = files.map((name) => ({
  version: Number(name.slice(0, 3)),
  sql: fs.readFileSync(root + "backend/migrations/" + name, "utf8"),
}));
const latest = migrations.at(-1).version;
const bundle = [
  "-- Generated from backend/migrations by validation/login-migration.mjs. Run as schema owner.",
  "BEGIN;",
  "SELECT pg_advisory_xact_lock(7842301);",
  "CREATE TABLE IF NOT EXISTS schema_migrations(version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());",
  `DO $upgrade$ BEGIN
IF EXISTS (SELECT 1 FROM schema_migrations WHERE version > ${latest}) THEN
  RAISE EXCEPTION 'Database is newer than this migration bundle';
END IF;
END $upgrade$;`,
  ...migrations.map(({ version, sql }) => `DO $upgrade$ BEGIN
IF NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version=${version}) THEN
  EXECUTE $migration_${version}$${sql}$migration_${version}$;
END IF;
END $upgrade$;`),
  "COMMIT;",
].join("\n\n") + "\n";
fs.writeFileSync(root + "docs/login-upgrade.sql", bundle);

function check(condition, message) {
  if (!condition) throw new Error(message);
}
for (const startingVersion of [0, 2, 4, 5, 6, 8]) {
  const db = new PGlite();
  await db.waitReady;
  for (const migration of migrations.filter((m) => m.version <= startingVersion)) await db.exec(migration.sql);
  if (startingVersion) {
    await db.exec(`INSERT INTO organizations(name) VALUES('Existing organization');
INSERT INTO users(org_id,name,email,password_hash,role,all_branches) VALUES(1,'Meiman','meiman@example.test','preserved-hash','admin',true);
INSERT INTO branches(org_id,code,name) VALUES(1,'HQ','Head office');
INSERT INTO user_activity_logs(org_id,event,method,path,status_code,duration_ms,request_id,peer_ip,user_agent) VALUES(1,'read','GET','/api/me',200,1,'before-upgrade','127.0.0.1','migration-test');`);
  }
  await db.exec(bundle);
  await db.exec(bundle);
  check((await db.query("SELECT version FROM schema_migrations ORDER BY version")).rows.length === latest, "Migration versions must be applied once");
  if (startingVersion) {
    const { rows } = await db.query(`SELECT o.code,u.employee_id,u.password_hash,u.active FROM users u JOIN organizations o ON o.id=u.org_id WHERE u.email='meiman@example.test'`);
    check(rows[0].code === "ORG-000001" && rows[0].employee_id === "EMP-1", "Existing account receives login identifiers");
    check(rows[0].password_hash === "preserved-hash" && rows[0].active, "Existing credentials preserved");
    check((await db.query("SELECT count(*)::int AS n FROM user_activity_logs WHERE request_id='before-upgrade'")).rows[0].n === 1, "Audit history preserved");
    const source = fs.readFileSync(root + "backend/internal/app/activity.go", "utf8");
    const query = source.match(/`(INSERT INTO user_activity_logs[^`]+)`/)[1];
    await db.query(query, [1, 1, "Meiman", [], true, "login_success", "POST", "/api/login", 200, 1, "after-upgrade", "127.0.0.1", "migration-test", "hash", null, null, "", "EMP-1"]);
  }
  await db.close();
  console.log(`PASS: upgrade from version ${startingVersion}, repeat application, account and audit compatibility`);
}
