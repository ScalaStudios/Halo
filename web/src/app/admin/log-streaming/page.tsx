import { hasRole } from "@/components/settings/roles";
import { LogExportForm, LogExportStatus } from "@/components/settings/log-export";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { PROTOCOLS } from "@/lib/labels";
import type { LogExportResponse } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export const metadata = { title: "Log streaming" };

export default async function LogStreamingPage() {
  const [data, me] = await Promise.all([apiGet<LogExportResponse>("/log-export"), apiGet<User>("/me")]);
  const canEdit = hasRole(me, "security_admin");
  return (
    <div className="flex max-w-4xl flex-col gap-8">
      <PageHeader
        title="Log streaming"
        description={
          data.exporter
            ? `Every sign-in and audit event is sent to ${data.exporter.endpoint} (${PROTOCOLS[data.exporter.protocol].label}).`
            : "Send every sign-in and audit event to a SIEM or log collector, as syslog or OpenTelemetry logs. Sign-ins never wait for the collector."
        }
      />
      <LogExportStatus key={data.exporter?.updatedAt} data={data} canEdit={canEdit} />
      <LogExportForm key={data.exporter?.updatedAt ?? "new"} data={data} canEdit={canEdit} />
    </div>
  );
}
