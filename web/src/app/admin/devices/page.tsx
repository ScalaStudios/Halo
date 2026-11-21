import { Laptop } from "lucide-react";
import { ComplianceSources } from "@/components/policy/compliance-sources";
import { DevicesTable } from "@/components/policy/devices-table";
import { Card } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { PageHeader } from "@/components/ui/page-header";
import { TabNav } from "@/components/ui/tab-nav";
import { apiGet } from "@/lib/api/server";
import { pluralize } from "@/lib/format";
import type { ComplianceSource, Device } from "@/lib/policy-types";
import type { Organization, User } from "@/lib/types";

export const metadata = { title: "Devices" };

export default async function DevicesPage({ searchParams }: { searchParams: Promise<{ tab?: string }> }) {
  const { tab = "devices" } = await searchParams;
  const [devices, users, sources, org] = await Promise.all([
    apiGet<Device[]>("/devices"),
    apiGet<User[]>("/users"),
    apiGet<ComplianceSource[]>("/compliance-sources"),
    apiGet<Organization>("/organization"),
  ]);
  const trusted = devices.filter((d) => d.trust === "trusted").length;
  const blocked = devices.filter((d) => d.trust === "blocked").length;
  const compliant = devices.filter((d) => d.compliance?.state === "compliant").length;
  const people = new Set(devices.map((d) => d.userId)).size;
  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title="Devices"
        description={`${pluralize(devices.length, "device")} used by ${pluralize(people, "person", "people")}, ${trusted} trusted, ${blocked} blocked and ${compliant} compliant. Blocking a device refuses new sign-ins from it and signs out its open sessions.`}
      />
      <TabNav
        label="Device sections"
        tabs={[
          { id: "devices", label: "Devices", count: devices.length },
          { id: "sources", label: "Compliance sources", count: sources.length },
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
            <EmptyState icon={Laptop} title="No devices yet" description="Halo registers a device the first time someone signs in from it. Trust or block it here afterwards." />
          </Card>
        )}
      </div>
    </div>
  );
}
