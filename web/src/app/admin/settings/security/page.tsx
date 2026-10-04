import { SecurityForm } from "@/components/settings/settings-forms";
import { hasRole } from "@/components/settings/roles";
import { SigningKeys } from "@/components/settings/signing-keys";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import type { SettingsResponse, SigningKeys as Keys } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export const metadata = { title: "Security defaults" };

export default async function SecurityDefaultsPage() {
  const [{ settings }, me, keys] = await Promise.all([apiGet<SettingsResponse>("/settings"), apiGet<User>("/me"), apiGet<Keys>("/signing-keys")]);
  return (
    <div className="flex max-w-4xl animate-page flex-col gap-8">
      <PageHeader
        title="Security defaults"
        description={`Sessions last ${settings.sessionHours} hours and ${settings.lockoutThreshold} incorrect codes lock an account for ${settings.lockoutMinutes} minutes. Changes reach every Halo server within 30 seconds.`}
      />
      <SecurityForm settings={settings} canEdit={hasRole(me, "security_admin")} />
      <SigningKeys keys={keys} canRotate={hasRole(me)} />
    </div>
  );
}
