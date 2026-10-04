import { expect, test, type APIRequestContext } from "@playwright/test";
import { adminApi, enroll, invite, json } from "./support";

const stamp = Date.now();
const email = `e2e-policy-${stamp}@example.com`;
const name = `Policy Tester ${stamp}`;
const policyName = `E2E block risky networks ${stamp}`;

type Policy = { id: string; name: string; description: string; enabled: boolean; mode: string; effect: string; conditions: { excludeGroupIds: string[] } & Record<string, unknown> };

let admin: APIRequestContext;
let userId = "";
let enrollUrl = "";
let groupId = "";
let zoneId = "";
let policy: Policy | undefined;
const excluded: Policy[] = [];

function body(p: Policy) {
  return { name: p.name, description: p.description, enabled: p.enabled, mode: p.mode, effect: p.effect, conditions: p.conditions };
}

test.beforeAll(async ({ playwright }) => {
  admin = await adminApi(playwright.request);
  groupId = (await json<{ id: string }>(await admin.post("/api/v1/groups", { data: { name: `E2E policy group ${stamp}`, kind: "assigned" } }))).id;
  ({ id: userId, enrollUrl } = await invite(admin, email, name, [groupId]));
});

test.afterAll(async () => {
  if (policy) await admin.delete(`/api/v1/policies/${policy.id}`);
  for (const p of excluded) {
    const current = await json<Policy>(await admin.get(`/api/v1/policies/${p.id}`));
    current.conditions.excludeGroupIds = current.conditions.excludeGroupIds.filter((id) => id !== groupId);
    await admin.put(`/api/v1/policies/${p.id}`, { data: body(current) });
  }
  if (zoneId) await admin.delete(`/api/v1/network-zones/${zoneId}`);
  if (userId) await admin.post(`/api/v1/users/${userId}/suspend`, { data: {} });
  if (groupId) await admin.delete(`/api/v1/groups/${groupId}`);
  await admin?.dispose();
});

test("an enforced policy blocks a passkey sign-in from a risky network and report-only lets it through", async ({ page }) => {
  await enroll(page, enrollUrl, name);

  zoneId = (await json<{ id: string }>(await admin.post("/api/v1/network-zones", { data: { name: `E2E loopback ${stamp}`, kind: "risky", cidrs: ["127.0.0.0/8", "::1/128"] } }))).id;
  for (const p of await json<Policy[]>(await admin.get("/api/v1/policies"))) {
    if (!p.enabled || p.mode !== "enforce") continue;
    p.conditions.excludeGroupIds = [...(p.conditions.excludeGroupIds ?? []), groupId];
    await json(await admin.put(`/api/v1/policies/${p.id}`, { data: body(p) }));
    excluded.push(p);
  }
  policy = await json<Policy>(
    await admin.post("/api/v1/policies", {
      data: {
        name: policyName,
        description: "Created by the policy end-to-end test.",
        enabled: true,
        mode: "enforce",
        effect: "block",
        conditions: { groupIds: [groupId], allApps: true, network: "in", zoneIds: [zoneId] },
      },
    }),
  );

  await page.context().clearCookies();
  await page.goto("/sign-in");
  await page.getByRole("button", { name: "Continue with passkey" }).click();
  await expect(page.getByRole("alert").filter({ hasText: "Your organization's access policy blocked this sign-in." })).toBeVisible();
  const events = await json<{ result: string; reason: string }[]>(await admin.get(`/api/v1/sign-ins?user=${userId}`));
  expect(events.find((e) => e.result === "failure")?.reason).toContain(policyName);

  await json(await admin.put(`/api/v1/policies/${policy.id}`, { data: { ...body(policy), mode: "report" } }));
  await page.getByRole("button", { name: "Continue with passkey" }).click();
  await page.waitForURL("**/account");
});
