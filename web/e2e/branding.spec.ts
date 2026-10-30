import { expect, test } from "@playwright/test";
import { adminApi, devSession, halo, json, useSession } from "./support";

const accent = "#2563eb";
const css = `@import url("https://example.com/halo-e2e.css");
:root { --halo-e2e: applied; }
body { background-image: url("https://example.com/halo-e2e.png"); }`;

test("colours and custom CSS reach the sign-in page, and the CSP keeps CSS on Halo's origin", async ({ page, playwright }) => {
  const cookie = devSession("luna@example.com");
  const admin = await adminApi(playwright.request, cookie);
  const patch = async (data: object) => json(await admin.patch("/api/v1/settings", { data }));
  await patch({ theme: { accent }, customCss: css, consoleBranding: false });
  try {
    const leaked: string[] = [];
    page.on("response", (response) => {
      if (new URL(response.url()).hostname === "example.com") leaked.push(response.url());
    });
    await page.addInitScript(() => {
      const blocked: string[] = [];
      Object.assign(window, { blocked });
      document.addEventListener("securitypolicyviolation", (event) => blocked.push(event.blockedURI));
    });
    const root = (property: string) => page.evaluate((name) => getComputedStyle(document.documentElement).getPropertyValue(name).trim(), property);

    await page.goto(`${halo}/sign-in`);
    expect(await root("--color-ember")).toBe(accent);
    expect(await root("--halo-e2e")).toBe("applied");
    await expect(page.getByRole("button", { name: "Continue with passkey" })).toHaveCSS("background-color", "rgb(37, 99, 235)");
    await expect
      .poll(() => page.evaluate(() => (window as unknown as { blocked: string[] }).blocked))
      .toEqual(expect.arrayContaining(["https://example.com/halo-e2e.css", "https://example.com/halo-e2e.png"]));
    expect(leaked).toEqual([]);

    await useSession(page.context(), cookie);
    await page.goto(`${halo}/admin`);
    expect(await root("--color-ember")).toBe("#fa7e26");
    await patch({ consoleBranding: true });
    await page.reload();
    expect(await root("--color-ember")).toBe(accent);
  } finally {
    await patch({ theme: { accent: "" }, customCss: "", consoleBranding: false });
    await admin.dispose();
  }
});
