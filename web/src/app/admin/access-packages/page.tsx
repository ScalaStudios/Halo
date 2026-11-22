import { PackagesTable } from "@/components/governance/packages-table";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import type { AccessPackage } from "@/lib/governance-types";
import { frameMessages } from "@/lib/i18n/console/frame";
import { governanceMessages } from "@/lib/i18n/console/governance";
import { requestLocale } from "@/lib/i18n/server";
import type { Group, User } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.accessPackages.label };
}

export default async function AccessPackagesPage() {
  const [packages, groups, users, locale] = await Promise.all([apiGet<AccessPackage[]>("/access-packages"), apiGet<Group[]>("/groups"), apiGet<User[]>("/users"), requestLocale()]);
  const requestable = packages.filter((p) => !p.archived).length;
  const pending = packages.reduce((sum, p) => sum + p.pendingRequests, 0);
  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title={frameMessages[locale].nav.items.accessPackages.label}
        description={governanceMessages[locale].pages.packagesDescription(requestable, pending)}
      />
      <PackagesTable
        packages={packages}
        groups={groups.filter((g) => g.kind === "assigned")}
        people={users.filter((u) => u.status !== "deprovisioned").map((u) => ({ id: u.id, name: u.name, email: u.email }))}
      />
    </div>
  );
}
