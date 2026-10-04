import type { ChildProcess } from "node:child_process";
import { expect, test, type APIRequestContext } from "@playwright/test";
import { adminApi, enroll, halo, invite, json, startExample, stopExample } from "./support";

const example = "http://localhost:9000";
const stamp = Date.now();
const email = `e2e-${stamp}@example.com`;
const groupName = `E2E testers ${stamp}`;

let admin: APIRequestContext;
let server: ChildProcess | undefined;
let enrollUrl = "";
let appId = "";
let userId = "";
let groupId = "";

test.beforeAll(async ({ playwright }) => {
  admin = await adminApi(playwright.request);
  groupId = (await json<{ id: string }>(await admin.post("/api/v1/groups", { data: { name: groupName, kind: "assigned" } }))).id;
  ({ id: userId, enrollUrl } = await invite(admin, email, "E2E Tester", [groupId]));
  const created = await json<{ application: { id: string; clientId: string }; clientSecret: string }>(
    await admin.post("/api/v1/applications", {
      data: { name: `E2E example app ${stamp}`, protocol: "oidc", type: "web", redirectUris: [`${example}/callback`], groupIds: [groupId] },
    }),
  );
  appId = created.application.id;
  server = await startExample("./examples/go-web-client", `${example}/`, { CLIENT_ID: created.application.clientId, CLIENT_SECRET: created.clientSecret, HALO_ISSUER: halo });
});

test.afterAll(async () => {
  await stopExample(server, `${example}/`);
  if (appId) await admin.delete(`/api/v1/applications/${appId}`);
  if (userId) await admin.post(`/api/v1/users/${userId}/suspend`, { data: {} });
  if (groupId) await admin.delete(`/api/v1/groups/${groupId}`);
  await admin?.dispose();
});

test("an invited person enrolls a passkey and signs in to an OpenID Connect app through Halo", async ({ page }) => {
  await enroll(page, enrollUrl, "E2E Tester");

  await page.goto(`${example}/`);
  await page.getByRole("link", { name: /sign in with halo/i }).click();
  await page.waitForURL(`${example}/callback**`);
  await expect(page.getByText(`Signed in through Halo as E2E Tester (${email})`)).toBeVisible();

  await page.request.post(`${halo}/api/v1/auth/sign-out`, { data: {} });
  await page.goto(`${example}/`);
  await page.getByRole("link", { name: /sign in with halo/i }).click();
  await page.waitForURL("**/sign-in?authRequest=**");
  await expect(page.getByText(/E2E example app/)).toBeVisible();
  await page.getByRole("button", { name: /continue with passkey/i }).click();
  await page.waitForURL(`${example}/callback**`);
  const claims = JSON.parse((await page.locator("#claims").textContent()) ?? "{}") as { sub: string; email: string; groups: string[] };
  expect(claims.sub).toBe(userId);
  expect(claims.email).toBe(email);
  expect(claims.groups).toContain(groupName);

  const events = await json<{ userId: string; appId: string; result: string; method: string }[]>(await admin.get(`/api/v1/sign-ins?user=${userId}`));
  expect(events.filter((e) => e.appId === appId && e.result === "success" && e.method === "passkey").length).toBeGreaterThanOrEqual(1);
});
