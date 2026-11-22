import { hasRole } from "@/components/settings/roles";
import { LogExportForm, LogExportStatus } from "@/components/settings/log-export";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { frameMessages } from "@/lib/i18n/console/frame";
import { infrastructureMessages } from "@/lib/i18n/console/infrastructure";
import { settingsMessages } from "@/lib/i18n/console/settings";
import { requestLocale } from "@/lib/i18n/server";
import type { LogExportResponse } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.logStreaming.label };
}

export default async function LogStreamingPage() {
  const [data, me, locale] = await Promise.all([apiGet<LogExportResponse>("/log-export"), apiGet<User>("/me"), requestLocale()]);
  const t = infrastructureMessages[locale].logStreaming;
  const canEdit = hasRole(me, "security_admin");
  return (
    <div className="flex max-w-4xl flex-col gap-8">
      <PageHeader
        title={frameMessages[locale].nav.items.logStreaming.label}
        description={data.exporter ? t.sending(data.exporter.endpoint, settingsMessages[locale].logExport.protocols[data.exporter.protocol].label) : t.description}
      />
      <LogExportStatus key={data.exporter?.updatedAt} data={data} canEdit={canEdit} />
      <LogExportForm key={data.exporter?.updatedAt ?? "new"} data={data} canEdit={canEdit} />
    </div>
  );
}
