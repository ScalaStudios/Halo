import { cookies } from "next/headers";
import { notFound, redirect } from "next/navigation";
import { toApiError } from "./error";

const API = process.env.HALO_API_URL ?? "http://localhost:8080";

export async function apiGet<T>(path: string): Promise<T> {
  const session = (await cookies()).get("halo_session")?.value;
  const res = await fetch(`${API}/api/v1${path}`, {
    headers: session ? { cookie: `halo_session=${session}` } : undefined,
    cache: "no-store",
  });
  if (res.status === 401) redirect("/sign-in");
  if (res.status === 403) redirect("/account");
  if (res.status === 404) notFound();
  if (!res.ok) throw await toApiError(res, "Halo could not load this page. Check that the Halo server is running.");
  return (await res.json()) as T;
}

export type Problem = { code: string; message: string };

export async function apiPeek<T>(path: string): Promise<{ data: T; problem: null } | { data: null; problem: Problem }> {
  const res = await fetch(`${API}/api/v1${path}`, { cache: "no-store" });
  if (res.ok) return { data: (await res.json()) as T, problem: null };
  const { code, message } = await toApiError(res, "Halo could not load this page. Check that the Halo server is running.");
  return { data: null, problem: { code, message } };
}
