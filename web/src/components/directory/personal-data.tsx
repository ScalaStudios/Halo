"use client";

import { Trash2 } from "lucide-react";
import { useId, useState, type ReactNode } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ExportButton } from "@/components/settings/export-button";
import { Button } from "@/components/ui/button";
import { Card, CardHeader } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import { useMessages } from "@/lib/i18n/client";
import { directoryMessages } from "@/lib/i18n/console/directory";

type Person = { id: string; name: string; email: string };

export function PersonalData({ user, self }: { user: Person; self: boolean }) {
  const [deleting, setDeleting] = useState(false);
  const m = useMessages(directoryMessages);
  const t = m.personalData;

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <CardHeader title={t.title} description={t.description} />
        <Row
          title={t.export}
          description={t.exportDescription(user.name)}
          action={<ExportButton path={`/users/${user.id}/export`} label={t.exportButton} variant="secondary" size="sm" />}
        />
      </Card>
      {self ? null : (
        <div id="danger-zone" className="scroll-mt-24">
          <Card>
            <CardHeader title={m.dangerZone} />
            <Row
              title={t.deletePermanently}
              description={t.deleteDescription(user.name)}
              action={
                <Button size="sm" variant="destructive" onClick={() => setDeleting(true)}>
                  <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
                  {t.deletePermanently}
                </Button>
              }
            />
          </Card>
          <DeleteDialog user={user} open={deleting} onClose={() => setDeleting(false)} />
        </div>
      )}
    </div>
  );
}

function Row({ title, description, action }: { title: string; description: string; action: ReactNode }) {
  return (
    <div className="flex flex-col gap-4 border-t border-border px-6 py-4 sm:flex-row sm:items-center sm:justify-between">
      <div className="flex flex-col gap-1">
        <p className="text-body-sm font-semibold text-fg">{title}</p>
        <p className="text-body-sm text-fg-3">{description}</p>
      </div>
      <div className="flex shrink-0 flex-col items-start self-start sm:items-end sm:self-center">{action}</div>
    </div>
  );
}

function DeleteDialog({ user, open, onClose }: { user: Person; open: boolean; onClose: () => void }) {
  const { pending, run, errorFor, clearError, toast, router } = useMutation();
  const formId = useId();
  const [typed, setTyped] = useState("");
  const m = useMessages(directoryMessages);
  const t = m.personalData;
  const confirmed = typed.trim().toLowerCase() === user.email.toLowerCase();

  function close() {
    setTyped("");
    clearError();
    onClose();
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => (next ? undefined : close())}
      error={errorFor("delete")}
      title={t.deleteTitle(user.name)}
      description={t.deleteDialogDescription}
      footer={
        <>
          <Button variant="secondary" onClick={close}>
            {m.cancel}
          </Button>
          <Button variant="destructive" type="submit" form={formId} disabled={!confirmed} loading={pending === "delete"}>
            {t.deletePermanently}
          </Button>
        </>
      }
    >
      <form
        id={formId}
        noValidate
        onSubmit={(event) => {
          event.preventDefault();
          if (!confirmed) return;
          void run(
            "delete",
            () => api<{ pseudonym: string }>(`/users/${user.id}`, { method: "DELETE" }),
            ({ pseudonym }) => {
              toast({ title: t.deleted(user.name), description: t.deletedHint(pseudonym) });
              router.push("/admin/users");
            },
            "inline",
          );
        }}
      >
        <Field label={t.confirm(user.email)}>
          {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={typed} onChange={(event) => setTyped(event.target.value)} />}
        </Field>
      </form>
    </Dialog>
  );
}
