import fs from "node:fs";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../", import.meta.url));
const origin = process.env.TEST_PRODUCTION_ORIGIN || "https://frontend-iota-five-52.vercel.app";
const source = fs.readFileSync(root + "backend/internal/app/server.go", "utf8");
const paths = [...source.matchAll(/(?:HandleFunc|add)\("GET ([^"\s]+)"/g)].map((match) => match[1].replaceAll("{id}", "1"));
const results = [];
for (const path of paths) {
  const expected = path.startsWith("/health/") || path === "/api/login/options" ? 200 : 401;
  try {
    const response = await fetch(origin + path, { signal: AbortSignal.timeout(15000), redirect: "error" });
    const body = await response.text();
    results.push({ path, expected, status: response.status, pass: response.status === expected, request_id: response.headers.get("x-request-id"), body: body.slice(0, 300) });
  } catch (err) {
    results.push({ path, expected, pass: false, error: err.message });
  }
}
const report = { checked_at: new Date().toISOString(), origin, scope: "Unauthenticated GET probes only; authenticated production workflows were not tested", passed: results.filter((r) => r.pass).length, total: results.length, results };
fs.writeFileSync(root + "docs/production-api-smoke.json", JSON.stringify(report, null, 2) + "\n");
console.log(JSON.stringify({ passed: report.passed, total: report.total, statuses: [...new Set(results.map((r) => r.status))], report: "docs/production-api-smoke.json" }));
process.exitCode = report.passed === report.total ? 0 : 1;
