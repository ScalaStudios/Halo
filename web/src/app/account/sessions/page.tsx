import type { Metadata } from "next";
import { PageHeader } from "@/components/ui/page-header";
import { SessionList } from "@/components/identity/session-list";
import { apiGet } from "@/lib/api/server";
import type { Session } from "@/lib/types";

export const metadata: Metadata = { title: "Sessions" };

export default async function SessionsPage() {
  const sessions = await apiGet<Session[]>("/me/sessions");
  return (
    <div className="flex animate-page flex-col gap-12">
      <PageHeader size="large"
        title="Sessions"
        description="Browsers, devices and Halo CLI sign-ins on your Halo account. If you don't recognise one, sign it out and review your security."
      />
      <SessionList sessions={sessions} />
    </div>
  );
}
