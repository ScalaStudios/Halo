import { hasRole } from "@/components/settings/roles";
import { RulesTable } from "@/components/settings/notification-rules";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { pluralize } from "@/lib/format";
import type { NotificationRule, WebhookEventFamily } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export const metadata = { title: "Notification rules" };

export default async function NotificationRulesPage() {
  const [rules, eventTypes, me] = await Promise.all([apiGet<NotificationRule[]>("/notification-rules"), apiGet<WebhookEventFamily[]>("/webhooks/event-types"), apiGet<User>("/me")]);
  const enabled = rules.filter((r) => r.enabled).length;
  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title="Notification rules"
        description={`${pluralize(rules.length, "rule")}, ${enabled} enabled. Each rule posts the security events it chooses to a Slack or Discord channel, or to a webhook, within about 15 seconds.`}
      />
      <RulesTable rules={rules} eventTypes={eventTypes} canEdit={hasRole(me, "security_admin")} />
    </div>
  );
}
