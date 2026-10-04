import type { ChildProcess } from "node:child_process";
import { expect, test, type APIRequestContext } from "@playwright/test";
import { adminApi, enroll, halo, invite, json, startExample, stopExample } from "./support";

const sp = "http://localhost:9100";
const stamp = Date.now();
const email = `e2e-saml-${stamp}@example.com`;
const name = `SAML Tester ${stamp}`;
const groupName = `E2E SAML testers ${stamp}`;
const entityId = `urn:halo:e2e:saml-sp:${stamp}`;

let admin: APIRequestContext;
let server: ChildProcess | undefined;
let userId = "";
let enrollUrl = "";
let groupId = "";
let appId = "";

test.beforeAll(async ({ playwright }) => {
  admin = await adminApi(playwright.request);
  groupId = (await json<{ id: string }>(await admin.post("/api/v1/groups", { data: { name: groupName, kind: "assigned" } }))).id;
  ({ id: userId, enrollUrl } = await invite(admin, email, name, [groupId]));
  const created = await json<{ application: { id: string } }>(
    await admin.post("/api/v1/applications", {
      data: { name: `E2E SAML app ${stamp}`, protocol: "saml", groupIds: [groupId], saml: { entityId, acsUrls: [`${sp}/saml/acs`] } },
    }),
  );
  appId = created.application.id;
  server = await startExample("./examples/go-saml-sp", `${sp}/saml/metadata`, { HALO_URL: halo, ENTITY_ID: entityId, BASE_URL: sp });
});

test.afterAll(async () => {
  await stopExample(server, `${sp}/saml/metadata`);
  if (appId) await admin.delete(`/api/v1/applications/${appId}`);
  if (userId) await admin.post(`/api/v1/users/${userId}/suspend`, { data: {} });
  if (groupId) await admin.delete(`/api/v1/groups/${groupId}`);
  await admin?.dispose();
});

test("a person signs in to a SAML service provider through Halo with a passkey", async ({ page }) => {
  await enroll(page, enrollUrl, name);
  await page.context().clearCookies();

  await page.goto(`${sp}/`);
  await page.waitForURL("**/sign-in?next=**");
  await page.getByRole("button", { name: "Continue with passkey" }).click();
  await page.waitForURL(`${sp}/saml/acs`);

  await expect(page.locator("#subject")).toHaveText(email);
  await expect(page.locator("#groups")).toContainText(groupName);
  const attributes = JSON.parse((await page.locator("#attributes").textContent()) ?? "{}") as Record<string, string[]>;
  expect(attributes.email).toEqual([email]);
  expect(attributes.name).toEqual([name]);
  expect(attributes.groups).toContain(groupName);

  const events = await json<{ appId: string; result: string; method: string }[]>(await admin.get(`/api/v1/sign-ins?user=${userId}`));
  expect(events.some((e) => e.appId === appId && e.result === "success" && e.method === "passkey")).toBeTruthy();
});
