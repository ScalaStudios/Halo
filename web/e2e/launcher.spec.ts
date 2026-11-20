import { expect, test, type APIRequestContext } from "@playwright/test";
import { adminApi, devSession, json, useSession } from "./support";

const stamp = Date.now();
const bookmarkName = `E2E bookmark ${stamp}`;
const collectionName = `E2E collection ${stamp}`;
const groupName = `E2E launcher ${stamp}`;

let admin: APIRequestContext;
let cookie = "";
let groupId = "";

test.beforeAll(async ({ playwright }) => {
  cookie = devSession("luna@example.com");
  admin = await adminApi(playwright.request, cookie);
  const me = await json<{ id: string }>(await admin.get("/api/v1/me"));
  groupId = (await json<{ id: string }>(await admin.post("/api/v1/groups", { data: { name: groupName, kind: "assigned" } }))).id;
  expect((await admin.post(`/api/v1/groups/${groupId}/members`, { data: { userId: me.id } })).ok()).toBeTruthy();
});

test.afterAll(async () => {
  const collections = await json<{ id: string; name: string }[]>(await admin.get("/api/v1/collections"));
  for (const c of collections.filter((c) => c.name === collectionName)) await admin.delete(`/api/v1/collections/${c.id}`);
  const apps = await json<{ id: string; name: string }[]>(await admin.get("/api/v1/applications"));
  for (const a of apps.filter((a) => a.name === bookmarkName)) await admin.delete(`/api/v1/applications/${a.id}`);
  if (groupId) await admin.delete(`/api/v1/groups/${groupId}`);
  await admin.put("/api/v1/me/applications", { data: { order: [], favourites: [] } });
  await admin?.dispose();
});

test.beforeEach(async ({ context }) => useSession(context, cookie));

test("an administrator adds a bookmark to a collection and a person marks it as a favourite", async ({ page }) => {
  await page.goto("/admin/applications/new");
  await page.locator("label").filter({ has: page.getByRole("radio", { name: /Bookmark/ }) }).click();
  await page.getByRole("button", { name: "Continue" }).click();
  await page.getByLabel("Name").fill(bookmarkName);
  await page.getByLabel("URL").fill("https://status.example.com");
  await page.getByRole("button", { name: "Create bookmark" }).click();
  await expect(page.getByRole("heading", { level: 1, name: bookmarkName })).toBeVisible();
  const bookmarkId = new URL(page.url()).pathname.split("/").at(-1) ?? "";
  await json(await admin.put(`/api/v1/applications/${bookmarkId}/groups`, { data: { groupIds: [groupId] } }));
  await page.goto(`/admin/applications/${bookmarkId}`);
  await expect(page.getByText("https://status.example.com", { exact: true })).toBeVisible();

  await page.goto("/admin/collections");
  await page.getByRole("button", { name: "Create collection" }).first().click();
  const dialog = page.getByRole("dialog", { name: "Create collection" });
  await dialog.getByLabel("Name").fill(collectionName);
  await dialog.getByRole("checkbox", { name: new RegExp(bookmarkName) }).check();
  await dialog.getByRole("checkbox", { name: new RegExp(groupName) }).check();
  await dialog.getByRole("button", { name: "Create collection" }).click();
  await expect(page.getByRole("row").filter({ hasText: collectionName })).toContainText(groupName);

  await page.goto("/account/applications");
  const collection = page.getByRole("region", { name: collectionName });
  await expect(collection.getByRole("link", { name: new RegExp(bookmarkName) })).toHaveAttribute("href", "https://status.example.com");
  await collection.getByRole("button", { name: `Options for ${bookmarkName}` }).click();
  await page.getByRole("menuitem", { name: "Add to favourites" }).click();
  const favourites = page.getByRole("region", { name: "Favourites" });
  await expect(favourites.getByRole("link", { name: new RegExp(bookmarkName) })).toBeVisible();

  await favourites.getByRole("button", { name: `Options for ${bookmarkName}` }).click();
  await page.getByRole("menuitem", { name: "Remove from favourites" }).click();
  await expect(favourites.getByRole("link", { name: new RegExp(bookmarkName) })).toHaveCount(0);

  const others = page.getByRole("region", { name: "Other applications" }).getByRole("listitem");
  const first = (await others.nth(0).getByRole("link").getAttribute("href")) ?? "";
  const second = (await others.nth(1).getByRole("link").getAttribute("href")) ?? "";
  await others.nth(0).getByRole("button", { name: /^Options for / }).click();
  await page.getByRole("menuitem", { name: "Move later" }).click();
  await expect(others.nth(0).getByRole("link")).toHaveAttribute("href", second);
  await expect(others.nth(1).getByRole("link")).toHaveAttribute("href", first);
});
