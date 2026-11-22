"use client";

import { Pencil, Plus, UserMinus, UserRound } from "lucide-react";
import { useId, useState, type FormEvent } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { PeoplePicker } from "@/components/governance/people-picker";
import type { Person } from "@/components/governance/shared";
import { Button } from "@/components/ui/button";
import { Card, CardHeader } from "@/components/ui/card";
import { DataTable } from "@/components/ui/data-table";
import { Dialog } from "@/components/ui/dialog";
import { HoldToConfirm } from "@/components/ui/hold-to-confirm";
import { Checkbox, Field, Input, Select } from "@/components/ui/input";
import { ApiError, api } from "@/lib/api/client";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { directoryMessages } from "@/lib/i18n/console/directory";
import type { Group, JoinPolicy, User } from "@/lib/types";
import { userColumns } from "./users-table";

type GroupBody = { name: string; description: string; rule?: string; joinPolicy?: JoinPolicy };

export function inlineRuleError(show: (message: string) => void) {
  return (error: unknown) => {
    if (!(error instanceof ApiError && error.code === "ERR_INVALID_RULE")) return false;
    show(error.message);
    return true;
  };
}

export function EditGroupButton({ group }: { group: Group }) {
  const [open, setOpen] = useState(false);
  const formId = useId();
  const { pending, run, toast } = useMutation();
  const t = useMessages(directoryMessages);

  function save(body: GroupBody, showRuleError: (message: string) => void) {
    void run(
      "save",
      () => api<Group>(`/groups/${group.id}`, { method: "PATCH", body }),
      (saved) => {
        setOpen(false);
        toast({ title: t.group.saved(saved.name), description: t.recorded });
      },
      inlineRuleError(showRuleError),
    );
  }

  return (
    <>
      <Button size="sm" variant="secondary" onClick={() => setOpen(true)} className="self-start">
        <Pencil aria-hidden="true" size={16} strokeWidth={1.75} />
        {t.group.edit}
      </Button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        title={t.group.edit}
        description={group.kind === "dynamic" ? t.group.editDynamic : t.group.editAssigned}
        footer={
          <>
            <Button variant="secondary" onClick={() => setOpen(false)}>
              {t.cancel}
            </Button>
            <Button type="submit" form={formId} variant="primary" loading={pending === "save"}>
              {t.group.save}
            </Button>
          </>
        }
      >
        <GroupForm id={formId} group={group} onSave={save} />
      </Dialog>
    </>
  );
}

function GroupForm({ id, group, onSave }: { id: string; group: Group; onSave: (body: GroupBody, showRuleError: (message: string) => void) => void }) {
  const [name, setName] = useState(group.name);
  const [description, setDescription] = useState(group.description);
  const [rule, setRule] = useState(group.rule ?? "");
  const [joinPolicy, setJoinPolicy] = useState(group.joinPolicy);
  const [errors, setErrors] = useState<{ name?: string; rule?: string }>({});
  const [preview, setPreview] = useState<User[] | null>(null);
  const [previewing, setPreviewing] = useState(false);
  const t = useMessages(directoryMessages);
  const dynamic = group.kind === "dynamic";
  const everyone = rule.trim() === "true";

  async function previewRule() {
    setPreviewing(true);
    try {
      setPreview(await api<User[]>(`/groups/${group.id}/members?rule=${encodeURIComponent(rule.trim())}`));
      setErrors({ ...errors, rule: undefined });
    } catch (error) {
      setPreview(null);
      setErrors({ ...errors, rule: error instanceof ApiError ? error.message : t.unreachable });
    } finally {
      setPreviewing(false);
    }
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    const next = {
      name: name.trim() ? undefined : t.validation.groupName,
      rule: dynamic && !rule.trim() ? t.group.ruleRequired : undefined,
    };
    setErrors(next);
    if (next.name || next.rule) return;
    onSave(
      { name: name.trim(), description: description.trim(), ...(dynamic ? { rule: rule.trim() } : { joinPolicy }) },
      (message) => setErrors({ rule: message }),
    );
  }

  return (
    <form id={id} className="flex flex-col gap-6" onSubmit={submit} noValidate>
      <Field label={t.fields.name} error={errors.name}>
        {(props) => <Input {...props} autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} />}
      </Field>
      <Field label={t.fields.description} hint={t.descriptionHint}>
        {(props) => <Input {...props} value={description} onChange={(event) => setDescription(event.target.value)} />}
      </Field>
      {dynamic ? null : (
        <Field
          label={t.group.whoCanJoin}
          hint={group.owners.length ? t.group.joinWithOwners : t.group.joinWithoutOwners}
        >
          {(props) => (
            <Select {...props} value={joinPolicy} onChange={(event) => setJoinPolicy(event.target.value as JoinPolicy)}>
              {(Object.keys(t.joinPolicy) as JoinPolicy[]).map((policy) => (
                <option key={policy} value={policy} disabled={policy === "request" && group.owners.length === 0}>
                  {t.joinPolicy[policy].label} — {t.joinPolicy[policy].description}
                </option>
              ))}
            </Select>
          )}
        </Field>
      )}
      {dynamic ? (
        <div className="flex flex-col gap-3">
          <label className="flex cursor-pointer items-start gap-3">
            <Checkbox
              className="mt-0.5"
              checked={everyone}
              onChange={(event) => {
                setRule(event.target.checked ? "true" : "");
                setPreview(null);
                setErrors({ ...errors, rule: undefined });
              }}
            />
            <span className="flex flex-col">
              <span className="text-body-sm text-fg">{t.group.allUsers}</span>
              <span className="text-caption text-fg-3">{t.group.allUsersHint}</span>
            </span>
          </label>
          {everyone ? null : (
            <Field
              label={t.fields.rule}
              error={errors.rule}
              hint={
                <>
                  {t.ruleHint}{" "}
                  <code className="text-code-sm text-fg-2">user.email endsWith &quot;@example.com&quot; and user.status == &quot;active&quot;</code>
                </>
              }
            >
              {(props) => (
                <Input
                  {...props}
                  mono
                  autoComplete="off"
                  spellCheck={false}
                  value={rule}
                  onChange={(event) => {
                    setRule(event.target.value);
                    setPreview(null);
                  }}
                />
              )}
            </Field>
          )}
          <div className="flex flex-wrap items-center gap-3">
            <Button size="sm" variant="secondary" loading={previewing} disabled={!rule.trim()} onClick={previewRule}>
              {t.group.preview}
            </Button>
            {preview ? (
              <p className="animate-enter text-body-sm text-fg-2" aria-live="polite">
                {preview.length === 0
                  ? t.group.nobodyMatches
                  : t.group.matches(
                      preview.length,
                      preview
                        .slice(0, 5)
                        .map((u) => u.name)
                        .join(", "),
                      preview.length - 5,
                    )}
              </p>
            ) : null}
          </div>
        </div>
      ) : null}
    </form>
  );
}

export function AddMembers({ group, people }: { group: Pick<Group, "id" | "name">; people: Person[] }) {
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState<string[]>([]);
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const t = useMessages(directoryMessages);

  function close() {
    setOpen(false);
    setSelected([]);
    clearError();
  }

  function add() {
    const count = selected.length;
    void run(
      "add",
      () => Promise.all(selected.map((userId) => api(`/groups/${group.id}/members`, { body: { userId } }))),
      () => {
        close();
        toast({ title: t.group.added(count), description: t.group.addedHint(group.name) });
      },
      "inline",
    );
  }

  return (
    <>
      <Button size="sm" variant="secondary" onClick={() => setOpen(true)} disabled={people.length === 0}>
        <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
        {t.group.addMembers}
      </Button>
      <Dialog
        open={open}
        onOpenChange={(next) => (next ? setOpen(true) : close())}
        error={errorFor("add")}
        title={t.group.addTitle(group.name)}
        description={t.group.addDescription}
        footer={
          <>
            <Button variant="secondary" onClick={close}>
              {t.cancel}
            </Button>
            <Button variant="primary" disabled={selected.length === 0} loading={pending === "add"} onClick={add}>
              {selected.length ? t.group.addPeople(selected.length) : t.group.addMembers}
            </Button>
          </>
        }
      >
        <Field label={t.group.people}>{(props) => <PeoplePicker {...props} people={people} value={selected} onChange={setSelected} />}</Field>
      </Dialog>
    </>
  );
}

export function EditOwners({ group, people }: { group: Group; people: Person[] }) {
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState<string[]>([]);
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const t = useMessages(directoryMessages);

  function show() {
    setSelected(group.owners.map((o) => o.id));
    clearError();
    setOpen(true);
  }

  function save() {
    void run(
      "owners",
      () => api<Group>(`/groups/${group.id}/owners`, { method: "PUT", body: { userIds: selected } }),
      (saved) => {
        setOpen(false);
        toast({ title: t.group.ownersSaved(saved.name), description: t.recorded });
      },
      "inline",
    );
  }

  return (
    <>
      <Button size="sm" variant="secondary" onClick={show}>
        <Pencil aria-hidden="true" size={16} strokeWidth={1.75} />
        {t.group.editOwners}
      </Button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        error={errorFor("owners")}
        title={t.group.ownersTitle(group.name)}
        description={t.group.ownersDialogDescription}
        footer={
          <>
            <Button variant="secondary" onClick={() => setOpen(false)}>
              {t.cancel}
            </Button>
            <Button variant="primary" loading={pending === "owners"} onClick={save}>
              {t.group.saveOwners}
            </Button>
          </>
        }
      >
        <Field label={t.group.owners}>{(props) => <PeoplePicker {...props} people={people} value={selected} onChange={setSelected} />}</Field>
      </Dialog>
    </>
  );
}

export function DeleteGroup({ group, appCount }: { group: Group; appCount: number }) {
  const { run, toast, router } = useMutation();
  const t = useMessages(directoryMessages);

  function remove() {
    return run(
      "delete",
      () => api(`/groups/${group.id}`, { method: "DELETE" }),
      () => {
        toast({ title: t.group.deleted(group.name), description: t.recorded });
        router.push("/admin/groups");
      },
    );
  }

  return (
    <Card>
      <CardHeader title={t.dangerZone} />
      <div className="flex flex-col gap-4 border-t border-border px-6 py-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex flex-col gap-1">
          <p className="text-body-sm font-semibold text-fg">{t.group.deleteGroup}</p>
          <p className="text-body-sm text-fg-3">
            {appCount > 0 ? t.group.unassignFirst(appCount) : null}
            {t.group.deleteDescription}
          </p>
        </div>
        <HoldToConfirm size="sm" doneLabel={t.group.deletedLabel} className="self-start sm:self-center" onConfirm={remove}>
          {t.group.holdToDelete}
        </HoldToConfirm>
      </div>
    </Card>
  );
}

export function GroupMembersTable({ group, members }: { group: Group; members: User[] }) {
  const { run, toast, router } = useMutation();
  const locale = useLocale();
  const t = directoryMessages[locale];
  const departments = [...new Set(members.map((u) => u.department).filter(Boolean))].sort();

  function remove(rows: User[]) {
    return run(
      "remove",
      () => Promise.all(rows.map((u) => api(`/groups/${group.id}/members/${u.id}`, { method: "DELETE" }))),
      () =>
        toast({
          title: t.addToGroup.removed(group.name),
          description: t.group.removedHint(rows.length, rows[0]?.name),
        }),
    );
  }

  return (
    <DataTable
      data={members}
      columns={userColumns(locale).filter((c) => ["name", "status", "strength", "department", "lastSignIn"].includes(c.id ?? ""))}
      getRowId={(u) => u.id}
      label={group.kind === "dynamic" ? t.group.matchingPeople : t.fields.members}
      noun={t.peopleNoun}
      storageKey="group-members"
      searchText={(u) => `${u.name} ${u.email} ${u.title} ${u.department} ${u.location}`}
      searchPlaceholder={t.searchPeople}
      facets={[
        { column: "status", label: t.fields.status, options: Object.entries(t.status).map(([value, label]) => ({ value, label })) },
        { column: "strength", label: t.fields.authentication, options: Object.entries(t.strength).map(([value, s]) => ({ value, label: s.label })) },
        { column: "department", label: t.fields.department, options: departments.map((d) => ({ value: d, label: d })) },
      ]}
      onRowOpen={(u) => router.push(`/admin/users/${u.id}`)}
      rowActions={[
        { label: t.openProfile, icon: UserRound, onSelect: (u) => router.push(`/admin/users/${u.id}`) },
        ...(group.kind === "dynamic" ? [] : [{ label: t.group.removeFromGroup, icon: UserMinus, danger: true, separatorBefore: true, onSelect: (u: User) => remove([u]) }]),
      ]}
      bulkActions={group.kind === "dynamic" ? undefined : [{ label: t.group.removeFromGroup, icon: UserMinus, danger: true, onSelect: remove }]}
    />
  );
}
