"use client";

import { Plus } from "lucide-react";
import { useId, useState, type FormEvent } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Field, Select, Textarea } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import type { AccessPackage, AccessRequest } from "@/lib/governance-types";
import { approversLabel, DURATIONS, durationLabel, joinNames } from "./shared";

function durationOptions(maxDays: number | null): string[] {
  if (maxDays === null) return [...DURATIONS.map(String), "none"];
  return [...new Set([...DURATIONS.filter((d) => d <= maxDays), maxDays])].sort((a, b) => a - b).map(String);
}

function defaultDuration(pkg: AccessPackage | undefined): string {
  if (!pkg) return "7";
  return pkg.maxDays === null ? "none" : String(Math.min(pkg.maxDays, 7));
}

export function RequestAccess({ packages, pendingIds }: { packages: AccessPackage[]; pendingIds: string[] }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant="primary" onClick={() => setOpen(true)}>
        <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
        Request access
      </Button>
      {open ? <RequestDialog packages={packages} pendingIds={pendingIds} onClose={() => setOpen(false)} /> : null}
    </>
  );
}

function RequestDialog({ packages, pendingIds, onClose }: { packages: AccessPackage[]; pendingIds: string[]; onClose: () => void }) {
  const formId = useId();
  const first = packages.find((p) => !pendingIds.includes(p.id));
  const [selected, setSelected] = useState(first?.id ?? "");
  const [duration, setDuration] = useState(defaultDuration(first));
  const [justification, setJustification] = useState("");
  const [error, setError] = useState<string>();
  const { pending, run, toast } = useMutation();
  const pkg = packages.find((p) => p.id === selected);

  function choose(next: AccessPackage) {
    setSelected(next.id);
    setDuration(defaultDuration(next));
    setError(undefined);
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    if (!pkg) return;
    if (pkg.requireJustification && !justification.trim()) {
      setError(`Add a reason so ${approversLabel(pkg.approvers)} can decide. One sentence is enough.`);
      return;
    }
    void run(
      "request",
      () =>
        api<AccessRequest>("/me/access-requests", {
          body: { packageId: pkg.id, durationDays: duration === "none" ? null : Number(duration), justification: justification.trim() },
        }),
      () => {
        onClose();
        toast({
          title: `Request sent to ${approversLabel(pkg.approvers)}`,
          description: `${pkg.name}, ${durationLabel(duration === "none" ? null : Number(duration)).toLowerCase()}. You'll get an email when they decide.`,
        });
      },
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(next) => (next ? undefined : onClose())}
      title="Request access"
      description="Choose what you need. An approver reads your reason and decides. Access is removed automatically when it ends."
      footer={
        <>
          <Button variant="secondary" disabled={pending === "request"} onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" type="submit" form={formId} loading={pending === "request"} disabled={!pkg}>
            Send request
          </Button>
        </>
      }
    >
      <form id={formId} noValidate onSubmit={submit} className="flex flex-col gap-6">
        <fieldset>
          <legend className="text-label text-fg">Access package</legend>
          <div className="mt-2 flex flex-col gap-2">
            {packages.map((option) => {
              const waiting = pendingIds.includes(option.id);
              return (
                <label
                  key={option.id}
                  className="flex cursor-pointer items-start gap-3 rounded-md border border-border p-4 transition-colors duration-fast ease-brand hover:border-border-strong has-checked:border-ember has-disabled:cursor-not-allowed has-disabled:opacity-45"
                >
                  <input
                    type="radio"
                    name="package"
                    value={option.id}
                    checked={selected === option.id}
                    disabled={waiting}
                    onChange={() => choose(option)}
                    className="mt-1 size-4 shrink-0 accent-ember"
                  />
                  <span className="flex min-w-0 flex-col gap-1">
                    <span className="text-body-sm font-semibold text-fg">{option.name}</span>
                    {option.description ? <span className="text-body-sm text-fg-3">{option.description}</span> : null}
                    <span className="text-caption text-fg-3">
                      {waiting
                        ? "You already asked for this. Waiting for a decision."
                        : `Grants ${joinNames(option.groups)} · ${option.maxDays === null ? "Until revoked" : `Up to ${durationLabel(option.maxDays).toLowerCase()}`} · Approved by ${approversLabel(option.approvers)}`}
                    </span>
                  </span>
                </label>
              );
            })}
          </div>
        </fieldset>
        {pkg ? (
          <>
            <Field
              label="Justification"
              hint={pkg.requireJustification ? "What you'll use it for. One or two sentences is enough." : "Optional. Helps the approver decide."}
              error={error}
            >
              {(field) => (
                <Textarea
                  {...field}
                  value={justification}
                  onChange={(event) => {
                    setJustification(event.target.value);
                    setError(undefined);
                  }}
                />
              )}
            </Field>
            <Field label="Duration" hint="Access is removed automatically when it ends.">
              {(field) => (
                <Select {...field} value={duration} onChange={(event) => setDuration(event.target.value)}>
                  {durationOptions(pkg.maxDays).map((value) => (
                    <option key={value} value={value}>
                      {durationLabel(value === "none" ? null : Number(value))}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
          </>
        ) : null}
      </form>
    </Dialog>
  );
}

export function CancelRequest({ request }: { request: AccessRequest }) {
  const { pending, run, toast } = useMutation();
  return (
    <Button
      size="sm"
      variant="quiet"
      loading={pending === "cancel"}
      onClick={() =>
        run(
          "cancel",
          () => api(`/me/access-requests/${request.id}/cancel`, { method: "POST" }),
          () => toast({ title: "Request cancelled", description: `Your request for ${request.package.name} was withdrawn. Approvers no longer see it.` }),
        )
      }
    >
      Cancel request
    </Button>
  );
}
