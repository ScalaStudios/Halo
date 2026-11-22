import { expect, test } from "@playwright/test";
import { addVirtualAuthenticator, adminApi, devSession, invite, json, useSession } from "./support";

test.describe("with a Spanish browser", () => {
  test.use({ locale: "es-MX" });

  test("sign-in localizes Spanish regional tags and labels", async ({ page }) => {
    await page.goto("/sign-in");

    const heading = page.getByRole("heading", { name: "Inicia sesión para continuar" });
    await expect(heading).toBeVisible();
    await expect(heading.locator("xpath=ancestor::*[@lang][1]")).toHaveAttribute("lang", "es");
    await expect(page).toHaveTitle(/Iniciar sesión/);
    await page.getByRole("button", { name: "Continuar", exact: true }).click();
    await expect(page.getByText("Introduce tu correo de trabajo, por ejemplo nombre@example.com.")).toBeVisible();
    await page.getByRole("button", { name: "Otras opciones" }).click();
    await expect(page.getByRole("button", { name: "Código de recuperación" })).toBeVisible();
    await page.route("**/api/v1/auth/identify", (route) => route.fulfill({ json: { methods: ["totp"] } }));
    await page.getByRole("textbox", { name: "Correo electrónico" }).fill("persona@example.com");
    await page.getByRole("button", { name: "Continuar", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Elige cómo iniciar sesión" })).toBeVisible();
    await page.getByRole("button", { name: "Aplicación de autenticación" }).click();
    await expect(page.getByRole("heading", { name: "Introduce el código de tu aplicación de autenticación" })).toBeVisible();
    await expect(page.getByRole("textbox", { name: "Código de seis dígitos" })).toBeVisible();
  });

  test("a Spanish-speaking invitee can set up a passkey", async ({ page, playwright }) => {
    const admin = await adminApi(playwright.request);
    const name = "María Rivera";
    const { id, enrollUrl } = await invite(admin, `e2e-es-${Date.now()}@example.com`, name);
    try {
      await addVirtualAuthenticator(page);
      await page.goto(enrollUrl);
      const heading = page.getByRole("heading", { name: `Configura tu clave de acceso, ${name}` });
      await expect(heading).toBeVisible();
      await expect(heading.locator("xpath=ancestor::*[@lang][1]")).toHaveAttribute("lang", "es");
      await expect(page).toHaveTitle(/Configura tu clave de acceso/);
      await page.getByRole("textbox", { name: "Nombre de la clave de acceso" }).fill("Llave de prueba");
      await page.getByRole("button", { name: "Crear clave de acceso" }).click();
      await page.waitForURL("**/account");
    } finally {
      await admin.post(`/api/v1/users/${id}/suspend`, { data: {} });
      await admin.dispose();
    }
  });

  test("a Spanish-speaking visitor can validate and submit a sign-up", async ({ page, playwright }) => {
    const admin = await adminApi(playwright.request);
    const { signUp, signUpDomains } = await json<{ signUp: string; signUpDomains: string[] }>(
      await admin.get("/api/v1/settings"),
    );
    try {
      expect((await admin.patch("/api/v1/settings", { data: { signUp: "open", signUpDomains: ["example.com"] } })).ok()).toBe(true);
      await page.goto("/sign-up");
      const heading = page.getByRole("heading", { name: "Crea tu cuenta" });
      await expect(heading).toBeVisible();
      await expect(heading.locator("xpath=ancestor::*[@lang][1]")).toHaveAttribute("lang", "es");
      await expect(page).toHaveTitle(/Registrarse/);
      const submit = page.getByRole("button", { name: "Continuar", exact: true });
      await submit.click();
      const name = page.getByRole("textbox", { name: "Nombre", exact: true });
      const email = page.getByRole("textbox", { name: "Correo electrónico", exact: true });
      await expect(name).toBeFocused();
      await expect(page.getByText("Introduce el nombre con el que quieres que te vean en tu organización.")).toBeVisible();
      await name.fill("María Rivera");
      await email.fill("maria@outside.example");
      await submit.click();
      await expect(page.getByText(/Este dominio de correo no admite el registro/)).toBeVisible();
      await email.fill(`e2e-signup-es-${Date.now()}@example.com`);
      await submit.click();
      await expect(page.getByRole("heading", { name: "Revisa tu bandeja de entrada" })).toBeVisible();
    } finally {
      await admin.patch("/api/v1/settings", { data: { signUp, signUpDomains } });
      await admin.dispose();
    }
  });

  test("incomplete and expired magic links explain how to continue in Spanish", async ({ page }) => {
    await page.goto("/sign-in/magic");
    await expect(page.getByRole("heading", { name: "Este enlace de inicio de sesión está incompleto" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Volver al inicio de sesión" })).toHaveAttribute("href", "/sign-in");

    await page.goto("/sign-in/magic?token=expired");
    await expect(page.getByRole("heading", { name: "Este enlace de inicio de sesión ha caducado" })).toBeVisible();
    await expect(page).toHaveTitle(/Iniciando sesión/);
  });

  test("a Spanish-speaking invitee can resume after an identity check", async ({ page }) => {
    await page.goto("/enroll?proofing=done");
    await expect(page.getByRole("heading", { name: "Verificación de identidad enviada" })).toBeVisible();
    await expect(page.getByText("Vuelve a abrir el enlace de configuración para ver el resultado y crear tu clave de acceso.")).toBeVisible();
  });

  test("the Spanish account home and menu match the browser language", async ({ page, context }) => {
    await useSession(context, devSession("luna@example.com"));
    await page.goto("/account");

    const heading = page.getByRole("heading", { name: "Cuenta", exact: true });
    await expect(heading).toBeVisible();
    await expect(heading.locator("xpath=ancestor::*[@lang][1]")).toHaveAttribute("lang", "es");
    await expect(page).toHaveTitle("Cuenta de Halo");
    await expect(page.getByRole("navigation", { name: "Cuenta" }).getByRole("link", { name: "Seguridad" })).toHaveAttribute("href", "/account/security");
    await expect(page.getByRole("button", { name: "Descargar tus datos" })).toBeVisible();
    await page.getByRole("button", { name: "Menú de la cuenta" }).click();
    const menu = page.getByRole("menu");
    await expect(menu).toHaveAttribute("lang", "es");
    await menu.getByRole("menuitem", { name: "Cerrar sesión" }).click();
    await expect(page).toHaveURL(/\/sign-in$/);
  });
});

test("sign-in negotiates only acceptable languages", async ({ request }) => {
  for (const [language, heading] of [
    ["fr-CA, es;q=0, *;q=0.5", "Sign in to continue"],
    ["es;q=0.4, en-GB;q=0.8", "Sign in to continue"],
    ["fr-CA, es;q=0.8, en;q=0.4", "Inicia sesión para continuar"],
  ]) {
    const response = await request.get("/sign-in", { headers: { "Accept-Language": language } });
    expect(response.ok()).toBe(true);
    expect(await response.text()).toContain(`>${heading}</h1>`);
  }
});
