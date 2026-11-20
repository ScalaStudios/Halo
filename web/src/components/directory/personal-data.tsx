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

type Person = { id: string; name: string; email: string };

export function PersonalData({ user, self }: { user: Person; self: boolean }) {
  const [deleting, setDeleting] = useState(false);

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <CardHeader title="Personal data" description="For access and portability requests under data protection law such as the GDPR." />
        <Row
          title="Export personal data"
          description={`Downloads everything Halo stores about ${user.name} as JSON: profile, sign-in methods without their secrets, groups, sessions, devices, sign-ins, audit entries about them, consents and access requests.`}
          action={<ExportButton path={`/users/${user.id}/export`} label="Export" variant="secondary" size="sm" />}
        />
      </Card>
      {self ? null : (
        <div id="danger-zone" className="scroll-mt-24">
          <Card>
            <CardHeader title="Danger zone" />
            <Row
              title="Delete permanently"
              description={`Removes ${user.name} with their sign-in methods, sessions, devices and group memberships, and replaces their name and email address in the logs with a pseudonym. This cannot be undone.`}
              action={
                <Button size="sm" variant="destructive" onClick={() => setDeleting(true)}>
                  <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
                  Delete permanently
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
      title={`Delete ${user.name} permanently?`}
      description="Halo removes their account, sign-in methods, sessions, tokens, devices, group memberships and consents, and replaces their name and email address in the sign-in and audit logs with a pseudonym. You can't undo this. Export their data first if you need a copy."
      footer={
        <>
          <Button variant="secondary" onClick={close}>
            Cancel
          </Button>
          <Button variant="destructive" type="submit" form={formId} disabled={!confirmed} loading={pending === "delete"}>
            Delete permanently
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
              toast({ title: `${user.name} deleted`, description: `The logs now show them as ${pseudonym}. Recorded in the audit log.` });
              router.push("/admin/users");
            },
            "inline",
          );
        }}
      >
        <Field label={`Type ${user.email} to confirm`}>
          {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={typed} onChange={(event) => setTyped(event.target.value)} />}
        </Field>
      </form>
    </Dialog>
  );
}
