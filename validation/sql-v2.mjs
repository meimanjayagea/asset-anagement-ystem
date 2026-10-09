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
  `INSERT INTO branches(org_id,code,name) VALUES(1,'BDG','Bandung');INSERT INTO locations(org_id,branch_id,name) VALUES(1,2,'Office');INSERT INTO users(org_id,name,email,password_hash,role) VALUES(1,'Scoped','scoped@company','test','operator');INSERT INTO user_branches(org_id,user_id,branch_id) VALUES(1,2,1);`,
);
check(
  (await db.query(`SELECT can_access_branch(1,1,2) AS ok`)).rows[0].ok,
  "admin global",
);
check(
  (await db.query(`SELECT can_access_branch(1,2,1) AS ok`)).rows[0].ok,
  "scoped allowed",
);
check(
  !(await db.query(`SELECT can_access_branch(1,2,2) AS ok`)).rows[0].ok,
  "scoped denied",
);
check(
  !(await db.query(`SELECT can_access_location(1,2,2) AS ok`)).rows[0].ok,
  "location denied",
);
await rejects(
  `INSERT INTO users(org_id,name,email,password_hash,role,all_branches) VALUES(1,'Bad Admin','bad@company','test','admin',false)`,
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
  (await db.query(list, [1, "", "", 25, 0, 2, 0])).rows.every(
    (a) => a.branch_id === 1,
  ),
  "exact API asset SQL enforces branch",
);
check(
  (await db.query(list, [1, "", "", 25, 0, 1, 2])).rows.length === 1,
  "branch filter correct",
);
await db.exec(
  `INSERT INTO maintenance(org_id,asset_id,title,due_date,branch_id) VALUES(1,1,'Service','2026-10-01',1);INSERT INTO requests(org_id,asset_id,kind,target_location_id,reason,expected_version,requested_by,source_branch_id,target_branch_id) VALUES(1,1,'transfer',2,'Transfer branch',1,1,1,2);`,
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
      if (q.includes("FROM assets a")) args = [1, 1, 0];
      else if (q.includes("FROM requests WHERE")) continue;
      else if (
        q.includes("FROM requests r") ||
        q.includes("FROM maintenance m") ||
        q.includes("FROM audit_logs")
      )
        continue;
      else if (q.includes("FROM stocktakes s")) args = [1, 25, 0, 1, 0];
      else if (q.includes("FROM stocktake_items i")) args = [1, 1, 25, 0, 1];
      else if (q.includes("FROM branches WHERE")) args = [1, 1];
      else if (q.includes("FROM locations l")) args = [1, 1, 0];
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
  [1, 25, 0, 1, 0],
);
checks++;
await db.query(
  workflow.find((q) => q.startsWith("SELECT m.id")),
  [1, 25, 0, 1, 0],
);
checks++;
await db.query(
  workflow.find((q) => q.startsWith("SELECT a.id,a.actor_id")),
  [1, 25, 0, false, 2, 0],
);
checks++;
await db.exec(
  `INSERT INTO user_activity_logs(org_id,actor_id,actor_name,actor_branch_ids,event,method,path,status_code,duration_ms,request_id,peer_ip,user_agent) VALUES(1,2,'Scoped',ARRAY[1]::bigint[],'read','GET','/api/assets',200,1,'r1','127.0.0.1','Test'),(1,NULL,'',ARRAY[]::bigint[],'login_failed','POST','/api/login',401,1,'r2','127.0.0.1','Test');`,
);
const aq = [
  ...fs
    .readFileSync(root + "/backend/internal/app/activity.go", "utf8")
    .matchAll(/`([^`]+)`/g),
]
  .map((x) => x[1])
  .find((q) => q.startsWith("SELECT id,actor_id"));
check(
  (await db.query(aq, [1, true, 1, [], "", 25, 0, 0])).rows.length === 2,
  "company activity",
);
check(
  (await db.query(aq, [1, false, 2, [1], "", 25, 0, 0])).rows.length === 1,
  "scoped activity visibility",
);
await rejects(`DELETE FROM user_activity_logs`, "P0001");
await rejects(`UPDATE user_activity_logs SET status_code=200`, "P0001");
await db.exec(
  `DELETE FROM user_branches WHERE user_id=2;INSERT INTO user_branches(org_id,user_id,branch_id) VALUES(1,2,2);`,
);
check(
  !(await db.query(`SELECT can_access_branch(1,2,1) ok`)).rows[0].ok,
  "scope removal",
);
const policy = [
  ...fs
    .readFileSync(root + "/backend/internal/app/category_policy.go", "utf8")
    .matchAll(/`([^`]+)`/g),
]
  .map((x) => x[1])
  .find((q) => q.startsWith("UPDATE assets a SET next_maintenance_date"));
await db.query(policy, [120, 1, 1]);
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
  (await db.query(locks, [1, 2, 2])).rows.length === 1,
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
const exported = (await db.query(exportQ, [1, 2, 0, "", "", 25, 0])).rows;
check(
  exported.length === 1 && exported.every((a) => a.tag === "NEW"),
  "server CSV export branch scope",
);
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
