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
import { formatDuration } from "@/lib/format";
import { METHOD } from "@/lib/labels";
import type { AuthContext } from "@/lib/policy-types";
import type { MethodKind } from "@/lib/types";
import { CheckList } from "./policy-form";
import { POLICY_METHODS, joinList } from "./shared";

const IDENTIFIER = /^[A-Za-z0-9][A-Za-z0-9._:-]{0,99}$/;

export function ContextsSection({ contexts, usedBy }: { contexts: AuthContext[]; usedBy: Record<string, string[]> }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [editing, setEditing] = useState<AuthContext | "new" | null>(null);
  const [deleting, setDeleting] = useState<AuthContext | null>(null);

  function remove() {
    const item = deleting;
    if (!item) return;
    void run("delete", () => api(`/authentication-contexts/${encodeURIComponent(item.id)}`, { method: "DELETE" }), () => {
      setDeleting(null);
      toast({ title: `${item.name} deleted`, description: `Applications that send acr_values=${item.id} no longer get it. Recorded in the audit log.` });
    }, "inline");
  }

  return (
    <div className="flex flex-col gap-6">
      <SectionTitle
        title="Authentication contexts"
        description="Named sign-in requirements. Applications ask for one with acr_values, policies require one with the effect “Require an authentication context”, and the ID token names the one that was met in its acr claim."
        action={
          <Button size="sm" variant="primary" onClick={() => setEditing("new")}>
            <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
            Add context
          </Button>
        }
      />
      {contexts.length ? (
        <SimpleTable
          caption="Authentication contexts"
          head={["Name", "Identifier", "Sign-in methods", "Signed in within", "Used by", "Actions"]}
          rows={contexts.map((item) => [
            <span key="name" className="font-medium whitespace-nowrap text-fg">
              {item.name}
            </span>,
            <span key="id" className="font-mono text-code-sm text-fg-2">
              {item.id}
            </span>,
            <span key="methods">{item.methods.length ? joinList(item.methods.map((m) => METHOD[m].label)) : <span className="text-fg-3">Any method</span>}</span>,
            <span key="age" className="whitespace-nowrap">
              {item.maxAge ? formatDuration(item.maxAge) : <span className="text-fg-3">Any time</span>}
            </span>,
            <span key="used">{usedBy[item.id]?.length ? joinList(usedBy[item.id]!, "and") : <span className="text-fg-3">No policy</span>}</span>,
            <span key="actions" className="flex gap-1">
              <Button size="icon-sm" variant="quiet" aria-label={`Edit ${item.name}`} onClick={() => setEditing(item)}>
                <Pencil aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
              <Button size="icon-sm" variant="quiet" aria-label={`Delete ${item.name}`} onClick={() => setDeleting(item)}>
                <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
            </span>,
          ])}
        />
      ) : (
        <Card>
          <EmptyState
            icon={ShieldCheck}
            title="No authentication contexts yet"
            description="Add one, such as “passkey-recent” for a passkey sign-in in the last 5 minutes, so an application can ask for a stronger sign-in before a sensitive action."
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
        title={`Delete ${deleting?.name ?? "context"}?`}
        description="Halo ignores it in acr_values from then on. Halo refuses if a policy still requires it."
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(null)}>
              Cancel
            </Button>
            <Button variant="destructive" loading={pending === "delete"} onClick={remove}>
              Delete context
            </Button>
          </>
        }
      />
    </div>
  );
}

function ContextDialog({ item, onClose }: { item: AuthContext | null; onClose: () => void }) {
  const { pending, run, errorFor, toast } = useMutation();
  const [identifier, setIdentifier] = useState(item?.id ?? "");
  const [name, setName] = useState(item?.name ?? "");
  const [methods, setMethods] = useState<MethodKind[]>(item?.methods ?? []);
  const [age, setAge] = useState(split(item?.maxAge ?? 0));
  const [errors, setErrors] = useState<{ identifier?: string; name?: string; age?: string; requirement?: string }>({});

  function save() {
    const next = {
      identifier: item || IDENTIFIER.test(identifier.trim()) ? undefined : "Use up to 100 letters, digits, dots, colons, hyphens or underscores, such as passkey-recent.",
      name: name.trim() ? undefined : "Enter a name, for example “Passkey in the last 5 minutes”.",
      age: wholeNumber(age),
      requirement: methods.length || toSeconds(age) ? undefined : "Choose at least one sign-in method, a maximum sign-in age, or both.",
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
        toast({ title: item ? `${saved.name} saved` : `${saved.name} added`, description: `Applications ask for it with acr_values=${saved.id}.` });
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
      title={item ? `Edit ${item.name}` : "Add authentication context"}
      description={item ? `Identifier ${item.id}. Changes apply from the next sign-in.` : "A sign-in meets the context when it used one of the chosen methods and is no older than the maximum age."}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={save}>
            {item ? "Save context" : "Add context"}
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
          <Field label="Identifier" error={errors.identifier} hint="The value applications send in acr_values and read back in the acr claim. It can't be changed later.">
            {(props) => <Input {...props} mono autoFocus autoComplete="off" spellCheck={false} value={identifier} onChange={(event) => setIdentifier(event.target.value)} placeholder="passkey-recent" />}
          </Field>
        )}
        <Field label="Name" error={errors.name}>
          {(props) => <Input {...props} autoFocus={item !== null} autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder="Passkey in the last 5 minutes" />}
        </Field>
        <div className="flex flex-col gap-2">
          <CheckList
            legend="Sign-in methods"
            options={POLICY_METHODS.map((kind) => ({ id: kind, name: METHOD[kind].label }))}
            selected={methods}
            onToggle={(id, on) => setMethods((current) => (on ? [...current, id as MethodKind] : current.filter((m) => m !== id)))}
            empty={null}
            scroll={false}
          />
          <p className={errors.requirement ? "text-caption text-danger" : "text-caption text-fg-3"}>{errors.requirement ?? "None checked accepts every method."}</p>
        </div>
        <DurationField label="Maximum sign-in age" hint="1 minute to 90 days. Set 0 for no limit. Older sign-ins are asked to sign in again." value={age} onChange={setAge} error={errors.age} />
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}
