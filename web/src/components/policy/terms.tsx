"use client";

import { FileCheck, Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Field, Input, Textarea } from "@/components/ui/input";
import { RelativeTime } from "@/components/ui/relative-time";
import { SectionTitle } from "@/components/ui/section-title";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import type { Terms } from "@/lib/policy-types";
import { joinList } from "./shared";

export function TermsSection({ terms, usedBy }: { terms: Terms[]; usedBy: Record<string, string[]> }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [editing, setEditing] = useState<Terms | "new" | null>(null);
  const [deleting, setDeleting] = useState<Terms | null>(null);

  function remove() {
    const item = deleting;
    if (!item) return;
    void run("delete", () => api(`/terms/${item.id}`, { method: "DELETE" }), () => {
      setDeleting(null);
      toast({ title: `${item.name} deleted`, description: "Past acceptances stay in the organization export." });
    }, "inline");
  }

  return (
    <div className="flex flex-col gap-6">
      <SectionTitle
        title="Terms of use"
        description="Documents people accept before they reach an application. Ask for them with a policy whose effect is “Require accepting terms of use”."
        action={
          <Button size="sm" variant="primary" onClick={() => setEditing("new")}>
            <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
            Add terms
          </Button>
        }
      />
      {terms.length ? (
        <SimpleTable
          caption="Terms of use"
          head={["Name", "Version", "Changed", "Used by", "Actions"]}
          rows={terms.map((item) => [
            <span key="name" className="font-medium whitespace-nowrap text-fg">
              {item.name}
            </span>,
            <span key="version" className="tabular-nums">
              {item.version}
            </span>,
            <RelativeTime key="changed" iso={item.updatedAt} />,
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
            icon={FileCheck}
            title="No terms of use yet"
            description="Add an acceptable use policy or terms of use, then require it with a policy. Halo records who accepted which version and when."
          />
        </Card>
      )}
      {editing ? <TermsDialog key={editing === "new" ? "new" : editing.id} item={editing === "new" ? null : editing} onClose={() => setEditing(null)} /> : null}
      <Dialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (open) return;
          setDeleting(null);
          clearError();
        }}
        error={errorFor("delete")}
        title={`Delete ${deleting?.name ?? "terms"}?`}
        description="Policies can no longer ask for them. Halo refuses if a policy still uses them. Past acceptances stay in the organization export."
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(null)}>
              Cancel
            </Button>
            <Button variant="destructive" loading={pending === "delete"} onClick={remove}>
              Delete terms
            </Button>
          </>
        }
      />
    </div>
  );
}

function TermsDialog({ item, onClose }: { item: Terms | null; onClose: () => void }) {
  const { pending, run, errorFor, toast } = useMutation();
  const [name, setName] = useState(item?.name ?? "");
  const [text, setText] = useState(item?.text ?? "");
  const [url, setUrl] = useState(item?.url ?? "");
  const [errors, setErrors] = useState<{ name?: string; content?: string }>({});
  const changed = item !== null && (text.trim() !== item.text || url.trim() !== item.url);

  function save() {
    const next = {
      name: name.trim() ? undefined : "Enter a name, for example “Acceptable use policy”.",
      content: text.trim() || url.trim() ? undefined : "Add the text of the terms, a link to them, or both.",
    };
    setErrors(next);
    if (next.name || next.content) return;
    const body = { name: name.trim(), text: text.trim(), url: url.trim() };
    void run(
      "save",
      () => (item ? api<Terms>(`/terms/${item.id}`, { method: "PUT", body }) : api<Terms>("/terms", { body })),
      (saved) => {
        toast({
          title: item ? `${saved.name} saved` : `${saved.name} added`,
          description: item && saved.version > item.version ? `Version ${saved.version} published. People accept it at their next sign-in.` : `Version ${saved.version}.`,
        });
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
      title={item ? `Edit ${item.name}` : "Add terms of use"}
      description={item ? `Version ${item.version}. Changing the text or link publishes version ${item.version + 1}.` : "Starts at version 1."}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={save}>
            {item ? (changed ? `Publish version ${item.version + 1}` : "Save terms") : "Add terms"}
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
        <Field label="Name" error={errors.name}>
          {(props) => <Input {...props} autoFocus autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder="Acceptable use policy" />}
        </Field>
        <Field label="Text" error={errors.content} hint="Plain text. People read it before they accept.">
          {(props) => <Textarea {...props} rows={8} value={text} onChange={(event) => setText(event.target.value)} />}
        </Field>
        <Field label="Link" hint="Optional with text. A web address of the full document, such as https://example.com/terms.">
          {(props) => <Input {...props} type="url" autoComplete="off" value={url} onChange={(event) => setUrl(event.target.value)} placeholder="https://example.com/terms" />}
        </Field>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}
