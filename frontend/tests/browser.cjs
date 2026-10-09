const { chromium } = require("playwright");
const path = require("node:path");
const base = path.resolve(__dirname, "../../docs");
const baseURL = process.env.TEST_BASE_URL || "http://127.0.0.1:5173";
(async () => {
  const browser = await chromium.launch({
    ...(process.env.CHROMIUM_PATH
      ? { executablePath: process.env.CHROMIUM_PATH }
      : {}),
    headless: true,
    args: ["--no-sandbox"],
  });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 1040 },
  });
  const page = await context.newPage();
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  let role = "admin";
  let lastPost = null;
  let holdPath = "",
    onResponseHeld,
    releaseResponse;
  const assets = [
    {
      id: 1,
      tag: "AST-000001",
      name: "Lenovo ThinkPad T14",
      serial_number: "SN-001",
      status: "available",
      category_name: "IT Equipment",
      category_id: 1,
      location_name: "Jakarta · Headquarters",
      location_id: 1,
      custodian: "",
      purchase_date: "2026-01-10",
      purchase_cost: 18000000,
      salvage_value: 1000000,
      book_value: 14812500,
      useful_life_months: 48,
      warranty_until: "2029-01-10",
      version: 1,
    },
    {
      id: 2,
      tag: "AST-000002",
      name: "Toyota Avanza Operational",
      serial_number: "SN-002",
      status: "assigned",
      category_name: "Vehicles",
      category_id: 2,
      location_name: "Bandung · Branch",
      location_id: 2,
      custodian: "EMP-002 / Budi",
      purchase_date: "2025-01-10",
      purchase_cost: 240000000,
      salvage_value: 40000000,
      book_value: 196250000,
      useful_life_months: 96,
      warranty_until: null,
      version: 2,
    },
    {
      id: 3,
      tag: "AST-000003",
      name: "Daikin AC 2 PK",
      serial_number: "SN-003",
      status: "maintenance",
      category_name: "Building Equipment",
      category_id: 3,
      location_name: "Jakarta · Headquarters",
      location_id: 1,
      custodian: "",
      purchase_date: "2024-01-10",
      purchase_cost: 12500000,
      salvage_value: 500000,
      book_value: 7000000,
      useful_life_months: 60,
      warranty_until: null,
      version: 3,
    },
  ];
  assets.forEach((a) => {
    a.branch_id = a.location_id;
    a.branch_name = a.location_id === 1 ? "Jakarta" : "Bandung";
    a.next_maintenance_date = "2026-10-10";
  });
  await page.route("**/api/**", async (route) => {
    const req = route.request();
    const path = new URL(req.url()).pathname;
    let result = {};
    if (req.method() === "POST") {
      lastPost = { path, body: JSON.parse(req.postData() || "{}") };
      if (path === "/api/exports/assets") {
        await route.fulfill({
          status: 200,
          contentType: "text/csv",
          body: "tag,name\r\nAST-000001,Laptop\r\n",
        });
        return;
      }
      result = { ok: true };
    } else if (path.endsWith("/me"))
      result = {
        id: 1,
        org_id: 1,
        name: "Arya · Demo",
        email: "demo@example.com",
        role,
        all_branches: role !== "auditor",
        branch_ids: role === "auditor" ? [1] : [],
      };
    else if (path.endsWith("/dashboard"))
      result = {
        total: 3,
        available: 1,
        assigned: 1,
        maintenance: 1,
        disposed: 0,
        purchase_value: 270500000,
        pending_requests: 1,
        overdue_maintenance: 1,
        maintenance_due_assets: 2,
      };
    else if (path.endsWith("/assets")) {
      const branch = Number(
        new URL(req.url()).searchParams.get("branch_id") || 0,
      );
      const scoped = assets.filter(
        (a) =>
          (!branch || a.branch_id === branch) &&
          (role !== "auditor" || a.branch_id === 1),
      );
      result = { items: scoped, total: scoped.length, page: 1, size: 25 };
    } else if (path.endsWith("/branches"))
      result =
        role === "auditor"
          ? [{ id: 1, code: "JKT", name: "Jakarta", address: "HQ" }]
          : [
              { id: 1, code: "JKT", name: "Jakarta", address: "HQ" },
              { id: 2, code: "BDG", name: "Bandung", address: "Branch" },
            ];
    else if (path.endsWith("/activity"))
      result = [
        {
          id: 1,
          actor_name: "Operator",
          event: "login_success",
          method: "POST",
          path: "/api/login",
          status_code: 200,
          duration_ms: 5,
          request_id: "demo-r1",
          peer_ip: "127.0.0.1",
          user_agent: "Demo browser",
          created_at: "2026-10-07T10:00:00Z",
        },
      ];
    else if (path.endsWith("/locations"))
      result = [
        {
          id: 1,
          name: "Headquarters",
          branch_id: 1,
          branch_name: "Jakarta",
          branch_code: "JKT",
        },
        {
          id: 2,
          name: "Office",
          branch_id: 2,
          branch_name: "Bandung",
          branch_code: "BDG",
        },
      ];
    else if (path.endsWith("/categories"))
      result = [
        {
          id: 1,
          name: "IT Equipment",
          useful_life_months: 48,
          maintenance_interval_days: 90,
          maintenance_instructions: "Inspect battery",
          version: 1,
        },
        {
          id: 2,
          name: "Vehicles",
          useful_life_months: 96,
          maintenance_interval_days: 180,
          maintenance_instructions: "Routine service",
          version: 1,
        },
      ];
    else if (path.endsWith("/requests"))
      result = [
        {
          id: 1,
          asset_id: 1,
          name: "Lenovo ThinkPad T14",
          tag: "AST-000001",
          kind: "transfer",
          reason: "Penempatan tim cabang",
          status: "pending",
          target_location: "Bandung · Branch",
          requester: "Operator",
          requested_by: 3,
          created_at: "2026-10-07T09:00:00Z",
        },
      ];
    else if (path.endsWith("/maintenance"))
      result = [
        {
          id: 1,
          asset_id: 3,
          tag: "AST-000003",
          name: "Daikin AC 2 PK",
          title: "Preventive service",
          due_date: "2026-10-01",
          status: "in_progress",
          cost: 0,
          asset_version: 3,
        },
      ];
    else if (path.endsWith("/stocktakes"))
      result = [
        {
          id: 1,
          title: "Quarterly HQ inspection",
          status: "open",
          location_name: "Jakarta · Headquarters",
          expected: 2,
          observed: 1,
          missing: 1,
        },
      ];
    else if (path.endsWith("/items"))
      result = [
        {
          asset_id: 1,
          expected_tag: "AST-000001",
          name: "ThinkPad",
          observed: false,
          notes: "",
          expected_version: 1,
          current_version: 1,
        },
      ];
    else if (path.endsWith("/audit"))
      result = [
        {
          id: 1,
          actor: "Operator",
          action: "create",
          entity: "asset",
          entity_id: 1,
          before_data: null,
          after_data: { tag: "AST-000001" },
          created_at: "2026-10-07T09:00:00Z",
        },
      ];
    else if (path.endsWith("/users"))
      result = [
        {
          id: 1,
          name: "Arya",
          email: "admin@example.com",
          role: "admin",
          active: true,
          all_branches: true,
          branch_ids: [],
        },
        {
          id: 2,
          name: "Manager",
          email: "manager@example.com",
          role: "manager",
          active: true,
          all_branches: false,
          branch_ids: [2],
        },
      ];
    if (path === holdPath) {
      holdPath = "";
      await new Promise((resolve) => {
        releaseResponse = resolve;
        onResponseHeld();
      });
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(result),
    });
  });
  let checks = 0;
  function check(x, s) {
    if (!x) throw Error(s);
    checks++;
  }
  await page.goto(baseURL);
  await page.getByRole("heading", { name: "Overview", exact: true }).waitFor();
  await page.locator(".loading").waitFor({ state: "hidden" });
  await page.screenshot({ path: base + "/dashboard-demo.png", fullPage: true });
  check(
    (await page.getByText("Rp 270.500.000", { exact: true }).count()) === 1,
    "value rendering",
  );
  await page
    .getByRole("button", { name: "Asset register", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Register asset", exact: true })
    .waitFor();
  await page.getByRole("button", { name: "Assign", exact: true }).click();
  await page.getByLabel("Nama / ID penanggung jawab").fill("EMP-100 / Arya");
  await page.getByRole("button", { name: "Simpan & konfirmasi" }).click();
  await page.getByText("Perubahan berhasil disimpan").waitFor();
  check(
    lastPost.path === "/api/assets/1/action" &&
      lastPost.body.version === 1 &&
      lastPost.body.custodian === "EMP-100 / Arya",
    "assign contract",
  );
  await page
    .getByRole("button", { name: "Register asset", exact: true })
    .click();
  check((await page.getByLabel("Asset tag").count()) === 1, "register form");
  check(
    (await page
      .getByLabel("Purchase cost (Rp)", { exact: true })
      .getAttribute("step")) === "1",
    "integer monetary input",
  );
  await page.getByRole("button", { name: "Batal", exact: true }).click();
  await page.locator(".loading").waitFor({ state: "hidden" });
  await page.screenshot({
    path: base + "/asset-register-demo.png",
    fullPage: true,
  });
  const downloadPromise = page.waitForEvent("download");
  await page
    .getByRole("button", { name: "Export halaman CSV", exact: true })
    .click();
  await downloadPromise;
  check(
    lastPost.path === "/api/exports/assets" && lastPost.body.size === 25,
    "audited server export contract",
  );
  await page.getByRole("button", { name: /Approvals/ }).click();
  await page.getByRole("button", { name: "Approve", exact: true }).click();
  await page.getByLabel("Decision note").fill("Reviewed");
  await page.getByRole("button", { name: "Simpan & konfirmasi" }).click();
  check(
    lastPost.path === "/api/requests/1/decision" && lastPost.body.approve,
    "approval contract",
  );
  await page
    .getByRole("button", { name: "Stocktake", exact: true })
    .first()
    .click();
  await page.getByRole("button", { name: "Snapshot", exact: true }).click();
  check(
    (await page.getByText("Stocktake snapshot", { exact: true }).count()) === 1,
    "stocktake dialog",
  );
  await page.getByRole("button", { name: "Batal", exact: true }).click();
  await page.getByRole("button", { name: "Observe tag", exact: true }).click();
  await page.getByLabel("Asset tag", { exact: true }).fill("AST-000001");
  await page.getByRole("button", { name: "Simpan & konfirmasi" }).click();
  check(
    lastPost.body.tag === "AST-000001" &&
      lastPost.path === "/api/stocktakes/1/observe",
    "stocktake contract",
  );
  await page
    .getByRole("button", { name: "Team & access", exact: true })
    .click();
  await page.getByRole("button", { name: "Edit access", exact: true }).click();
  await page.getByRole("dialog").locator("select").selectOption("auditor");
  await page.getByLabel("Active account").uncheck();
  await page.getByRole("button", { name: "Simpan & konfirmasi" }).click();
  check(
    lastPost.body.role === "auditor" && !lastPost.body.active,
    "access contract",
  );
  check(
    lastPost.body.all_branches === false &&
      lastPost.body.branch_ids.includes(2),
    "existing user branch assignment preserved",
  );
  await page.getByRole("button", { name: "Branches", exact: true }).click();
  await page.getByRole("button", { name: "Cabang", exact: true }).click();
  await page.getByLabel("Code", { exact: true }).fill("SBY");
  await page.getByLabel("Name", { exact: true }).fill("Surabaya");
  await page.getByRole("button", { name: "Simpan & konfirmasi" }).click();
  check(
    lastPost.path === "/api/branches" && lastPost.body.code === "SBY",
    "branch creation contract",
  );
  await page.getByRole("button", { name: "Categories", exact: true }).click();
  await page
    .getByRole("button", { name: "Edit policy", exact: true })
    .first()
    .click();
  await page.getByLabel("Maintenance interval (days)").fill("120");
  await page.getByRole("button", { name: "Simpan & konfirmasi" }).click();
  check(
    lastPost.path === "/api/categories/1/policy" &&
      lastPost.body.maintenance_interval_days === 120 &&
      lastPost.body.version === 1,
    "category policy contract",
  );
  await page
    .getByRole("button", { name: "User activity", exact: true })
    .click();
  await page
    .locator("table")
    .getByText("login_success", { exact: true })
    .waitFor();
  check(
    (await page
      .locator("table")
      .getByText("login_success", { exact: true })
      .count()) === 1,
    "activity view",
  );
  holdPath = "/api/categories";
  const heldResponse = new Promise((resolve) => {
    onResponseHeld = resolve;
  });
  await page.getByRole("button", { name: "Categories", exact: true }).click();
  await heldResponse;
  await page
    .getByRole("button", { name: "User activity", exact: true })
    .click();
  await page
    .locator("table")
    .getByText("login_success", { exact: true })
    .waitFor();
  const staleCategoriesResponse = page.waitForResponse(
    (response) => new URL(response.url()).pathname === "/api/categories",
  );
  releaseResponse();
  await staleCategoriesResponse;
  await page.waitForLoadState("networkidle");
  check(
    (await page
      .locator("table")
      .getByText("login_success", { exact: true })
      .count()) === 1 &&
      (await page.getByText("IT Equipment", { exact: true }).count()) === 0,
    "stale response cannot overwrite navigation",
  );
  await page.locator(".loading").waitFor({ state: "hidden" });
  await page.locator(".toast").waitFor({ state: "hidden" });
  await page.screenshot({ path: base + "/activity-demo.png", fullPage: true });
  await page
    .getByRole("button", { name: "Asset register", exact: true })
    .click();
  await page.getByLabel("Filter cabang").selectOption("1");
  await page.locator(".loading").waitFor({ state: "hidden" });
  check(
    (await page
      .getByText("Toyota Avanza Operational", { exact: true })
      .count()) === 0,
    "branch-filter rendering",
  );
  await page.getByLabel("Filter cabang").selectOption("0");
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "Overview", exact: true }).click();
  await page.locator(".loading").waitFor({ state: "hidden" });
  await page.screenshot({ path: base + "/mobile-demo.png", fullPage: true });
  check(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
    "no mobile page overflow",
  );
  role = "auditor";
  await page.reload();
  await page.getByRole("heading", { name: "Overview", exact: true }).waitFor();
  await page
    .getByRole("button", { name: "Asset register", exact: true })
    .click();
  check(
    (await page
      .getByRole("button", { name: "Register asset", exact: true })
      .count()) === 0,
    "auditor create hidden",
  );
  check(
    (await page
      .getByRole("button", { name: "Assign", exact: true })
      .count()) === 0,
    "auditor write hidden",
  );
  check(
    (await page
      .getByLabel("Filter cabang")
      .locator('option[value="2"]')
      .count()) === 0,
    "auditor foreign branch hidden",
  );
  check(errors.length === 0, "no runtime errors");
  console.log(
    JSON.stringify({
      checks,
      errors,
      note: "Browser contract tests use mocked API fixtures; screenshots contain demo data.",
    }),
  );
  await browser.close();
})().catch((e) => {
  console.error(e);
  process.exit(1);
});
