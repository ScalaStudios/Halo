import type { Metadata } from "next";
import { PageHeader } from "@/components/ui/page-header";
import { SecurityEmails, type SecurityEmail } from "@/components/account/security-emails";
import { SecuritySettings, type PasskeyRules } from "@/components/account/security-settings";
import { apiGet } from "@/lib/api/server";
import { accountMessages } from "@/lib/i18n/account";
import { requestLocale } from "@/lib/i18n/server";
import type { User } from "@/lib/types";

export async function generateMetadata(): Promise<Metadata> {
  return { title: accountMessages(await requestLocale()).nav.security };
}

export default async function SecurityPage() {
  const [user, password, rules, emails, locale] = await Promise.all([
    apiGet<User>("/me"),
    apiGet<{ allowed: boolean; secondFactor: boolean }>("/me/password"),
    apiGet<PasskeyRules>("/me/passkey-rules"),
    apiGet<SecurityEmail[]>("/me/security-emails"),
    requestLocale(),
  ]);
  const copy = accountMessages(locale);
  return (
    <div className="flex flex-col gap-12">
      <PageHeader size="large"
        title={copy.nav.security}
        description={copy.security.description}
      />
      <SecuritySettings methods={user.methods} email={user.email} password={password} rules={rules} />
      <SecurityEmails emails={emails} address={user.email} />
    </div>
  );
}
