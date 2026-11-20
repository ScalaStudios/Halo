import type { ApprovalRequest } from "@/lib/settings-types";
import { ApprovalPending, toApiError } from "./error";

export { ApiError, ApprovalPending } from "./error";

export async function api<T = void>(path: string, init: { method?: string; body?: unknown } = {}): Promise<T> {
  const hasBody = init.body !== undefined;
  const res = await fetch(`/api/v1${path}`, {
    method: init.method ?? (hasBody ? "POST" : "GET"),
    headers: hasBody ? { "content-type": "application/json" } : undefined,
    body: hasBody ? JSON.stringify(init.body) : undefined,
    credentials: "same-origin",
  });
  if (!res.ok) throw await toApiError(res, "Halo could not complete the request. Try again.");
  if (res.status === 204) return undefined as T;
  const body = await res.json();
  if (res.status === 202 && body?.status === "pending") throw new ApprovalPending(body as ApprovalRequest);
  return body as T;
}
