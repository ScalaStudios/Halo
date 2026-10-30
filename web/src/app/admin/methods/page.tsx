import { MethodSettings } from "@/components/policy/method-settings";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import type { MethodSetting } from "@/lib/policy-types";
import type { SettingsResponse } from "@/lib/settings-types";
import type { Group } from "@/lib/types";

export const metadata = { title: "Methods" };

export default async function MethodsPage() {
  const [methods, { settings }, groups] = await Promise.all([apiGet<MethodSetting[]>("/methods"), apiGet<SettingsResponse>("/settings"), apiGet<Group[]>("/groups")]);
  const allowed = methods.filter((m) => m.enabled).length;
  return (
    <div className="flex max-w-4xl flex-col gap-8">
      <PageHeader
        title="Methods"
        description={`${allowed} of ${methods.length} sign-in methods are allowed. Turning a method off blocks every sign-in that uses it, for everyone in the organization.`}
      />
      <MethodSettings methods={methods} settings={settings} groups={groups.map((g) => ({ id: g.id, name: g.name, detail: g.description }))} />
    </div>
  );
}
