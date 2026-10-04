import { ProvidersConsole } from "@/components/federation/providers-console";
import { apiGet } from "@/lib/api/server";
import type { IdentityProviderList } from "@/lib/federation-types";
import type { Group } from "@/lib/types";

export const metadata = { title: "Identity providers" };

export default async function IdentityProvidersPage() {
  const [{ callbackUrl, providers }, groups] = await Promise.all([apiGet<IdentityProviderList>("/identity-providers"), apiGet<Group[]>("/groups")]);
  return <ProvidersConsole callbackUrl={callbackUrl} providers={providers} groups={groups} />;
}
