import type { Metadata } from "next";
import type { ReactNode } from "react";
import { AccountShell } from "@/components/account/shell";
import { BrandStyle } from "@/components/brand-style";
import { apiGet } from "@/lib/api/server";
import { accountMessages } from "@/lib/i18n/account";
import { requestLocale } from "@/lib/i18n/server";
import type { OrganizationProfile } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export async function generateMetadata(): Promise<Metadata> {
  const t = accountMessages(await requestLocale());
  return { title: { default: t.title, template: t.titleTemplate } };
}

export default async function AccountLayout({ children }: { children: ReactNode }) {
  const [user, org, locale] = await Promise.all([apiGet<User>("/me"), apiGet<OrganizationProfile>("/organization"), requestLocale()]);
  return (
    <>
      <BrandStyle org={org} />
      <AccountShell locale={locale} user={{ name: user.name, email: user.email, roles: user.roles, avatarUrl: user.avatarUrl, impersonation: user.impersonation }} featuresOff={org.featuresOff}>
        {children}
      </AccountShell>
    </>
  );
}
