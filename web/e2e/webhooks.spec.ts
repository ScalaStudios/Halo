import { expect, test, type APIRequestContext, type Locator } from "@playwright/test";
import { adminApi, devSession, json, useSession } from "./support";

const url = `https://hooks.example.com/e2e-${Date.now()}`;

type Webhook = { id: string; url: string; events: string[] };

let admin: APIRequestContext;
let cookie = "";

async function endpoint() {
  const webhooks = await json<Webhook[]>(await admin.get("/api/v1/webhooks"));
  return webhooks.find((w) => w.url === url);
}

function event(scope: Locator, type: string) {
  return scope.getByRole("checkbox", { name: new RegExp(`^${type.replace(/[.*]/g, "\\$&")} `) });
}

test.beforeAll(async ({ playwright }) => {
  cookie = devSession("luna@example.com");
  admin = await adminApi(playwright.request, cookie);
});

test.afterAll(async () => {
  const created = await endpoint();
  if (created) await admin.delete(`/api/v1/webhooks/${created.id}`);
  await admin?.dispose();
});

test("a webhook endpoint takes single events and whole families, and editing keeps them", async ({ page }) => {
  await useSession(page.context(), cookie);
  await page.goto("/admin/webhooks");
  await page.getByRole("button", { name: "Add endpoint" }).first().click();
  const dialog = page.getByRole("dialog", { name: "Add endpoint" });
  await dialog.getByLabel("URL").fill(url);
  await dialog.getByLabel("Events", { exact: true }).selectOption("some");
  const picker = dialog.getByRole("group", { name: "Events to send" });

  await picker.getByRole("button", { name: "Events in user.*" }).click();
  await event(picker, "user.suspend").check();
  await expect(picker.getByRole("checkbox", { name: "user.*", exact: true })).toBeChecked({ indeterminate: true });
  await picker.getByRole("checkbox", { name: "sign_in.*", exact: true }).check();
  await picker.getByRole("button", { name: "Events in sign_in.*" }).click();
  await expect(event(picker, "sign_in.failure")).toBeChecked();
  await event(picker, "sign_in.failure").uncheck();
  await expect(picker.getByRole("checkbox", { name: "sign_in.*", exact: true })).toBeChecked({ indeterminate: true });
  await dialog.getByRole("searchbox", { name: "Search events" }).fill("suspended through scim");
  await expect(picker.getByRole("checkbox")).toHaveCount(2);
  await dialog.getByRole("button", { name: "Add endpoint" }).click();
  await page.getByRole("dialog", { name: "Endpoint added" }).getByRole("button", { name: "Done" }).click();
  const created = await endpoint();
  expect(created?.events.toSorted()).toEqual(["sign_in.interrupted", "sign_in.success", "user.suspend"]);

  const kept = ["application.*", "scim.user.*", "user.provision", "user.suspend"];
  await json(await admin.put(`/api/v1/webhooks/${created?.id}`, { data: { url, events: kept } }));
  await page.goto(`/admin/webhooks/${created?.id}`);
  await page.getByRole("button", { name: "More endpoint actions" }).click();
  await page.getByRole("menuitem", { name: "Edit endpoint" }).click();
  const edit = page.getByRole("dialog", { name: "Edit endpoint" });
  await expect(edit.getByRole("checkbox", { name: "application.*", exact: true })).toBeChecked();
  await expect(edit.getByRole("checkbox", { name: "scim.*", exact: true })).toBeChecked({ indeterminate: true });
  await expect(event(edit, "scim.user.create")).toBeChecked();
  await expect(event(edit, "user.provision")).toBeChecked();
  await edit.getByRole("button", { name: "Save endpoint" }).click();
  await expect(page.getByText("Endpoint saved", { exact: true })).toBeVisible();
  expect((await endpoint())?.events.toSorted()).toEqual(kept);
});
