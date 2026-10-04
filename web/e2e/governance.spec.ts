import { expect, test, type APIRequestContext, type Page } from "@playwright/test";
import { adminApi, devSession, enroll, halo, invite, json, useSession } from "./support";

const stamp = Date.now();
const email = `e2e-requester-${stamp}@example.com`;
const name = `Requester ${stamp}`;
const groupName = `E2E package group ${stamp}`;
const packageName = `E2E package ${stamp}`;

let admin: APIRequestContext;
let cookie = "";
let userId = "";
let enrollUrl = "";
let groupId = "";
let packageId = "";

async function myGroups(page: Page) {
  const groups = await json<{ name: string }[]>(await page.request.get(`${halo}/api/v1/me/groups`));
  return groups.map((g) => g.name);
}

test.beforeAll(async ({ playwright }) => {
  cookie = devSession("luna@example.com");
  admin = await adminApi(playwright.request, cookie);
  const approver = await json<{ id: string }>(await admin.get("/api/v1/me"));
  groupId = (await json<{ id: string }>(await admin.post("/api/v1/groups", { data: { name: groupName, kind: "assigned" } }))).id;
  ({ id: userId, enrollUrl } = await invite(admin, email, name));
  const created = await json<{ id: string }>(
    await admin.post("/api/v1/access-packages", {
      data: { name: packageName, description: "Created by the governance end-to-end test.", groupIds: [groupId], approverIds: [approver.id], maxDays: null, requireJustification: true },
    }),
  );
  packageId = created.id;
});

test.afterAll(async () => {
  if (packageId) {
    const archive = { name: packageName, groupIds: [groupId], approverIds: [], maxDays: null, archived: true };
    if (!(await admin.delete(`/api/v1/access-packages/${packageId}`)).ok()) await admin.put(`/api/v1/access-packages/${packageId}`, { data: archive });
  }
  if (userId) await admin.post(`/api/v1/users/${userId}/suspend`, { data: {} });
  if (groupId) await admin.delete(`/api/v1/groups/${groupId}`);
  await admin?.dispose();
});

test("a person requests an access package, an approver approves it in the console, and revoking removes the access", async ({ page, browser }) => {
  await enroll(page, enrollUrl, name);
  expect(await myGroups(page)).not.toContain(groupName);

  await page.goto("/account/access");
  await page.getByRole("button", { name: "Request access" }).click();
  const request = page.getByRole("dialog", { name: "Request access" });
  await request.getByRole("radio", { name: new RegExp(packageName) }).check();
  await request.getByLabel("Justification").fill("Needed to verify access requests end to end.");
  await request.getByRole("button", { name: "Send request" }).click();
  await expect(page.getByText(/^Request sent to/)).toBeVisible();

  const approverContext = await browser.newContext();
  await useSession(approverContext, cookie);
  const requests = await approverContext.newPage();
  await requests.goto(`${halo}/admin/requests`);
  const pending = requests.getByRole("listitem").filter({ hasText: name }).filter({ hasText: packageName });
  await expect(pending).toContainText("Needed to verify access requests end to end.");
  await pending.getByRole("button", { name: "Approve" }).click();
  await requests.getByRole("dialog").getByRole("button", { name: "Approve request" }).click();
  await expect(requests.getByText("Request approved", { exact: true })).toBeVisible();

  await expect.poll(() => myGroups(page)).toContain(groupName);
  await page.goto("/account/access");
  await expect(page.getByText(groupName).first()).toBeVisible();

  await requests.reload();
  const decided = requests.getByRole("row").filter({ hasText: name }).filter({ hasText: packageName });
  await decided.getByRole("button", { name: "Revoke" }).click();
  await requests.getByRole("dialog").getByRole("button", { name: "Revoke access" }).click();
  await expect(requests.getByText("Access revoked", { exact: true })).toBeVisible();

  await expect.poll(() => myGroups(page)).not.toContain(groupName);
  await approverContext.close();
});
