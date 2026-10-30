import type { Metadata } from "next";
import { redirect } from "next/navigation";
import type { ReactNode } from "react";
import { BrandStyle } from "@/components/brand-style";
import { ConsoleShell } from "@/components/console/shell";
import { apiGet } from "@/lib/api/server";
import type { OrganizationProfile } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export const metadata: Metadata = {
  title: { default: "Halo administration", template: "%s · Halo administration" },
};

export default async function AdminLayout({ children }: { children: ReactNode }) {
  const [me, org] = await Promise.all([apiGet<User>("/me"), apiGet<OrganizationProfile>("/organization")]);
  if (me.roles.length === 0) redirect("/account");
  return (
    <>
      {org.consoleBranding ? <BrandStyle org={org} /> : null}
      <ConsoleShell user={{ name: me.name, email: me.email, avatarUrl: me.avatarUrl }} orgName={org.name}>
        {children}
      </ConsoleShell>
    </>
  );
}
