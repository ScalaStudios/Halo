import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { resolve } from "node:path";
import { expect, type APIRequest, type APIRequestContext, type APIResponse, type BrowserContext, type Page } from "@playwright/test";

export const repo = resolve(__dirname, "../..");
export const halo = process.env.HALO_URL ?? "http://localhost:3200";
const envFile = resolve(repo, process.env.HALO_ENV_FILE ?? ".env");

export function devSession(email: string): string {
  const script = 'set -a && . "$0" && set +a && exec go run ./cmd/halo dev-session --email "$1"';
  return execFileSync("bash", ["-c", script, envFile, email], { cwd: repo, encoding: "utf8" }).trim();
}

export async function adminApi(request: APIRequest, cookie = devSession("luna@example.com")) {
  return request.newContext({ baseURL: halo, extraHTTPHeaders: { cookie } });
}

export async function useSession(context: BrowserContext, cookie: string) {
  const at = cookie.indexOf("=");
  await context.addCookies([{ name: cookie.slice(0, at), value: cookie.slice(at + 1), url: halo }]);
}

export async function json<T>(response: APIResponse): Promise<T> {
  expect(response.ok(), `${response.url()} ${response.status()}: ${await response.text()}`).toBeTruthy();
  return (await response.json()) as T;
}

export async function addVirtualAuthenticator(page: Page) {
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("WebAuthn.enable");
  await cdp.send("WebAuthn.addVirtualAuthenticator", {
    options: { protocol: "ctap2", transport: "internal", hasResidentKey: true, hasUserVerification: true, isUserVerified: true, automaticPresenceSimulation: true },
  });
}

export async function invite(admin: APIRequestContext, email: string, name: string, groupIds: string[] = []) {
  const created = await json<{ user: { id: string }; enrollUrl: string }>(await admin.post("/api/v1/users", { data: { email, name, groupIds } }));
  return { id: created.user.id, enrollUrl: created.enrollUrl };
}

export async function enroll(page: Page, enrollUrl: string, name: string) {
  await addVirtualAuthenticator(page);
  await page.goto(enrollUrl);
  await expect(page.getByText(name)).toBeVisible();
  await page.getByRole("button", { name: /create passkey/i }).click();
  await page.waitForURL("**/account");
}

async function listening(url: string) {
  return fetch(url).then(
    () => true,
    () => false,
  );
}

export async function startExample(path: string, url: string, env: Record<string, string>): Promise<ChildProcess> {
  if (await listening(url)) throw new Error(`${url} is already in use. Stop the process listening there and run the test again.`);
  const child = spawn("go", ["run", path], { cwd: repo, env: { ...process.env, ...env }, stdio: "inherit", detached: true });
  for (let i = 0; i < 240; i++) {
    if (await listening(url)) return child;
    if (child.exitCode !== null) break;
    await new Promise((r) => setTimeout(r, 500));
  }
  await stopExample(child, url);
  throw new Error(`${path} did not start on ${url}`);
}

export async function stopExample(child: ChildProcess | undefined, url: string) {
  if (!child?.pid) return;
  try {
    process.kill(-child.pid, "SIGTERM");
  } catch {}
  for (let i = 0; i < 40 && (await listening(url)); i++) await new Promise((r) => setTimeout(r, 250));
}
