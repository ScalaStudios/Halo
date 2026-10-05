import type { Metadata } from "next";
import type { ReactNode } from "react";
import { AccountShell } from "@/components/account/shell";
import { apiGet } from "@/lib/api/server";
import type { User } from "@/lib/types";

export const metadata: Metadata = {
  title: { default: "Halo account", template: "%s · Halo account" },
};

export default async function AccountLayout({ children }: { children: ReactNode }) {
  const user = await apiGet<User>("/me");
  return <AccountShell user={{ name: user.name, email: user.email, roles: user.roles, avatarUrl: user.avatarUrl }}>{children}</AccountShell>;
}
