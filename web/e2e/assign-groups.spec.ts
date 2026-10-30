import { expect, test, type APIRequestContext } from "@playwright/test";
import { adminApi, devSession, json, useSession } from "./support";

const stamp = Date.now();
const appName = `E2E assign ${stamp}`;
const option = (name: string) => ({ name: new RegExp(`E2E ${name} ${stamp}`) });

let admin: APIRequestContext;
let cookie = "";
let appId = "";
const groupIds: string[] = [];

test.beforeAll(async ({ playwright }) => {
  cookie = devSession("luna@example.com");
  admin = await adminApi(playwright.request, cookie);
  for (const [name, rule] of [["Alpha", null], ["Beta", null], ["Rule", `user.department == "E2E ${stamp}"`]]) {
    const data = { name: `E2E ${name} ${stamp}`, kind: rule ? "dynamic" : "assigned", rule };
    groupIds.push((await json<{ id: string }>(await admin.post("/api/v1/groups", { data }))).id);
  }
  const data = { name: appName, protocol: "oidc", type: "web", redirectUris: ["http://localhost:9000/callback"], groupIds: [] };
  appId = (await json<{ application: { id: string } }>(await admin.post("/api/v1/applications", { data }))).application.id;
});

test.afterAll(async () => {
  if (appId) await admin.delete(`/api/v1/applications/${appId}`);
  for (const id of groupIds) await admin.delete(`/api/v1/groups/${id}`);
  await admin?.dispose();
});

test.beforeEach(async ({ context }) => useSession(context, cookie));

test("assign group searches and assigns several groups at once from the keyboard", async ({ page }) => {
  await page.goto(`/admin/applications/${appId}?tab=users`);
  await page.getByRole("button", { name: "Assign group" }).click();
  const dialog = page.getByRole("dialog", { name: `Assign groups to ${appName}` });
  await expect(dialog.getByRole("combobox")).toBeFocused();
  await page.keyboard.type(`e2e ${stamp}`);
  await expect(dialog.getByRole("option")).toHaveCount(3);
  await page.keyboard.press("Enter");
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Enter");
  await dialog.getByRole("button", { name: "Assign 2 groups" }).click();
  await expect(dialog).toBeHidden();
  const table = page.getByRole("table", { name: "Assigned groups" });
  await expect(table.getByText(`E2E Alpha ${stamp}`)).toBeVisible();
  await expect(table.getByText(`E2E Beta ${stamp}`)).toBeVisible();

  await page.getByRole("button", { name: "Assign group" }).click();
  await expect(dialog.getByRole("combobox")).toBeFocused();
  await page.keyboard.type(String(stamp));
  await expect(dialog.getByRole("option", option("Alpha"))).toBeDisabled();
  await expect(dialog.getByRole("option", option("Rule"))).toBeEnabled();
});

test("add to group on a user page leaves rule-based groups out of reach", async ({ page }) => {
  const me = await json<{ id: string; name: string }>(await admin.get("/api/v1/me"));
  await page.goto(`/admin/users/${me.id}?tab=groups`);
  await page.getByRole("button", { name: "Add to group" }).click();
  const dialog = page.getByRole("dialog", { name: `Add ${me.name} to groups` });
  await expect(dialog.getByRole("combobox")).toBeFocused();
  await page.keyboard.type(String(stamp));
  await expect(dialog.getByRole("option", option("Rule"))).toBeDisabled();
  await dialog.getByRole("option", option("Beta")).click();
  await page.keyboard.press("ArrowUp");
  await page.keyboard.press("Space");
  await dialog.getByRole("button", { name: "Add to 2 groups" }).click();
  await expect(dialog).toBeHidden();
  const table = page.getByRole("table", { name: "Groups" });
  await expect(table.getByText(`E2E Alpha ${stamp}`)).toBeVisible();
  await expect(table.getByText(`E2E Beta ${stamp}`)).toBeVisible();
});
