"use client";

import { Pencil, Plus, ServerCog, Trash2, X } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ActionError } from "@/components/ui/action-error";
import { Badge, Tag } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Field, Input, Select } from "@/components/ui/input";
import { SectionTitle } from "@/components/ui/section-title";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { pluralize } from "@/lib/format";
import type { HostGroup, HostRule, PosixSettings } from "@/lib/infra-types";
import type { Group } from "@/lib/types";

type SudoMode = "none" | "all" | "commands";
type RuleDraft = { groupId: string; mode: SudoMode; commands: string };

function left(next: number, max: number) {
  return Math.max(0, max - next + 1);
}

export function PosixSettingsCard({ settings }: { settings: PosixSettings }) {
  const { pending, run, errorFor, toast } = useMutation();
  const [draft, setDraft] = useState({ ...settings });
  const changed = (["uidMin", "uidMax", "gidMin", "gidMax", "loginShell"] as const).some((key) => draft[key] !== settings[key]);
  const number = (key: "uidMin" | "uidMax" | "gidMin" | "gidMax", label: string) => (
    <Field label={label}>
      {(props) => (
        <Input {...props} mono inputMode="numeric" value={String(draft[key])} onChange={(event) => setDraft({ ...draft, [key]: Number(event.target.value.replace(/\D/g, "")) })} />
      )}
    </Field>
  );

  return (
    <Card>
      <CardHeader
        title="Linux accounts"
        description="Every person and group gets a user or group number from these ranges. Numbers never change and never come back after a deletion. Changing a range applies to people and groups created from now on."
      />
      <CardBody>
        <form
          className="flex flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            const body = { uidMin: draft.uidMin, uidMax: draft.uidMax, gidMin: draft.gidMin, gidMax: draft.gidMax, loginShell: draft.loginShell.trim() };
            void run("posix", () => api<PosixSettings>("/ssh/posix", { method: "PUT", body }), () =>
              toast({ title: "Linux account settings saved", description: "Applies to people and groups created from now on. Recorded in the audit log." }),
              "inline",
            );
          }}
        >
          <div className="grid grid-cols-1 gap-6 sm:grid-cols-2 xl:grid-cols-4">
            {number("uidMin", "First user number")}
            {number("uidMax", "Last user number")}
            {number("gidMin", "First group number")}
            {number("gidMax", "Last group number")}
          </div>
          <p className="text-body-sm text-fg-3">
            {pluralize(left(settings.nextUid, settings.uidMax), "user number", "user numbers")} and {pluralize(left(settings.nextGid, settings.gidMax), "group number", "group numbers")} left.
            Both ranges start at 65536 or above, clear of local accounts and nobody, and must not overlap.
          </p>
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start">
            <Field label="Default login shell" hint="New people get this shell. Change it per person on their Access tab." className="sm:w-64">
              {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={draft.loginShell} onChange={(event) => setDraft({ ...draft, loginShell: event.target.value })} />}
            </Field>
            <Button type="submit" variant="secondary" loading={pending === "posix"} disabled={!changed} className="sm:mt-6">
              Save settings
            </Button>
            <ActionError message={errorFor("posix")} className="sm:mt-8" />
          </div>
        </form>
      </CardBody>
    </Card>
  );
}

function describeSudo(rule: HostRule) {
  if (rule.sudo.length === 0) return <span className="text-fg-3">No sudo</span>;
  if (rule.sudo[0] === "ALL") return <Badge tone="ember">Full sudo</Badge>;
  return (
    <span className="flex flex-wrap gap-1">
      {rule.sudo.map((command) => (
        <Tag key={command}>{command}</Tag>
      ))}
    </span>
  );
}

export function HostGroupsSection({ hostGroups, groups }: { hostGroups: HostGroup[]; groups: Group[] }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [editing, setEditing] = useState<HostGroup | "new" | null>(null);
  const [deleting, setDeleting] = useState<HostGroup | null>(null);
  const groupName = (id: string) => groups.find((g) => g.id === id)?.name ?? "Deleted group";

  function remove() {
    const hostGroup = deleting;
    if (!hostGroup) return;
    void run("delete", () => api(`/ssh/host-groups/${hostGroup.id}`, { method: "DELETE" }), () => {
      setDeleting(null);
      toast({ title: `${hostGroup.name} deleted`, description: "Its hosts let nobody in through halo-hostd from their next refresh. Recorded in the audit log." });
    }, "inline");
  }

  return (
    <div className="flex flex-col gap-6">
      <SectionTitle
        title="Host groups"
        description="halo-hostd lets the members of a host group's rules log in to its hosts with their Halo certificate, as their own Linux account, and installs their sudo rules. A host in no host group lets nobody in."
        action={
          <Button size="sm" variant="primary" onClick={() => setEditing("new")}>
            <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
            Add host group
          </Button>
        }
      />
      {hostGroups.length ? (
        <SimpleTable
          caption="Host groups"
          head={["Host group", "Who may log in", "Hosts", "Actions"]}
          rows={hostGroups.map((hostGroup) => [
            <span key="name" className="flex flex-col">
              <span className="font-medium whitespace-nowrap text-fg">{hostGroup.name}</span>
              {hostGroup.description ? <span className="text-caption text-fg-3">{hostGroup.description}</span> : null}
            </span>,
            hostGroup.rules.length ? (
              <ul key="rules" className="flex flex-col gap-2">
                {hostGroup.rules.map((rule) => (
                  <li key={rule.groupId} className="flex flex-wrap items-center gap-2">
                    <span className="whitespace-nowrap text-fg">{groupName(rule.groupId)}</span>
                    {describeSudo(rule)}
                  </li>
                ))}
              </ul>
            ) : (
              <span key="rules" className="text-fg-3">
                Nobody
              </span>
            ),
            <span key="hosts" className="tnum whitespace-nowrap">
              {pluralize(hostGroup.hostCount, "host", "hosts")}
            </span>,
            <span key="actions" className="flex gap-1">
              <Button size="icon-sm" variant="quiet" aria-label={`Edit ${hostGroup.name}`} onClick={() => setEditing(hostGroup)}>
                <Pencil aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
              <Button size="icon-sm" variant="quiet" aria-label={`Delete ${hostGroup.name}`} onClick={() => setDeleting(hostGroup)}>
                <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
            </span>,
          ])}
        />
      ) : (
        <Card>
          <EmptyState
            icon={ServerCog}
            title="No host groups yet"
            description="Create one, such as Production, with the groups that may log in to its hosts, then move your hosts into it."
          />
        </Card>
      )}
      {editing ? <HostGroupDialog key={editing === "new" ? "new" : editing.id} hostGroup={editing === "new" ? null : editing} groups={groups} onClose={() => setEditing(null)} /> : null}
      <Dialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (open) return;
          setDeleting(null);
          clearError();
        }}
        error={errorFor("delete")}
        title={`Delete ${deleting?.name ?? "this host group"}?`}
        description={`${deleting ? pluralize(deleting.hostCount, "host leaves", "hosts leave") : "Its hosts leave"} the group and let nobody in through halo-hostd from their next refresh. Certificates already issued stay valid, but no host names their principal any more.`}
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(null)}>
              Cancel
            </Button>
            <Button variant="destructive" loading={pending === "delete"} onClick={remove}>
              Delete host group
            </Button>
          </>
        }
      />
    </div>
  );
}

function toDraft(rule: HostRule): RuleDraft {
  if (rule.sudo.length === 0) return { groupId: rule.groupId, mode: "none", commands: "" };
  if (rule.sudo[0] === "ALL") return { groupId: rule.groupId, mode: "all", commands: "" };
  return { groupId: rule.groupId, mode: "commands", commands: rule.sudo.join(", ") };
}

function HostGroupDialog({ hostGroup, groups, onClose }: { hostGroup: HostGroup | null; groups: Group[]; onClose: () => void }) {
  const { pending, run, errorFor, toast } = useMutation();
  const [name, setName] = useState(hostGroup?.name ?? "");
  const [description, setDescription] = useState(hostGroup?.description ?? "");
  const [rules, setRules] = useState<RuleDraft[]>(hostGroup?.rules.map(toDraft) ?? []);
  const [errors, setErrors] = useState<{ name?: string; rules?: string }>({});
  const update = (index: number, change: Partial<RuleDraft>) => setRules(rules.map((rule, i) => (i === index ? { ...rule, ...change } : rule)));

  function save() {
    const next = {
      name: name.trim() ? undefined : "Name the host group, such as Production.",
      rules: rules.some((rule) => !rule.groupId)
        ? "Choose a group for every rule, or remove the empty one."
        : rules.some((rule) => rule.mode === "commands" && !rule.commands.trim())
          ? "List the commands, or choose full sudo or no sudo."
          : undefined,
    };
    setErrors(next);
    if (next.name || next.rules) return;
    const body = {
      name: name.trim(),
      description: description.trim(),
      rules: rules.map((rule) => ({
        groupId: rule.groupId,
        sudo: rule.mode === "all" ? ["ALL"] : rule.mode === "commands" ? rule.commands.split(",").map((c) => c.trim()).filter(Boolean) : [],
      })),
    };
    void run(
      "save",
      () => (hostGroup ? api<HostGroup>(`/ssh/host-groups/${hostGroup.id}`, { method: "PUT", body }) : api<HostGroup>("/ssh/host-groups", { body })),
      (saved) => {
        toast({ title: `${saved.name} saved`, description: "Hosts in it pick up the rules at their next refresh. Recorded in the audit log." });
        onClose();
      },
      "inline",
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      error={errorFor("save")}
      title={hostGroup ? `Edit ${hostGroup.name}` : "Add host group"}
      description="Members of these groups may log in to the group's hosts. Sudo runs as root without a password prompt, because Linux accounts from Halo have no password."
      width="wide"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={save}>
            {hostGroup ? "Save host group" : "Add host group"}
          </Button>
        </>
      }
    >
      <form
        className="flex flex-col gap-6"
        onSubmit={(event) => {
          event.preventDefault();
          save();
        }}
      >
        <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
          <Field label="Name" error={errors.name}>
            {(props) => <Input {...props} autoFocus autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder="Production" />}
          </Field>
          <Field label="Description">
            {(props) => <Input {...props} autoComplete="off" value={description} onChange={(event) => setDescription(event.target.value)} placeholder="Customer-facing servers" />}
          </Field>
        </div>
        <fieldset className="flex flex-col gap-4">
          <legend className="mb-2 text-body-sm font-medium text-fg">Who may log in</legend>
          {rules.length === 0 ? <p className="text-body-sm text-fg-3">Nobody yet. Add a rule for each group that may log in.</p> : null}
          {rules.map((rule, index) => (
            <div key={index} className="flex flex-col gap-3 rounded-md border border-border p-4">
              <div className="flex items-start gap-3">
                <div className="grid flex-1 grid-cols-1 gap-3 sm:grid-cols-2">
                  <Field label="Group">
                    {(props) => (
                      <Select {...props} value={rule.groupId} onChange={(event) => update(index, { groupId: event.target.value })}>
                        <option value="" disabled>
                          Choose a group
                        </option>
                        {groups
                          .filter((g) => g.id === rule.groupId || !rules.some((r) => r.groupId === g.id))
                          .map((g) => (
                            <option key={g.id} value={g.id}>
                              {g.name}
                            </option>
                          ))}
                      </Select>
                    )}
                  </Field>
                  <Field label="Sudo">
                    {(props) => (
                      <Select {...props} value={rule.mode} onChange={(event) => update(index, { mode: event.target.value as SudoMode })}>
                        <option value="none">No sudo</option>
                        <option value="commands">Some commands</option>
                        <option value="all">Full sudo</option>
                      </Select>
                    )}
                  </Field>
                </div>
                <Button size="icon-sm" variant="quiet" className="mt-7" aria-label="Remove this rule" onClick={() => setRules(rules.filter((_, i) => i !== index))}>
                  <X aria-hidden="true" size={16} strokeWidth={1.75} />
                </Button>
              </div>
              {rule.mode === "commands" ? (
                <Field label="Commands" hint="Absolute paths with plain arguments, separated by commas, such as /usr/bin/systemctl restart nginx, /usr/bin/journalctl. No wildcards or quotes.">
                  {(props) => (
                    <Input {...props} mono autoComplete="off" spellCheck={false} value={rule.commands} onChange={(event) => update(index, { commands: event.target.value })} placeholder="/usr/bin/journalctl" />
                  )}
                </Field>
              ) : null}
            </div>
          ))}
          {errors.rules ? <ActionError message={errors.rules} /> : null}
          <div>
            <Button size="sm" variant="secondary" onClick={() => setRules([...rules, { groupId: "", mode: "none", commands: "" }])} disabled={rules.length >= groups.length}>
              <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
              Add rule
            </Button>
          </div>
        </fieldset>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}
