import { hasRole } from "@/components/settings/roles";
import { WebhooksTable } from "@/components/settings/webhooks";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { pluralize } from "@/lib/format";
import type { Webhook } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export const metadata = { title: "Webhooks" };

export default async function WebhooksPage() {
  const [webhooks, eventTypes, me] = await Promise.all([apiGet<Webhook[]>("/webhooks"), apiGet<string[]>("/webhooks/event-types"), apiGet<User>("/me")]);
  const enabled = webhooks.filter((w) => w.enabled).length;
  return (
    <div className="flex animate-page flex-col gap-8">
      <PageHeader
        title="Webhooks"
        description={`${pluralize(webhooks.length, "endpoint")}, ${enabled} enabled. Halo checks for new sign-ins and audit events every 10 seconds and posts each one as signed JSON.`}
      />
      <WebhooksTable webhooks={webhooks} eventTypes={eventTypes} canEdit={hasRole(me, "security_admin")} />
    </div>
  );
}
