"use client";

import { Pencil, Plus, Trash2, UsersRound } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Tag } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Field, Input, Select } from "@/components/ui/input";
import { SectionTitle } from "@/components/ui/section-title";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { formatDuration, pluralize } from "@/lib/format";
import type { PrincipalMapping, SshAuthority } from "@/lib/infra-types";
import type { Group } from "@/lib/types";

const LIFETIMES = [900, 3600, 7200, 14400, 28800, 43200, 86400];

export function LifetimeSetting({ lifetime }: { lifetime: number }) {
  const { pending, run, toast } = useMutation();
  const [value, setValue] = useState(lifetime);
  const options = LIFETIMES.includes(lifetime) ? LIFETIMES : [...LIFETIMES, lifetime].sort((a, b) => a - b);

  return (
    <form
      className="flex flex-col gap-3 sm:flex-row sm:items-start"
      onSubmit={(event) => {
        event.preventDefault();
        void run("lifetime", () => api<SshAuthority>("/ssh/authority", { method: "PUT", body: { certificateLifetime: value } }), () =>
          toast({ title: `Certificates now last ${formatDuration(value)}`, description: "Applies to certificates issued from now on. Recorded in the audit log." }),
        );
      }}
    >
      <Field label="Certificate lifetime" hint="People request a new certificate with halo ssh-cert when theirs expires." className="sm:w-64">
        {(props) => (
          <Select {...props} value={value} onChange={(event) => setValue(Number(event.target.value))}>
            {options.map((seconds) => (
              <option key={seconds} value={seconds}>
                {formatDuration(seconds)}
                {seconds === 28800 ? " (default)" : ""}
              </option>
            ))}
          </Select>
        )}
      </Field>
      <Button type="submit" variant="secondary" loading={pending === "lifetime"} disabled={value === lifetime} className="sm:mt-6">
        Save lifetime
      </Button>
    </form>
  );
}

export function MappingsSection({ mappings, groups }: { mappings: PrincipalMapping[]; groups: Group[] }) {
  const { pending, run, toast } = useMutation();
  const [editing, setEditing] = useState<PrincipalMapping | "new" | null>(null);
  const [deleting, setDeleting] = useState<PrincipalMapping | null>(null);
  const groupName = (id: string) => groups.find((g) => g.id === id)?.name ?? "Deleted group";

  function remove() {
    const mapping = deleting;
    if (!mapping) return;
    void run("delete", () => api(`/ssh/principal-mappings/${mapping.id}`, { method: "DELETE" }), () => {
      setDeleting(null);
      toast({ title: `${groupName(mapping.groupId)} unmapped`, description: "Its members get no certificate for these accounts from now on. Recorded in the audit log." });
    });
  }

  return (
    <div className="flex flex-col gap-6">
      <SectionTitle
        title="Principal mappings"
        description="Members of a group may log in as these unix accounts. A certificate lists every principal from every group the person belongs to."
        action={
          <Button size="sm" variant="primary" onClick={() => setEditing("new")}>
            <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
            Add mapping
          </Button>
        }
      />
      {mappings.length ? (
        <SimpleTable
          caption="Principal mappings"
          head={["Group", "Principals", "Members", "Actions"]}
          rows={mappings.map((mapping) => {
            const group = groups.find((g) => g.id === mapping.groupId);
            return [
              <span key="group" className="font-medium whitespace-nowrap text-fg">
                {groupName(mapping.groupId)}
              </span>,
              <span key="principals" className="flex flex-wrap gap-1">
                {mapping.principals.map((principal) => (
                  <Tag key={principal}>{principal}</Tag>
                ))}
              </span>,
              <span key="members" className="tnum whitespace-nowrap">
                {group ? pluralize(group.memberCount, "person", "people") : "—"}
              </span>,
              <span key="actions" className="flex gap-1">
                <Button size="icon-sm" variant="quiet" aria-label={`Edit the mapping for ${groupName(mapping.groupId)}`} onClick={() => setEditing(mapping)}>
                  <Pencil aria-hidden="true" size={16} strokeWidth={1.75} />
                </Button>
                <Button size="icon-sm" variant="quiet" aria-label={`Delete the mapping for ${groupName(mapping.groupId)}`} onClick={() => setDeleting(mapping)}>
                  <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
                </Button>
              </span>,
            ];
          })}
        />
      ) : (
        <Card>
          <EmptyState
            icon={UsersRound}
            title="No principal mappings yet"
            description="Map a group to the unix accounts its members use, for example Infrastructure to ops. Without a mapping nobody can get a certificate."
          />
        </Card>
      )}
      {editing ? (
        <MappingDialog
          key={editing === "new" ? "new" : editing.id}
          mapping={editing === "new" ? null : editing}
          groups={groups.filter((g) => (editing !== "new" && g.id === editing.groupId) || !mappings.some((m) => m.groupId === g.id))}
          onClose={() => setEditing(null)}
        />
      ) : null}
      <Dialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (!open) setDeleting(null);
        }}
        title={`Delete the mapping for ${deleting ? groupName(deleting.groupId) : "this group"}?`}
        description="Certificates already issued stay valid until they expire, because sshd cannot check revocation."
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(null)}>
              Cancel
            </Button>
            <Button variant="destructive" loading={pending === "delete"} onClick={remove}>
              Delete mapping
            </Button>
          </>
        }
      />
    </div>
  );
}

function MappingDialog({ mapping, groups, onClose }: { mapping: PrincipalMapping | null; groups: Group[]; onClose: () => void }) {
  const { pending, run, toast } = useMutation();
  const [groupId, setGroupId] = useState(mapping?.groupId ?? "");
  const [principals, setPrincipals] = useState(mapping?.principals.join(", ") ?? "");
  const [errors, setErrors] = useState<{ group?: string; principals?: string }>({});

  function save() {
    const list = principals
      .split(/[\s,]+/)
      .map((value) => value.trim())
      .filter(Boolean);
    const next = {
      group: groupId ? undefined : "Choose the group whose members get these principals.",
      principals: list.length ? undefined : "Add at least one unix account name, such as ops.",
    };
    setErrors(next);
    if (next.group || next.principals) return;
    const body = { groupId, principals: list };
    void run(
      "save",
      () =>
        mapping
          ? api<PrincipalMapping>(`/ssh/principal-mappings/${mapping.id}`, { method: "PUT", body })
          : api<PrincipalMapping>("/ssh/principal-mappings", { body }),
      (saved) => {
        const name = groups.find((g) => g.id === saved.groupId)?.name ?? "The group";
        toast({ title: `${name} mapped to ${saved.principals.join(", ")}`, description: "Applies to the next certificate each member requests." });
        onClose();
      },
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={mapping ? "Edit principal mapping" : "Add principal mapping"}
      description="Principals are the account names on your servers that a certificate lets someone log in as."
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={save}>
            {mapping ? "Save mapping" : "Add mapping"}
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
        <Field label="Group" error={errors.group}>
          {(props) => (
            <Select {...props} value={groupId} onChange={(event) => setGroupId(event.target.value)}>
              <option value="" disabled>
                Choose a group
              </option>
              {groups.map((g) => (
                <option key={g.id} value={g.id}>
                  {g.name}
                </option>
              ))}
            </Select>
          )}
        </Field>
        <Field label="Principals" error={errors.principals} hint="Unix account names separated by commas or spaces, such as ops, deploy.">
          {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={principals} onChange={(event) => setPrincipals(event.target.value)} placeholder="ops" />}
        </Field>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}
