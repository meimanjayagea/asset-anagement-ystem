const { chromium } = require("playwright");
const path = require("node:path");
const { navigate } = require("./navigation.cjs");
const responsive = require("./responsive.cjs");
const setup = require("./general-setup.cjs");
const base = path.resolve(
  process.env.SCREENSHOT_DIR || path.resolve(__dirname, "../../docs"),
);
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
  await context.addInitScript(() => localStorage.setItem("assetflow-locale", "en"));
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  let role = "admin";
  const adminCapabilities = [
    "general_setup.read", "general_setup.manage", "demo.manage",
    "dashboard.read", "assets.read", "assets.write", "assets.operate",
    "assets.archive", "assets.finance", "assets.export", "requests.read",
    "requests.create", "requests.decide", "maintenance.read", "maintenance.manage",
    "stocktakes.read", "stocktakes.manage", "stocktakes.observe", "stocktakes.close",
    "branches.read", "branches.manage", "locations.read", "locations.manage",
    "locations.archive", "categories.read", "categories.manage", "categories.archive",
    "users.read", "users.manage", "users.archive", "audit.read", "activity.read",
    "assets.history", "contracts.read", "contracts.manage", "finance.read", "finance.manage",
    "valuation.propose", "valuation.decide", "reports.read", "reports.export",
  ];
  const branchAdminCapabilities = adminCapabilities.filter(
    (capability) => !capability.startsWith("general_setup.") && capability !== "demo.manage" && capability !== "branches.manage" && capability !== "users.manage_admin",
  );
  let lastPost = null;
  const setupFixtures=setup.fixtures();
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
      supplier_name: "Example supplier",
      acquisition_reference: "PO-2026-001",
      depreciation_method: "straight_line",
      depreciation_start_date: "2026-01-10",
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
      supplier_name: "Example supplier",
      acquisition_reference: "PO-2025-002",
      depreciation_method: "declining_balance",
      depreciation_start_date: "2025-01-10",
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
      supplier_name: "Example supplier",
      acquisition_reference: "PO-2024-003",
      depreciation_method: "non_depreciable",
      depreciation_start_date: "2024-01-10",
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
    const setupResponse=setupFixtures(req);
    if(setupResponse!==undefined){
      if(req.method()!=="GET")lastPost={path,body:JSON.parse(req.postData()||"{}"),method:req.method()};
      await route.fulfill({status:200,contentType:"application/json",body:JSON.stringify(setupResponse)});
      return;
    }
    let result = {};
    if (path === "/api/login/options")
      result = {
        organizations: [{
          id: 1,
          code: "ORG-000001",
          name: "Example Corp",
        }],
      };
    else if (path === "/api/user-roles")
      result = [
        { id: "admin", label: "Administrator Pusat" },
        { id: "branch_admin", label: "Administrator Cabang" },
        { id: "manager", label: "Manager" },
        { id: "operator", label: "Operator" },
        { id: "staff", label: "Staff" },
        { id: "employee", label: "Karyawan" },
        { id: "finance", label: "Finance" },
        { id: "it_support", label: "IT Support" },
        { id: "it_developer", label: "IT Developer" },
        { id: "auditor", label: "Auditor" },
      ];
    else if (req.method() === "POST" || req.method() === "PUT") {
      lastPost = { path, body: JSON.parse(req.postData() || "{}"),method:req.method() };
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
        organization_name: "Example Corp",
        all_branches: role === "admin",
        branch_ids: role === "admin" ? [] : [1],
        active_branch_id: role === "admin" ? 0 : 1,
        capabilities:
          role === "admin"
            ? adminCapabilities
            : role === "branch_admin"
              ? branchAdminCapabilities
            : [
                "dashboard.read", "assets.read", "requests.read",
                "maintenance.read", "stocktakes.read", "branches.read",
                "locations.read", "categories.read",
              ],
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
          depreciation_method: "straight_line",
          version: 1,
          branch_id: null,
          branch_name: null,
        },
        {
          id: 2,
          name: "Vehicles",
          useful_life_months: 96,
          maintenance_interval_days: 180,
          maintenance_instructions: "Routine service",
          depreciation_method: "declining_balance",
          version: 1,
          branch_id: 1,
          branch_name: "Jakarta",
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
    else if (["/api/notifications", "/api/loans", "/api/maintenance/planned", "/api/maintenance/assignees"].includes(path)) result = [];
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
    else if (path === "/api/finance/valuations")
      result = [
        {
          id: 1,
          asset_id: 1,
          name: "Lenovo ThinkPad T14",
          tag: "AST-000001",
          branch_code: "JKT",
          effective_date: "2026-10-10",
          revalued_amount: 19000000,
          carrying_value_before: 14812500,
          reason: "Presentation appraisal",
          status: "pending",
          requested_by: 1,
        },
        {
          id: 2,
          asset_id: 2,
          name: "Toyota Avanza Operational",
          tag: "AST-000002",
          branch_code: "BDG",
          effective_date: "2026-10-10",
          revalued_amount: 210000000,
          carrying_value_before: 196250000,
          reason: "Independent appraisal",
          status: "pending",
          requested_by: 2,
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
      body: JSON.stringify(Array.isArray(result)?result.map(r=>({...r,version:r.version||1})):result),
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
  await page.locator("header .appearance-tools select").selectOption("id");
  await page.getByRole("button", { name: "Daftar aset", exact: true }).waitFor();
  await page.locator("header .appearance-tools select").selectOption("en");
  await page.getByRole("button", { name: "Asset register", exact: true }).waitFor();
  await page.getByRole("button", { name: "Theme" }).click();
  check((await page.locator("html").getAttribute("data-theme")) === "dark", "dark theme toggle");
  const darkLabelContrast = await page.locator(".kpi.featured > span").evaluate((element) => {
    const luminance = (color) => color.match(/\d+/g).slice(0, 3).map((channel) => {
      const value = Number(channel) / 255;
      return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
    }).reduce((total, channel, index) => total + channel * [0.2126, 0.7152, 0.0722][index], 0);
    const foreground = luminance(getComputedStyle(element).color);
    const background = luminance(getComputedStyle(element.parentElement).backgroundColor);
    return (Math.max(foreground, background) + 0.05) / (Math.min(foreground, background) + 0.05);
  });
  check(darkLabelContrast >= 4.5, "dark theme dashboard label contrast");
  await page.screenshot({ path: base + "/dashboard-dark-demo.png", fullPage: true });
  await page.getByRole("button", { name: "Theme" }).click();
  check((await page.locator("html").getAttribute("data-theme")) === "light", "light theme restores");
  await page.screenshot({ path: base + "/dashboard-demo.png", fullPage: true });
  check(
    (await page.getByText(/270,500,000/).count()) === 1,
    "value rendering",
  );
  await navigate(page, "Asset register");
  await page
    .getByRole("button", { name: "Register asset", exact: true })
    .waitFor();
  await page.getByRole("button", { name: "Assign", exact: true }).click();
  await page.getByLabel("Custodian name / ID").fill("EMP-100 / Arya");
  await page.getByRole("button", { name: "Save & confirm" }).click();
  await page.getByText("Changes saved successfully").waitFor();
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
      .getByLabel("Acquisition cost (IDR)", { exact: true })
      .getAttribute("step")) === "1",
    "integer monetary input",
  );
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await page.locator(".loading").waitFor({ state: "hidden" });
  await page.screenshot({
    path: base + "/asset-register-demo.png",
    fullPage: true,
  });
  const downloadPromise = page.waitForEvent("download");
  await page
    .getByRole("button", { name: "Export page CSV", exact: true })
    .click();
  await downloadPromise;
  check(
    lastPost.path === "/api/exports/assets" && lastPost.body.size === 25,
    "audited server export contract",
  );
  await navigate(page, /Approvals/);
  await page.getByRole("button", { name: "Approve", exact: true }).click();
  await page.getByLabel("Decision note").fill("Reviewed");
  await page.getByRole("button", { name: "Save & confirm" }).click();
  check(
    lastPost.path === "/api/requests/1/decision" && lastPost.body.approve,
    "approval contract",
  );
  await navigate(page, "Stocktake");
  await page.getByRole("button", { name: "Snapshot", exact: true }).click();
  check(
    (await page.getByText("Stocktake snapshot", { exact: true }).count()) === 1,
    "stocktake dialog",
  );
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await page.getByRole("button", { name: "Observe tag", exact: true }).click();
  await page.getByLabel("Asset tag", { exact: true }).fill("AST-000001");
  await page.getByRole("button", { name: "Save & confirm" }).click();
  check(
    lastPost.body.tag === "AST-000001" &&
      lastPost.path === "/api/stocktakes/1/observe",
    "stocktake contract",
  );
  await navigate(page, "Team & access");
  await page.getByRole("button", { name: "Edit access", exact: true }).click();
  await page.getByRole("dialog").locator("select").selectOption("auditor");
  await page.getByLabel("Active account").uncheck();
  await page.getByRole("button", { name: "Save & confirm" }).click();
  check(
    lastPost.body.role === "auditor" && !lastPost.body.active,
    "access contract",
  );
  check(
    lastPost.body.all_branches === false &&
      lastPost.body.branch_ids.includes(2),
    "existing user branch assignment preserved",
  );
  await navigate(page, "Branches");
  await page.getByRole("button", { name: "Branch", exact: true }).click();
  await page.getByLabel("Code", { exact: true }).fill("SBY");
  await page.getByLabel("Name", { exact: true }).fill("Surabaya");
  await page.getByRole("button", { name: "Save & confirm" }).click();
  check(
    lastPost.path === "/api/branches" && lastPost.body.code === "SBY",
    "branch creation contract",
  );
  await navigate(page, "Categories");
  await page
    .getByRole("button", { name: "Edit policy", exact: true })
    .first()
    .click();
  await page.getByLabel("Maintenance interval (days)").fill("120");
  await page.getByRole("button", { name: "Save & confirm" }).click();
  check(
    lastPost.path === "/api/categories/1/policy" &&
      lastPost.body.maintenance_interval_days === 120 &&
      lastPost.body.version === 1,
    "category policy contract",
  );
  await navigate(page, "User activity");
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
  await navigate(page, "Categories");
  await heldResponse;
  await navigate(page, "User activity");
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
  await navigate(page, "Finance & reports");
  await page.getByRole("tab", { name: "Revaluations" }).click();
  await page.getByText("Independent appraisal", { exact: true }).waitFor();
  check(
    (await page.getByRole("button", { name: "Approve", exact: true }).count()) === 1,
    "valuation proposer cannot approve their own request",
  );
  await page.getByRole("button", { name: "Propose valuation", exact: true }).click();
  await page.locator(".finance-form select").selectOption("1");
  await page.getByLabel("New value (IDR)").fill("19500000");
  await page.getByLabel("Reason", { exact: true }).fill("Presentation condition review");
  await page.getByRole("button", { name: "Propose valuation", exact: true }).last().click();
  check(
    lastPost.path === "/api/finance/valuations" &&
      lastPost.body.revalued_amount === 19500000 &&
      lastPost.body.asset_id === 1,
    "valuation proposal contract",
  );
  await page.locator(".loading").waitFor({ state: "hidden" });
  await page.locator(".toast").waitFor({ state: "hidden" });
  await page.screenshot({ path: base + "/activity-demo.png", fullPage: true });
  await navigate(page, "Asset register");
  await page.getByLabel("Branch filter").selectOption("1");
  await page.locator(".loading").waitFor({ state: "hidden" });
  check(
    (await page
      .getByText("Toyota Avanza Operational", { exact: true })
      .count()) === 0,
    "branch-filter rendering",
  );
  await page.getByLabel("Branch filter").selectOption("0");
  await setup.exercise(page,check,()=>lastPost,base);
  await responsive(page, check, base);
  await page.setViewportSize({ width: 390, height: 844 });
  await navigate(page, "Overview");
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
  await navigate(page, "Asset register");
  await page.getByRole("button", { name: "Open menu", exact: true }).click();
  check(await page.locator(".nav-group-toggle").filter({ hasText: "Control & access" }).count() === 0, "empty permission group hidden");
  check(await page.locator('.app-navigation .nav-item').filter({ hasText: "Team & access" }).count() === 0, "unauthorized navigation item not rendered");
  await page.getByRole("button", { name: "Close menu", exact: true }).click();
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
    (await page.getByLabel("Branch filter").count()) === 0,
    "single-branch filter hidden",
  );
  role = "branch_admin";
  await page.reload();
  await page.getByRole("heading", { name: "Overview", exact: true }).waitFor();
  await navigate(page, "Categories");
  await page.getByText("Vehicles", { exact: true }).waitFor();
  check(
    (await page.getByRole("button", { name: "Category", exact: true }).count()) === 1,
    "branch admin can create local categories",
  );
  check(
    (await page.getByRole("button", { name: "Edit policy", exact: true }).count()) === 1,
    "branch admin can edit local policy but not shared catalog",
  );
  await page.getByRole("button", { name: "Category", exact: true }).click();
  await page.getByLabel("Name", { exact: true }).fill("Branch-only devices");
  await page.getByRole("button", { name: "Save & confirm" }).click();
  check(
    lastPost.path === "/api/categories" && lastPost.body.branch_id === 1,
    "branch category creation carries local scope",
  );
  await navigate(page, "Asset register");
  await page.getByText("Lenovo ThinkPad T14", { exact: true }).waitFor();
  await page.getByRole("button", { name: "Edit", exact: true }).first().click();
  await page.getByLabel("Name", { exact: true }).fill("Branch-updated laptop");
  await page.getByRole("button", { name: "Save & confirm" }).click();
  check(
    lastPost.path === "/api/assets/1" && lastPost.body.version === 1,
    "branch asset edit sends version guard",
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
