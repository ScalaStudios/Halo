"use client";

import { Check } from "lucide-react";
import { useMutation } from "@/components/console/use-mutation";
import { Checkbox } from "@/components/ui/input";
import { SectionTitle } from "@/components/ui/section-title";
import { Spinner } from "@/components/ui/spinner";
import { api } from "@/lib/api/client";

export type SecurityEmail = { kind: string; label: string; on: boolean; canTurnOff: boolean };

export function SecurityEmails({ emails, address }: { emails: SecurityEmail[]; address: string }) {
  const { pending, run, toast } = useMutation();
  const sorted = [...emails].sort((a, b) => Number(b.canTurnOff) - Number(a.canTurnOff));

  function toggle(email: SecurityEmail, on: boolean) {
    const off = emails.filter((e) => e.canTurnOff && (e.kind === email.kind ? !on : !e.on)).map((e) => e.kind);
    void run(
      email.kind,
      () => api<SecurityEmail[]>("/me/security-emails", { method: "PUT", body: { off } }),
      () =>
        toast({
          title: `${email.label} emails turned ${on ? "on" : "off"}`,
          description: on ? `Halo emails ${address} when your account signs in from a new device or network.` : "Halo still emails you about the other changes on this list.",
        }),
    );
  }

  return (
    <section className="flex flex-col gap-4">
      <SectionTitle
        title="Security emails"
        description={`Halo emails ${address} when something changes on your account, so you can act quickly if it wasn't you. They link to this portal and never contain a sign-in link or code.`}
      />
      {sorted.length > 0 ? (
        <ul className="overflow-hidden rounded-lg border border-border bg-surface">
          {sorted.map((email) => (
            <li key={email.kind} className="border-t border-border first:border-t-0">
              {email.canTurnOff ? (
                <label className="flex cursor-pointer items-center gap-3 p-4 sm:px-6">
                  <Checkbox checked={email.on} disabled={pending !== null} onChange={(event) => toggle(email, event.target.checked)} />
                  <span className="min-w-0 flex-1 text-body-sm text-fg">{email.label}</span>
                  {pending === email.kind ? <Spinner className="text-fg-3" /> : <span className="text-caption text-fg-3">{email.on ? "On" : "Off"}</span>}
                </label>
              ) : (
                <div className="flex items-center gap-3 p-4 sm:px-6">
                  <Check aria-hidden="true" size={16} strokeWidth={1.75} className="shrink-0 text-fg-3" />
                  <span className="min-w-0 flex-1 text-body-sm text-fg">{email.label}</span>
                  <span className="text-caption text-fg-3">Always sent</span>
                </div>
              )}
            </li>
          ))}
        </ul>
      ) : (
        <p className="rounded-lg border border-border px-6 py-4 text-body-sm text-fg-3">Your organization turned off security emails.</p>
      )}
    </section>
  );
}
