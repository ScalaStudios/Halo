import type { Metadata } from "next";
import { PageHeader } from "@/components/ui/page-header";
import { SecuritySettings } from "@/components/account/security-settings";
import { apiGet } from "@/lib/api/server";
import type { User } from "@/lib/types";

export const metadata: Metadata = { title: "Security" };

export default async function SecurityPage() {
  const user = await apiGet<User>("/me");
  return (
    <div className="flex animate-page flex-col gap-12">
      <PageHeader size="large"
        title="Security"
        description="How you prove it's you. Keep at least two passkeys or security keys so losing one device doesn't lock you out."
      />
      <SecuritySettings methods={user.methods} email={user.email} />
    </div>
  );
}
