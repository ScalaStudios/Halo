import { expect, test, type APIRequestContext } from "@playwright/test";
import { adminApi, enroll, halo, invite, json } from "./support";

const stamp = Date.now();
const email = `e2e-magic-${stamp}@example.com`;
const name = `Magic Tester ${stamp}`;

let admin: APIRequestContext;
let userId = "";
let enrollUrl = "";

type Message = { to: string; text: string };

test.beforeAll(async ({ playwright }) => {
  admin = await adminApi(playwright.request);
  ({ id: userId, enrollUrl } = await invite(admin, email, name));
});

test.afterAll(async () => {
  if (userId) await admin.post(`/api/v1/users/${userId}/suspend`, { data: {} });
  await admin?.dispose();
});

test("a person requests a magic link on the sign-in page and the link signs them in", async ({ page }) => {
  await enroll(page, enrollUrl, name);
  await page.context().clearCookies();

  await page.goto("/sign-in");
  await page.getByLabel("Email").fill(email);
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Choose how to sign in" })).toBeVisible();
  await page.getByRole("button", { name: "Email me a sign-in link" }).click();
  await expect(page.getByRole("heading", { name: "Check your inbox" })).toBeVisible();

  let link = "";
  await expect
    .poll(async () => {
      const outbox = await json<Message[]>(await admin.get("/api/v1/dev/outbox"));
      link = outbox.find((m) => m.to === email)?.text.match(/https?:\/\/\S+\/sign-in\/magic\?token=[\w-]+/)?.[0] ?? "";
      return link;
    })
    .toMatch(/^https?:\/\//);
  expect(new URL(link).origin).toBe(new URL(halo).origin);

  await page.goto(link);
  await page.waitForURL("**/account");
  await expect(page.getByText(name).first()).toBeVisible();

  await page.goto(link);
  await expect(page.getByRole("heading", { name: "This sign-in link has expired" })).toBeVisible();
});
