import Link from "next/link";
import { ArrowLeft, BellRing } from "lucide-react";
import { RuleActions, RuleConditions } from "@/components/settings/notification-rules";
import { hasRole } from "@/components/settings/roles";
import { DeliveryLog, EnabledBadge, EventTags } from "@/components/settings/webhooks";
import { DescriptionList } from "@/components/ui/description-list";
import { SectionTitle } from "@/components/ui/section-title";
import { apiGet } from "@/lib/api/server";
import { formatRelative } from "@/lib/format";
import { frameMessages } from "@/lib/i18n/console/frame";
import { policyMessages } from "@/lib/i18n/console/policy";
import { requestLocale } from "@/lib/i18n/server";
import { DESTINATIONS } from "@/lib/labels";
import type { NotificationRule, WebhookDelivery, WebhookEventFamily } from "@/lib/settings-types";
import type { User } from "@/lib/types";

type Detail = { rule: NotificationRule; deliveries: WebhookDelivery[] };

export async function generateMetadata({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const { rule } = await apiGet<Detail>(`/notification-rules/${encodeURIComponent(id)}`);
  return { title: rule.name };
}

export default async function NotificationRulePage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const [{ rule, deliveries }, eventTypes, me, locale] = await Promise.all([
    apiGet<Detail>(`/notification-rules/${encodeURIComponent(id)}`),
    apiGet<WebhookEventFamily[]>("/webhooks/event-types"),
    apiGet<User>("/me"),
    requestLocale(),
  ]);
  const t = policyMessages[locale].notificationRules;
  const failed = deliveries.filter((d) => d.status === "failed").length;

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-6">
        <Link href="/admin/notification-rules" className="inline-flex w-fit items-center gap-2 rounded-sm text-body-sm text-fg-3 transition-colors duration-fast hover:text-fg">
          <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
          {frameMessages[locale].nav.items.notificationRules.label}
        </Link>
        <header className="flex flex-col gap-6 lg:flex-row lg:items-start lg:justify-between">
          <div className="flex min-w-0 items-center gap-4">
            <span className="grid size-16 shrink-0 place-items-center rounded-lg border border-border bg-sunken text-fg-3">
              <BellRing aria-hidden="true" size={24} strokeWidth={1.75} />
            </span>
            <div className="flex min-w-0 flex-col gap-1">
              <div className="flex flex-wrap items-center gap-3">
                <h1 className="truncate text-h4 text-fg">{rule.name}</h1>
                <EnabledBadge enabled={rule.enabled} />
              </div>
              <p className="text-body text-fg-3">
                {DESTINATIONS[rule.destination].label} · <span className="font-mono text-code-sm">{rule.host}</span>
              </p>
            </div>
          </div>
          {hasRole(me, "security_admin") ? <RuleActions rule={rule} eventTypes={eventTypes} /> : null}
        </header>
        <DescriptionList
          className="grid-cols-2 rounded-lg border border-border bg-surface p-6 md:grid-cols-4"
          items={[
            { label: t.events, value: <EventTags events={rule.events} /> },
            { label: t.conditions, value: <RuleConditions rule={rule} /> },
            { label: t.lastMessage, value: rule.lastSentAt ? formatRelative(rule.lastSentAt, locale) : t.nothingSent },
            { label: t.ruleId, value: rule.id, mono: true },
          ]}
        />
      </div>

      <section className="flex flex-col gap-6">
        <SectionTitle
          title={t.messages}
          description={t.messagesDescription(deliveries.length, failed)}
        />
        <DeliveryLog deliveries={deliveries} />
      </section>
    </div>
  );
}
