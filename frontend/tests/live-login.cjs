const assert = require("node:assert/strict");
const fs = require("node:fs");
const http = require("node:http");
const path = require("node:path");
const { chromium } = require("playwright");

(async () => {
  if (!process.env.TEST_LOGIN_PASSWORD)
    throw new Error(
      "TEST_LOGIN_PASSWORD is required for a local/staging test account",
    );
  const dist = path.resolve(__dirname, "../dist");
  const api = process.env.TEST_API_URL || "http://127.0.0.1:18088";
  const server = http.createServer(async (req, res) => {
    try {
      const pathname = new URL(req.url, "http://localhost").pathname;
      if (pathname.startsWith("/api/") || pathname.startsWith("/health/")) {
        const chunks = [];
        for await (const chunk of req) chunks.push(chunk);
        const headers = { ...req.headers };
        delete headers.host;
        const response = await fetch(api + req.url, {
          method: req.method,
          headers,
          body: ["GET", "HEAD"].includes(req.method)
            ? undefined
            : Buffer.concat(chunks),
          signal: AbortSignal.timeout(15000),
        });
        res.statusCode = response.status;
        for (const [key, value] of response.headers) {
          if (
            ![
              "set-cookie",
              "content-length",
              "content-encoding",
              "transfer-encoding",
            ].includes(key)
          )
            res.setHeader(key, value);
        }
        const cookies = response.headers.getSetCookie();
        if (cookies.length) res.setHeader("Set-Cookie", cookies);
        return res.end(Buffer.from(await response.arrayBuffer()));
      }
      const file = path.resolve(
        dist,
        "." + (pathname === "/" ? "/index.html" : pathname),
      );
      if (!file.startsWith(dist + path.sep) || !fs.existsSync(file))
        return res.writeHead(404).end();
      res.setHeader(
        "Content-Type",
        file.endsWith(".js")
          ? "text/javascript"
          : file.endsWith(".css")
            ? "text/css"
            : "text/html",
      );
      res.end(fs.readFileSync(file));
    } catch {
      res.writeHead(502).end("Test proxy unavailable");
    }
  });
  await new Promise((resolve) => server.listen(18089, "127.0.0.1", resolve));
  let browser;
  try {
    browser = await chromium.launch({
      headless: true,
      ...(process.env.CHROMIUM_PATH
        ? { executablePath: process.env.CHROMIUM_PATH }
        : {}),
    });
    const context = await browser.newContext();
    await context.addInitScript(() =>
      localStorage.setItem("assetflow-locale", "en"),
    );
    const page = await context.newPage();
    const runtimeErrors = [];
    page.on("pageerror", (err) => runtimeErrors.push(err.message));
    await page.goto("http://127.0.0.1:18089");
    await page
      .getByLabel("Organization code", { exact: true })
      .fill(process.env.TEST_ORG_CODE || "ORG-000001");
    await page
      .getByLabel("Employee ID", { exact: true })
      .fill(process.env.TEST_EMPLOYEE_ID || "EMP-1");
    await page.getByLabel("Password", { exact: true }).fill("wrong-password");
    await page.getByRole("button", { name: "Sign in", exact: true }).click();
    await page
      .getByRole("alert")
      .filter({ hasText: "Kredensial tidak valid" })
      .waitFor();
    await page
      .getByLabel("Password", { exact: true })
      .fill(process.env.TEST_LOGIN_PASSWORD);
    await page.getByRole("button", { name: "Sign in", exact: true }).click();
    await page
      .getByRole("heading", { name: "Overview", exact: true })
      .waitFor();
    const cookies = await context.cookies();
    assert(
      cookies.some(
        (cookie) =>
          cookie.name === "assetflow_session" &&
          cookie.httpOnly &&
          cookie.sameSite === "Strict",
      ),
    );
    await page.reload();
    await page
      .getByRole("heading", { name: "Overview", exact: true })
      .waitFor();
    await page
      .getByRole("button", { name: "Asset register", exact: true })
      .click();
    await page
      .getByRole("button", { name: "Register asset", exact: true })
      .click();
    const tag = "LIVE-" + Date.now();
    await page.getByLabel("Asset tag", { exact: true }).fill(tag);
    await page
      .getByLabel("Name", { exact: true })
      .fill("Live browser test laptop");
    await page
      .getByLabel("Acquisition date", { exact: true })
      .fill("2026-01-01");
    await page
      .getByLabel("Depreciation starts", { exact: true })
      .fill("2026-01-01");
    await page
      .getByLabel("Acquisition cost (IDR)", { exact: true })
      .fill("1000000");
    await page
      .getByRole("button", { name: "Save & confirm", exact: true })
      .click();
    await page.getByText(tag, { exact: false }).waitFor();
    await page.reload();
    await page
      .getByRole("button", { name: "Asset register", exact: true })
      .click();
    await page.getByText(tag, { exact: false }).waitFor();
    try {
      await require("./lifecycle.cjs")(
        page,
        context,
        "http://127.0.0.1:18089",
        tag,
      );
    } catch (error) {
      await page.screenshot({
        path: path.join(
          process.env.SCREENSHOT_DIR || path.resolve(__dirname, "../../docs"),
          "lifecycle-failure.png",
        ),
        fullPage: true,
      });
      console.error(
        await page
          .locator(".life-form")
          .innerText({timeout:1000})
          .catch(() => "No lifecycle form"),
      );
      throw error;
    }
    await page.getByRole("button", { name: "Logout", exact: true }).click();
    await page.getByLabel("Password", { exact: true }).waitFor();
    const session = await context.request.get("http://127.0.0.1:18089/api/me");
    assert.equal(session.status(), 401);
    assert.deepEqual(runtimeErrors, []);
    console.log(
      "PASS: live PostgreSQL login, invalid password, session cookie, refresh, asset registration, persistence and logout",
    );
  } finally {
    if (browser) await browser.close();
    await new Promise((resolve) => server.close(resolve));
  }
})().catch((err) => {
  console.error(err);
  process.exitCode = 1;
});
