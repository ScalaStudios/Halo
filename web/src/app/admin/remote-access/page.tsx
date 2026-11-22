import { SetupGuideCard } from "@/components/applications/shared";
import { OutpostsSection } from "@/components/remote/outposts";
import { RemoteSessions } from "@/components/remote/sessions";
import { hasRole } from "@/components/settings/roles";
import { PageHeader } from "@/components/ui/page-header";
import { SectionTitle } from "@/components/ui/section-title";
import { apiGet } from "@/lib/api/server";
import { frameMessages } from "@/lib/i18n/console/frame";
import { remoteMessages } from "@/lib/i18n/console/infrastructure/remote";
import { requestLocale } from "@/lib/i18n/server";
import type { Outpost, RemoteSession } from "@/lib/infra-types";
import type { Organization, User } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.remoteAccess.label };
}

export default async function RemoteAccessPage() {
  const [outposts, sessions, org, me, locale] = await Promise.all([apiGet<Outpost[]>("/outposts"), apiGet<RemoteSession[]>("/remote-sessions"), apiGet<Organization>("/organization"), apiGet<User>("/me"), requestLocale()]);
  const t = remoteMessages[locale].page;
  const live = sessions.filter((s) => s.status === "active").length;
  const connected = outposts.filter((o) => o.connected).length;

  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title={frameMessages[locale].nav.items.remoteAccess.label}
        description={t.description(connected, live)}
      />

      <OutpostsSection outposts={outposts} issuer={org.issuer} canEdit={hasRole(me, "security_admin")} />

      <SetupGuideCard
        guide={{
          title: t.guide,
          description: t.guideHint,
          file: t.guideFile,
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
          title={t.sessions}
          description={t.sessionsHint}
        />
        <RemoteSessions
          sessions={sessions}
          canEnd={hasRole(me, "security_admin", "app_admin")}
          canDownload={hasRole(me, "security_admin", "auditor")}
          empty={t.empty}
        />
      </div>
    </div>
  );
}
