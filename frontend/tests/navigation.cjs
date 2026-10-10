async function navigate(page, name) {
  await page.locator(".workspace").waitFor({ state: "visible" });
  const sidebar = page.locator(".app-sidebar");
  if (!(await sidebar.isVisible())) await page.getByRole("button", { name: /^(Open menu|Buka menu)$/ }).click();
  const item = sidebar.getByRole("button", { name, exact: typeof name === "string", includeHidden: true });
  const group = sidebar.locator(".nav-group").filter({
    has: page.getByRole("button", { name, exact: typeof name === "string", includeHidden: true }),
  });
  if (await group.count()) {
    const toggle = group.locator(".nav-group-toggle");
    if (await toggle.getAttribute("aria-expanded") === "false") await toggle.click();
  }
  await item.click();
}
module.exports = { navigate };
