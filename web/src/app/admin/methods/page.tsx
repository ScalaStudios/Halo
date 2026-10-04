import { MethodSettings } from "@/components/policy/method-settings";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import type { MethodSetting } from "@/lib/policy-types";

export const metadata = { title: "Methods" };

export default async function MethodsPage() {
  const methods = await apiGet<MethodSetting[]>("/methods");
  const allowed = methods.filter((m) => m.enabled).length;
  return (
    <div className="flex max-w-4xl animate-page flex-col gap-8">
      <PageHeader
        title="Methods"
        description={`${allowed} of ${methods.length} sign-in methods are allowed. Turning a method off blocks every sign-in that uses it, for everyone in the organization.`}
      />
      <MethodSettings methods={methods} />
    </div>
  );
}
