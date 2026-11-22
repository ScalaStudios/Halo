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
import { useMessages } from "@/lib/i18n/client";
import { infrastructureMessages } from "@/lib/i18n/console/infrastructure";
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
  const t = useMessages(infrastructureMessages);
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
        title={t.hostLogin.linux}
        description={t.hostLogin.linuxHint}
      />
      <CardBody>
        <form
          className="flex flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            const body = { uidMin: draft.uidMin, uidMax: draft.uidMax, gidMin: draft.gidMin, gidMax: draft.gidMax, loginShell: draft.loginShell.trim() };
            void run("posix", () => api<PosixSettings>("/ssh/posix", { method: "PUT", body }), () =>
              toast({ title: t.hostLogin.posixSaved, description: t.hostLogin.posixSavedHint }),
              "inline",
            );
          }}
        >
          <div className="grid grid-cols-1 gap-6 sm:grid-cols-2 xl:grid-cols-4">
            {number("uidMin", t.hostLogin.uidMin)}
            {number("uidMax", t.hostLogin.uidMax)}
            {number("gidMin", t.hostLogin.gidMin)}
            {number("gidMax", t.hostLogin.gidMax)}
          </div>
          <p className="text-body-sm text-fg-3">
            {t.hostLogin.left(left(settings.nextUid, settings.uidMax), left(settings.nextGid, settings.gidMax))}
          </p>
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start">
            <Field label={t.hostLogin.shell} hint={t.hostLogin.shellHint} className="sm:w-64">
              {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={draft.loginShell} onChange={(event) => setDraft({ ...draft, loginShell: event.target.value })} />}
            </Field>
            <Button type="submit" variant="secondary" loading={pending === "posix"} disabled={!changed} className="sm:mt-6">
              {t.hostLogin.saveSettings}
            </Button>
            <ActionError message={errorFor("posix")} className="sm:mt-8" />
          </div>
        </form>
      </CardBody>
    </Card>
  );
}

function describeSudo(rule: HostRule, t: (typeof infrastructureMessages)["en"]) {
  if (rule.sudo.length === 0) return <span className="text-fg-3">{t.hostLogin.noSudo}</span>;
  if (rule.sudo[0] === "ALL") return <Badge tone="ember">{t.hostLogin.fullSudo}</Badge>;
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
  const t = useMessages(infrastructureMessages);
  const groupName = (id: string) => groups.find((g) => g.id === id)?.name ?? t.deletedGroup;

  function remove() {
    const hostGroup = deleting;
    if (!hostGroup) return;
    void run("delete", () => api(`/ssh/host-groups/${hostGroup.id}`, { method: "DELETE" }), () => {
      setDeleting(null);
      toast({ title: t.deleted(hostGroup.name), description: t.hostLogin.deletedHint });
    }, "inline");
  }

  return (
    <div className="flex flex-col gap-6">
      <SectionTitle
        title={t.hostLogin.hostGroups}
        description={t.hostLogin.hostGroupsHint}
        action={
          <Button size="sm" variant="primary" onClick={() => setEditing("new")}>
            <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
            {t.hostLogin.addHostGroup}
          </Button>
        }
      />
      {hostGroups.length ? (
        <SimpleTable
          caption={t.hostLogin.hostGroups}
          head={t.hostLogin.head}
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
                    {describeSudo(rule, t)}
                  </li>
                ))}
              </ul>
            ) : (
              <span key="rules" className="text-fg-3">
                {t.hostLogin.nobody}
              </span>
            ),
            <span key="hosts" className="tnum whitespace-nowrap">
              {t.hostLogin.hosts(hostGroup.hostCount)}
            </span>,
            <span key="actions" className="flex gap-1">
              <Button size="icon-sm" variant="quiet" aria-label={t.edit(hostGroup.name)} onClick={() => setEditing(hostGroup)}>
                <Pencil aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
              <Button size="icon-sm" variant="quiet" aria-label={t.delete(hostGroup.name)} onClick={() => setDeleting(hostGroup)}>
                <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
            </span>,
          ])}
        />
      ) : (
        <Card>
          <EmptyState
            icon={ServerCog}
            title={t.hostLogin.noHostGroups}
            description={t.hostLogin.noHostGroupsHint}
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
        title={t.hostLogin.deleteTitle(deleting?.name ?? null)}
        description={t.hostLogin.deleteHint(deleting ? deleting.hostCount : null)}
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(null)}>
              {t.cancel}
            </Button>
            <Button variant="destructive" loading={pending === "delete"} onClick={remove}>
              {t.hostLogin.deleteConfirm}
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
  const t = useMessages(infrastructureMessages);
  const update = (index: number, change: Partial<RuleDraft>) => setRules(rules.map((rule, i) => (i === index ? { ...rule, ...change } : rule)));

  function save() {
    const next = {
      name: name.trim() ? undefined : t.hostLogin.nameError,
      rules: rules.some((rule) => !rule.groupId)
        ? t.hostLogin.ruleGroupError
        : rules.some((rule) => rule.mode === "commands" && !rule.commands.trim())
          ? t.hostLogin.commandsError
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
        toast({ title: t.saved(saved.name), description: t.hostLogin.savedHint });
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
      title={hostGroup ? t.edit(hostGroup.name) : t.hostLogin.addHostGroup}
      description={t.hostLogin.dialogHint}
      width="wide"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t.cancel}
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={save}>
            {hostGroup ? t.hostLogin.saveHostGroup : t.hostLogin.addHostGroup}
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
          <Field label={t.name} error={errors.name}>
            {(props) => <Input {...props} autoFocus autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder={t.hostLogin.namePlaceholder} />}
          </Field>
          <Field label={t.description}>
            {(props) => <Input {...props} autoComplete="off" value={description} onChange={(event) => setDescription(event.target.value)} placeholder={t.hostLogin.descriptionPlaceholder} />}
          </Field>
        </div>
        <fieldset className="flex flex-col gap-4">
          <legend className="mb-2 text-body-sm font-medium text-fg">{t.hostLogin.whoMayLogIn}</legend>
          {rules.length === 0 ? <p className="text-body-sm text-fg-3">{t.hostLogin.noRules}</p> : null}
          {rules.map((rule, index) => (
            <div key={index} className="flex flex-col gap-3 rounded-md border border-border p-4">
              <div className="flex items-start gap-3">
                <div className="grid flex-1 grid-cols-1 gap-3 sm:grid-cols-2">
                  <Field label={t.group}>
                    {(props) => (
                      <Select {...props} value={rule.groupId} onChange={(event) => update(index, { groupId: event.target.value })}>
                        <option value="" disabled>
                          {t.chooseGroup}
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
                  <Field label={t.hostLogin.sudo}>
                    {(props) => (
                      <Select {...props} value={rule.mode} onChange={(event) => update(index, { mode: event.target.value as SudoMode })}>
                        <option value="none">{t.hostLogin.noSudo}</option>
                        <option value="commands">{t.hostLogin.someCommands}</option>
                        <option value="all">{t.hostLogin.fullSudo}</option>
                      </Select>
                    )}
                  </Field>
                </div>
                <Button size="icon-sm" variant="quiet" className="mt-7" aria-label={t.hostLogin.removeRule} onClick={() => setRules(rules.filter((_, i) => i !== index))}>
                  <X aria-hidden="true" size={16} strokeWidth={1.75} />
                </Button>
              </div>
              {rule.mode === "commands" ? (
                <Field label={t.hostLogin.commands} hint={t.hostLogin.commandsHint}>
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
              {t.hostLogin.addRule}
            </Button>
          </div>
        </fieldset>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}
