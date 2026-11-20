"use client";

import { VenetianMask } from "lucide-react";
import { useId, useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Field, Textarea } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import { formatTime } from "@/lib/format";
import type { Impersonation } from "@/lib/types";

async function start(id: string) {
  const { redirect } = await api<{ redirect: string }>(`/impersonations/${id}/start`, { method: "POST" });
  window.location.assign(redirect);
  return new Promise<never>(() => {});
}

export function ImpersonateDialog({ user, open, onOpenChange }: { user: { id: string; name: string }; open: boolean; onOpenChange: (open: boolean) => void }) {
  const { pending, run, errorFor, clearError } = useMutation();
  const formId = useId();
  const [reason, setReason] = useState("");

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        clearError();
        onOpenChange(next);
      }}
      error={errorFor("impersonate")}
      title={`Impersonate ${user.name}?`}
      description={`See Halo as ${user.name} for 30 minutes. You can look but not change anything or open applications. Starting signs you out of the console, emails ${user.name}, and records both of your names in the audit and sign-in logs.`}
      footer={
        <>
          <Button variant="secondary" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button variant="primary" type="submit" form={formId} loading={pending === "impersonate"}>
            <VenetianMask aria-hidden="true" size={16} strokeWidth={1.75} />
            Start impersonation
          </Button>
        </>
      }
    >
      <form
        id={formId}
        noValidate
        onSubmit={(event) => {
          event.preventDefault();
          void run("impersonate", async () => start((await api<Impersonation>(`/users/${user.id}/impersonations`, { body: { reason } })).id), undefined, "inline");
        }}
      >
        <Field label="Reason" hint={`For example a ticket number and what fails. ${user.name} reads it in the email.`}>
          {(props) => <Textarea {...props} rows={3} maxLength={500} value={reason} onChange={(event) => setReason(event.target.value)} />}
        </Field>
      </form>
    </Dialog>
  );
}

export function ImpersonationNotices({ impersonations, meId }: { impersonations: Impersonation[]; meId: string }) {
  const { pending, run, toast } = useMutation();
  const open = impersonations.filter((i) => i.status === "active" || (i.status === "ready" && i.actor.id === meId));
  if (open.length === 0) return null;

  return (
    <ul className="flex flex-col gap-3">
      {open.map((i) => (
        <li key={i.id} className="flex flex-col gap-3 rounded-lg border border-warning/20 bg-warning-container p-4 sm:flex-row sm:items-center sm:px-6">
          <VenetianMask aria-hidden="true" size={20} strokeWidth={1.75} className="shrink-0 text-warning" />
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <p className="text-body-sm font-semibold text-fg">
              {i.status === "active" ? `${i.actor.name} is impersonating ${i.user.name} until ${formatTime(i.expiresAt)}` : `Approved: impersonate ${i.user.name} before ${formatTime(i.expiresAt)}`}
            </p>
            <p className="truncate text-body-sm text-fg-2">{i.reason}</p>
          </div>
          <div className="flex shrink-0 gap-2">
            {i.status === "ready" ? (
              <Button size="sm" variant="primary" loading={pending === `start-${i.id}`} onClick={() => run(`start-${i.id}`, () => start(i.id))}>
                Start
              </Button>
            ) : null}
            <Button
              size="sm"
              variant="secondary"
              loading={pending === `end-${i.id}`}
              onClick={() =>
                run(
                  `end-${i.id}`,
                  () => api(`/impersonations/${i.id}/end`, { method: "POST" }),
                  () => toast({ title: i.status === "active" ? "Impersonation ended" : "Impersonation cancelled", description: "Recorded in the audit log." }),
                )
              }
            >
              {i.status === "active" ? "End impersonation" : "Cancel"}
            </Button>
          </div>
        </li>
      ))}
    </ul>
  );
}
