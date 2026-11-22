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
import { useMessages } from "@/lib/i18n/client";
import { policyMessages } from "@/lib/i18n/console/policy";
import type { Terms } from "@/lib/policy-types";
import { joinList } from "./shared";

export function TermsSection({ terms, usedBy }: { terms: Terms[]; usedBy: Record<string, string[]> }) {
  const copy = useMessages(policyMessages);
  const t = copy.terms;
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [editing, setEditing] = useState<Terms | "new" | null>(null);
  const [deleting, setDeleting] = useState<Terms | null>(null);

  function remove() {
    const item = deleting;
    if (!item) return;
    void run("delete", () => api(`/terms/${item.id}`, { method: "DELETE" }), () => {
      setDeleting(null);
      toast({ title: copy.deleted(item.name), description: t.deletedDescription });
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
      {terms.length ? (
        <SimpleTable
          caption={t.title}
          head={[copy.name, t.version, t.changed, copy.usedBy, copy.actions]}
          rows={terms.map((item) => [
            <span key="name" className="font-medium whitespace-nowrap text-fg">
              {item.name}
            </span>,
            <span key="version" className="tabular-nums">
              {item.version}
            </span>,
            <RelativeTime key="changed" iso={item.updatedAt} />,
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
            icon={FileCheck}
            title={t.empty}
            description={t.emptyDescription}
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

function TermsDialog({ item, onClose }: { item: Terms | null; onClose: () => void }) {
  const copy = useMessages(policyMessages);
  const t = copy.terms;
  const { pending, run, errorFor, toast } = useMutation();
  const [name, setName] = useState(item?.name ?? "");
  const [text, setText] = useState(item?.text ?? "");
  const [url, setUrl] = useState(item?.url ?? "");
  const [errors, setErrors] = useState<{ name?: string; content?: string }>({});
  const changed = item !== null && (text.trim() !== item.text || url.trim() !== item.url);

  function save() {
    const next = {
      name: name.trim() ? undefined : copy.enterName(t.nameExample),
      content: text.trim() || url.trim() ? undefined : t.contentRequired,
    };
    setErrors(next);
    if (next.name || next.content) return;
    const body = { name: name.trim(), text: text.trim(), url: url.trim() };
    void run(
      "save",
      () => (item ? api<Terms>(`/terms/${item.id}`, { method: "PUT", body }) : api<Terms>("/terms", { body })),
      (saved) => {
        toast({
          title: item ? copy.saved(saved.name) : copy.added(saved.name),
          description: item && saved.version > item.version ? t.published(saved.version) : t.versionOnly(saved.version),
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
      title={item ? copy.edit(item.name) : t.addTitle}
      description={item ? t.editDescription(item.version) : t.addDescription}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {copy.cancel}
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={save}>
            {item ? (changed ? t.publish(item.version + 1) : t.save) : t.add}
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
        <Field label={copy.name} error={errors.name}>
          {(props) => <Input {...props} autoFocus autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder={t.nameExample} />}
        </Field>
        <Field label={t.text} error={errors.content} hint={t.textHint}>
          {(props) => <Textarea {...props} rows={8} value={text} onChange={(event) => setText(event.target.value)} />}
        </Field>
        <Field label={t.link} hint={t.linkHint}>
          {(props) => <Input {...props} type="url" autoComplete="off" value={url} onChange={(event) => setUrl(event.target.value)} placeholder="https://example.com/terms" />}
        </Field>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}
