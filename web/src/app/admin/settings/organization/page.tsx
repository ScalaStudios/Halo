import { OrganizationForm } from "@/components/settings/settings-forms";
import { hasRole } from "@/components/settings/roles";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import type { SettingsResponse } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export const metadata = { title: "Organization" };

export default async function OrganizationPage() {
  const [data, me] = await Promise.all([apiGet<SettingsResponse>("/settings"), apiGet<User>("/me")]);
  return (
    <div className="flex max-w-4xl animate-page flex-col gap-8">
      <PageHeader title="Organization" description="The name and contact address people see when they sign in, and the issuer URL your applications trust." />
      <OrganizationForm settings={data.settings} defaultName={data.defaultOrganizationName} issuer={data.issuer} canEdit={hasRole(me)} />
    </div>
  );
}
