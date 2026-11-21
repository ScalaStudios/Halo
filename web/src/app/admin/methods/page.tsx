import { CertificateAuthorities } from "@/components/policy/certificate-authorities";
import { KerberosSettings } from "@/components/policy/kerberos";
import { MethodSettings } from "@/components/policy/method-settings";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import type { CertificateAuthority, KerberosSource, MethodSetting, NetworkZone } from "@/lib/policy-types";
import type { OrganizationProfile, SettingsResponse } from "@/lib/settings-types";
import type { Group } from "@/lib/types";

export const metadata = { title: "Methods" };

export default async function MethodsPage() {
  const org = await apiGet<OrganizationProfile>("/organization");
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
      <PageHeader
        title="Methods"
        description={`${allowed} of ${methods.length} sign-in methods are allowed. Turning a method off blocks every sign-in that uses it, for everyone in the organization.`}
      />
      <MethodSettings methods={methods} settings={settings} groups={groups.map((g) => ({ id: g.id, name: g.name, detail: g.description }))} />
      <CertificateAuthorities authorities={authorities} />
      {windows ? <KerberosSettings key={kerberos?.updatedAt ?? "new"} source={kerberos} zones={zones} host={new URL(org.issuer).hostname} /> : null}
    </div>
  );
}
