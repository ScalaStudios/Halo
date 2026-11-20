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
import { pluralize } from "@/lib/format";
import { JOIN_POLICY, STATUS, STRENGTH } from "@/lib/labels";
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

  function save(body: GroupBody, showRuleError: (message: string) => void) {
    void run(
      "save",
      () => api<Group>(`/groups/${group.id}`, { method: "PATCH", body }),
      (saved) => {
        setOpen(false);
        toast({ title: `${saved.name} saved`, description: "Recorded in the audit log." });
      },
      inlineRuleError(showRuleError),
    );
  }

  return (
    <>
      <Button size="sm" variant="secondary" onClick={() => setOpen(true)} className="self-start">
        <Pencil aria-hidden="true" size={16} strokeWidth={1.75} />
        Edit group
      </Button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        title="Edit group"
        description={
          group.kind === "dynamic"
            ? "Members follow the rule. Preview it before saving to see who it includes."
            : "Members are added by hand. To follow a rule instead, create a rule-based group."
        }
        footer={
          <>
            <Button variant="secondary" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" form={formId} variant="primary" loading={pending === "save"}>
              Save group
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
  const dynamic = group.kind === "dynamic";
  const everyone = rule.trim() === "true";

  async function previewRule() {
    setPreviewing(true);
    try {
      setPreview(await api<User[]>(`/groups/${group.id}/members?rule=${encodeURIComponent(rule.trim())}`));
      setErrors({ ...errors, rule: undefined });
    } catch (error) {
      setPreview(null);
      setErrors({ ...errors, rule: error instanceof ApiError ? error.message : "Halo could not reach the server. Try again." });
    } finally {
      setPreviewing(false);
    }
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    const next = {
      name: name.trim() ? undefined : "Enter a group name.",
      rule: dynamic && !rule.trim() ? "Enter a rule. Rule-based groups need one." : undefined,
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
      <Field label="Name" error={errors.name}>
        {(props) => <Input {...props} autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} />}
      </Field>
      <Field label="Description" hint="Optional. Helps other administrators pick the right group.">
        {(props) => <Input {...props} value={description} onChange={(event) => setDescription(event.target.value)} />}
      </Field>
      {dynamic ? null : (
        <Field
          label="Who can join"
          hint={group.owners.length ? "People join or ask from the Groups page of their account." : "Add an owner to let people ask to join. Owners approve requests."}
        >
          {(props) => (
            <Select {...props} value={joinPolicy} onChange={(event) => setJoinPolicy(event.target.value as JoinPolicy)}>
              {(Object.keys(JOIN_POLICY) as JoinPolicy[]).map((policy) => (
                <option key={policy} value={policy} disabled={policy === "request" && group.owners.length === 0}>
                  {JOIN_POLICY[policy].label} — {JOIN_POLICY[policy].description}
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
              <span className="text-body-sm text-fg">All users</span>
              <span className="text-caption text-fg-3">Everyone in the directory is a member.</span>
            </span>
          </label>
          {everyone ? null : (
            <Field
              label="Rule"
              error={errors.rule}
              hint={
                <>
                  Combine attributes with and, or and not, for example{" "}
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
              Preview matches
            </Button>
            {preview ? (
              <p className="animate-enter text-body-sm text-fg-2" aria-live="polite">
                {preview.length === 0
                  ? "Nobody matches this rule yet."
                  : `${pluralize(preview.length, "person", "people")} ${preview.length === 1 ? "matches" : "match"}: ${preview
                      .slice(0, 5)
                      .map((u) => u.name)
                      .join(", ")}${preview.length > 5 ? ` and ${preview.length - 5} more` : ""}.`}
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
        toast({ title: `Added ${pluralize(count, "person", "people")}`, description: `They get ${group.name}'s applications at their next sign-in.` });
      },
      "inline",
    );
  }

  return (
    <>
      <Button size="sm" variant="secondary" onClick={() => setOpen(true)} disabled={people.length === 0}>
        <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
        Add members
      </Button>
      <Dialog
        open={open}
        onOpenChange={(next) => (next ? setOpen(true) : close())}
        error={errorFor("add")}
        title={`Add members to ${group.name}`}
        description="Members get the group's applications at their next sign-in. Recorded in the audit log."
        footer={
          <>
            <Button variant="secondary" onClick={close}>
              Cancel
            </Button>
            <Button variant="primary" disabled={selected.length === 0} loading={pending === "add"} onClick={add}>
              {selected.length ? `Add ${pluralize(selected.length, "person", "people")}` : "Add members"}
            </Button>
          </>
        }
      >
        <Field label="People">{(props) => <PeoplePicker {...props} people={people} value={selected} onChange={setSelected} />}</Field>
      </Dialog>
    </>
  );
}

export function EditOwners({ group, people }: { group: Group; people: Person[] }) {
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState<string[]>([]);
  const { pending, run, errorFor, clearError, toast } = useMutation();

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
        toast({ title: `${saved.name} owners saved`, description: "Recorded in the audit log." });
      },
      "inline",
    );
  }

  return (
    <>
      <Button size="sm" variant="secondary" onClick={show}>
        <Pencil aria-hidden="true" size={16} strokeWidth={1.75} />
        Edit owners
      </Button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        error={errorFor("owners")}
        title={`Owners of ${group.name}`}
        description="Owners add and remove members and approve requests to join from their account. They don't get an administrator role."
        footer={
          <>
            <Button variant="secondary" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button variant="primary" loading={pending === "owners"} onClick={save}>
              Save owners
            </Button>
          </>
        }
      >
        <Field label="Owners">{(props) => <PeoplePicker {...props} people={people} value={selected} onChange={setSelected} />}</Field>
      </Dialog>
    </>
  );
}

export function DeleteGroup({ group, appCount }: { group: Group; appCount: number }) {
  const { run, toast, router } = useMutation();

  function remove() {
    return run(
      "delete",
      () => api(`/groups/${group.id}`, { method: "DELETE" }),
      () => {
        toast({ title: `${group.name} deleted`, description: "Recorded in the audit log." });
        router.push("/admin/groups");
      },
    );
  }

  return (
    <Card>
      <CardHeader title="Danger zone" />
      <div className="flex flex-col gap-4 border-t border-border px-6 py-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex flex-col gap-1">
          <p className="text-body-sm font-semibold text-fg">Delete group</p>
          <p className="text-body-sm text-fg-3">
            {appCount > 0 ? `Unassign it from ${pluralize(appCount, "application")} first. ` : null}
            Halo keeps the group while an application, access package or policy uses it. Deleting removes memberships, not people.
          </p>
        </div>
        <HoldToConfirm size="sm" doneLabel="Deleted" className="self-start sm:self-center" onConfirm={remove}>
          Hold to delete
        </HoldToConfirm>
      </div>
    </Card>
  );
}

export function GroupMembersTable({ group, members }: { group: Group; members: User[] }) {
  const { run, toast, router } = useMutation();
  const departments = [...new Set(members.map((u) => u.department).filter(Boolean))].sort();

  function remove(rows: User[]) {
    return run(
      "remove",
      () => Promise.all(rows.map((u) => api(`/groups/${group.id}/members/${u.id}`, { method: "DELETE" }))),
      () =>
        toast({
          title: `Removed from ${group.name}`,
          description: `${rows.length === 1 ? rows[0]?.name : pluralize(rows.length, "person", "people")} ${rows.length === 1 ? "loses" : "lose"} the group's applications at next sign-in.`,
        }),
    );
  }

  return (
    <DataTable
      data={members}
      columns={userColumns.filter((c) => ["name", "status", "strength", "department", "lastSignIn"].includes(c.id ?? ""))}
      getRowId={(u) => u.id}
      label={group.kind === "dynamic" ? "Matching people" : "Members"}
      noun={["person", "people"]}
      storageKey="group-members"
      searchText={(u) => `${u.name} ${u.email} ${u.title} ${u.department} ${u.location}`}
      searchPlaceholder="Search name, email or title"
      facets={[
        { column: "status", label: "Status", options: Object.entries(STATUS).map(([value, s]) => ({ value, label: s.label })) },
        { column: "strength", label: "Authentication", options: Object.entries(STRENGTH).map(([value, s]) => ({ value, label: s.label })) },
        { column: "department", label: "Department", options: departments.map((d) => ({ value: d, label: d })) },
      ]}
      onRowOpen={(u) => router.push(`/admin/users/${u.id}`)}
      rowActions={[
        { label: "Open profile", icon: UserRound, onSelect: (u) => router.push(`/admin/users/${u.id}`) },
        ...(group.kind === "dynamic" ? [] : [{ label: "Remove from group", icon: UserMinus, danger: true, separatorBefore: true, onSelect: (u: User) => remove([u]) }]),
      ]}
      bulkActions={group.kind === "dynamic" ? undefined : [{ label: "Remove from group", icon: UserMinus, danger: true, onSelect: remove }]}
    />
  );
}
