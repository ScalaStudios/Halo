import type { ApprovalRequest } from "@/lib/settings-types";
import { ApprovalPending, toApiError } from "./error";

export { ApiError, ApprovalPending } from "./error";

type Confirm = () => Promise<boolean>;

let confirmIdentity: Confirm | null = null;
let confirming: Promise<boolean> | null = null;

export function handleReauth(confirm: Confirm | null) {
  confirmIdentity = confirm;
}

export async function send(path: string, init: RequestInit, fallback: string): Promise<Response> {
  const request = () => fetch(`/api/v1${path}`, { ...init, credentials: "same-origin" });
  let res = await request();
  if (!res.ok) {
    const error = await toApiError(res, fallback);
    if (path === "/auth/continue" || error.code !== "ERR_REAUTH_REQUIRED" || !confirmIdentity) throw error;
    confirming ??= confirmIdentity().finally(() => (confirming = null));
    if (!(await confirming)) throw error;
    res = await request();
    if (!res.ok) throw await toApiError(res, fallback);
  }
  return res;
}

export async function api<T = void>(path: string, init: { method?: string; body?: unknown } = {}): Promise<T> {
  const hasBody = init.body !== undefined;
  const res = await send(
    path,
    {
      method: init.method ?? (hasBody ? "POST" : "GET"),
      headers: hasBody ? { "content-type": "application/json" } : undefined,
      body: hasBody ? JSON.stringify(init.body) : undefined,
    },
    "Halo could not complete the request. Try again.",
  );
  if (res.status === 204) return undefined as T;
  const body = await res.json();
  if (res.status === 202 && body?.status === "pending") throw new ApprovalPending(body as ApprovalRequest);
  return body as T;
}
