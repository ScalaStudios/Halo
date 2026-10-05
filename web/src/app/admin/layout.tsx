import type { Metadata } from "next";
import { redirect } from "next/navigation";
import type { ReactNode } from "react";
import { ConsoleShell } from "@/components/console/shell";
import { apiGet } from "@/lib/api/server";
import type { Organization, User } from "@/lib/types";

export const metadata: Metadata = {
  title: { default: "Halo administration", template: "%s · Halo administration" },
};

export default async function AdminLayout({ children }: { children: ReactNode }) {
  const [me, org] = await Promise.all([apiGet<User>("/me"), apiGet<Organization>("/organization")]);
  if (me.roles.length === 0) redirect("/account");
  return (
    <ConsoleShell user={{ name: me.name, email: me.email, avatarUrl: me.avatarUrl }} orgName={org.name}>
      {children}
    </ConsoleShell>
  );
}
