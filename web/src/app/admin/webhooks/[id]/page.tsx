import Link from "next/link";
import { ArrowLeft, Webhook as WebhookIcon } from "lucide-react";
import { SetupGuideCard } from "@/components/applications/shared";
import { hasRole } from "@/components/settings/roles";
import { DeliveryLog, EnabledBadge, EventTags, WebhookActions } from "@/components/settings/webhooks";
import { DescriptionList } from "@/components/ui/description-list";
import { SectionTitle } from "@/components/ui/section-title";
import { apiGet } from "@/lib/api/server";
import { formatDate, formatRelative, pluralize } from "@/lib/format";
import type { Webhook, WebhookDelivery } from "@/lib/settings-types";
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
  const [{ webhook, deliveries }, eventTypes, me] = await Promise.all([
    apiGet<Detail>(`/webhooks/${encodeURIComponent(id)}`),
    apiGet<string[]>("/webhooks/event-types"),
    apiGet<User>("/me"),
  ]);
  const failed = deliveries.filter((d) => d.status === "failed").length;

  return (
    <div className="flex animate-page flex-col gap-8">
      <div className="flex flex-col gap-6">
        <Link href="/admin/webhooks" className="inline-flex w-fit items-center gap-2 rounded-sm text-body-sm text-fg-3 transition-colors duration-fast hover:text-fg">
          <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
          Webhooks
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
              <p className="text-body text-fg-3">{webhook.description || "Webhook endpoint without a description."}</p>
            </div>
          </div>
          {hasRole(me, "security_admin") ? <WebhookActions webhook={webhook} eventTypes={eventTypes} /> : null}
        </header>
        <DescriptionList
          className="grid-cols-2 rounded-lg border border-border bg-surface p-6 md:grid-cols-4"
          items={[
            { label: "Events", value: <EventTags events={webhook.events} /> },
            { label: "Last delivery", value: webhook.lastDeliveryAt ? formatRelative(webhook.lastDeliveryAt) : "Nothing sent yet" },
            { label: "Created", value: formatDate(webhook.createdAt) },
            { label: "Endpoint ID", value: webhook.id, mono: true },
          ]}
        />
      </div>

      <section className="flex flex-col gap-6">
        <SectionTitle
          title="Deliveries"
          description={`${deliveries.length ? `The latest ${pluralize(deliveries.length, "delivery", "deliveries")}, newest first.` : ""}${failed ? ` ${failed} gave up after 8 attempts.` : ""} Failed requests are retried after 30 seconds, then 1, 2, 4, 8, 16 and 32 minutes.`.trim()}
        />
        <DeliveryLog deliveries={deliveries} />
      </section>

      <SetupGuideCard
        guide={{
          title: "Verify the signature",
          description:
            "Every request carries Halo-Signature: t=<unix time>,v1=<hex HMAC-SHA256 of t, a dot and the raw body, keyed with the signing secret>. Reject requests with a wrong signature or a timestamp more than 5 minutes old, and use the id field to ignore repeats.",
          file: "verify.go",
          code: VERIFY,
        }}
      />
    </div>
  );
}
