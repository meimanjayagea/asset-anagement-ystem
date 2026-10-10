const fs = require("node:fs");
const path = require("node:path");
const http = require("node:http");
const { spawn } = require("node:child_process");

(async () => {
  const dist = path.resolve(__dirname, "../dist");
  const server = http.createServer((req, res) => {
    const pathname = new URL(req.url, "http://localhost").pathname;
    const file = path.resolve(dist, "." + (pathname === "/" ? "/index.html" : pathname));
    if (!file.startsWith(dist + path.sep) || !fs.existsSync(file)) return res.writeHead(404).end();
    res.setHeader("Content-Type", file.endsWith(".js") ? "text/javascript" : file.endsWith(".css") ? "text/css" : "text/html");
    res.end(fs.readFileSync(file));
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  try {
    const code = await new Promise((resolve, reject) => {
      const child = spawn(process.execPath, [path.join(__dirname, "browser.cjs")], {
        stdio: "inherit", windowsHide: true,
        env: { ...process.env, TEST_BASE_URL: `http://127.0.0.1:${server.address().port}` },
      });
      child.once("error", reject);
      child.once("close", resolve);
    });
    if (code !== 0) throw new Error(`Browser contracts failed: ${code}`);
  } finally {
    await new Promise((resolve) => server.close(resolve));
  }
})().catch((err) => { console.error(err); process.exitCode = 1; });
