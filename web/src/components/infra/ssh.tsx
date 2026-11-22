"use client";

import { Pencil, Plus, Trash2, UsersRound } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ActionError } from "@/components/ui/action-error";
import { Tag } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Field, Input, Select } from "@/components/ui/input";
import { SectionTitle } from "@/components/ui/section-title";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { useMessages } from "@/lib/i18n/client";
import { infrastructureMessages } from "@/lib/i18n/console/infrastructure";
import type { PrincipalMapping, SshAuthority } from "@/lib/infra-types";
import type { Group } from "@/lib/types";

const LIFETIMES = [900, 3600, 7200, 14400, 28800, 43200, 86400];
const HOST_LIFETIMES = [604800, 2592000, 7776000, 31536000];

export function LifetimeSetting({ lifetime, host = false }: { lifetime: number; host?: boolean }) {
  const { pending, run, errorFor, toast } = useMutation();
  const t = useMessages(infrastructureMessages);
  const [value, setValue] = useState(lifetime);
  const choices = host ? HOST_LIFETIMES : LIFETIMES;
  const options = choices.includes(lifetime) ? choices : [...choices, lifetime].sort((a, b) => a - b);

  return (
    <form
      className="flex flex-col gap-3 sm:flex-row sm:items-start"
      onSubmit={(event) => {
        event.preventDefault();
        void run("lifetime", () => api<SshAuthority>(host ? "/ssh/host-authority" : "/ssh/authority", { method: "PUT", body: { certificateLifetime: value } }), () =>
          toast({ title: t.ssh.lifetimeSaved(host, t.duration(value)), description: t.ssh.lifetimeSavedHint }),
          "inline",
        );
      }}
    >
      <Field
        label={host ? t.ssh.hostLifetime : t.ssh.lifetime}
        hint={host ? t.ssh.hostLifetimeHint : t.ssh.lifetimeHint}
        className="sm:w-64"
      >
        {(props) => (
          <Select {...props} value={value} onChange={(event) => setValue(Number(event.target.value))}>
            {options.map((seconds) => (
              <option key={seconds} value={seconds}>
                {t.duration(seconds)}
                {seconds === (host ? 2592000 : 28800) ? t.ssh.defaultSuffix : ""}
              </option>
            ))}
          </Select>
        )}
      </Field>
      <Button type="submit" variant="secondary" loading={pending === "lifetime"} disabled={value === lifetime} className="sm:mt-6">
        {t.ssh.saveLifetime}
      </Button>
      <ActionError message={errorFor("lifetime")} className="sm:mt-8" />
    </form>
  );
}

export function MappingsSection({ mappings, groups }: { mappings: PrincipalMapping[]; groups: Group[] }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [editing, setEditing] = useState<PrincipalMapping | "new" | null>(null);
  const [deleting, setDeleting] = useState<PrincipalMapping | null>(null);
  const t = useMessages(infrastructureMessages);
  const groupName = (id: string) => groups.find((g) => g.id === id)?.name ?? t.deletedGroup;

  function remove() {
    const mapping = deleting;
    if (!mapping) return;
    void run("delete", () => api(`/ssh/principal-mappings/${mapping.id}`, { method: "DELETE" }), () => {
      setDeleting(null);
      toast({ title: t.ssh.unmapped(groupName(mapping.groupId)), description: t.ssh.unmappedHint });
    }, "inline");
  }

  return (
    <div className="flex flex-col gap-6">
      <SectionTitle
        title={t.ssh.mappings}
        description={t.ssh.mappingsHint}
        action={
          <Button size="sm" variant="primary" onClick={() => setEditing("new")}>
            <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
            {t.ssh.addMapping}
          </Button>
        }
      />
      {mappings.length ? (
        <SimpleTable
          caption={t.ssh.mappings}
          head={t.ssh.head}
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
                {group ? t.ssh.people(group.memberCount) : "—"}
              </span>,
              <span key="actions" className="flex gap-1">
                <Button size="icon-sm" variant="quiet" aria-label={t.ssh.editMapping(groupName(mapping.groupId))} onClick={() => setEditing(mapping)}>
                  <Pencil aria-hidden="true" size={16} strokeWidth={1.75} />
                </Button>
                <Button size="icon-sm" variant="quiet" aria-label={t.ssh.deleteMapping(groupName(mapping.groupId))} onClick={() => setDeleting(mapping)}>
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
            title={t.ssh.noMappings}
            description={t.ssh.noMappingsHint}
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
          if (open) return;
          setDeleting(null);
          clearError();
        }}
        error={errorFor("delete")}
        title={t.ssh.deleteTitle(deleting ? groupName(deleting.groupId) : null)}
        description={t.ssh.deleteHint}
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(null)}>
              {t.cancel}
            </Button>
            <Button variant="destructive" loading={pending === "delete"} onClick={remove}>
              {t.ssh.deleteConfirm}
            </Button>
          </>
        }
      />
    </div>
  );
}

function MappingDialog({ mapping, groups, onClose }: { mapping: PrincipalMapping | null; groups: Group[]; onClose: () => void }) {
  const { pending, run, errorFor, toast } = useMutation();
  const [groupId, setGroupId] = useState(mapping?.groupId ?? "");
  const [principals, setPrincipals] = useState(mapping?.principals.join(", ") ?? "");
  const [errors, setErrors] = useState<{ group?: string; principals?: string }>({});
  const t = useMessages(infrastructureMessages);

  function save() {
    const list = principals
      .split(/[\s,]+/)
      .map((value) => value.trim())
      .filter(Boolean);
    const next = {
      group: groupId ? undefined : t.ssh.groupError,
      principals: list.length ? undefined : t.ssh.principalsError,
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
        const name = groups.find((g) => g.id === saved.groupId)?.name ?? t.ssh.theGroup;
        toast({ title: t.ssh.mapped(name, saved.principals.join(", ")), description: t.ssh.mappedHint });
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
      title={mapping ? t.ssh.editTitle : t.ssh.addTitle}
      description={t.ssh.dialogHint}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t.cancel}
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={save}>
            {mapping ? t.ssh.saveMapping : t.ssh.addMapping}
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
        <Field label={t.group} error={errors.group}>
          {(props) => (
            <Select {...props} value={groupId} onChange={(event) => setGroupId(event.target.value)}>
              <option value="" disabled>
                {t.chooseGroup}
              </option>
              {groups.map((g) => (
                <option key={g.id} value={g.id}>
                  {g.name}
                </option>
              ))}
            </Select>
          )}
        </Field>
        <Field label={t.ssh.principals} error={errors.principals} hint={t.ssh.principalsHint}>
          {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={principals} onChange={(event) => setPrincipals(event.target.value)} placeholder="ops" />}
        </Field>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}
