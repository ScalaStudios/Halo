import { CertificateAuthorities } from "@/components/policy/certificate-authorities";
import { KerberosSettings } from "@/components/policy/kerberos";
import { MethodSettings } from "@/components/policy/method-settings";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { frameMessages } from "@/lib/i18n/console/frame";
import { settingsMessages } from "@/lib/i18n/console/settings";
import { requestLocale } from "@/lib/i18n/server";
import type { CertificateAuthority, KerberosSource, MethodSetting, NetworkZone } from "@/lib/policy-types";
import type { OrganizationProfile, SettingsResponse } from "@/lib/settings-types";
import type { Group } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.methods.label };
}

export default async function MethodsPage() {
  const [org, locale] = await Promise.all([apiGet<OrganizationProfile>("/organization"), requestLocale()]);
  const windows = !org.featuresOff.includes("kerberos");
  const [all, { settings }, groups, authorities, kerberos, zones] = await Promise.all([
    apiGet<MethodSetting[]>("/methods"),
    apiGet<SettingsResponse>("/settings"),
    apiGet<Group[]>("/groups"),
    apiGet<CertificateAuthority[]>("/certificate-authorities"),
    windows ? apiGet<KerberosSource | null>("/kerberos") : null,
    windows ? apiGet<NetworkZone[]>("/network-zones") : [],
  ]);
  const methods = all.filter((m) => windows || m.method !== "kerberos");
  const allowed = methods.filter((m) => m.enabled).length;
  return (
    <div className="flex max-w-4xl flex-col gap-8">
      <PageHeader title={frameMessages[locale].nav.items.methods.label} description={settingsMessages[locale].methods.description(allowed, methods.length)} />
      <MethodSettings methods={methods} settings={settings} groups={groups.map((g) => ({ id: g.id, name: g.name, detail: g.description }))} />
      <CertificateAuthorities authorities={authorities} />
      {windows ? <KerberosSettings key={kerberos?.updatedAt ?? "new"} source={kerberos} zones={zones} host={new URL(org.issuer).hostname} /> : null}
    </div>
  );
}
