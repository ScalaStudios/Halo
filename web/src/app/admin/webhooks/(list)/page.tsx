import { hasRole } from "@/components/settings/roles";
import { WebhooksTable } from "@/components/settings/webhooks";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { frameMessages } from "@/lib/i18n/console/frame";
import { infrastructureMessages } from "@/lib/i18n/console/infrastructure";
import { requestLocale } from "@/lib/i18n/server";
import type { Webhook, WebhookEventFamily } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.webhooks.label };
}

export default async function WebhooksPage() {
  const [webhooks, eventTypes, me, locale] = await Promise.all([apiGet<Webhook[]>("/webhooks"), apiGet<WebhookEventFamily[]>("/webhooks/event-types"), apiGet<User>("/me"), requestLocale()]);
  const enabled = webhooks.filter((w) => w.enabled).length;
  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title={frameMessages[locale].nav.items.webhooks.label}
        description={infrastructureMessages[locale].webhooks.description(webhooks.length, enabled)}
      />
      <WebhooksTable webhooks={webhooks} eventTypes={eventTypes} canEdit={hasRole(me, "security_admin")} />
    </div>
  );
}
