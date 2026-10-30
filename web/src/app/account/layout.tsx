import type { Metadata } from "next";
import type { ReactNode } from "react";
import { AccountShell } from "@/components/account/shell";
import { BrandStyle } from "@/components/brand-style";
import { apiGet } from "@/lib/api/server";
import type { OrganizationProfile } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export const metadata: Metadata = {
  title: { default: "Halo account", template: "%s · Halo account" },
};

export default async function AccountLayout({ children }: { children: ReactNode }) {
  const [user, org] = await Promise.all([apiGet<User>("/me"), apiGet<OrganizationProfile>("/organization")]);
  return (
    <>
      <BrandStyle org={org} />
      <AccountShell user={{ name: user.name, email: user.email, roles: user.roles, avatarUrl: user.avatarUrl }}>{children}</AccountShell>
    </>
  );
}
