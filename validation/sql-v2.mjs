import fs from "node:fs";
import { fileURLToPath } from "node:url";
import { PGlite } from "@electric-sql/pglite";
const root = fileURLToPath(new URL("../", import.meta.url));
const db = new PGlite();
await db.waitReady;
let checks = 0;
const check = (x, s) => {
  if (!x) throw Error(s);
  checks++;
};
async function rejects(sql, code) {
  try {
    await db.exec(sql);
    throw Error("Unexpected success");
  } catch (e) {
    try {
      await db.exec("ROLLBACK");
    } catch {}
    check(e.code === code, "SQLSTATE " + code + " got " + e.code);
  }
}
await db.exec(
  fs.readFileSync(root + "/backend/migrations/001_init.sql", "utf8"),
);
await db.exec(
  `INSERT INTO organizations(name) VALUES('Example Corp');INSERT INTO users(org_id,name,email,password_hash,role) VALUES(1,'Old Admin','old@company','test','admin');INSERT INTO locations(org_id,name) VALUES(1,'Office');INSERT INTO categories(org_id,name,useful_life_months) VALUES(1,'Laptop',48);INSERT INTO assets(org_id,tag,name,category_id,location_id,purchase_date,purchase_cost,useful_life_months) VALUES(1,'LEGACY','Old laptop',1,1,'2026-01-01',100,48);INSERT INTO audit_logs(org_id,actor_id,action,entity,entity_id) VALUES(1,1,'create','asset',1);`,
);
await db.exec(
  fs.readFileSync(
    root + "/backend/migrations/002_branches_activity.sql",
    "utf8",
  ),
);
check(
  (await db.query(`SELECT count(*)::int n FROM assets`)).rows[0].n === 1,
  "legacy assets preserved",
);
check(
  (await db.query(`SELECT all_branches FROM users WHERE id=1`)).rows[0]
    .all_branches,
  "legacy users maintain explicit company access",
);
check(
  (await db.query(`SELECT branch_id FROM locations WHERE id=1`)).rows[0]
    .branch_id === 1,
  "HQ location backfill",
);
check(
  (await db.query(`SELECT branch_id FROM audit_logs WHERE id=1`)).rows[0]
    .branch_id === null,
  "legacy immutable audit unchanged",
);
await db.exec(
  `INSERT INTO branches(org_id,code,name) VALUES(1,'BDG','Bandung'),(1,'JKT','Jakarta');INSERT INTO locations(org_id,branch_id,name) VALUES(1,2,'Office'),(1,3,'Office');UPDATE assets SET location_id=2 WHERE id=1;INSERT INTO users(org_id,name,email,password_hash,role) VALUES(1,'Scoped','scoped@company','test','operator');INSERT INTO user_branches(org_id,user_id,branch_id) VALUES(1,2,2);INSERT INTO users(org_id,name,email,password_hash,role,all_branches) VALUES(1,'Legacy Manager','manager@company','test','manager',true);`,
);
await db.exec(
  fs.readFileSync(
    root + "/backend/migrations/003_roles_scope_archive.sql",
    "utf8",
  ),
);
await db.exec(fs.readFileSync(root + "/backend/migrations/004_finance_lifecycle.sql", "utf8"));
check((await db.query(`SELECT version FROM schema_migrations ORDER BY version`)).rows.length === 4, "all four migrations applied");
await db.exec(`INSERT INTO asset_movements(org_id,asset_id,actor_id,event,to_location_id,to_branch_id,note) VALUES(1,1,1,'registered',2,2,'seed');INSERT INTO asset_valuations(org_id,asset_id,branch_id,requested_by,effective_date,carrying_value_before,revalued_amount,remaining_life_months,reason,expected_version) VALUES(1,1,2,1,'2026-10-01',90,100,38,'Market review',1);INSERT INTO service_contracts(org_id,branch_id,asset_id,name,vendor,start_date,end_date,created_by) VALUES(1,2,1,'Support','Vendor','2026-01-01','2027-01-01',1);INSERT INTO accounting_profiles(org_id,updated_by) VALUES(1,1);`);
check((await db.query(`SELECT depreciation_method FROM assets WHERE id=1`)).rows[0].depreciation_method === "straight_line", "legacy assets receive a safe depreciation default");
check((await db.query(`SELECT count(*)::int n FROM asset_movements WHERE asset_id=1`)).rows[0].n === 1, "append-only movement timeline persists");
await rejects(`UPDATE asset_movements SET note='tampered' WHERE id=1`, "P0001");
await rejects(`INSERT INTO asset_valuations(org_id,asset_id,branch_id,requested_by,effective_date,carrying_value_before,revalued_amount,remaining_life_months,reason,expected_version) VALUES(1,1,2,1,'2026-10-01',90,110,38,'Duplicate pending review',1)`, "23505");
check((await db.query(`SELECT count(*)::int n FROM service_contracts WHERE org_id=1 AND deleted_at IS NULL`)).rows[0].n === 1, "contract tracking schema available");
check((await db.query(`SELECT asset_account FROM accounting_profiles WHERE org_id=1`)).rows[0].asset_account === "1500", "generic account mapping defaults available");
check(
  !(await db.query(`SELECT all_branches FROM users WHERE id=3`)).rows[0]
    .all_branches,
  "legacy non-admin global scope converted to explicit memberships",
);
check(
  (await db.query(`SELECT can_access_branch(1,3,2) AS ok`)).rows[0].ok,
  "legacy multi-branch scope preserved outside HQ",
);
check(
  (await db.query(`SELECT can_access_branch(1,1,2) AS ok`)).rows[0].ok,
  "admin global",
);
check(
  (await db.query(`SELECT can_access_branch(1,2,2) AS ok`)).rows[0].ok,
  "scoped allowed",
);
check(
  !(await db.query(`SELECT can_access_branch(1,2,1) AS ok`)).rows[0].ok,
  "branch users cannot access HQ",
);
await db.exec(
  `INSERT INTO categories(org_id,name,useful_life_months,branch_id) VALUES(1,'Branch Equipment',48,2),(1,'Branch Equipment',48,3);`,
);
const categoryList = [...fs.readFileSync(root + "/backend/internal/app/branches.go", "utf8").matchAll(/`([^`]+)`/g)]
  .map((x) => x[1])
  .find((q) => q.startsWith("SELECT c.id,c.name"));
const branchCategories = await db.query(categoryList, [1, false, 2, 2]);
check(
  branchCategories.rows.length === 2 && branchCategories.rows.every((c) => c.name !== "Branch Equipment" || c.branch_id === 2),
  "branch admin sees only its local category plus shared catalog",
);
const categoryAvailableQuery = [...fs.readFileSync(root + "/backend/internal/app/assets.go", "utf8").matchAll(/`([^`]+)`/g)]
  .map((x) => x[1])
  .find((q) => q.startsWith("SELECT c.depreciation_method FROM categories c JOIN locations"));
check(
  (await db.query(categoryAvailableQuery, [1, 2, 2])).rows.length === 1,
  "asset can use category owned by its branch",
);
check(
  (await db.query(categoryAvailableQuery, [1, 3, 2])).rows.length === 0,
  "asset cannot use category owned by another branch",
);
check(
  (await db.query(`SELECT can_access_location(1,2,2) AS ok`)).rows[0].ok,
  "branch location allowed",
);
await rejects(
  `INSERT INTO users(org_id,name,email,password_hash,role,all_branches) VALUES(1,'Bad Admin','bad@company','test','admin',false)`,
  "23514",
);
await rejects(
  `INSERT INTO users(org_id,name,email,password_hash,role,all_branches) VALUES(1,'Bad Manager','bad-manager@company','test','manager',true)`,
  "23514",
);
await db.exec("BEGIN");
const branchAdmin = await db.query(
  `INSERT INTO users(org_id,name,email,password_hash,role) VALUES(1,'Branch Admin','branch-admin@company','test','branch_admin') RETURNING id`,
);
await db.query(
  `INSERT INTO user_branches(org_id,user_id,branch_id) VALUES(1,$1,2)`,
  [branchAdmin.rows[0].id],
);
await db.exec("COMMIT");
check(
  (
    await db.query(`SELECT can_access_branch(1,$1,2) AS ok`, [
      branchAdmin.rows[0].id,
    ])
  ).rows[0].ok,
  "branch administrator access to assigned branch",
);
check(
  !(
    await db.query(`SELECT can_access_branch(1,$1,1) AS ok`, [
      branchAdmin.rows[0].id,
    ])
  ).rows[0].ok,
  "branch administrator cannot access HQ",
);
await rejects(
  `BEGIN;UPDATE user_branches SET branch_id=1 WHERE user_id=${branchAdmin.rows[0].id};COMMIT;`,
  "23514",
);
await rejects(
  `INSERT INTO branches(org_id,code,name) VALUES(1,'BDG','Duplicate')`,
  "23505",
);
await db.exec(
  `UPDATE categories SET maintenance_interval_days=90,maintenance_instructions='Inspect battery' WHERE id=1;`,
);
const code = fs.readFileSync(root + "/backend/internal/app/assets.go", "utf8");
const raw = [...code.matchAll(/`([^`]+)`/g)].map((x) => x[1]);
const create = raw.find((q) => q.startsWith("INSERT INTO assets"));
await db.query(create, [
  1,
  "NEW",
  "New laptop",
  "SN2",
  1,
  2,
  "2026-01-01",
  10000,
  100,
  48,
  null,
  "straight_line",
  "2026-01-01",
  "Supplier",
  "PO-1",
]);
check(
  (
    await db.query(
      `SELECT next_maintenance_date::text AS date FROM assets WHERE tag='NEW'`,
    )
  ).rows[0].date === "2026-04-01",
  "creation maintenance schedule",
);
const list = raw.find(
  (q) => q.startsWith("SELECT a.id") && q.includes("count(*) OVER"),
);
check(
  (await db.query(list, [1, "", "", 25, 0, 2, 0, false, "2026-10-10"])).rows.every(
    (a) => a.branch_id === 2,
  ),
  "exact API asset SQL enforces branch",
);
check(
  (await db.query(list, [1, "", "", 25, 0, 1, 2, false, "2026-10-10"])).rows.length === 2,
  "branch filter correct",
);
await db.exec(
  `INSERT INTO maintenance(org_id,asset_id,title,due_date,branch_id) VALUES(1,1,'Service','2026-10-01',2);INSERT INTO requests(org_id,asset_id,kind,target_location_id,reason,expected_version,requested_by,source_branch_id,target_branch_id) VALUES(1,1,'transfer',1,'Transfer branch',1,1,2,1);`,
);
for (const file of ["branches.go", "workflows.go", "stocktake.go"]) {
  for (const match of fs
    .readFileSync(root + "/backend/internal/app/" + file, "utf8")
    .matchAll(/`([^`]+)`/g)) {
    const q = match[1];
    if (
      q.startsWith("SELECT ") &&
      q.includes("LIMIT") &&
      !q.includes("WHERE org_id=$1 ORDER BY id")
    ) {
      const max = Math.max(...[...q.matchAll(/\$(\d+)/g)].map((x) => +x[1]));
      let args;
      if (q.includes("FROM assets a")) args = [1, 1, 0, false];
      else if (q.includes("FROM requests WHERE")) continue;
      else if (
        q.includes("FROM requests r") ||
        q.includes("FROM maintenance m") ||
        q.includes("FROM audit_logs")
      )
        continue;
      else if (q.includes("FROM stocktakes s")) args = [1, 25, 0, 2, 0];
      else if (q.includes("FROM stocktake_items i")) args = [1, 1, 25, 0, 2];
      else if (q.includes("FROM branches WHERE")) args = [1, 2, false, false];
      else if (q.includes("FROM locations l")) args = [1, 2, 0, false];
      else args = [1];
      if (args.length === max) {
        await db.query(q, args);
        checks++;
      }
    }
  }
}
const workflow = [
  ...fs
    .readFileSync(root + "/backend/internal/app/workflows.go", "utf8")
    .matchAll(/`([^`]+)`/g),
].map((x) => x[1]);
await db.query(
  workflow.find((q) => q.startsWith("SELECT r.id")),
  [1, 25, 0, 2, 0, false],
);
checks++;
await db.query(
  workflow.find((q) => q.startsWith("SELECT m.id")),
  [1, 25, 0, 2, 0, false],
);
checks++;
await db.query(
  workflow.find((q) => q.startsWith("SELECT a.id,a.actor_id")),
  [1, 25, 0, false, 2, 0],
);
checks++;
await db.exec(
  `INSERT INTO user_activity_logs(org_id,actor_id,actor_name,actor_branch_ids,branch_id,event,method,path,status_code,duration_ms,request_id,peer_ip,user_agent) VALUES(1,2,'Scoped',ARRAY[2]::bigint[],2,'read','GET','/api/assets',200,1,'r1','127.0.0.1','Test'),(1,NULL,'',ARRAY[]::bigint[],NULL,'login_failed','POST','/api/login',401,1,'r2','127.0.0.1','Test');`,
);
const aq = [
  ...fs
    .readFileSync(root + "/backend/internal/app/activity.go", "utf8")
    .matchAll(/`([^`]+)`/g),
]
  .map((x) => x[1])
  .find((q) => q.startsWith("SELECT id,actor_id"));
check(
  (await db.query(aq, [1, true, 1, "", 25, 0, 0])).rows.length === 2,
  "company activity",
);
const branchActivity = (await db.query(aq, [1, false, 2, "", 25, 0, 0])).rows;
check(
  branchActivity.length === 1 && branchActivity[0].event === "read",
  "branch administrators see scoped activity but not unscoped activity",
);
check(
  (await db.query(aq, [1, false, 2, "", 25, 0, 2])).rows.length === 1,
  "scoped activity visibility",
);
await rejects(`DELETE FROM user_activity_logs`, "P0001");
await rejects(`UPDATE user_activity_logs SET status_code=200`, "P0001");
await db.exec(
  `DELETE FROM user_branches WHERE user_id=2;INSERT INTO user_branches(org_id,user_id,branch_id) VALUES(1,2,1);`,
);
check(
  !(await db.query(`SELECT can_access_branch(1,2,2) ok`)).rows[0].ok,
  "scope removal",
);
const policy = [
  ...fs
    .readFileSync(root + "/backend/internal/app/category_policy.go", "utf8")
    .matchAll(/`([^`]+)`/g),
]
  .map((x) => x[1])
  .find((q) => q.startsWith("UPDATE assets a SET next_maintenance_date"));
await db.query(policy, [120, 1, 1, 1]);
check(
  (
    await db.query(
      `SELECT next_maintenance_date::text d FROM assets WHERE tag='NEW'`,
    )
  ).rows[0].d === "2026-05-01",
  "policy recompute skips pending asset and recalculates free asset",
);
const locks = raw.find(
  (q) => q.startsWith("SELECT a.id") && q.includes("FOR UPDATE OF a"),
);
check(
  (await db.query(locks, [1, 1, 3])).rows.length === 1,
  "lock asset respects new branch assignment",
);
check(
  (await db.query(locks, [1, 1, 2])).rows.length === 0,
  "lock asset denies removed branch",
);
const exportQ = [
  ...fs
    .readFileSync(root + "/backend/internal/app/export.go", "utf8")
    .matchAll(/`([^`]+)`/g),
]
  .map((x) => x[1])
  .find((q) => q.startsWith("SELECT a.tag"));
const exported = (await db.query(exportQ, [1, 3, 2, "NEW", "", 25, 0, "2026-10-10"])).rows;
check(
  exported.length === 1 && exported.every((a) => a.tag === "NEW"),
  "server CSV export branch scope",
);
const financeSql = [...fs.readFileSync(root + "/backend/internal/app/finance.go", "utf8").matchAll(/`([^`]+)`/g)].map((x) => x[1]);
await db.query(financeSql.find((q) => q.startsWith("SELECT a.id,a.tag,a.name,a.purchase_date")), [1, 2, 0]);
checks++;
await db.query(financeSql.find((q) => q.startsWith("SELECT c.id,c.branch_id")), [1, 1, 0, 25, 0, true, "2026-10-10"]);
checks++;
await db.query(financeSql.find((q) => q.startsWith("SELECT v.id,v.asset_id")), [1, 1, 0, 25, 0, true, true, 1]);
checks++;
const complianceCounts = await db.query(financeSql.find((q) => q.startsWith("SELECT count(*) FILTER")), [1, 1, 0, "2026-10-10"]);
check(complianceCounts.fields.length === 5, "compliance aggregate count matches API scan destinations");
checks++;
const movementSql = [...fs.readFileSync(root + "/backend/internal/app/movements.go", "utf8").matchAll(/`([^`]+)`/g)].map((x) => x[1]).find((q) => q.startsWith("SELECT m.id,m.event"));
await db.query(movementSql, [1, 1, 2, 0, 25, 0]);
checks++;
await db.query(financeSql.find((q) => q.startsWith("SELECT revalued_amount,remaining_life_months,effective_date")), [1, 1, "2026-10-10"]);
checks++;
await db.query(financeSql.find((q) => q.startsWith("SELECT remaining_life_months,effective_date")), [1, 1, "2026-10-10"]);
checks++;
await db.query(financeSql.find((q) => q.startsWith("SELECT a.tag,a.name,b.code,(a.disposed_at")), [1, "2026-10-01", "2026-10-31", 1, 0]);
checks++;
await db.query(financeSql.find((q) => q.startsWith("SELECT a.tag,a.name,b.code,v.effective_date")), [1, "2026-10-01", "2026-10-31", 1, 0]);
checks++;
console.log(
  JSON.stringify({
    checks,
    result: "PASS",
    engine: "PGlite PostgreSQL WASM",
    scope:
      "Forward migration, branch scopes, exact API SELECTs, maintenance date and audit constraints; not native multi-session testing",
  }),
);
await db.close();
