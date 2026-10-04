"use client";

import { MoreHorizontal, Pause, Pencil, Play, Plus, Send, Trash2, Webhook as WebhookIcon } from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import { SecretWarning } from "@/components/applications/shared";
import { useMutation } from "@/components/console/use-mutation";
import { Badge, StatusDot, Tag, type Tone } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Checkbox, Field, Input, Select } from "@/components/ui/input";
import { Menu, MenuContent, MenuItem, MenuSeparator, MenuTrigger } from "@/components/ui/menu";
import { RelativeTime } from "@/components/ui/relative-time";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { formatDateTime, formatTime } from "@/lib/format";
import type { DeliveryStatus, Webhook, WebhookDelivery } from "@/lib/settings-types";

export const DELIVERY_STATUS: Record<DeliveryStatus, { label: string; tone: Tone }> = {
  delivered: { label: "Delivered", tone: "success" },
  pending: { label: "Retrying", tone: "warning" },
  failed: { label: "Failed", tone: "danger" },
};

export function EventTags({ events }: { events: string[] }) {
  if (events.includes("*")) return <Tag>All events</Tag>;
  return (
    <span className="flex flex-wrap items-center gap-1">
      {events.slice(0, 3).map((event) => (
        <Tag key={event}>{event}</Tag>
      ))}
      {events.length > 3 ? <span className="text-caption whitespace-nowrap text-fg-3">and {events.length - 3} more</span> : null}
    </span>
  );
}

export function EnabledBadge({ enabled }: { enabled: boolean }) {
  return (
    <Badge tone={enabled ? "success" : "neutral"} dot>
      {enabled ? "Enabled" : "Paused"}
    </Badge>
  );
}

export function WebhooksTable({ webhooks, eventTypes, canEdit }: { webhooks: Webhook[]; eventTypes: string[]; canEdit: boolean }) {
  const [creating, setCreating] = useState(false);
  const add = canEdit ? (
    <Button size="sm" variant="primary" onClick={() => setCreating(true)}>
      <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
      Add endpoint
    </Button>
  ) : null;

  return (
    <>
      {webhooks.length === 0 ? (
        <Card>
          <EmptyState
            icon={WebhookIcon}
            title="No webhook endpoints yet"
            description="Add an HTTPS endpoint to receive sign-ins and administrative changes as signed JSON within seconds of them happening."
            action={add}
          />
        </Card>
      ) : (
        <div className="flex flex-col gap-4">
          {add ? <div className="flex justify-end">{add}</div> : null}
          <SimpleTable
            caption="Webhook endpoints"
            head={["Endpoint", "Events", "Status", "Last delivery"]}
            rows={webhooks.map((w) => [
              <Link key="url" href={`/admin/webhooks/${w.id}`} className="flex min-w-64 flex-col gap-1 py-2">
                <span className="truncate font-mono text-code-sm text-fg hover:text-ember">{w.url}</span>
                {w.description ? <span className="truncate text-caption text-fg-3">{w.description}</span> : null}
              </Link>,
              <EventTags key="events" events={w.events} />,
              <EnabledBadge key="status" enabled={w.enabled} />,
              w.lastStatus && w.lastDeliveryAt ? (
                <span key="last" className="inline-flex items-center gap-2 whitespace-nowrap" title={formatDateTime(w.lastDeliveryAt)}>
                  <StatusDot tone={DELIVERY_STATUS[w.lastStatus].tone} />
                  {DELIVERY_STATUS[w.lastStatus].label} · <RelativeTime iso={w.lastDeliveryAt} />
                </span>
              ) : (
                <span key="last" className="text-fg-3">
                  Nothing sent yet
                </span>
              ),
            ])}
          />
        </div>
      )}
      {creating ? <WebhookDialog webhook={null} eventTypes={eventTypes} onClose={() => setCreating(false)} /> : null}
    </>
  );
}

function WebhookDialog({ webhook, eventTypes, onClose }: { webhook: Webhook | null; eventTypes: string[]; onClose: () => void }) {
  const { pending, run, toast, router } = useMutation();
  const [url, setUrl] = useState(webhook?.url ?? "");
  const [description, setDescription] = useState(webhook?.description ?? "");
  const [all, setAll] = useState(webhook ? webhook.events.includes("*") : true);
  const [events, setEvents] = useState<string[]>(webhook?.events.filter((e) => e !== "*") ?? []);
  const [enabled, setEnabled] = useState(webhook?.enabled ?? true);
  const [errors, setErrors] = useState<{ url?: string; events?: string }>({});
  const [created, setCreated] = useState<{ webhook: Webhook; secret: string } | null>(null);
  const choices = [...new Set([...eventTypes, ...events])].sort();

  function submit() {
    const next = {
      url: /^https:\/\/\S+$|^http:\/\/(localhost|127\.0\.0\.1|\[::1\])(:\d+)?(\/\S*)?$/.test(url.trim()) ? undefined : "Enter an https:// URL. Plain http:// works only for localhost.",
      events: all || events.length ? undefined : "Choose at least one event family, or send all events.",
    };
    setErrors(next);
    if (next.url || next.events) return;
    const body = { url: url.trim(), description: description.trim(), events: all ? ["*"] : events, enabled };
    if (!webhook) {
      void run("save", () => api<{ webhook: Webhook; secret: string }>("/webhooks", { body }), setCreated);
      return;
    }
    void run("save", () => api<Webhook>(`/webhooks/${webhook.id}`, { method: "PUT", body }), () => {
      toast({ title: "Endpoint saved", description: "Events from now on follow the new settings." });
      onClose();
    });
  }

  if (created) {
    return (
      <Dialog
        open
        onOpenChange={(open) => {
          if (!open) router.push(`/admin/webhooks/${created.webhook.id}`);
        }}
        title="Endpoint added"
        description="Verify the Halo-Signature header of every request with this signing secret."
        footer={
          <Button variant="primary" onClick={() => router.push(`/admin/webhooks/${created.webhook.id}`)}>
            Done
          </Button>
        }
      >
        <div className="flex animate-enter flex-col gap-4">
          <CopyField label="Signing secret" value={created.secret} secret />
          <SecretWarning>This is the only time Halo shows this secret. Store it with the receiving service before closing this dialog.</SecretWarning>
        </div>
      </Dialog>
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={webhook ? "Edit endpoint" : "Add endpoint"}
      description="Halo sends each event as a signed JSON POST and retries failures up to 8 times over about an hour."
      width="wide"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={submit}>
            {webhook ? "Save endpoint" : "Add endpoint"}
          </Button>
        </>
      }
    >
      <form
        noValidate
        className="flex flex-col gap-6"
        onSubmit={(event) => {
          event.preventDefault();
          submit();
        }}
      >
        <Field label="URL" error={errors.url}>
          {(props) => <Input {...props} mono autoFocus type="url" autoComplete="off" spellCheck={false} value={url} placeholder="https://hooks.example.com/halo" onChange={(event) => setUrl(event.target.value)} />}
        </Field>
        <Field label="Description" hint="Optional. Say which system receives the events.">
          {(props) => <Input {...props} maxLength={200} autoComplete="off" value={description} placeholder="SIEM ingestion" onChange={(event) => setDescription(event.target.value)} />}
        </Field>
        <div className="flex flex-col gap-4">
          <Field label="Events" error={errors.events}>
            {(props) => (
              <Select {...props} value={all ? "all" : "some"} onChange={(event) => setAll(event.target.value === "all")}>
                <option value="all">All events, including types added in later Halo versions</option>
                <option value="some">Only the event families I choose</option>
              </Select>
            )}
          </Field>
          {all ? null : (
            <fieldset className="grid grid-cols-1 gap-2 sm:grid-cols-2 md:grid-cols-3">
              <legend className="sr-only">Event families</legend>
              {choices.map((type) => (
                <label key={type} className="flex cursor-pointer items-center gap-2">
                  <Checkbox checked={events.includes(type)} onChange={(event) => setEvents(event.target.checked ? [...events, type] : events.filter((e) => e !== type))} />
                  <span className="font-mono text-code-sm text-fg-2">{type}</span>
                </label>
              ))}
            </fieldset>
          )}
        </div>
        <label className="flex cursor-pointer items-start gap-3">
          <Checkbox className="mt-0.5" checked={enabled} onChange={(event) => setEnabled(event.target.checked)} />
          <span className="flex flex-col">
            <span className="text-body-sm text-fg">Send events</span>
            <span className="text-caption text-fg-3">Paused endpoints keep their settings and queued deliveries, and receive only test events until you resume them.</span>
          </span>
        </label>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}

export function WebhookActions({ webhook, eventTypes }: { webhook: Webhook; eventTypes: string[] }) {
  const { pending, run, toast, router } = useMutation();
  const [editing, setEditing] = useState(false);
  const [deleting, setDeleting] = useState(false);

  function sendTest() {
    void run("test", () => api<WebhookDelivery>(`/webhooks/${webhook.id}/test`, { method: "POST" }), (delivery) =>
      delivery.status === "delivered"
        ? toast({ title: "Test event delivered", description: `The endpoint answered ${delivery.responseStatus}.` })
        : toast({ title: "Test event not delivered", description: delivery.error || "The endpoint did not answer with a 2xx status.", tone: "danger" }),
    );
  }

  function setEnabled(enabled: boolean) {
    void run("toggle", () => api(`/webhooks/${webhook.id}`, { method: "PUT", body: { url: webhook.url, description: webhook.description, events: webhook.events, enabled } }), () =>
      toast({ title: enabled ? "Endpoint resumed" : "Endpoint paused", description: enabled ? "Queued and new events are delivered again." : "Events are queued but not sent until you resume it." }),
    );
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button variant="primary" loading={pending === "test"} onClick={sendTest}>
        <Send aria-hidden="true" size={16} strokeWidth={1.75} />
        Send test event
      </Button>
      <Menu>
        <MenuTrigger asChild>
          <Button size="icon" variant="secondary" aria-label="More endpoint actions" loading={pending === "toggle"}>
            <MoreHorizontal aria-hidden="true" size={16} strokeWidth={1.75} />
          </Button>
        </MenuTrigger>
        <MenuContent>
          <MenuItem icon={Pencil} onSelect={() => setEditing(true)}>
            Edit endpoint
          </MenuItem>
          <MenuItem icon={webhook.enabled ? Pause : Play} onSelect={() => setEnabled(!webhook.enabled)}>
            {webhook.enabled ? "Pause endpoint" : "Resume endpoint"}
          </MenuItem>
          <MenuSeparator />
          <MenuItem icon={Trash2} danger onSelect={() => setDeleting(true)}>
            Delete endpoint
          </MenuItem>
        </MenuContent>
      </Menu>
      {editing ? <WebhookDialog webhook={webhook} eventTypes={eventTypes} onClose={() => setEditing(false)} /> : null}
      <Dialog
        open={deleting}
        onOpenChange={setDeleting}
        title="Delete this endpoint?"
        description="Halo stops sending events to it and deletes its delivery log. Queued retries are dropped."
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              loading={pending === "delete"}
              onClick={() =>
                void run("delete", () => api(`/webhooks/${webhook.id}`, { method: "DELETE" }), () => {
                  toast({ title: "Endpoint deleted", description: "Recorded in the audit log." });
                  router.push("/admin/webhooks");
                })
              }
            >
              Delete endpoint
            </Button>
          </>
        }
      />
    </div>
  );
}

export function DeliveryLog({ deliveries }: { deliveries: WebhookDelivery[] }) {
  const [open, setOpen] = useState<WebhookDelivery | null>(null);
  if (deliveries.length === 0) {
    return (
      <Card>
        <EmptyState icon={Send} title="No deliveries yet" description="Deliveries appear here within about 15 seconds of a matching event. Send a test event to check the endpoint now." />
      </Card>
    );
  }
  return (
    <>
      <SimpleTable
        caption="Deliveries"
        head={["Event", "Status", "Attempts", "Response", "Last attempt", "Payload"]}
        rows={deliveries.map((d) => [
          <span key="event" className="flex flex-col items-start gap-1">
            <Tag>{d.eventType}</Tag>
            <span className="font-mono text-code-sm text-fg-3">{d.eventId}</span>
          </span>,
          <span key="status" className="flex flex-col items-start gap-1">
            <Badge tone={DELIVERY_STATUS[d.status].tone} dot>
              {DELIVERY_STATUS[d.status].label}
            </Badge>
            {d.status === "pending" && d.attempts > 0 ? <span className="text-caption whitespace-nowrap text-fg-3">Next try {formatTime(d.nextAttemptAt)}</span> : null}
          </span>,
          <span key="attempts" className="tnum">
            {d.attempts} of 8
          </span>,
          <span key="response" className="flex max-w-64 flex-col gap-1">
            <span className="tnum font-mono text-code-sm">{d.responseStatus ?? "—"}</span>
            {d.error ? <span className="truncate text-caption text-fg-3" title={d.error}>{d.error}</span> : null}
          </span>,
          <span key="last" className="tnum whitespace-nowrap" title={d.lastAttemptAt ? formatDateTime(d.lastAttemptAt) : undefined}>
            {d.lastAttemptAt ? <RelativeTime iso={d.lastAttemptAt} /> : "Queued"}
          </span>,
          <Button key="payload" size="sm" variant="quiet" onClick={() => setOpen(d)}>
            View
          </Button>,
        ])}
      />
      <Dialog
        open={open !== null}
        onOpenChange={(next) => {
          if (!next) setOpen(null);
        }}
        title={open ? open.eventType : "Payload"}
        description="The exact JSON body Halo signs and sends. Retries send the same body."
        width="wide"
      >
        {open ? <pre className="overflow-x-auto rounded-md border border-border bg-sunken p-4 font-mono text-code-sm text-fg-2">{JSON.stringify(JSON.parse(open.body), null, 2)}</pre> : null}
      </Dialog>
    </>
  );
}
