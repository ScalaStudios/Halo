import { expect, test } from "@playwright/test";
import { devSession, enroll, halo, invite, json } from "./support";

test.use({ ignoreHTTPSErrors: true });

for (const protocol of ["vnc", "rdp"] as const) {
  test(`browser ${protocol.toUpperCase()} displays a desktop and can be terminated`, async ({ page, playwright }) => {
    const outpostId = process.env.HALO_TEST_OUTPOST_ID;
    test.skip(!outpostId, "Requires an enrolled outpost and disposable RDP and VNC hosts.");
    const prefix = `HALO_TEST_${protocol.toUpperCase()}_`;
    const credential = process.env[`${prefix}PASSWORD`];
    const cookie = devSession("luna@example.com");
    const admin = await playwright.request.newContext({ baseURL: halo, ignoreHTTPSErrors: true, extraHTTPHeaders: { cookie } });
    const group = await json<{ id: string }>(await admin.post("/api/v1/groups", { data: { name: `Desktop test ${protocol} ${Date.now()}`, kind: "assigned" } }));
    let appId: string | undefined;
    let userId: string | undefined;
    try {
      const name = `Desktop user ${protocol}`;
      const user = await invite(admin, `desktop-${protocol}-${Date.now()}@example.com`, name, [group.id]);
      userId = user.id;
      await enroll(page, user.enrollUrl, name);
      const { application } = await json<{ application: { id: string } }>(
        await admin.post("/api/v1/applications", { data: {
          name: `Browser ${protocol.toUpperCase()} fixture ${Date.now()}`,
          protocol: "remote",
          groupIds: [group.id],
          remote: {
            protocol, outpostId, host: "127.0.0.1", port: Number(process.env[`${prefix}PORT`]),
            auth: "password", username: process.env[`${prefix}USERNAME`] ?? "", credential,
            ...(protocol === "rdp" ? { hostKey: process.env[`${prefix}FINGERPRINT`], rdpSecurity: "tls" } : {}),
          },
        } }),
      );
      appId = application.id;
      const outbound: string[] = [];
      const inbound: string[] = [];
      page.on("websocket", (socket) => {
        socket.on("framesent", ({ payload }) => outbound.push(String(payload)));
        socket.on("framereceived", ({ payload }) => inbound.push(String(payload)));
      });
      const started = page.waitForResponse((res) => res.url().endsWith("/api/v1/me/remote-sessions") && res.ok());
      await page.goto(`/account/applications/${appId}/connect`);
      const session = (await (await started).json()) as { id: string };
      const display = page.locator(`[aria-label^="${protocol.toUpperCase()} remote desktop"]`);
      await expect(display).toBeFocused();
      await expect(display.locator("canvas").first()).toBeVisible();
      await expect.poll(() => inbound.some((frame) => /\.(cfill|img|jpeg|png),/.test(frame))).toBe(true);
      await page.keyboard.press("a");
      await expect.poll(() => outbound.some((frame) => frame.includes('"opcode":"key"'))).toBe(true);
      await display.click({ position: { x: 10, y: 10 }, timeout: 5000 });
      await expect.poll(() => outbound.some((frame) => frame.includes('"opcode":"mouse"'))).toBe(true);
      await page.keyboard.press("Shift+Escape");
      await expect(display).not.toBeFocused();
      expect((await admin.post(`/api/v1/remote-sessions/${session.id}/terminate`)).ok()).toBe(true);
      await expect(page.getByText(/ended the session from the Halo console/)).toBeVisible();
      const sessions = await json<{ id: string; status: string; protocol: string; startedAt: string; endedAt: string }[]>(
        await admin.get(`/api/v1/remote-sessions?app=${appId}`),
      );
      const saved = sessions.find((row) => row.id === session.id);
      expect(saved).toMatchObject({ status: "ended", protocol, startedAt: expect.any(String), endedAt: expect.any(String) });
      expect(credential).toBeTruthy();
      expect(inbound.join("")).not.toContain(credential);
    } finally {
      if (appId) await admin.delete(`/api/v1/applications/${appId}`);
      if (userId) await admin.delete(`/api/v1/users/${userId}`);
      await admin.delete(`/api/v1/groups/${group.id}`);
      await admin.dispose();
    }
  });
}
