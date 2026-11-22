import { LogoEditor } from "@/components/settings/logo-editor";
import { hasRole } from "@/components/settings/roles";
import { CustomCssForm, SignInMessageForm } from "@/components/settings/settings-forms";
import { ThemeEditor } from "@/components/settings/theme-editor";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { frameMessages } from "@/lib/i18n/console/frame";
import { settingsMessages } from "@/lib/i18n/console/settings";
import { requestLocale } from "@/lib/i18n/server";
import type { OrganizationProfile, SettingsResponse } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.branding.label };
}

export default async function BrandingPage() {
  const [org, { settings, customCssDisabled }, me, locale] = await Promise.all([
    apiGet<OrganizationProfile>("/organization"),
    apiGet<SettingsResponse>("/settings"),
    apiGet<User>("/me"),
    requestLocale(),
  ]);
  const canEdit = hasRole(me);
  return (
    <div className="flex max-w-4xl flex-col gap-8">
      <PageHeader title={frameMessages[locale].nav.items.branding.label} description={settingsMessages[locale].branding.description} />
      <LogoEditor org={org} canEdit={canEdit} />
      <SignInMessageForm settings={settings} canEdit={canEdit} />
      <ThemeEditor org={org} saved={settings.theme} canEdit={canEdit} />
      <CustomCssForm settings={settings} disabled={customCssDisabled} canEdit={canEdit} />
    </div>
  );
}
