import Link from "next/link";
import { ArrowLeft, Webhook as WebhookIcon } from "lucide-react";
import { SetupGuideCard } from "@/components/applications/shared";
import { hasRole } from "@/components/settings/roles";
import { DeliveryLog, EnabledBadge, EventTags, WebhookActions } from "@/components/settings/webhooks";
import { DescriptionList } from "@/components/ui/description-list";
import { SectionTitle } from "@/components/ui/section-title";
import { apiGet } from "@/lib/api/server";
import { formatDate, formatRelative } from "@/lib/format";
import { frameMessages } from "@/lib/i18n/console/frame";
import { infrastructureMessages } from "@/lib/i18n/console/infrastructure";
import { requestLocale } from "@/lib/i18n/server";
import type { Webhook, WebhookDelivery, WebhookEventFamily } from "@/lib/settings-types";
import type { User } from "@/lib/types";

type Detail = { webhook: Webhook; deliveries: WebhookDelivery[] };

export async function generateMetadata({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const { webhook } = await apiGet<Detail>(`/webhooks/${encodeURIComponent(id)}`);
  return { title: new URL(webhook.url).host };
}

const VERIFY = `func verify(secret []byte, header string, body []byte) bool {
	var t, sig string
	for _, part := range strings.Split(header, ",") {
		key, value, _ := strings.Cut(part, "=")
		if key == "t" {
			t = value
		} else if key == "v1" {
			sig = value
		}
	}
	unix, err := strconv.ParseInt(t, 10, 64)
	if err != nil || time.Since(time.Unix(unix, 0)).Abs() > 5*time.Minute {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(t + "."))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(want), []byte(sig))
}`;

export default async function WebhookPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const [{ webhook, deliveries }, eventTypes, me, locale] = await Promise.all([
    apiGet<Detail>(`/webhooks/${encodeURIComponent(id)}`),
    apiGet<WebhookEventFamily[]>("/webhooks/event-types"),
    apiGet<User>("/me"),
    requestLocale(),
  ]);
  const t = infrastructureMessages[locale];
  const failed = deliveries.filter((d) => d.status === "failed").length;

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-6">
        <Link href="/admin/webhooks" className="inline-flex w-fit items-center gap-2 rounded-sm text-body-sm text-fg-3 transition-colors duration-fast hover:text-fg">
          <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
          {frameMessages[locale].nav.items.webhooks.label}
        </Link>
        <header className="flex flex-col gap-6 lg:flex-row lg:items-start lg:justify-between">
          <div className="flex min-w-0 items-center gap-4">
            <span className="grid size-16 shrink-0 place-items-center rounded-lg border border-border bg-sunken text-fg-3">
              <WebhookIcon aria-hidden="true" size={24} strokeWidth={1.75} />
            </span>
            <div className="flex min-w-0 flex-col gap-1">
              <div className="flex flex-wrap items-center gap-3">
                <h1 className="truncate font-mono text-h4 text-fg">{webhook.url}</h1>
                <EnabledBadge enabled={webhook.enabled} />
              </div>
              <p className="text-body text-fg-3">{webhook.description || t.webhooks.noDescription}</p>
            </div>
          </div>
          {hasRole(me, "security_admin") ? <WebhookActions webhook={webhook} eventTypes={eventTypes} /> : null}
        </header>
        <DescriptionList
          className="grid-cols-2 rounded-lg border border-border bg-surface p-6 md:grid-cols-4"
          items={[
            { label: t.webhooks.events, value: <EventTags events={webhook.events} /> },
            { label: t.webhooks.lastDelivery, value: webhook.lastDeliveryAt ? formatRelative(webhook.lastDeliveryAt, locale) : t.webhooks.nothingSent },
            { label: t.created, value: formatDate(webhook.createdAt, locale) },
            { label: t.webhooks.endpointId, value: webhook.id, mono: true },
          ]}
        />
      </div>

      <section className="flex flex-col gap-6">
        <SectionTitle
          title={t.webhooks.deliveries}
          description={t.webhooks.deliveriesHint(deliveries.length, failed)}
        />
        <DeliveryLog deliveries={deliveries} />
      </section>

      <SetupGuideCard
        guide={{
          title: t.webhooks.verify,
          description: t.webhooks.verifyHint,
          file: "verify.go",
          code: VERIFY,
        }}
      />
    </div>
  );
}
