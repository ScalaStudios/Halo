import { RulesTable } from "@/components/settings/notification-rules";
import { hasRole } from "@/components/settings/roles";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { frameMessages } from "@/lib/i18n/console/frame";
import { policyMessages } from "@/lib/i18n/console/policy";
import { requestLocale } from "@/lib/i18n/server";
import type { NotificationRule, WebhookEventFamily } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.notificationRules.label };
}

export default async function NotificationRulesPage() {
  const [rules, eventTypes, me, locale] = await Promise.all([
    apiGet<NotificationRule[]>("/notification-rules"),
    apiGet<WebhookEventFamily[]>("/webhooks/event-types"),
    apiGet<User>("/me"),
    requestLocale(),
  ]);
  const enabled = rules.filter((r) => r.enabled).length;
  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title={frameMessages[locale].nav.items.notificationRules.label}
        description={policyMessages[locale].notificationRules.summary(rules.length, enabled)}
      />
      <RulesTable rules={rules} eventTypes={eventTypes} canEdit={hasRole(me, "security_admin")} />
    </div>
  );
}
