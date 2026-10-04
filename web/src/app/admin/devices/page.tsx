import { Laptop } from "lucide-react";
import { DevicesTable } from "@/components/policy/devices-table";
import { Card } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { pluralize } from "@/lib/format";
import type { Device } from "@/lib/policy-types";
import type { User } from "@/lib/types";

export const metadata = { title: "Devices" };

export default async function DevicesPage() {
  const [devices, users] = await Promise.all([apiGet<Device[]>("/devices"), apiGet<User[]>("/users")]);
  const trusted = devices.filter((d) => d.trust === "trusted").length;
  const blocked = devices.filter((d) => d.trust === "blocked").length;
  const people = new Set(devices.map((d) => d.userId)).size;
  return (
    <div className="flex animate-page flex-col gap-8">
      <PageHeader
        title="Devices"
        description={`${pluralize(devices.length, "device")} used by ${pluralize(people, "person", "people")}, ${trusted} trusted and ${blocked} blocked. Blocking a device refuses new sign-ins from it and signs out its open sessions.`}
      />
      {devices.length ? (
        <DevicesTable devices={devices} owners={Object.fromEntries(users.map((u) => [u.id, { name: u.name, email: u.email }]))} />
      ) : (
        <Card>
          <EmptyState icon={Laptop} title="No devices yet" description="Halo registers a device the first time someone signs in from it. Trust or block it here afterwards." />
        </Card>
      )}
    </div>
  );
}
