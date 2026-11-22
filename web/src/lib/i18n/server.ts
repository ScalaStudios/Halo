import { cookies, headers } from "next/headers";
import { cache } from "react";
import { API } from "@/lib/api/server";
import { preferredLocale, type Locale } from "./locale";

async function chosenLocale(path: string, session?: string) {
  const res = await fetch(`${API}/api/v1${path}`, { headers: session ? { cookie: `halo_session=${session}` } : undefined, cache: "no-store" }).catch(() => null);
  return res?.ok ? ((await res.json()) as { locale: Locale | null }).locale : null;
}

export const requestLocale = cache(async (): Promise<Locale> => {
  const session = (await cookies()).get("halo_session")?.value;
  const [personal, organization] = await Promise.all([session ? chosenLocale("/me", session) : null, chosenLocale("/organization")]);
  return personal ?? preferredLocale((await headers()).get("accept-language")) ?? organization ?? "en";
});
