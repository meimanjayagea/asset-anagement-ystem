const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const QRCode = require("qrcode");
const ExcelJS = require("exceljs");
const { navigate } = require("./navigation.cjs");

module.exports = async function lifecycle(page, context, baseURL, tag) {
  const request = async (endpoint, body) => {
    const response = await context.request.post(baseURL + "/api" + endpoint, {
      data: body,
      headers: { "X-Requested-With": "AssetFlow", Origin: baseURL },
    });
    assert(response.ok(), endpoint + ": " + (await response.text()));
    return response.json();
  };
  const assets = await (
    await context.request.get(baseURL + "/api/assets?search=" + tag)
  ).json();
  const registered = assets.items[0];
  assert(registered);
  const branch = await request("/branches", {
    code: "F" + Date.now(),
    name: "Field office",
    address: "Test site",
  });
  const location = await request("/locations", {
    branch_id: branch.id,
    name: "Field room",
    building: "Tower A",
    floor: "2",
    room: "201",
  });
  tag += "-FIELD";
  await request("/assets", {
    tag,
    name: "Live browser test laptop",
    category_id: registered.category_id,
    location_id: location.id,
    purchase_date: "2026-01-01",
    purchase_cost: 1000000,
    salvage_value: 0,
    useful_life_months: 48,
  });
  const asset = (
    await (
      await context.request.get(baseURL + "/api/assets?search=" + tag)
    ).json()
  ).items[0];
  const user = await request("/users", {
    name: "Field borrower",
    email: "field-" + Date.now() + "@example.test",
    password: "AssetFlow-UAT-2026",
    role: "employee",
    all_branches: false,
    branch_ids: [asset.branch_id],
  });
  const borrower = await context.browser().newContext();
  const login = await borrower.request.post(baseURL + "/api/login", {
    data: {
      org_code: "ORG-000001",
      employee_id: user.employee_id,
      password: "AssetFlow-UAT-2026",
    },
    headers: { "X-Requested-With": "AssetFlow", Origin: baseURL },
  });
  assert(login.ok());
  await navigate(page, "Asset lifecycle");
  await page
    .getByRole("button", { name: "Detail " + tag, exact: true })
    .click();
  const dialog = page.getByRole("dialog", { name: "Live browser test laptop" });
  const image = await QRCode.toBuffer(baseURL + "/?asset_tag=" + tag, {
    width: 200,
  });
  await dialog.locator("input[type=file][multiple]").setInputFiles({
    name: "asset-condition.png",
    mimeType: "image/png",
    buffer: image,
  });
  await dialog.locator(".photo-main").waitFor();
  assert(
    await dialog
      .locator(".photo-main")
      .evaluate((img) => img.complete && img.naturalWidth > 0),
    "asset image rendered",
  );
  await dialog
    .getByRole("button", { name: "Spesifikasi", exact: true })
    .click();
  await page.getByLabel("Merek", { exact: true }).fill("Lenovo");
  await page.getByLabel("Model", { exact: true }).fill("ThinkPad");
  await page.getByLabel("Tag RFID", { exact: true }).fill("RFID-" + tag);
  await page.getByRole("button", { name: "Simpan", exact: true }).click();
  await page
    .getByRole("dialog", { name: "Spesifikasi aset" })
    .waitFor({ state: "hidden" });
  const label = page.waitForEvent("download");
  await dialog
    .getByRole("button", { name: "Label QR / barcode", exact: true })
    .click();
  const labelDownload = await label;
  assert(
    (await fs.promises.readFile(await labelDownload.path()))
      .subarray(0, 5)
      .equals(Buffer.from("%PDF-")),
  );
  await dialog.getByRole("button", { name: "Tutup detail aset" }).click();
  await page
    .getByLabel("Scan gambar QR/barcode", { exact: true })
    .setInputFiles({ name: "label.png", mimeType: "image/png", buffer: image });
  await dialog.waitFor();
  await dialog.getByRole("button", { name: "Tutup detail aset" }).click();
  const borrowerResponse = await borrower.request.post(baseURL + "/api/loans", {
    data: {
      asset_id: asset.id,
      version: 2,
      due_at: new Date(Date.now() + 86400000).toISOString(),
      reason: "Site inspection",
    },
    headers: { "X-Requested-With": "AssetFlow", Origin: baseURL },
  });
  assert(borrowerResponse.ok(), await borrowerResponse.text());
  await page.getByRole("button", { name: "Peminjaman", exact: true }).click();
  await page.getByRole("button", { name: "Serahkan", exact: true }).click();
  await page
    .getByLabel("Catatan kondisi", { exact: true })
    .fill("Good before checkout");
  await page.locator(".life-form input[type=file][multiple]").setInputFiles({
    name: "before.png",
    mimeType: "image/png",
    buffer: image,
  });
  await page.locator(".life-form .evidence-thumbnails img").waitFor();
  await page.getByRole("button", { name: "Simpan", exact: true }).click();
  await page
    .getByRole("button", { name: "Terima kembali", exact: true })
    .waitFor();
  await page.getByRole("button", { name: "Notifikasi", exact: true }).click();
  await page.getByText("Pengembalian pinjaman", { exact: false }).waitFor();
  await page
    .getByRole("button", { name: "Tutup notifikasi", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Terima kembali", exact: true })
    .click();
  await page
    .getByLabel("Catatan kondisi", { exact: true })
    .fill("Good after return");
  await page
    .locator(".life-form input[type=file][multiple]")
    .setInputFiles({ name: "after.png", mimeType: "image/png", buffer: image });
  await page.locator(".life-form .evidence-thumbnails img").waitFor();
  await page.getByRole("button", { name: "Simpan", exact: true }).click();
  await page
    .locator("tr").filter({hasText:tag}).locator("td")
    .filter({ hasText: /^returned$/ })
    .waitFor();
  const proof = async (name) => {
    await page
      .locator(".life-form input[type=file][multiple]")
      .setInputFiles({
        name: name + ".png",
        mimeType: "image/png",
        buffer: image,
      });
    await page.locator(".life-form .evidence-thumbnails img").waitFor();
  };
  await page
    .getByRole("button", { name: "Servis & perbaikan", exact: true })
    .click();
  await page.getByRole("button", { name: "Work order", exact: true }).click();
  await page.getByLabel("Aset",{exact:true}).selectOption(String(asset.id));
  await page.getByLabel("Jenis",{exact:true}).selectOption("repair");
  await page.getByLabel("Judul", { exact: true }).fill("Field screen repair "+tag);
  await page
    .getByLabel("Checklist servis", { exact: true })
    .fill("Inspect screen");
  await proof("damage");
  await page.getByRole("button", { name: "Simpan", exact: true }).click();
  const workRow = page.locator("tr").filter({ hasText: "Field screen repair "+tag });
  await workRow.getByRole("button", { name: "Mulai", exact: true }).click();
  await workRow
    .getByRole("button", { name: "Selesaikan", exact: true })
    .click();
  await page
    .getByLabel("Catatan kondisi", { exact: true })
    .fill("Screen serviced");
  await page.getByLabel("Total biaya servis", { exact: true }).fill("50000");
  await page.getByLabel("Inspect screen", { exact: true }).check();
  await proof("repaired");
  await page.getByRole("button", { name: "Simpan", exact: true }).click();
  await workRow.getByText("completed", { exact: true }).waitFor();
  await page
    .getByRole("button", { name: "Opname lapangan", exact: true })
    .click();
  await page.getByRole("button", { name: "Opname", exact: true }).click();
  await page.getByLabel("Judul", { exact: true }).fill("Field physical audit "+tag);
  await page
    .getByLabel("Lokasi", { exact: true })
    .selectOption(String(location.id));
  await page.getByRole("button", { name: "Simpan", exact: true }).click();
  await page
    .locator("tr")
    .filter({ hasText: "Field physical audit "+tag })
    .getByRole("button", { name: "Rekonsiliasi", exact: true })
    .click();
  await page.getByRole("button", { name: "Verifikasi", exact: true }).click();
  assert(
    await page
      .locator(".life-form img.photo-main")
      .evaluate((img) => img.complete && img.naturalWidth > 0),
  );
  await page.getByLabel("Temuan", { exact: true }).selectOption("damaged");
  await page
    .getByLabel("Catatan", { exact: true })
    .fill("Physical finding with evidence");
  await proof("audit-damage");
  await page.getByRole("button", { name: "Simpan", exact: true }).click();
  await page
    .locator(".audit-items td")
    .filter({ hasText: /^damaged$/ })
    .waitFor();
  await page.getByRole("button", { name: "Tutup opname", exact: true }).click();
  await page.getByRole("alertdialog").getByRole("button", { name: /^(Tutup opname|Close stocktake)$/ }).click();
  await page
    .locator("tr")
    .filter({ hasText: "Field physical audit "+tag })
    .getByText("closed", { exact: true })
    .waitFor();
  await page.getByRole("button", { name: "Laporan", exact: true }).click();
  const excel = page.waitForEvent("download");
  await page.getByRole("button", { name: "Excel", exact: true }).click();
  const workbook = new ExcelJS.Workbook();
  await workbook.xlsx.readFile(await (await excel).path());
  const sheet = workbook.getWorksheet("Lifecycle");
  assert(sheet.rowCount >= 2);
  assert(sheet.getRow(1).values.includes("closing_book_value"));
  const pdf = page.waitForEvent("download");
  await page.getByRole("button", { name: "PDF", exact: true }).click();
  assert(
    (await fs.promises.readFile(await (await pdf).path()))
      .subarray(0, 5)
      .equals(Buffer.from("%PDF-")),
  );
  await page
    .getByRole("button", { name: "Katalog visual", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Detail " + tag, exact: true })
    .waitFor();
  const screenshots =
    process.env.SCREENSHOT_DIR || path.resolve(__dirname, "../../docs");
  fs.mkdirSync(screenshots, { recursive: true });
  for (const viewport of [
    { width: 1440, height: 1040 },
    { width: 390, height: 844 },
  ]) {
    await page.setViewportSize(viewport);
    await page
      .getByRole("button", { name: "Detail " + tag, exact: true })
      .click();
    await dialog.locator(".photo-main").waitFor();
    const imageBox = await dialog.locator(".photo-main").boundingBox();
    assert(imageBox.width > 100 && imageBox.height > 75);
    assert(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
      "no horizontal page overflow",
    );
    await page.screenshot({
      path: path.join(screenshots, "lifecycle-" + viewport.width + ".png"),
      fullPage: true,
    });
    await dialog.getByRole("button", { name: "Tutup detail aset" }).click();
  }
  await page.setViewportSize({ width: 1440, height: 1040 });
  await page
    .getByRole("button", { name: "Detail " + tag, exact: true })
    .click();
  await dialog.getByRole("button", { name: "Disposal", exact: true }).click();
  await page
    .getByLabel("Alasan", { exact: true })
    .fill("Retired after field inspection");
  await proof("disposal");
  await page.getByRole("button", { name: "Simpan", exact: true }).click();
  await page
    .getByRole("dialog", { name: "Pengajuan disposal" })
    .waitFor({ state: "hidden" });
  const requests = await (
    await context.request.get(
      baseURL + "/api/requests?branch_id=" + asset.branch_id,
    )
  ).json();
  const disposal = requests.find(
    (item) => item.asset_id === asset.id && item.kind === "dispose",
  );
  assert(disposal?.photo_ids?.length === 1, "disposal final evidence attached");
  const checker = await request("/users", {
    name: "Field checker",
    email: "checker-" + Date.now() + "@example.test",
    password: "AssetFlow-UAT-2026",
    role: "manager",
    all_branches: false,
    branch_ids: [asset.branch_id],
  });
  await borrower.request.post(baseURL + "/api/logout", {
    data: {},
    headers: { "X-Requested-With": "AssetFlow", Origin: baseURL },
  });
  assert(
    (
      await borrower.request.post(baseURL + "/api/login", {
        data: {
          org_code: "ORG-000001",
          employee_id: checker.employee_id,
          password: "AssetFlow-UAT-2026",
        },
        headers: { "X-Requested-With": "AssetFlow", Origin: baseURL },
      })
    ).ok(),
  );
  const approval = await borrower.request.post(
    baseURL + "/api/requests/" + disposal.id + "/decision",
    {
      data: { approve: true, note: "Approved retirement" },
      headers: { "X-Requested-With": "AssetFlow", Origin: baseURL },
    },
  );
  assert(approval.ok(), await approval.text());
  await dialog.getByRole("button",{name:"Tutup detail aset"}).click();
  await borrower.close();
  console.log(
    "PASS: real lifecycle gallery, upload, specs, QR scan/label, loans, reminders, repair/checklist, physical audit, disposal, PDF/Excel and desktop/mobile",
  );
};
