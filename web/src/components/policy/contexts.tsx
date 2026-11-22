"use client";

import { Pencil, Plus, ShieldCheck, Trash2 } from "lucide-react";
import { useState } from "react";
import { DurationField, split, toSeconds, wholeNumber } from "@/components/applications/edit-application";
import { useMutation } from "@/components/console/use-mutation";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Field, Input } from "@/components/ui/input";
import { SectionTitle } from "@/components/ui/section-title";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { useMessages } from "@/lib/i18n/client";
import { policyMessages } from "@/lib/i18n/console/policy";
import type { AuthContext } from "@/lib/policy-types";
import type { MethodKind } from "@/lib/types";
import { CheckList } from "./policy-form";
import { POLICY_METHODS, joinList } from "./shared";

const IDENTIFIER = /^[A-Za-z0-9][A-Za-z0-9._:-]{0,99}$/;

export function ContextsSection({ contexts, usedBy }: { contexts: AuthContext[]; usedBy: Record<string, string[]> }) {
  const copy = useMessages(policyMessages);
  const t = copy.contexts;
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [editing, setEditing] = useState<AuthContext | "new" | null>(null);
  const [deleting, setDeleting] = useState<AuthContext | null>(null);

  function remove() {
    const item = deleting;
    if (!item) return;
    void run("delete", () => api(`/authentication-contexts/${encodeURIComponent(item.id)}`, { method: "DELETE" }), () => {
      setDeleting(null);
      toast({ title: copy.deleted(item.name), description: t.deletedDescription(item.id) });
    }, "inline");
  }

  return (
    <div className="flex flex-col gap-6">
      <SectionTitle
        title={t.title}
        description={t.description}
        action={
          <Button size="sm" variant="primary" onClick={() => setEditing("new")}>
            <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
            {t.add}
          </Button>
        }
      />
      {contexts.length ? (
        <SimpleTable
          caption={t.title}
          head={[copy.name, t.identifier, t.methods, t.within, copy.usedBy, copy.actions]}
          rows={contexts.map((item) => [
            <span key="name" className="font-medium whitespace-nowrap text-fg">
              {item.name}
            </span>,
            <span key="id" className="font-mono text-code-sm text-fg-2">
              {item.id}
            </span>,
            <span key="methods">{item.methods.length ? joinList(item.methods.map((m) => copy.labels.method[m]), copy.or) : <span className="text-fg-3">{t.anyMethod}</span>}</span>,
            <span key="age" className="whitespace-nowrap">
              {item.maxAge ? copy.duration(item.maxAge) : <span className="text-fg-3">{t.anyTime}</span>}
            </span>,
            <span key="used">{usedBy[item.id]?.length ? joinList(usedBy[item.id]!, copy.and) : <span className="text-fg-3">{copy.noPolicy}</span>}</span>,
            <span key="actions" className="flex gap-1">
              <Button size="icon-sm" variant="quiet" aria-label={copy.edit(item.name)} onClick={() => setEditing(item)}>
                <Pencil aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
              <Button size="icon-sm" variant="quiet" aria-label={copy.delete(item.name)} onClick={() => setDeleting(item)}>
                <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
            </span>,
          ])}
        />
      ) : (
        <Card>
          <EmptyState
            icon={ShieldCheck}
            title={t.empty}
            description={t.emptyDescription}
          />
        </Card>
      )}
      {editing ? <ContextDialog key={editing === "new" ? "new" : editing.id} item={editing === "new" ? null : editing} onClose={() => setEditing(null)} /> : null}
      <Dialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (open) return;
          setDeleting(null);
          clearError();
        }}
        error={errorFor("delete")}
        title={copy.confirmDelete(deleting?.name ?? t.deleteFallback)}
        description={t.deleteDescription}
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(null)}>
              {copy.cancel}
            </Button>
            <Button variant="destructive" loading={pending === "delete"} onClick={remove}>
              {t.delete}
            </Button>
          </>
        }
      />
    </div>
  );
}

function ContextDialog({ item, onClose }: { item: AuthContext | null; onClose: () => void }) {
  const copy = useMessages(policyMessages);
  const t = copy.contexts;
  const { pending, run, errorFor, toast } = useMutation();
  const [identifier, setIdentifier] = useState(item?.id ?? "");
  const [name, setName] = useState(item?.name ?? "");
  const [methods, setMethods] = useState<MethodKind[]>(item?.methods ?? []);
  const [age, setAge] = useState(split(item?.maxAge ?? 0));
  const [errors, setErrors] = useState<{ identifier?: string; name?: string; age?: string; requirement?: string }>({});

  function save() {
    const next = {
      identifier: item || IDENTIFIER.test(identifier.trim()) ? undefined : t.identifierInvalid,
      name: name.trim() ? undefined : copy.enterName(t.nameExample),
      age: wholeNumber(age),
      requirement: methods.length || toSeconds(age) ? undefined : t.requirement,
    };
    setErrors(next);
    if (Object.values(next).some(Boolean)) return;
    const body = { id: item?.id ?? identifier.trim(), name: name.trim(), methods, maxAge: toSeconds(age) };
    void run(
      "save",
      () =>
        item
          ? api<AuthContext>(`/authentication-contexts/${encodeURIComponent(item.id)}`, { method: "PUT", body })
          : api<AuthContext>("/authentication-contexts", { body }),
      (saved) => {
        toast({ title: item ? copy.saved(saved.name) : copy.added(saved.name), description: t.savedDescription(saved.id) });
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
      width="wide"
      error={errorFor("save")}
      title={item ? copy.edit(item.name) : t.addTitle}
      description={item ? t.editDescription(item.id) : t.addDescription}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {copy.cancel}
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={save}>
            {item ? t.save : t.add}
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
        {item ? null : (
          <Field label={t.identifier} error={errors.identifier} hint={t.identifierHint}>
            {(props) => <Input {...props} mono autoFocus autoComplete="off" spellCheck={false} value={identifier} onChange={(event) => setIdentifier(event.target.value)} placeholder="passkey-recent" />}
          </Field>
        )}
        <Field label={copy.name} error={errors.name}>
          {(props) => <Input {...props} autoFocus={item !== null} autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder={t.nameExample} />}
        </Field>
        <div className="flex flex-col gap-2">
          <CheckList
            legend={t.methods}
            options={POLICY_METHODS.map((kind) => ({ id: kind, name: copy.labels.method[kind] }))}
            selected={methods}
            onToggle={(id, on) => setMethods((current) => (on ? [...current, id as MethodKind] : current.filter((m) => m !== id)))}
            empty={null}
            scroll={false}
          />
          <p className={errors.requirement ? "text-caption text-danger" : "text-caption text-fg-3"}>{errors.requirement ?? t.anyChecked}</p>
        </div>
        <DurationField label={t.maxAge} hint={t.maxAgeHint} value={age} onChange={setAge} error={errors.age} />
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}
