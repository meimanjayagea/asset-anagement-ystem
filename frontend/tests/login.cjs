const assert = require("node:assert/strict");
const fs = require("node:fs");
const http = require("node:http");
const path = require("node:path");
const { chromium } = require("playwright");

(async () => {
  const dist = path.resolve(__dirname, "../dist");
  const server = http.createServer((req, res) => {
    const pathname = new URL(req.url, "http://localhost").pathname;
    const file = path.resolve(dist, "." + (pathname === "/" ? "/index.html" : pathname));
    if (!file.startsWith(dist + path.sep) || !fs.existsSync(file)) {
      res.writeHead(404).end();
      return;
    }
    res.setHeader("Content-Type", file.endsWith(".js") ? "text/javascript" : file.endsWith(".css") ? "text/css" : "text/html");
    res.end(fs.readFileSync(file));
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  let browser;
  try {
    browser = await chromium.launch({ headless: true, ...(process.env.CHROMIUM_PATH ? { executablePath: process.env.CHROMIUM_PATH } : {}) });
    for (const width of [390, 1440]) {
      const page = await browser.newPage({ viewport: { width, height: 900 } });
      const errors = [];
      page.on("pageerror", (err) => errors.push(err.message));
      let failure = "audit";
      await page.route("**/api/**", async (route) => {
        if (new URL(route.request().url()).pathname === "/api/login/options") {
          if (failure === "html") return route.fulfill({ status: 502, contentType: "text/html", body: "Bad gateway" });
          if (failure === "audit") return route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: "Audit aktivitas tidak tersedia; refresh untuk memastikan hasil aksi" }) });
          return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ organizations: [{ id: 1, code: "ORG-000001", name: "Example Corp" }] }) });
        }
        return route.fulfill({ status: 401, contentType: "application/json", body: JSON.stringify({ error: "Unauthenticated" }) });
      });
      const url = `http://127.0.0.1:${server.address().port}`;
      await page.goto(url);
      await page.getByText("Audit aktivitas tidak tersedia; refresh untuk memastikan hasil aksi", { exact: true }).waitFor();
      failure = "html";
      await page.reload();
      await page.getByText("Layanan tidak tersedia. Coba lagi nanti.", { exact: true }).waitFor();
      failure = "";
      await page.reload();
      await page.locator('input[type="password"]').waitFor();
      assert.equal(await page.getByText("Unauthenticated", { exact: true }).count(), 0);
      assert.deepEqual(errors, []);
      await page.close();
      console.log(`PASS: login initialization and service errors at ${width}px`);
    }
  } finally {
    if (browser) await browser.close();
    await new Promise((resolve) => server.close(resolve));
  }
})().catch((err) => { console.error(err); process.exitCode = 1; });
