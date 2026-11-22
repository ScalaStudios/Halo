import {
  BotProtectionForm,
  ClientRegistrationForm,
  IdentityProofingForm,
  ImpersonationForm,
  SecurityEmailsForm,
  SecurityForm,
  SessionLimitForm,
  SignUpForm,
} from "@/components/settings/settings-forms";
import { hasRole } from "@/components/settings/roles";
import { SigningKeys } from "@/components/settings/signing-keys";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { frameMessages } from "@/lib/i18n/console/frame";
import { settingsMessages } from "@/lib/i18n/console/settings";
import { requestLocale } from "@/lib/i18n/server";
import type { Domain, EmailTemplate, SettingsResponse, SigningKeys as Keys } from "@/lib/settings-types";
import type { Group, User } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.security.label };
}

export default async function SecurityDefaultsPage() {
  const [{ settings, issuer, botProtectionSecretSet, identityProofingKeySet, identityProofingWebhookSecretSet }, me, keys, groups, domains, templates, locale] = await Promise.all([
    apiGet<SettingsResponse>("/settings"),
    apiGet<User>("/me"),
    apiGet<Keys>("/signing-keys"),
    apiGet<Group[]>("/groups"),
    apiGet<Domain[]>("/domains"),
    apiGet<EmailTemplate[]>("/email-templates"),
    requestLocale(),
  ]);
  return (
    <div className="flex max-w-4xl flex-col gap-8">
      <PageHeader
        title={frameMessages[locale].nav.items.security.label}
        description={settingsMessages[locale].security.description(settings.sessionHours, settings.lockoutThreshold, settings.lockoutMinutes)}
      />
      <SecurityForm settings={settings} canEdit={hasRole(me, "security_admin")} canEditGlobal={hasRole(me)} />
      <SessionLimitForm settings={settings} groups={groups} canEdit={hasRole(me, "security_admin")} />
      <SecurityEmailsForm settings={settings} templates={templates} canEdit={hasRole(me, "security_admin")} />
      <ImpersonationForm settings={settings} groups={groups} canEdit={hasRole(me)} />
      <ClientRegistrationForm settings={settings} issuer={issuer} canEdit={hasRole(me, "security_admin")} />
      <SignUpForm settings={settings} groups={groups} domains={domains} canEdit={hasRole(me, "security_admin")} />
      <IdentityProofingForm
        settings={settings}
        groups={groups}
        issuer={issuer}
        keySet={identityProofingKeySet}
        webhookSecretSet={identityProofingWebhookSecretSet}
        canEdit={hasRole(me, "security_admin")}
      />
      <BotProtectionForm settings={settings} secretSet={botProtectionSecretSet} canEdit={hasRole(me, "security_admin")} />
      <SigningKeys keys={keys} canRotate={hasRole(me)} />
    </div>
  );
}
