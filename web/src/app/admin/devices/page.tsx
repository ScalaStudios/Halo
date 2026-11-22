import { Laptop } from "lucide-react";
import { ComplianceSources } from "@/components/policy/compliance-sources";
import { DevicesTable } from "@/components/policy/devices-table";
import { Card } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { PageHeader } from "@/components/ui/page-header";
import { TabNav } from "@/components/ui/tab-nav";
import { apiGet } from "@/lib/api/server";
import { directoryMessages } from "@/lib/i18n/console/directory";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";
import type { ComplianceSource, Device } from "@/lib/policy-types";
import type { Organization, User } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.devices.label };
}

export default async function DevicesPage({ searchParams }: { searchParams: Promise<{ tab?: string }> }) {
  const { tab = "devices" } = await searchParams;
  const [devices, users, sources, org, locale] = await Promise.all([
    apiGet<Device[]>("/devices"),
    apiGet<User[]>("/users"),
    apiGet<ComplianceSource[]>("/compliance-sources"),
    apiGet<Organization>("/organization"),
    requestLocale(),
  ]);
  const t = directoryMessages[locale];
  const title = frameMessages[locale].nav.items.devices.label;
  const trusted = devices.filter((d) => d.trust === "trusted").length;
  const blocked = devices.filter((d) => d.trust === "blocked").length;
  const compliant = devices.filter((d) => d.compliance?.state === "compliant").length;
  const people = new Set(devices.map((d) => d.userId)).size;
  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title={title}
        description={t.devices.description(devices.length, people, trusted, blocked, compliant)}
      />
      <TabNav
        label={t.devices.sections}
        tabs={[
          { id: "devices", label: title, count: devices.length },
          { id: "sources", label: t.devices.complianceSources, count: sources.length },
        ]}
        active={tab}
        hrefFor={(id) => (id === "devices" ? "/admin/devices" : `/admin/devices?tab=${id}`)}
      />
      <div key={tab} className="flex animate-enter flex-col gap-8">
        {tab === "sources" ? (
          <ComplianceSources sources={sources} issuer={org.issuer} />
        ) : devices.length ? (
          <DevicesTable devices={devices} sources={sources} owners={Object.fromEntries(users.map((u) => [u.id, { name: u.name, email: u.email }]))} />
        ) : (
          <Card>
            <EmptyState icon={Laptop} title={t.devices.emptyTitle} description={t.devices.emptyDescription} />
          </Card>
        )}
      </div>
    </div>
  );
}
