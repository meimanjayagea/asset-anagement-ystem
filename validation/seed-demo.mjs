import { randomBytes } from "node:crypto";

const execute = process.argv.includes("--execute");
const confirmation = process.env.DEMO_SEED_CONFIRM;
const rawURL = process.env.ASSETFLOW_URL;

if (!execute) {
  console.log(
    JSON.stringify({
      mode: "dry-run",
      writes: 0,
      roles: [
        "admin",
        "branch_admin",
        "manager",
        "operator",
        "staff",
        "employee",
        "finance",
        "it_support",
        "it_developer",
        "auditor",
      ],
      sampleBranches: ["BDG", "JKT", "SBY"],
      note: "Pass --execute and DEMO_SEED_CONFIRM=SEED_ASSETFLOW_DEMO_DATA to write through the application API.",
    }),
  );
  process.exit(0);
}

if (confirmation !== "SEED_ASSETFLOW_DEMO_DATA") {
  throw new Error("Explicit demo-seed confirmation is required.");
}
if (!rawURL) throw new Error("ASSETFLOW_URL is required.");
const base = new URL(rawURL);
if (base.protocol !== "https:" || ["localhost", "127.0.0.1"].includes(base.hostname)) {
  throw new Error("Execution requires the verified HTTPS application URL.");
}
const orgID = Number(process.env.ASSETFLOW_ORG_ID);
const email = process.env.ASSETFLOW_ADMIN_EMAIL;
const password = process.env.ASSETFLOW_ADMIN_PASSWORD;
if (!Number.isSafeInteger(orgID) || orgID < 1 || !email || !password) {
  throw new Error("ASSETFLOW_ORG_ID, ASSETFLOW_ADMIN_EMAIL, and ASSETFLOW_ADMIN_PASSWORD are required.");
}

let sessionCookie = "";
async function api(path, method = "GET", body) {
  const headers = {
    "X-Requested-With": "AssetFlow",
    Origin: base.origin,
  };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (sessionCookie) headers.Cookie = sessionCookie;
  const response = await fetch(new URL("/api" + path, base), {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    redirect: "error",
  });
  if (path === "/login") {
    const cookies = response.headers.getSetCookie?.() || [response.headers.get("set-cookie") || ""];
    const session = cookies.find((value) => value.startsWith("assetflow_session="));
    if (session) sessionCookie = session.split(";", 1)[0];
  }
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(`${method} ${path}: ${response.status} ${data.error || "request failed"}`);
  }
  return data;
}

const login = await api("/login", "POST", {
  email,
  password,
  org_id: orgID,
  branch_id: 0,
});
if (login.role !== "admin" || !login.all_branches) {
  throw new Error("The seed account must be an all-branches central admin.");
}
if (!sessionCookie) throw new Error("Login did not return a session cookie.");
const me = await api("/me");
if (me.role !== "admin" || !me.all_branches) {
  throw new Error("The authenticated account is not a central administrator.");
}

const roles = [
  ["admin", "Demo Central Admin"],
  ["branch_admin", "Demo Branch Admin"],
  ["manager", "Demo Manager"],
  ["operator", "Demo Operator"],
  ["staff", "Demo Staff"],
  ["employee", "Demo Employee"],
  ["finance", "Demo Finance"],
  ["it_support", "Demo IT Support"],
  ["it_developer", "Demo IT Developer"],
  ["auditor", "Demo Auditor"],
];
const demoEmails = roles.map(([role]) => `assetflow-demo+${role}@example.test`);
const currentUsers = await api("/users");
const existingDemoUsers = currentUsers.filter((user) => demoEmails.includes(user.email));
if (existingDemoUsers.length) {
  throw new Error("Demo role accounts already exist; refusing to reset or replace their passwords.");
}

const branches = new Map((await api("/branches?branch_id=0")).map((row) => [row.code, row]));
for (const [code, name, address] of [
  ["BDG", "Bandung Demo Office", "Jl. Asia Afrika 88, Bandung"],
  ["JKT", "Jakarta Demo Office", "Jl. HR Rasuna Said 22, Jakarta"],
  ["SBY", "Surabaya Demo Office", "Jl. Basuki Rahmat 45, Surabaya"],
]) {
  if (!branches.has(code)) {
    const created = await api("/branches", "POST", { code, name, address });
    branches.set(code, { id: created.id, code, name, address });
  }
}

const locations = new Map(
  (await api("/locations?branch_id=0")).map((row) => [`${row.branch_id}:${row.name}`, row]),
);
for (const code of ["BDG", "JKT", "SBY"]) {
  const branch = branches.get(code);
  const name = `Demo Operations - ${code}`;
  const key = `${branch.id}:${name}`;
  if (!locations.has(key)) {
    const created = await api("/locations", "POST", { branch_id: branch.id, name });
    locations.set(key, { id: created.id, branch_id: branch.id, name });
  }
}

const categoryRows = await api("/categories?branch_id=0");
const categories = new Map(
  categoryRows.map((row) => [`${row.branch_id ?? 0}:${row.name}`, row]),
);
async function ensureCategory(name, branchID, life, interval) {
  const key = `${branchID || 0}:${name}`;
  if (categories.has(key)) return categories.get(key);
  const created = await api("/categories", "POST", {
    name,
    useful_life_months: life,
    maintenance_interval_days: interval,
    maintenance_instructions: interval ? "Inspect condition and record the service result." : "",
    branch_id: branchID || 0,
  });
  const row = { id: created.id, name, branch_id: branchID || null };
  categories.set(key, row);
  return row;
}
const itCategory = await ensureCategory("Demo - IT Equipment", 0, 48, 180);
const vehicleCategory = await ensureCategory("Demo - Vehicles", 0, 96, 365);
const facilityCategory = await ensureCategory("Demo - Facilities", 0, 60, 180);
const localFacilities = new Map();
for (const code of ["BDG", "JKT", "SBY"]) {
  const branch = branches.get(code);
  localFacilities.set(code, await ensureCategory(`Demo - ${code} Local Equipment`, branch.id, 48, 120));
}

const assetData = (await api("/assets?branch_id=0&search=DEMO-&size=100&page=1")).items;
const assetsByTag = new Map(assetData.map((asset) => [asset.tag, asset]));
const sampleAssets = [];
for (const code of ["BDG", "JKT", "SBY"]) {
  const branch = branches.get(code);
  const location = locations.get(`${branch.id}:Demo Operations - ${code}`);
  for (const sample of [
    { suffix: "LAP-001", name: "Lenovo ThinkPad E14 - Demo", category: itCategory, cost: 14500000, life: 48 },
    { suffix: "CAR-001", name: "Toyota Avanza Operasional - Demo", category: vehicleCategory, cost: 245000000, life: 96 },
    { suffix: "HVAC-001", name: `Daikin 2 PK - ${code} Demo`, category: localFacilities.get(code), cost: 12500000, life: 48 },
    { suffix: "PROJ-001", name: `Epson Projector - ${code} Demo`, category: facilityCategory, cost: 11800000, life: 60 },
  ]) {
    const tag = `DEMO-${code}-${sample.suffix}`;
    let asset = assetsByTag.get(tag);
    if (!asset) {
      const created = await api("/assets", "POST", {
        tag,
        name: sample.name,
        serial_number: `DEMO-SN-${code}-${sample.suffix}`,
        category_id: sample.category.id,
        location_id: location.id,
        purchase_date: "2025-03-10",
        purchase_cost: sample.cost,
        salvage_value: Math.round(sample.cost * 0.1),
        useful_life_months: sample.life,
        warranty_until: "2028-03-10",
      });
      asset = {
        id: created.id,
        tag,
        name: sample.name,
        location_id: location.id,
        branch_id: branch.id,
        status: "available",
        version: 1,
      };
      assetsByTag.set(tag, asset);
    }
    sampleAssets.push(asset);
  }
}

for (const code of ["BDG", "JKT", "SBY"]) {
  const vehicle = assetsByTag.get(`DEMO-${code}-CAR-001`);
  if (vehicle.status === "available") {
    await api(`/assets/${vehicle.id}/action`, "POST", {
      action: "assign",
      custodian: `DEMO-EMP-${code}-001`,
      version: vehicle.version,
    });
    vehicle.status = "assigned";
    vehicle.version += 1;
  }
}

const maintenanceRows = await api("/maintenance?branch_id=0");
const maintenanceAsset = assetsByTag.get("DEMO-JKT-HVAC-001");
let maintenance = maintenanceRows.find((row) => row.asset_id === maintenanceAsset.id && ["scheduled", "in_progress"].includes(row.status));
if (!maintenance) {
  const created = await api("/maintenance", "POST", {
    asset_id: maintenanceAsset.id,
    title: "Preventive service - demo HVAC",
    due_date: new Date(Date.now() + 7 * 86400000).toISOString().slice(0, 10),
    version: maintenanceAsset.version,
  });
  maintenance = { id: created.id, status: "scheduled" };
}
if (maintenance.status === "scheduled") {
  await api(`/maintenance/${maintenance.id}/action`, "POST", {
    action: "start",
    cost: 0,
    notes: "Demo service currently in progress.",
    version: maintenanceAsset.version,
  });
  maintenanceAsset.status = "maintenance";
  maintenanceAsset.version += 1;
}

const requests = await api("/requests?branch_id=0");
const requestAsset = assetsByTag.get("DEMO-BDG-LAP-001");
if (!requests.some((row) => row.asset_id === requestAsset.id && row.status === "pending")) {
  const target = locations.get(`${branches.get("JKT").id}:Demo Operations - JKT`);
  await api("/requests", "POST", {
    asset_id: requestAsset.id,
    kind: "transfer",
    target_location_id: target.id,
    reason: "Demo transfer request for client presentation.",
    version: requestAsset.version,
  });
}

const stocktakes = await api("/stocktakes?branch_id=0");
for (const code of ["BDG", "JKT", "SBY"]) {
  const title = `Demo stocktake - ${code}`;
  if (!stocktakes.some((row) => row.title === title && row.status === "open")) {
    const branch = branches.get(code);
    const location = locations.get(`${branch.id}:Demo Operations - ${code}`);
    await api("/stocktakes", "POST", { title, location_id: location.id });
  }
}

const credentials = [];
for (const [role, name] of roles) {
  const generatedPassword = randomBytes(24).toString("base64url");
  const accountEmail = `assetflow-demo+${role}@example.test`;
  try {
    await api("/users", "POST", {
      name,
      email: accountEmail,
      password: generatedPassword,
      role,
      all_branches: role === "admin",
      branch_ids: role === "admin" ? [] : [branches.get("BDG").id],
    });
  } catch (error) {
    try {
      const observedUsers = await api("/users");
      if (observedUsers.some((user) => user.email === accountEmail)) {
        credentials.push({ role, email: accountEmail, password: generatedPassword });
      }
    } catch {
      // Preserve credentials already confirmed by earlier successful responses.
    }
    console.error(
      JSON.stringify({ result: "PARTIAL", users: credentials, error: String(error) }, null, 2),
    );
    throw error;
  }
  credentials.push({ role, email: accountEmail, password: generatedPassword });
}

console.log(
  JSON.stringify(
    {
      result: "PASS",
      organization: me.organization_name,
      branches: ["BDG", "JKT", "SBY"].map((code) => ({ code, name: branches.get(code).name })),
      sampleAssets: sampleAssets.length,
      users: credentials,
      note: "Demo records and users were created through audited application endpoints.",
    },
    null,
    2,
  ),
);
