"use client";

import { BellRing, MoreHorizontal, Pause, Pencil, Play, Plus, Send, Trash2 } from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import { SecretWarning } from "@/components/applications/shared";
import { useMutation } from "@/components/console/use-mutation";
import { ROLE_ORDER } from "@/components/settings/roles";
import { DELIVERY_TONE, EnabledBadge, EventPicker, EventTags } from "@/components/settings/webhooks";
import { StatusDot } from "@/components/ui/badge";
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
import { formatDateTime } from "@/lib/format";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { settingsMessages } from "@/lib/i18n/console/settings";
import { DESTINATIONS } from "@/lib/labels";
import type { NotificationDestination, NotificationRule, WebhookDelivery, WebhookEventFamily } from "@/lib/settings-types";

export function RuleConditions({ rule }: { rule: NotificationRule }) {
  const { notificationRules: t, roles } = useMessages(settingsMessages);
  const parts = [
    rule.minRisk ? t.minRisk[rule.minRisk] : null,
    rule.role ? t.gives(roles.names[rule.role]) : null,
    rule.threshold > 1 ? t.threshold(rule.threshold, rule.windowMinutes) : t.everyEvent,
    t.quiet(rule.quietMinutes),
  ].filter(Boolean);
  return <span className="text-body-sm text-fg-2">{parts.join(" · ")}</span>;
}

export function RulesTable({ rules, eventTypes, canEdit }: { rules: NotificationRule[]; eventTypes: WebhookEventFamily[]; canEdit: boolean }) {
  const [creating, setCreating] = useState(false);
  const locale = useLocale();
  const { notificationRules: t, webhooks, common } = settingsMessages[locale];
  const add = canEdit ? (
    <Button size="sm" variant="primary" onClick={() => setCreating(true)}>
      <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
      {t.add}
    </Button>
  ) : null;

  return (
    <>
      {rules.length === 0 ? (
        <Card>
          <EmptyState
            icon={BellRing}
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
            head={[t.rule, common.events, t.conditions, common.status, t.lastMessage]}
            rows={rules.map((r) => [
              <Link key="name" href={`/admin/notification-rules/${r.id}`} className="flex min-w-48 flex-col gap-1 py-2">
                <span className="truncate text-body-sm text-fg hover:text-ember">{r.name}</span>
                <span className="truncate text-caption text-fg-3">
                  {DESTINATIONS[r.destination].label} · {r.host}
                </span>
              </Link>,
              <EventTags key="events" events={r.events} />,
              <RuleConditions key="conditions" rule={r} />,
              <EnabledBadge key="status" enabled={r.enabled} />,
              r.lastStatus && r.lastSentAt ? (
                <span key="last" className="inline-flex items-center gap-2 whitespace-nowrap" title={formatDateTime(r.lastSentAt, locale)}>
                  <StatusDot tone={DELIVERY_TONE[r.lastStatus]} />
                  {webhooks.status[r.lastStatus]} · <RelativeTime iso={r.lastSentAt} />
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
      {creating ? <RuleDialog rule={null} eventTypes={eventTypes} onClose={() => setCreating(false)} /> : null}
    </>
  );
}

function body(rule: NotificationRule) {
  const { name, destination, events, minRisk, role, threshold, windowMinutes, quietMinutes, enabled } = rule;
  return { name, destination, url: "", events, minRisk, role, threshold, windowMinutes, quietMinutes, enabled };
}

function NumberInput({ label, value, max, onChange }: { label: string; value: string; max: number; onChange: (value: string) => void }) {
  return (
    <div className="w-24 shrink-0">
      <Input aria-label={label} type="number" inputMode="numeric" min={1} max={max} step={1} className="tnum" value={value} onChange={(event) => onChange(event.target.value)} />
    </div>
  );
}

function RuleDialog({ rule, eventTypes, onClose }: { rule: NotificationRule | null; eventTypes: WebhookEventFamily[]; onClose: () => void }) {
  const { pending, run, errorFor, toast, router } = useMutation();
  const [name, setName] = useState(rule?.name ?? "");
  const [destination, setDestination] = useState<NotificationDestination>(rule?.destination ?? "slack");
  const [url, setUrl] = useState("");
  const [all, setAll] = useState(rule ? rule.events.includes("*") : false);
  const [events, setEvents] = useState<string[]>(rule?.events.filter((e) => e !== "*") ?? []);
  const [minRisk, setMinRisk] = useState<NotificationRule["minRisk"]>(rule?.minRisk ?? "");
  const [role, setRole] = useState<NotificationRule["role"]>(rule?.role ?? "");
  const [threshold, setThreshold] = useState(String(rule?.threshold ?? 1));
  const [windowMinutes, setWindowMinutes] = useState(String(rule?.windowMinutes ?? 10));
  const [quietMinutes, setQuietMinutes] = useState(String(rule?.quietMinutes ?? 15));
  const [enabled, setEnabled] = useState(rule?.enabled ?? true);
  const [errors, setErrors] = useState<{ name?: string; url?: string; events?: string; numbers?: string }>({});
  const [secret, setSecret] = useState<{ rule: NotificationRule; secret: string } | null>(null);
  const { notificationRules: t, roles, common } = useMessages(settingsMessages);

  function submit() {
    const within = (value: string, max: number) => /^\d+$/.test(value) && Number(value) >= 1 && Number(value) <= max;
    const next = {
      name: name.trim() ? undefined : t.nameError,
      url: (rule && !url.trim() && destination === rule.destination) || /^https:\/\/\S+$/.test(url.trim()) ? undefined : t.urlError,
      events: all || events.length ? undefined : common.eventsError,
      numbers: within(threshold, 100) && within(windowMinutes, 1440) && within(quietMinutes, 1440) ? undefined : t.numbersError,
    };
    setErrors(next);
    if (next.name || next.url || next.events || next.numbers) return;
    const payload = {
      name: name.trim(),
      destination,
      url: url.trim(),
      events: all ? ["*"] : events,
      minRisk,
      role,
      threshold: Number(threshold),
      windowMinutes: Number(windowMinutes),
      quietMinutes: Number(quietMinutes),
      enabled,
    };
    if (!rule) {
      void run(
        "save",
        () => api<{ rule: NotificationRule; secret?: string }>("/notification-rules", { body: payload }),
        (created) => {
          if (created.secret) return setSecret({ rule: created.rule, secret: created.secret });
          toast({ title: t.added, description: t.addedHint });
          router.push(`/admin/notification-rules/${created.rule.id}`);
        },
        "inline",
      );
      return;
    }
    void run("save", () => api<NotificationRule>(`/notification-rules/${rule.id}`, { method: "PUT", body: payload }), () => {
      toast({ title: t.saved, description: common.followSettings });
      onClose();
    }, "inline");
  }

  if (secret) {
    return (
      <Dialog
        open
        onOpenChange={(open) => {
          if (!open) router.push(`/admin/notification-rules/${secret.rule.id}`);
        }}
        title={t.added}
        description={t.secretDescription}
        footer={
          <Button variant="primary" onClick={() => router.push(`/admin/notification-rules/${secret.rule.id}`)}>
            {common.done}
          </Button>
        }
      >
        <div className="flex animate-enter flex-col gap-4">
          <CopyField label={common.signingSecret} value={secret.secret} secret />
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
      title={rule ? t.edit : t.add}
      description={t.dialogDescription}
      width="wide"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {common.cancel}
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={submit}>
            {rule ? t.save : t.add}
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
        <Field label={common.name} error={errors.name} hint={t.nameHint}>
          {(props) => <Input {...props} autoFocus maxLength={100} autoComplete="off" value={name} placeholder={t.namePlaceholder} onChange={(event) => setName(event.target.value)} />}
        </Field>
        <div className="grid gap-6 sm:grid-cols-[12rem_1fr]">
          <Field label={t.sendTo}>
            {(props) => (
              <Select {...props} value={destination} onChange={(event) => setDestination(event.target.value as NotificationDestination)}>
                <option value="slack">Slack</option>
                <option value="discord">Discord</option>
                <option value="webhook">Webhook</option>
              </Select>
            )}
          </Field>
          <Field
            label={t.url}
            error={errors.url}
            hint={rule && destination === rule.destination ? t.keepUrl(rule.host) : t.urlHint}
          >
            {(props) => (
              <Input
                {...props}
                mono
                type="url"
                autoComplete="off"
                spellCheck={false}
                value={url}
                placeholder={rule && destination === rule.destination ? t.unchanged : DESTINATIONS[destination].placeholder}
                onChange={(event) => setUrl(event.target.value)}
              />
            )}
          </Field>
        </div>
        <div className="flex flex-col gap-4">
          <Field label={common.events} error={errors.events}>
            {(props) => (
              <Select {...props} value={all ? "all" : "some"} onChange={(event) => setAll(event.target.value === "all")}>
                <option value="some">{common.someEvents}</option>
                <option value="all">{common.allEvents}</option>
              </Select>
            )}
          </Field>
          {all ? null : <EventPicker families={eventTypes} value={events} onChange={setEvents} />}
        </div>
        <div className="grid gap-6 sm:grid-cols-2">
          <Field label={t.risk} hint={t.riskHint}>
            {(props) => (
              <Select {...props} value={minRisk} onChange={(event) => setMinRisk(event.target.value as NotificationRule["minRisk"])}>
                <option value="">{t.anyRisk}</option>
                <option value="low">{t.low}</option>
                <option value="medium">{t.medium}</option>
                <option value="high">{t.high}</option>
              </Select>
            )}
          </Field>
          <Field label={t.role} hint={t.roleHint}>
            {(props) => (
              <Select {...props} value={role} onChange={(event) => setRole(event.target.value as NotificationRule["role"])}>
                <option value="">{t.anyChange}</option>
                {ROLE_ORDER.map((key) => (
                  <option key={key} value={key}>
                    {roles.names[key]}
                  </option>
                ))}
              </Select>
            )}
          </Field>
        </div>
        <fieldset className="flex flex-col gap-3">
          <legend className="mb-1 text-label text-fg">{t.thresholdLegend}</legend>
          <div className="flex flex-wrap items-center gap-3 text-body-sm text-fg-2">
            <span>{t.sendAfter}</span>
            <NumberInput label={t.matching} value={threshold} max={100} onChange={setThreshold} />
            <span>{t.within}</span>
            <NumberInput label={t.window} value={windowMinutes} max={1440} onChange={setWindowMinutes} />
            <span>{t.quietFor}</span>
            <NumberInput label={t.quietPeriod} value={quietMinutes} max={1440} onChange={setQuietMinutes} />
            <span>{t.minutes}</span>
          </div>
          {errors.numbers ? (
            <p className="text-caption text-danger">{errors.numbers}</p>
          ) : (
            <p className="text-caption text-fg-3">{t.burst}</p>
          )}
        </fieldset>
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

export function RuleActions({ rule, eventTypes }: { rule: NotificationRule; eventTypes: WebhookEventFamily[] }) {
  const { pending, run, errorFor, clearError, toast, router } = useMutation();
  const [editing, setEditing] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const { notificationRules: t, common } = useMessages(settingsMessages);

  function sendTest() {
    void run("test", () => api<WebhookDelivery>(`/notification-rules/${rule.id}/test`, { method: "POST" }), (delivery) =>
      delivery.status === "delivered"
        ? toast({ title: t.testSent, description: t.testSentHint(DESTINATIONS[rule.destination].label, delivery.responseStatus) })
        : toast({ title: t.testFailed, description: delivery.error || t.testFailedHint, tone: "danger" }),
    );
  }

  function setEnabled(enabled: boolean) {
    void run("toggle", () => api(`/notification-rules/${rule.id}`, { method: "PUT", body: { ...body(rule), enabled } }), () =>
      toast({ title: enabled ? t.resumed : t.paused, description: enabled ? t.resumedHint : t.pausedHint }),
    );
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button variant="primary" loading={pending === "test"} onClick={sendTest}>
        <Send aria-hidden="true" size={16} strokeWidth={1.75} />
        {t.sendTest}
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
          <MenuItem icon={rule.enabled ? Pause : Play} onSelect={() => setEnabled(!rule.enabled)}>
            {rule.enabled ? t.pause : t.resume}
          </MenuItem>
          <MenuSeparator />
          <MenuItem icon={Trash2} danger onSelect={() => setDeleting(true)}>
            {t.delete}
          </MenuItem>
        </MenuContent>
      </Menu>
      {editing ? <RuleDialog rule={rule} eventTypes={eventTypes} onClose={() => setEditing(false)} /> : null}
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
                void run("delete", () => api(`/notification-rules/${rule.id}`, { method: "DELETE" }), () => {
                  toast({ title: t.deleted, description: common.recorded });
                  router.push("/admin/notification-rules");
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
