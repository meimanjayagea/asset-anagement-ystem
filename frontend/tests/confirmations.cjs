const path = require("node:path");
const { navigate } = require("./navigation.cjs");

module.exports = async function exercise(page, check, getLastPost, directory) {
  let nativeDialogs = 0;
  const nativeHandler = async dialog => { nativeDialogs++; await dialog.dismiss(); };
  page.on("dialog", nativeHandler);
  const dialog = page.getByRole("alertdialog");
  await navigate(page, "General Code");
  const trigger = page.getByRole("button", { name: "Archive COMPANY", exact: true });
  const lastPost = getLastPost();
  await trigger.click();
  check(await dialog.getAttribute("aria-describedby") === "confirmation-description", "confirmation describes consequences");
  check(await dialog.getByRole("button", { name: "Cancel", exact: true }).evaluate(e => e === document.activeElement), "confirmation initially focuses safe cancel");
  check(await page.evaluate(() => document.body.style.overflow === "hidden"), "confirmation locks body scrolling");
  for (let index = 0; index < 8; index++) {
    await page.keyboard.press("Tab");
    check(await dialog.evaluate(e => e.contains(document.activeElement)), "confirmation keyboard focus stays trapped");
  }
  await page.keyboard.press("Escape");
  await dialog.waitFor({ state: "hidden" });
  check(getLastPost() === lastPost, "Escape does not mutate data");
  check(await trigger.evaluate(e => e === document.activeElement), "confirmation restores trigger focus");
  check(await page.evaluate(() => document.body.style.overflow !== "hidden"), "confirmation restores body scrolling");
  for (const theme of ["light", "dark"]) {
    if (await page.locator("html").getAttribute("data-theme") !== theme) await page.getByRole("button", { name: "Theme", exact: true }).click();
    for (const width of [1440, 390, 320]) {
      await page.setViewportSize({ width, height: width === 320 ? 568 : 900 });
      await trigger.click();
      check(await dialog.evaluate(e => { const r = e.getBoundingClientRect(); return r.left >= 0 && r.right <= innerWidth + 1 && r.top >= 0 && r.bottom <= innerHeight + 1; }), `confirmation fits ${theme} ${width}`);
      check(await dialog.evaluate(e => e.scrollWidth <= e.clientWidth + 1), `confirmation content has no overflow ${theme} ${width}`);
      await page.screenshot({ path: path.join(directory, `confirmation-${theme}-${width}.png`), fullPage: true });
      await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
      await dialog.waitFor({ state: "hidden" });
    }
  }
  await page.setViewportSize({ width: 1440, height: 1040 });
  if (await page.locator("html").getAttribute("data-theme") !== "light") await page.getByRole("button", { name: "Theme", exact: true }).click();
  await trigger.click();
  await page.mouse.click(5, 5);
  await dialog.waitFor({ state: "hidden" });
  check(getLastPost() === lastPost, "backdrop and Cancel never mutate records");
  await page.getByRole("button", { name: "DEMO sample data", exact: true }).click();
  check((await dialog.textContent()).includes("company reports"), "DEMO confirmation warns about company reports");
  await dialog.getByRole("button", { name: "Add DEMO data", exact: true }).click();
  await dialog.waitFor({ state: "hidden" });
  await page.getByRole("status").filter({ hasText: "DEMO:" }).waitFor();
  check(getLastPost().body.confirmation === "SEED_DEMO_KEEP_EXISTING", "DEMO only submits after confirmation");
  for (const [menu, title] of [["Asset register", "Archive asset?"], ["Branches", "Archive branch?"], ["Locations", "Archive location?"], ["Categories", "Archive category?"], ["Team & access", "Archive account?"]]) {
    await navigate(page, menu);
    const button = page.locator(".main table").getByRole("button", { name: /^(Archive|Arsipkan)$/ }).first();
    await button.click();
    await page.getByRole("alertdialog", { name: title }).waitFor();
    await dialog.getByRole("button", { name: "Close", exact: true }).click();
    await dialog.waitFor({ state: "hidden" });
  }
  await navigate(page, "Finance & reports");
  await page.getByRole("tab", { name: "Revaluations", exact: true }).click();
  await page.getByRole("button", { name: "Reject", exact: true }).click();
  const reject = dialog.getByRole("button", { name: "Reject proposal", exact: true });
  check(await reject.isDisabled(), "rejection requires a reason");
  await dialog.getByLabel("Reason for rejection").fill("   ");
  check(await reject.isDisabled(), "whitespace rejection reason blocked");
  await dialog.getByLabel("Reason for rejection").fill("  Supporting evidence missing  ");
  await page.screenshot({ path: path.join(directory, "confirmation-rejection.png"), fullPage: true });
  await reject.click();
  await dialog.waitFor({ state: "hidden" });
  await page.getByRole("button", { name: "Reject", exact: true }).waitFor();
  check(getLastPost().path === "/api/finance/valuations/2/decision" && getLastPost().body.note === "Supporting evidence missing" && !getLastPost().body.approve, "rejection submits trimmed reason to original API");
  await page.getByRole("button", { name: "Approve", exact: true }).click();
  await dialog.getByRole("button", { name: "Approve valuation", exact: true }).click();
  await dialog.waitFor({ state: "hidden" });
  check(getLastPost().body.approve === true, "valuation approval requires explicit confirmation");
  check(nativeDialogs === 0, "no native browser confirmation or prompt");
  page.off("dialog", nativeHandler);
};
