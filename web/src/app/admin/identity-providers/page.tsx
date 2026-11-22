import { ProvidersConsole } from "@/components/federation/providers-console";
import { apiGet } from "@/lib/api/server";
import type { IdentityProviderList } from "@/lib/federation-types";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";
import type { Group } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.identityProviders.label };
}

export default async function IdentityProvidersPage() {
  const [{ callbackUrl, providers }, groups] = await Promise.all([apiGet<IdentityProviderList>("/identity-providers"), apiGet<Group[]>("/groups")]);
  return <ProvidersConsole callbackUrl={callbackUrl} providers={providers} groups={groups} />;
}
