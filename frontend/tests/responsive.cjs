const { navigate } = require("./navigation.cjs");

module.exports = async function responsive(page, check, directory) {
  const groups = ["Assets & inventory", "Operations", "Master data", "Control & access"];
  const noOverflow = async message => check(await page.evaluate(() =>
    document.documentElement.scrollWidth <= innerWidth), message);
  for (const viewport of [
    { width: 320, height: 740 }, { width: 390, height: 844 },
    { width: 768, height: 900 }, { width: 960, height: 640 },
    { width: 1024, height: 600 }, { width: 1440, height: 900 },
    { width: 844, height: 390 },
  ]) {
    await page.setViewportSize(viewport);
    await navigate(page, "Overview");
    await page.locator(".loading").waitFor({ state: "hidden" });
    await noOverflow(`dashboard fits ${viewport.width}x${viewport.height}`);
    const drawer = viewport.width <= 960;
    const openMenu = page.getByRole("button", { name: "Open menu", exact: true });
    if (drawer) {
      check(!(await page.locator(".app-sidebar").isVisible()), "mobile menu starts closed");
      await openMenu.click();
      await page.getByRole("dialog", { name: "Main navigation" }).waitFor();
      check(await page.locator(".main").getAttribute("inert") !== null, "drawer background inert");
      check(await page.evaluate(() => document.body.style.overflow === "hidden"), "drawer locks body scroll");
      const close = page.getByRole("button", { name: "Close menu", exact: true });
      await close.focus();
      await page.keyboard.press("Shift+Tab");
      check(await page.locator(".app-sidebar").evaluate(e => e.contains(document.activeElement)), "drawer traps reverse tab");
      await page.keyboard.press("Tab");
      check(await close.evaluate(e => e === document.activeElement), "drawer traps forward tab");
    }
    check(await page.locator(".nav-group-toggle").count() === 4, "four business navigation groups");
    for (const group of groups) {
      const toggle = page.getByRole("button", { name: group, exact: true });
      if (await toggle.getAttribute("aria-expanded") === "false") await toggle.click();
      check(await toggle.getAttribute("aria-expanded") === "true", "group opens " + group);
      const control = await toggle.getAttribute("aria-controls");
      check(await page.locator("#" + control).isVisible(), "group disclosure controls visible children");
    }
    const master = page.getByRole("button", { name: "Master data", exact: true });
    await master.press("Enter");
    check(await master.getAttribute("aria-expanded") === "false" && !(await page.locator("#nav-group-master").isVisible()), "keyboard collapses dropdown");
    await master.press("Enter");
    check(await page.locator("#nav-group-master").isVisible(), "keyboard expands dropdown");
    const scrolling = await page.locator(".app-navigation").evaluate(e => ({
      scrolls: e.scrollHeight > e.clientHeight, overflow: getComputedStyle(e).overflowY,
    }));
    check(scrolling.overflow === "auto", "navigation allows vertical scrolling");
    if (viewport.height <= 640) check(scrolling.scrolls, "short screen navigation needs scrollbar");
    await page.locator(".app-navigation").evaluate(e => e.scrollTop = e.scrollHeight);
    await page.getByRole("button", { name: "Finance & reports", exact: true }).click();
    if (drawer) check(!(await page.locator(".app-sidebar").isVisible()), "selection closes drawer");
    await page.getByRole("tab", { name: "Revaluations" }).click();
    await page.getByText("Independent appraisal", { exact: true }).waitFor();
    await noOverflow("finance tables stay within their scroll container");
    await navigate(page, "Asset register");
    await page.getByRole("button", { name: "Register asset", exact: true }).click();
    const dialog = page.getByRole("dialog");
    await dialog.waitFor();
    check(await dialog.evaluate(e => {
      const r = e.getBoundingClientRect();
      return r.left >= 0 && r.right <= innerWidth + 1 && r.top >= 0 && r.bottom <= innerHeight + 1;
    }), "asset form fits viewport");
    await page.keyboard.press("Escape");
    await navigate(page, "Asset lifecycle");
    await page.getByRole("button", { name: "Servis & perbaikan", exact: true }).click();
    await page.locator(".lifecycle-workspace .loading").waitFor({ state: "hidden" });
    await noOverflow("lifecycle calendar fits viewport");
    check(await page.locator(".calendar-heading").evaluate(e => {
      const title = e.querySelector("h2").getBoundingClientRect();
      const input = e.querySelector("input").getBoundingClientRect();
      return title.bottom <= input.top + 1 || title.right <= input.left;
    }), "calendar title does not overlap month control");
    await page.getByRole("button", { name: "Notifikasi", exact: true }).click();
    await page.getByText("Tidak ada pengingat jatuh tempo.").waitFor();
    check(await page.locator(".notification-menu").evaluate(e => {
      const r = e.getBoundingClientRect();
      return r.left >= 0 && r.right <= innerWidth + 1 && r.top >= 0 && r.bottom <= innerHeight + 1;
    }), "notification panel fits viewport");
    await page.getByRole("button", { name: "Tutup notifikasi", exact: true }).click();
    await page.screenshot({ path: `${directory}/responsive-${viewport.width}x${viewport.height}.png`, fullPage: true });
    if (drawer) {
      await openMenu.click();
      await page.screenshot({ path: `${directory}/navigation-${viewport.width}x${viewport.height}.png` });
      await page.keyboard.press("Escape");
      check(!(await page.locator(".app-sidebar").isVisible()), "escape closes drawer");
      check(await openMenu.evaluate(e => e === document.activeElement), "drawer restores trigger focus");
      check(await page.evaluate(() => document.body.style.overflow !== "hidden"), "drawer restores body scrolling");
      await openMenu.click();
      await page.locator(".nav-backdrop").click({ position: { x: viewport.width - 5, y: 10 } });
      check(!(await page.locator(".app-sidebar").isVisible()), "backdrop closes drawer");
    }
  }
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "Open menu", exact: true }).click();
  await page.setViewportSize({ width: 1440, height: 900 });
  check(await page.locator(".app-sidebar").isVisible() && await page.locator(".main").getAttribute("inert") === null,
    "resize to desktop clears mobile overlay and inert state");
  await navigate(page, "Overview");
};
