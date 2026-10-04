import { expect, test, type Page } from "@playwright/test";
import { adminApi, devSession, halo, json, useSession } from "./support";

const viewports = { desktop: { width: 1440, height: 900 }, phone: { width: 390, height: 844 } };

const details = [
  ["user", "/api/v1/users", "/admin/users/"],
  ["group", "/api/v1/groups", "/admin/groups/"],
  ["application", "/api/v1/applications", "/admin/applications/"],
  ["policy", "/api/v1/policies", "/admin/policies/"],
  ["access review", "/api/v1/access-reviews", "/admin/access-reviews/"],
  ["API resource", "/api/v1/api-resources", "/admin/api-resources/"],
  ["service account", "/api/v1/service-accounts", "/admin/service-accounts/"],
  ["webhook", "/api/v1/webhooks", "/admin/webhooks/"],
] as const;

let cookie = "";
const paths: string[] = [];
const missing: string[] = [];

test.beforeAll(async ({ playwright, browser }) => {
  cookie = devSession("luna@example.com");
  const context = await browser.newContext();
  await useSession(context, cookie);
  const page = await context.newPage();
  await page.goto(`${halo}/admin`);
  const nav = page.getByRole("navigation", { name: "Administration" }).getByRole("link");
  await expect(nav.first()).toBeVisible();
  paths.push(...new Set(await nav.evaluateAll((links) => links.map((a) => a.getAttribute("href") ?? ""))));
  await context.close();

  const admin = await adminApi(playwright.request, cookie);
  for (const [kind, list, base] of details) {
    const [first] = await json<{ id: string }[]>(await admin.get(list));
    if (first) paths.push(base + first.id);
    else missing.push(`No ${kind} exists, so its detail page was not visited.`);
  }
  await admin.dispose();
});

async function visit(page: Page, path: string, errors: string[]) {
  await test.step(path, async () => {
    errors.length = 0;
    const response = await page.goto(path);
    expect.soft(response?.status(), `${path} status`).toBeLessThan(400);
    expect.soft(new URL(page.url()).pathname, `${path} redirected`).toBe(path);
    await expect.soft(page.getByRole("heading", { level: 1 }).first(), `${path} heading`).toBeVisible();
    await expect.soft(page.getByRole("heading", { name: "Page not found" }), `${path} not found`).toHaveCount(0);
    await expect.soft(page.getByText(/Application error|Unhandled Runtime Error|Halo could not load this page/), `${path} error page`).toHaveCount(0);
    await expect.soft(page, `${path} title`).toHaveTitle(/^(?!Page not found).*Halo administration$/);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect.soft(overflow, `${path} overflows horizontally by ${overflow}px`).toBeLessThanOrEqual(0);
    expect.soft(errors, `${path} uncaught errors`).toEqual([]);
  });
}

for (const [name, viewport] of Object.entries(viewports)) {
  test(`every console page renders without errors or horizontal overflow on ${name}`, async ({ page, context }) => {
    test.setTimeout(600_000);
    test.info().annotations.push(...missing.map((description) => ({ type: "no data", description })));
    await page.setViewportSize(viewport);
    await useSession(context, cookie);
    const errors: string[] = [];
    page.on("pageerror", (error) => {
      if (!error.message.startsWith("Hydration failed")) return errors.push(error.message);
      const diff = error.message.split("\n").filter((line) => /^[+-]\s{2,}/.test(line)).map((line) => line.replace(/\s+/g, " "));
      test.info().annotations.push({ type: "hydration mismatch", description: `${new URL(page.url()).pathname}: ${diff.join(" ")}` });
    });
    for (const path of paths) await visit(page, path, errors);
  });
}
