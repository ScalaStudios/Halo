import { SetupGuideCard } from "@/components/applications/shared";
import { OutpostsSection } from "@/components/remote/outposts";
import { RemoteSessions } from "@/components/remote/sessions";
import { hasRole } from "@/components/settings/roles";
import { PageHeader } from "@/components/ui/page-header";
import { SectionTitle } from "@/components/ui/section-title";
import { apiGet } from "@/lib/api/server";
import { pluralize } from "@/lib/format";
import type { Outpost, RemoteSession } from "@/lib/infra-types";
import type { Organization, User } from "@/lib/types";

export const metadata = { title: "Remote access" };

export default async function RemoteAccessPage() {
  const [outposts, sessions, org, me] = await Promise.all([apiGet<Outpost[]>("/outposts"), apiGet<RemoteSession[]>("/remote-sessions"), apiGet<Organization>("/organization"), apiGet<User>("/me")]);
  const live = sessions.filter((s) => s.status === "active").length;
  const connected = outposts.filter((o) => o.connected).length;

  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title="Remote access"
        description={`SSH, RDP, and VNC sessions in the browser to hosts in your networks, started from the account portal under the same checks as an application sign-in. ${pluralize(connected, "outpost is", "outposts are")} connected and ${pluralize(live, "session is", "sessions are")} live.`}
      />

      <OutpostsSection outposts={outposts} issuer={org.issuer} canEdit={hasRole(me, "security_admin")} />

      <SetupGuideCard
        guide={{
          title: "Run an outpost",
          description:
            "Run halo-outpost on a machine that reaches your hosts, with the token from Register outpost. It keeps one HTTPS connection open to Halo and opens a TCP connection to a host only when a session to it starts. Then add a remote access application for each host.",
          file: "Outpost with Docker",
          code: [
            "docker run -d --name halo-outpost --restart unless-stopped \\",
            "  -v halo-outpost:/home/nonroot \\",
            "  --entrypoint /usr/local/bin/halo-outpost \\",
            "  ghcr.io/scalastudios/halo-server \\",
            `  --server ${org.issuer} --token <enrollment token> --key-file /home/nonroot/halo-outpost.key`,
          ].join("\n"),
        }}
      />

      <div className="flex flex-col gap-6">
        <SectionTitle
          title="Sessions"
          description="The 200 most recent sessions, newest first. Halo keeps them, with their recordings, for as long as the sign-in log. Ending a session closes it at once."
        />
        <RemoteSessions
          sessions={sessions}
          canEnd={hasRole(me, "security_admin", "app_admin")}
          canDownload={hasRole(me, "security_admin", "auditor")}
          empty="Sessions appear here when someone opens a remote access application from their account portal."
        />
      </div>
    </div>
  );
}
