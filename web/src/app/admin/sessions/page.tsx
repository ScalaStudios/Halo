import { SessionsTable } from "@/components/monitoring/sessions-table";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { pluralize } from "@/lib/format";
import type { Session, User } from "@/lib/types";

export const metadata = { title: "Sessions" };

export default async function SessionsPage() {
  const [sessions, users] = await Promise.all([apiGet<Session[]>("/sessions"), apiGet<User[]>("/users")]);
  const people = new Set(sessions.map((s) => s.userId)).size;
  return (
    <div className="flex animate-page flex-col gap-8">
      <PageHeader
        title="Sessions"
        description={`${pluralize(sessions.length, "active session")} across ${pluralize(people, "person", "people")}. Revoking a session signs that device out of Halo.`}
      />
      <SessionsTable sessions={sessions} userNames={Object.fromEntries(users.map((u) => [u.id, u.name]))} />
    </div>
  );
}
