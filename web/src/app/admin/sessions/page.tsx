import { SessionsTable } from "@/components/monitoring/sessions-table";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { directoryMessages } from "@/lib/i18n/console/directory";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";
import type { Session, User } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.sessions.label };
}

export default async function SessionsPage() {
  const [sessions, users, locale] = await Promise.all([apiGet<Session[]>("/sessions"), apiGet<User[]>("/users"), requestLocale()]);
  const people = new Set(sessions.map((s) => s.userId)).size;
  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title={frameMessages[locale].nav.items.sessions.label}
        description={directoryMessages[locale].sessions.description(sessions.length, people)}
      />
      <SessionsTable sessions={sessions} userNames={Object.fromEntries(users.map((u) => [u.id, u.name]))} />
    </div>
  );
}
