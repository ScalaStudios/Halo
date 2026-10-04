import { PackagesTable } from "@/components/governance/packages-table";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { pluralize } from "@/lib/format";
import type { AccessPackage } from "@/lib/governance-types";
import type { Group, User } from "@/lib/types";

export const metadata = { title: "Access packages" };

export default async function AccessPackagesPage() {
  const [packages, groups, users] = await Promise.all([apiGet<AccessPackage[]>("/access-packages"), apiGet<Group[]>("/groups"), apiGet<User[]>("/users")]);
  const requestable = packages.filter((p) => !p.archived).length;
  const pending = packages.reduce((sum, p) => sum + p.pendingRequests, 0);
  return (
    <div className="flex animate-page flex-col gap-8">
      <PageHeader
        title="Access packages"
        description={`${pluralize(requestable, "package")} people can request, ${pluralize(pending, "request")} waiting for a decision. Approved access is removed automatically when it expires.`}
      />
      <PackagesTable
        packages={packages}
        groups={groups.filter((g) => g.kind === "assigned")}
        people={users.filter((u) => u.status !== "deprovisioned").map((u) => ({ id: u.id, name: u.name, email: u.email }))}
      />
    </div>
  );
}
