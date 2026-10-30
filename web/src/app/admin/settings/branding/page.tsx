import { LogoEditor } from "@/components/settings/logo-editor";
import { hasRole } from "@/components/settings/roles";
import { CustomCssForm, SignInMessageForm } from "@/components/settings/settings-forms";
import { ThemeEditor } from "@/components/settings/theme-editor";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import type { OrganizationProfile, SettingsResponse } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export const metadata = { title: "Branding" };

export default async function BrandingPage() {
  const [org, { settings, customCssDisabled }, me] = await Promise.all([apiGet<OrganizationProfile>("/organization"), apiGet<SettingsResponse>("/settings"), apiGet<User>("/me")]);
  const canEdit = hasRole(me);
  return (
    <div className="flex max-w-4xl animate-page flex-col gap-8">
      <PageHeader title="Branding" description="Your logo, colours and a short message on the pages people see when they sign in, set up a passkey or manage their account." />
      <LogoEditor org={org} canEdit={canEdit} />
      <SignInMessageForm settings={settings} canEdit={canEdit} />
      <ThemeEditor org={org} saved={settings.theme} canEdit={canEdit} />
      <CustomCssForm settings={settings} disabled={customCssDisabled} canEdit={canEdit} />
    </div>
  );
}
