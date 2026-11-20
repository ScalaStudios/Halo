"use client";

import { BellRing, MoreHorizontal, Pause, Pencil, Play, Plus, Send, Trash2 } from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import { SecretWarning } from "@/components/applications/shared";
import { useMutation } from "@/components/console/use-mutation";
import { ROLES } from "@/components/provisioning/shared";
import { ROLE_ORDER } from "@/components/settings/roles";
import { DELIVERY_STATUS, EnabledBadge, EventPicker, EventTags } from "@/components/settings/webhooks";
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
import { formatDateTime, pluralize } from "@/lib/format";
import { DESTINATIONS, RISK } from "@/lib/labels";
import type { NotificationDestination, NotificationRule, WebhookDelivery, WebhookEventFamily } from "@/lib/settings-types";

export function RuleConditions({ rule }: { rule: NotificationRule }) {
  const parts = [
    rule.minRisk ? `${RISK[rule.minRisk].label} risk or higher` : null,
    rule.role ? `Gives ${ROLES[rule.role].label}` : null,
    rule.threshold > 1 ? `${rule.threshold} events within ${pluralize(rule.windowMinutes, "minute")}` : "Every event",
    `Quiet for ${pluralize(rule.quietMinutes, "minute")}`,
  ].filter(Boolean);
  return <span className="text-body-sm text-fg-2">{parts.join(" · ")}</span>;
}

export function RulesTable({ rules, eventTypes, canEdit }: { rules: NotificationRule[]; eventTypes: WebhookEventFamily[]; canEdit: boolean }) {
  const [creating, setCreating] = useState(false);
  const add = canEdit ? (
    <Button size="sm" variant="primary" onClick={() => setCreating(true)}>
      <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
      Add rule
    </Button>
  ) : null;

  return (
    <>
      {rules.length === 0 ? (
        <Card>
          <EmptyState
            icon={BellRing}
            title="No notification rules yet"
            description="Add a rule to post risky sign-ins, new administrators or other security events to a Slack or Discord channel, or to a webhook."
            action={add}
          />
        </Card>
      ) : (
        <div className="flex flex-col gap-4">
          {add ? <div className="flex justify-end">{add}</div> : null}
          <SimpleTable
            caption="Notification rules"
            head={["Rule", "Events", "Conditions", "Status", "Last message"]}
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
                <span key="last" className="inline-flex items-center gap-2 whitespace-nowrap" title={formatDateTime(r.lastSentAt)}>
                  <StatusDot tone={DELIVERY_STATUS[r.lastStatus].tone} />
                  {DELIVERY_STATUS[r.lastStatus].label} · <RelativeTime iso={r.lastSentAt} />
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

  function submit() {
    const within = (value: string, max: number) => /^\d+$/.test(value) && Number(value) >= 1 && Number(value) <= max;
    const next = {
      name: name.trim() ? undefined : "Name the rule, for example New global administrator.",
      url: (rule && !url.trim() && destination === rule.destination) || /^https:\/\/\S+$/.test(url.trim()) ? undefined : "Enter the https:// URL of the incoming webhook.",
      events: all || events.length ? undefined : "Choose at least one event or family, or send all events.",
      numbers: within(threshold, 100) && within(windowMinutes, 1440) && within(quietMinutes, 1440) ? undefined : "Use 1 to 100 events, and 1 to 1,440 minutes for the window and the quiet period.",
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
          toast({ title: "Rule added", description: "Send a test message to check the channel." });
          router.push(`/admin/notification-rules/${created.rule.id}`);
        },
        "inline",
      );
      return;
    }
    void run("save", () => api<NotificationRule>(`/notification-rules/${rule.id}`, { method: "PUT", body: payload }), () => {
      toast({ title: "Rule saved", description: "Events from now on follow the new settings." });
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
        title="Rule added"
        description="Verify the Halo-Signature header of every request with this signing secret, as for webhook endpoints."
        footer={
          <Button variant="primary" onClick={() => router.push(`/admin/notification-rules/${secret.rule.id}`)}>
            Done
          </Button>
        }
      >
        <div className="flex animate-enter flex-col gap-4">
          <CopyField label="Signing secret" value={secret.secret} secret />
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
      error={errorFor("save")}
      title={rule ? "Edit rule" : "Add rule"}
      description="Halo checks new sign-ins and audit events every 10 seconds and posts a message when they match the rule."
      width="wide"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={submit}>
            {rule ? "Save rule" : "Add rule"}
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
        <Field label="Name" error={errors.name} hint="The title of every message.">
          {(props) => <Input {...props} autoFocus maxLength={100} autoComplete="off" value={name} placeholder="New global administrator" onChange={(event) => setName(event.target.value)} />}
        </Field>
        <div className="grid gap-6 sm:grid-cols-[12rem_1fr]">
          <Field label="Send to">
            {(props) => (
              <Select {...props} value={destination} onChange={(event) => setDestination(event.target.value as NotificationDestination)}>
                <option value="slack">Slack</option>
                <option value="discord">Discord</option>
                <option value="webhook">Webhook</option>
              </Select>
            )}
          </Field>
          <Field
            label="Webhook URL"
            error={errors.url}
            hint={
              rule && destination === rule.destination
                ? `Leave empty to keep the URL for ${rule.host}. Halo stores it encrypted and never shows it again.`
                : "Halo stores it encrypted and never shows it again. Private and local addresses are refused."
            }
          >
            {(props) => (
              <Input
                {...props}
                mono
                type="url"
                autoComplete="off"
                spellCheck={false}
                value={url}
                placeholder={rule && destination === rule.destination ? "Unchanged" : DESTINATIONS[destination].placeholder}
                onChange={(event) => setUrl(event.target.value)}
              />
            )}
          </Field>
        </div>
        <div className="flex flex-col gap-4">
          <Field label="Events" error={errors.events}>
            {(props) => (
              <Select {...props} value={all ? "all" : "some"} onChange={(event) => setAll(event.target.value === "all")}>
                <option value="some">Only the events I choose</option>
                <option value="all">All events, including types added in later Halo versions</option>
              </Select>
            )}
          </Field>
          {all ? null : <EventPicker families={eventTypes} value={events} onChange={setEvents} />}
        </div>
        <div className="grid gap-6 sm:grid-cols-2">
          <Field label="Sign-in risk" hint="Events without a risk, such as audit events, never match a minimum.">
            {(props) => (
              <Select {...props} value={minRisk} onChange={(event) => setMinRisk(event.target.value as NotificationRule["minRisk"])}>
                <option value="">Any risk</option>
                <option value="low">Low or higher</option>
                <option value="medium">Medium or higher</option>
                <option value="high">High</option>
              </Select>
            )}
          </Field>
          <Field label="Role given" hint="Matches role.update events that give someone this role.">
            {(props) => (
              <Select {...props} value={role} onChange={(event) => setRole(event.target.value as NotificationRule["role"])}>
                <option value="">Any change</option>
                {ROLE_ORDER.map((key) => (
                  <option key={key} value={key}>
                    {ROLES[key].label}
                  </option>
                ))}
              </Select>
            )}
          </Field>
        </div>
        <fieldset className="flex flex-col gap-3">
          <legend className="mb-1 text-label text-fg">Threshold and quiet period</legend>
          <div className="flex flex-wrap items-center gap-3 text-body-sm text-fg-2">
            <span>Send after</span>
            <NumberInput label="Matching events" value={threshold} max={100} onChange={setThreshold} />
            <span>matching events within</span>
            <NumberInput label="Window in minutes" value={windowMinutes} max={1440} onChange={setWindowMinutes} />
            <span>minutes, then stay quiet for</span>
            <NumberInput label="Quiet period in minutes" value={quietMinutes} max={1440} onChange={setQuietMinutes} />
            <span>minutes.</span>
          </div>
          {errors.numbers ? (
            <p className="text-caption text-danger">{errors.numbers}</p>
          ) : (
            <p className="text-caption text-fg-3">A burst sends one message. Events that match during the quiet period are not sent, and stay in the sign-in and audit logs.</p>
          )}
        </fieldset>
        <label className="flex cursor-pointer items-start gap-3">
          <Checkbox className="mt-0.5" checked={enabled} onChange={(event) => setEnabled(event.target.checked)} />
          <span className="flex flex-col">
            <span className="text-body-sm text-fg">Send messages</span>
            <span className="text-caption text-fg-3">Paused rules keep their settings and send only test messages until you resume them.</span>
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

  function sendTest() {
    void run("test", () => api<WebhookDelivery>(`/notification-rules/${rule.id}/test`, { method: "POST" }), (delivery) =>
      delivery.status === "delivered"
        ? toast({ title: "Test message sent", description: `${DESTINATIONS[rule.destination].label} answered ${delivery.responseStatus}.` })
        : toast({ title: "Test message not sent", description: delivery.error || "The destination did not answer with a 2xx status.", tone: "danger" }),
    );
  }

  function setEnabled(enabled: boolean) {
    void run("toggle", () => api(`/notification-rules/${rule.id}`, { method: "PUT", body: { ...body(rule), enabled } }), () =>
      toast({ title: enabled ? "Rule resumed" : "Rule paused", description: enabled ? "Matching events are sent again." : "Matching events are not sent until you resume it." }),
    );
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button variant="primary" loading={pending === "test"} onClick={sendTest}>
        <Send aria-hidden="true" size={16} strokeWidth={1.75} />
        Send test message
      </Button>
      <Menu>
        <MenuTrigger asChild>
          <Button size="icon" variant="secondary" aria-label="More rule actions" loading={pending === "toggle"}>
            <MoreHorizontal aria-hidden="true" size={16} strokeWidth={1.75} />
          </Button>
        </MenuTrigger>
        <MenuContent>
          <MenuItem icon={Pencil} onSelect={() => setEditing(true)}>
            Edit rule
          </MenuItem>
          <MenuItem icon={rule.enabled ? Pause : Play} onSelect={() => setEnabled(!rule.enabled)}>
            {rule.enabled ? "Pause rule" : "Resume rule"}
          </MenuItem>
          <MenuSeparator />
          <MenuItem icon={Trash2} danger onSelect={() => setDeleting(true)}>
            Delete rule
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
        title="Delete this rule?"
        description="Halo stops sending its messages and deletes its delivery log. Queued retries are dropped."
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              loading={pending === "delete"}
              onClick={() =>
                void run("delete", () => api(`/notification-rules/${rule.id}`, { method: "DELETE" }), () => {
                  toast({ title: "Rule deleted", description: "Recorded in the audit log." });
                  router.push("/admin/notification-rules");
                }, "inline")
              }
            >
              Delete rule
            </Button>
          </>
        }
      />
    </div>
  );
}
