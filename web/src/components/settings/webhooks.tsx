"use client";

import { ChevronDown, MoreHorizontal, Pause, Pencil, Play, Plus, Search, Send, Trash2, Webhook as WebhookIcon } from "lucide-react";
import Link from "next/link";
import { useId, useState } from "react";
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
import { cn } from "@/lib/cn";
import { formatDateTime, formatTime } from "@/lib/format";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { settingsMessages } from "@/lib/i18n/console/settings";
import type { DeliveryStatus, Webhook, WebhookDelivery, WebhookEventFamily } from "@/lib/settings-types";

export const DELIVERY_TONE: Record<DeliveryStatus, Tone> = { delivered: "success", pending: "warning", failed: "danger" };

export function EventTags({ events }: { events: string[] }) {
  const t = useMessages(settingsMessages).webhooks;
  if (events.includes("*")) return <Tag>{t.allEvents}</Tag>;
  return (
    <span className="flex flex-wrap items-center gap-1">
      {events.slice(0, 3).map((event) => (
        <Tag key={event}>{event}</Tag>
      ))}
      {events.length > 3 ? <span className="text-caption whitespace-nowrap text-fg-3">{t.more(events.length - 3)}</span> : null}
    </span>
  );
}

export function EnabledBadge({ enabled }: { enabled: boolean }) {
  const t = useMessages(settingsMessages).webhooks;
  return (
    <Badge tone={enabled ? "success" : "neutral"} dot>
      {enabled ? t.enabled : t.paused}
    </Badge>
  );
}

function covers(pattern: string, type: string) {
  return pattern === type || (pattern.endsWith(".*") && type.startsWith(pattern.slice(0, -1)));
}

export function EventPicker({ families, value, onChange }: { families: WebhookEventFamily[]; value: string[]; onChange: (value: string[]) => void }) {
  const id = useId();
  const copy = useMessages(settingsMessages).webhooks;
  const [query, setQuery] = useState("");
  const covered = (type: string) => value.some((p) => covers(p, type));
  const [expanded, setExpanded] = useState(() => families.filter((f) => !value.includes(f.family) && f.types.some((t) => covered(t.type))).map((f) => f.family));
  const [others] = useState(() => value.filter((p) => !families.some((f) => f.family === p || f.types.some((t) => t.type === p))));
  const term = query.trim().toLowerCase();
  const matches = (text: string) => text.toLowerCase().includes(term);
  const shownOthers = others.filter(matches);
  const empty = !shownOthers.length && !families.some((f) => f.types.some((t) => matches(`${t.type} ${t.description}`)));

  function setFamily(family: string, on: boolean) {
    onChange([...value.filter((p) => !p.startsWith(family.slice(0, -1))), ...(on ? [family] : [])]);
  }

  function setType(f: WebhookEventFamily, type: string, on: boolean) {
    if (on) return onChange([...value, type]);
    const kept = value.filter((p) => !covers(p, type));
    onChange([...kept, ...f.types.map((t) => t.type).filter((t) => t !== type && covered(t) && !kept.some((p) => covers(p, t)))]);
  }

  return (
    <div className="flex flex-col gap-3">
      <Input
        size="sm"
        type="search"
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Enter") event.preventDefault();
        }}
        placeholder={copy.searchEvents}
        aria-label={copy.searchEvents}
        leading={<Search size={16} strokeWidth={1.75} />}
      />
      <fieldset>
        <legend className="sr-only">{copy.eventsToSend}</legend>
        <div className="max-h-80 overflow-y-auto rounded-md border border-border bg-sunken p-2">
          {empty ? (
            <p className="px-2 py-4 text-body-sm text-fg-3">{copy.noMatch(query.trim())}</p>
          ) : (
            <ul className="flex flex-col">
              {families.map((f) => {
                const visible = f.types.filter((t) => matches(`${t.type} ${t.description}`));
                if (!visible.length) return null;
                const whole = value.includes(f.family);
                const count = f.types.filter((t) => covered(t.type)).length;
                const open = term !== "" || expanded.includes(f.family);
                const domId = `${id}-${f.family}`;
                return (
                  <li key={f.family} className="flex items-start">
                    {term ? (
                      <span className="size-8 shrink-0" />
                    ) : (
                      <button
                        type="button"
                        aria-label={copy.eventsIn(f.family)}
                        aria-expanded={open}
                        aria-controls={open ? `${domId}-types` : undefined}
                        onClick={() => setExpanded(open ? expanded.filter((e) => e !== f.family) : [...expanded, f.family])}
                        className="grid size-8 shrink-0 place-items-center rounded-md text-fg-3 transition-colors duration-fast hover:bg-hover hover:text-fg"
                      >
                        <ChevronDown aria-hidden="true" size={16} strokeWidth={1.75} className={cn("transition-transform duration-fast ease-brand", !open && "-rotate-90")} />
                      </button>
                    )}
                    <div className="flex min-w-0 flex-1 items-start gap-3">
                      <Checkbox
                        id={`${domId}-all`}
                        aria-describedby={`${domId}-count`}
                        className="mt-2"
                        checked={whole}
                        ref={(el) => {
                          if (el) el.indeterminate = !whole && count > 0;
                        }}
                        onChange={(event) => setFamily(f.family, event.target.checked)}
                      />
                      <div className="flex min-w-0 flex-1 flex-col">
                        <div className="flex h-8 items-center justify-between gap-3">
                          <label htmlFor={`${domId}-all`} className="cursor-pointer truncate font-mono text-code-sm text-fg">
                            {f.family}
                          </label>
                          <span id={`${domId}-count`} className="shrink-0 tnum text-caption text-fg-3">
                            {whole ? copy.allEvents : count ? copy.countOf(count, f.types.length) : copy.eventCount(f.types.length)}
                          </span>
                        </div>
                        {open ? (
                          <ul id={`${domId}-types`} className="flex animate-enter flex-col gap-3 pt-1 pb-3">
                            {visible.map((t) => (
                              <li key={t.type}>
                                <label className="flex cursor-pointer items-start gap-3">
                                  <Checkbox className="mt-0.5" checked={covered(t.type)} onChange={(event) => setType(f, t.type, event.target.checked)} />
                                  <span className="flex min-w-0 flex-col">
                                    <span className="font-mono text-code-sm break-all text-fg">{t.type}</span>
                                    <span className="text-caption text-fg-3">{t.description}</span>
                                  </span>
                                </label>
                              </li>
                            ))}
                          </ul>
                        ) : null}
                      </div>
                    </div>
                  </li>
                );
              })}
              {shownOthers.length ? (
                <li className="flex flex-col gap-3 pt-2 pb-3 pl-8">
                  <span className="flex h-8 items-center text-label text-fg-3">{copy.other}</span>
                  {shownOthers.map((p) => (
                    <label key={p} className="flex cursor-pointer items-start gap-3">
                      <Checkbox className="mt-0.5" checked={value.includes(p)} onChange={(event) => onChange(event.target.checked ? [...value, p] : value.filter((e) => e !== p))} />
                      <span className="flex min-w-0 flex-col">
                        <span className="font-mono text-code-sm break-all text-fg">{p}</span>
                        <span className="text-caption text-fg-3">{p.endsWith(".*") ? copy.every(p.slice(0, -2)) : copy.unknown}</span>
                      </span>
                    </label>
                  ))}
                </li>
              ) : null}
            </ul>
          )}
        </div>
      </fieldset>
      <p className="text-caption text-fg-3">{copy.familyNote}</p>
    </div>
  );
}

export function WebhooksTable({ webhooks, eventTypes, canEdit }: { webhooks: Webhook[]; eventTypes: WebhookEventFamily[]; canEdit: boolean }) {
  const [creating, setCreating] = useState(false);
  const locale = useLocale();
  const { webhooks: t, common } = settingsMessages[locale];
  const add = canEdit ? (
    <Button size="sm" variant="primary" onClick={() => setCreating(true)}>
      <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
      {t.add}
    </Button>
  ) : null;

  return (
    <>
      {webhooks.length === 0 ? (
        <Card>
          <EmptyState
            icon={WebhookIcon}
            title={t.emptyTitle}
            description={t.emptyDescription}
            action={add}
          />
        </Card>
      ) : (
        <div className="flex flex-col gap-4">
          {add ? <div className="flex justify-end">{add}</div> : null}
          <SimpleTable
            caption={t.caption}
            head={[t.endpoint, common.events, common.status, t.lastDelivery]}
            rows={webhooks.map((w) => [
              <Link key="url" href={`/admin/webhooks/${w.id}`} className="flex min-w-64 flex-col gap-1 py-2">
                <span className="truncate font-mono text-code-sm text-fg hover:text-ember">{w.url}</span>
                {w.description ? <span className="truncate text-caption text-fg-3">{w.description}</span> : null}
              </Link>,
              <EventTags key="events" events={w.events} />,
              <EnabledBadge key="status" enabled={w.enabled} />,
              w.lastStatus && w.lastDeliveryAt ? (
                <span key="last" className="inline-flex items-center gap-2 whitespace-nowrap" title={formatDateTime(w.lastDeliveryAt, locale)}>
                  <StatusDot tone={DELIVERY_TONE[w.lastStatus]} />
                  {t.status[w.lastStatus]} · <RelativeTime iso={w.lastDeliveryAt} />
                </span>
              ) : (
                <span key="last" className="text-fg-3">
                  {common.nothingSent}
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

function WebhookDialog({ webhook, eventTypes, onClose }: { webhook: Webhook | null; eventTypes: WebhookEventFamily[]; onClose: () => void }) {
  const { pending, run, errorFor, toast, router } = useMutation();
  const [url, setUrl] = useState(webhook?.url ?? "");
  const [description, setDescription] = useState(webhook?.description ?? "");
  const [all, setAll] = useState(webhook ? webhook.events.includes("*") : true);
  const [events, setEvents] = useState<string[]>(webhook?.events.filter((e) => e !== "*") ?? []);
  const [enabled, setEnabled] = useState(webhook?.enabled ?? true);
  const [errors, setErrors] = useState<{ url?: string; events?: string }>({});
  const [created, setCreated] = useState<{ webhook: Webhook; secret: string } | null>(null);
  const { webhooks: t, common } = useMessages(settingsMessages);

  function submit() {
    const next = {
      url: /^https:\/\/\S+$|^http:\/\/(localhost|127\.0\.0\.1|\[::1\])(:\d+)?(\/\S*)?$/.test(url.trim()) ? undefined : t.urlError,
      events: all || events.length ? undefined : common.eventsError,
    };
    setErrors(next);
    if (next.url || next.events) return;
    const body = { url: url.trim(), description: description.trim(), events: all ? ["*"] : events, enabled };
    if (!webhook) {
      void run("save", () => api<{ webhook: Webhook; secret: string }>("/webhooks", { body }), setCreated, "inline");
      return;
    }
    void run("save", () => api<Webhook>(`/webhooks/${webhook.id}`, { method: "PUT", body }), () => {
      toast({ title: t.saved, description: common.followSettings });
      onClose();
    }, "inline");
  }

  if (created) {
    return (
      <Dialog
        open
        onOpenChange={(open) => {
          if (!open) router.push(`/admin/webhooks/${created.webhook.id}`);
        }}
        title={t.added}
        description={t.secretDescription}
        footer={
          <Button variant="primary" onClick={() => router.push(`/admin/webhooks/${created.webhook.id}`)}>
            {common.done}
          </Button>
        }
      >
        <div className="flex animate-enter flex-col gap-4">
          <CopyField label={common.signingSecret} value={created.secret} secret />
          <SecretWarning>{common.secretOnce}</SecretWarning>
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
      error={errorFor("save")}
      title={webhook ? t.edit : t.add}
      description={t.dialogDescription}
      width="wide"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {common.cancel}
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={submit}>
            {webhook ? t.save : t.add}
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
        <Field label={t.url} error={errors.url}>
          {(props) => <Input {...props} mono autoFocus type="url" autoComplete="off" spellCheck={false} value={url} placeholder="https://hooks.example.com/halo" onChange={(event) => setUrl(event.target.value)} />}
        </Field>
        <Field label={t.description} hint={t.descriptionHint}>
          {(props) => <Input {...props} maxLength={200} autoComplete="off" value={description} placeholder={t.descriptionPlaceholder} onChange={(event) => setDescription(event.target.value)} />}
        </Field>
        <div className="flex flex-col gap-4">
          <Field label={common.events} error={errors.events}>
            {(props) => (
              <Select {...props} value={all ? "all" : "some"} onChange={(event) => setAll(event.target.value === "all")}>
                <option value="all">{common.allEvents}</option>
                <option value="some">{common.someEvents}</option>
              </Select>
            )}
          </Field>
          {all ? null : <EventPicker families={eventTypes} value={events} onChange={setEvents} />}
        </div>
        <label className="flex cursor-pointer items-start gap-3">
          <Checkbox className="mt-0.5" checked={enabled} onChange={(event) => setEnabled(event.target.checked)} />
          <span className="flex flex-col">
            <span className="text-body-sm text-fg">{t.send}</span>
            <span className="text-caption text-fg-3">{t.sendHint}</span>
          </span>
        </label>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}

export function WebhookActions({ webhook, eventTypes }: { webhook: Webhook; eventTypes: WebhookEventFamily[] }) {
  const { pending, run, errorFor, clearError, toast, router } = useMutation();
  const [editing, setEditing] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const { webhooks: t, common } = useMessages(settingsMessages);

  function sendTest() {
    void run("test", () => api<WebhookDelivery>(`/webhooks/${webhook.id}/test`, { method: "POST" }), (delivery) =>
      delivery.status === "delivered"
        ? toast({ title: t.testDelivered, description: t.testDeliveredHint(delivery.responseStatus) })
        : toast({ title: t.testFailed, description: delivery.error || t.testFailedHint, tone: "danger" }),
    );
  }

  function setEnabled(enabled: boolean) {
    void run("toggle", () => api(`/webhooks/${webhook.id}`, { method: "PUT", body: { url: webhook.url, description: webhook.description, events: webhook.events, enabled } }), () =>
      toast({ title: enabled ? t.resumed : t.pausedToast, description: enabled ? t.resumedHint : t.pausedHint }),
    );
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button variant="primary" loading={pending === "test"} onClick={sendTest}>
        <Send aria-hidden="true" size={16} strokeWidth={1.75} />
        {common.sendTestEvent}
      </Button>
      <Menu>
        <MenuTrigger asChild>
          <Button size="icon" variant="secondary" aria-label={t.moreActions} loading={pending === "toggle"}>
            <MoreHorizontal aria-hidden="true" size={16} strokeWidth={1.75} />
          </Button>
        </MenuTrigger>
        <MenuContent>
          <MenuItem icon={Pencil} onSelect={() => setEditing(true)}>
            {t.edit}
          </MenuItem>
          <MenuItem icon={webhook.enabled ? Pause : Play} onSelect={() => setEnabled(!webhook.enabled)}>
            {webhook.enabled ? t.pause : t.resume}
          </MenuItem>
          <MenuSeparator />
          <MenuItem icon={Trash2} danger onSelect={() => setDeleting(true)}>
            {t.delete}
          </MenuItem>
        </MenuContent>
      </Menu>
      {editing ? <WebhookDialog webhook={webhook} eventTypes={eventTypes} onClose={() => setEditing(false)} /> : null}
      <Dialog
        open={deleting}
        onOpenChange={(next) => {
          clearError();
          setDeleting(next);
        }}
        error={errorFor("delete")}
        title={t.deleteTitle}
        description={t.deleteDescription}
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(false)}>
              {common.cancel}
            </Button>
            <Button
              variant="destructive"
              loading={pending === "delete"}
              onClick={() =>
                void run("delete", () => api(`/webhooks/${webhook.id}`, { method: "DELETE" }), () => {
                  toast({ title: t.deleted, description: common.recorded });
                  router.push("/admin/webhooks");
                }, "inline")
              }
            >
              {t.delete}
            </Button>
          </>
        }
      />
    </div>
  );
}

export function DeliveryLog({ deliveries, payloadNote }: { deliveries: WebhookDelivery[]; payloadNote?: string }) {
  const [open, setOpen] = useState<WebhookDelivery | null>(null);
  const locale = useLocale();
  const { webhooks: t, common } = settingsMessages[locale];
  if (deliveries.length === 0) {
    return (
      <Card>
        <EmptyState icon={Send} title={t.noDeliveries} description={t.noDeliveriesHint} />
      </Card>
    );
  }
  return (
    <>
      <SimpleTable
        caption={t.deliveries}
        head={[t.event, common.status, t.attempts, t.response, t.lastAttempt, t.payload]}
        rows={deliveries.map((d) => [
          <span key="event" className="flex flex-col items-start gap-1">
            <Tag>{d.eventType}</Tag>
            <span className="font-mono text-code-sm text-fg-3">{d.eventId}</span>
          </span>,
          <span key="status" className="flex flex-col items-start gap-1">
            <Badge tone={DELIVERY_TONE[d.status]} dot>
              {t.status[d.status]}
            </Badge>
            {d.status === "pending" && d.attempts > 0 ? <span className="text-caption whitespace-nowrap text-fg-3">{t.nextTry(formatTime(d.nextAttemptAt))}</span> : null}
          </span>,
          <span key="attempts" className="tnum">
            {t.attemptsOf(d.attempts)}
          </span>,
          <span key="response" className="flex max-w-64 flex-col gap-1">
            <span className="tnum font-mono text-code-sm">{d.responseStatus ?? "—"}</span>
            {d.error ? <span className="truncate text-caption text-fg-3" title={d.error}>{d.error}</span> : null}
          </span>,
          <span key="last" className="tnum whitespace-nowrap" title={d.lastAttemptAt ? formatDateTime(d.lastAttemptAt, locale) : undefined}>
            {d.lastAttemptAt ? <RelativeTime iso={d.lastAttemptAt} /> : t.queued}
          </span>,
          <Button key="payload" size="sm" variant="quiet" onClick={() => setOpen(d)}>
            {t.view}
          </Button>,
        ])}
      />
      <Dialog
        open={open !== null}
        onOpenChange={(next) => {
          if (!next) setOpen(null);
        }}
        title={open ? open.eventType : t.payload}
        description={payloadNote ?? t.payloadNote}
        width="wide"
      >
        {open ? <pre className="overflow-x-auto rounded-md border border-border bg-sunken p-4 font-mono text-code-sm text-fg-2">{JSON.stringify(JSON.parse(open.body), null, 2)}</pre> : null}
      </Dialog>
    </>
  );
}
