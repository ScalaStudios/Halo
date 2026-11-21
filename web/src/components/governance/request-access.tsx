"use client";

import { Plus } from "lucide-react";
import { useId, useState, type FormEvent } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ROLES } from "@/components/provisioning/shared";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input, Select, Textarea } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import { formatDateTime, pluralize } from "@/lib/format";
import type { AccessPackage, AccessRequest, NamedRef } from "@/lib/governance-types";
import type { RoleKey } from "@/lib/provisioning-types";
import type { RoleActivation } from "@/lib/types";
import { DURATIONS, durationLabel, joinNames, stagesLabel } from "./shared";

function durationOptions(maxDays: number | null): string[] {
  if (maxDays === null) return [...DURATIONS.map(String), "none"];
  return [...new Set([...DURATIONS.filter((d) => d <= maxDays), maxDays])].sort((a, b) => a - b).map(String);
}

function defaultDuration(pkg: AccessPackage | undefined): string {
  if (!pkg) return "7";
  return pkg.maxDays === null ? "none" : String(Math.min(pkg.maxDays, 7));
}

export function RequestAccess({ packages, pendingIds, reports }: { packages: AccessPackage[]; pendingIds: string[]; reports: NamedRef[] }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant="primary" onClick={() => setOpen(true)}>
        <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
        Request access
      </Button>
      {open ? <RequestDialog packages={packages} pendingIds={pendingIds} reports={reports} onClose={() => setOpen(false)} /> : null}
    </>
  );
}

export function ExtendAccess({ grant, pkg }: { grant: AccessRequest; pkg: AccessPackage }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button size="sm" variant="secondary" onClick={() => setOpen(true)}>
        Request extension
      </Button>
      {open ? <RequestDialog packages={[pkg]} pendingIds={[]} reports={[]} extending={grant} onClose={() => setOpen(false)} /> : null}
    </>
  );
}

function RequestDialog({
  packages,
  pendingIds,
  reports,
  extending,
  onClose,
}: {
  packages: AccessPackage[];
  pendingIds: string[];
  reports: NamedRef[];
  extending?: AccessRequest;
  onClose: () => void;
}) {
  const formId = useId();
  const [beneficiaryId, setBeneficiaryId] = useState("");
  const blocked = beneficiaryId ? [] : pendingIds;
  const first = packages.find((p) => !blocked.includes(p.id));
  const [selected, setSelected] = useState(first?.id ?? "");
  const [duration, setDuration] = useState(defaultDuration(first));
  const [justification, setJustification] = useState("");
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [errors, setErrors] = useState<Record<string, string>>({});
  const { pending, run, errorFor, toast } = useMutation();
  const pkg = packages.find((p) => p.id === selected);
  const beneficiary = reports.find((r) => r.id === beneficiaryId);
  const whose = beneficiary ? `${beneficiary.name}'s` : "your";

  function choose(next: AccessPackage) {
    setSelected(next.id);
    setDuration(defaultDuration(next));
    setAnswers({});
    setErrors({});
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    if (!pkg) return;
    const next: Record<string, string> = {};
    if (pkg.requireJustification && !justification.trim()) next.justification = "Add a reason so the approvers can decide. One sentence is enough.";
    for (const q of pkg.questions) if (q.required && !answers[q.id]?.trim()) next[q.id] = `Answer this. ${pkg.name} asks it of everyone who requests it.`;
    setErrors(next);
    if (Object.keys(next).length) return;
    const days = duration === "none" ? null : Number(duration);
    void run(
      "request",
      () =>
        api<AccessRequest>("/me/access-requests", {
          body: {
            packageId: pkg.id,
            ...(beneficiary ? { beneficiaryId: beneficiary.id } : {}),
            ...(extending ? { extendsId: extending.id } : {}),
            durationDays: days,
            justification: justification.trim(),
            answers,
          },
        }),
      () => {
        onClose();
        toast({
          title: extending ? "Extension requested" : `Request sent to ${stagesLabel(pkg.stages.slice(0, 1), whose)}`,
          description: beneficiary
            ? `${pkg.name} for ${beneficiary.name}. They get an email now and when it's decided.`
            : `${pkg.name}, ${extending && days !== null ? pluralize(days, "more day") : durationLabel(days).toLowerCase()}. You'll get an email when it's decided.`,
        });
      },
      "inline",
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(next) => (next ? undefined : onClose())}
      error={errorFor("request")}
      title={extending ? `Extend ${extending.package.name}` : "Request access"}
      description={
        extending?.expiresAt
          ? `Ask for more time before your access ends ${formatDateTime(extending.expiresAt)}. The request goes through the package's approval again, and approved time is added to the end date.`
          : "Choose what you need. Approvers read your reason and decide. Access is removed automatically when it ends."
      }
      footer={
        <>
          <Button variant="secondary" disabled={pending === "request"} onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" type="submit" form={formId} loading={pending === "request"} disabled={!pkg}>
            {extending ? "Request extension" : "Send request"}
          </Button>
        </>
      }
    >
      <form id={formId} noValidate onSubmit={submit} className="flex flex-col gap-6">
        {reports.length ? (
          <Field label="Request for" hint={beneficiary ? `${beneficiary.name} gets an email and sees the request in their account.` : "You can also ask on behalf of people who report to you."}>
            {(field) => (
              <Select {...field} value={beneficiaryId} onChange={(event) => setBeneficiaryId(event.target.value)}>
                <option value="">Myself</option>
                {reports.map((report) => (
                  <option key={report.id} value={report.id}>
                    {report.name}
                  </option>
                ))}
              </Select>
            )}
          </Field>
        ) : null}
        <fieldset hidden={Boolean(extending)}>
          <legend className="text-label text-fg">Access package</legend>
          <div className="mt-2 flex flex-col gap-2">
            {packages.map((option) => {
              const waiting = blocked.includes(option.id);
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
                        : `Grants ${joinNames(option.groups)} · ${option.maxDays === null ? "Until revoked" : `Up to ${durationLabel(option.maxDays).toLowerCase()}`} · Approved by ${stagesLabel(option.stages, whose)}`}
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
              hint={pkg.requireJustification ? "What it's for. One or two sentences is enough." : "Optional. Helps the approvers decide."}
              error={errors.justification}
            >
              {(field) => (
                <Textarea
                  {...field}
                  value={justification}
                  onChange={(event) => {
                    setJustification(event.target.value);
                    setErrors(({ justification: _, ...rest }) => rest);
                  }}
                />
              )}
            </Field>
            {pkg.questions.map((q) => (
              <Field key={q.id} label={q.text} hint={q.required ? undefined : "Optional."} error={errors[q.id]}>
                {(field) => {
                  const change = (value: string) => {
                    setAnswers((current) => ({ ...current, [q.id]: value }));
                    setErrors(({ [q.id]: _, ...rest }) => rest);
                  };
                  return q.kind === "choice" ? (
                    <Select {...field} value={answers[q.id] ?? ""} onChange={(event) => change(event.target.value)}>
                      <option value="">Choose an answer</option>
                      {q.choices.map((choice) => (
                        <option key={choice} value={choice}>
                          {choice}
                        </option>
                      ))}
                    </Select>
                  ) : (
                    <Input {...field} autoComplete="off" value={answers[q.id] ?? ""} onChange={(event) => change(event.target.value)} />
                  );
                }}
              </Field>
            ))}
            <Field label={extending ? "Extra time" : "Duration"} hint={extending ? "Added to the current end date." : "Access is removed automatically when it ends."}>
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

const ACTIVATION_HOURS = [1, 2, 3, 4, 5, 6, 7, 8];

export function ActivateRole({ role }: { role: RoleKey }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button size="sm" variant="secondary" onClick={() => setOpen(true)}>
        Activate
      </Button>
      {open ? <ActivateDialog role={role} onClose={() => setOpen(false)} /> : null}
    </>
  );
}

function ActivateDialog({ role, onClose }: { role: RoleKey; onClose: () => void }) {
  const formId = useId();
  const [hours, setHours] = useState("1");
  const [justification, setJustification] = useState("");
  const [error, setError] = useState<string>();
  const { pending, run, errorFor, toast } = useMutation();
  const label = ROLES[role].label;

  function submit(event: FormEvent) {
    event.preventDefault();
    if (!justification.trim()) {
      setError("Add a reason. It's recorded in the audit log with the activation.");
      return;
    }
    void run(
      "activate",
      () => api<RoleActivation>("/me/role-activations", { body: { role, durationHours: Number(hours), justification: justification.trim() } }),
      (activation) => {
        onClose();
        toast({ title: `${label} is active`, description: `Until ${formatDateTime(activation.expiresAt)}. Halo removes the role then.` });
      },
      "inline",
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(next) => (next ? undefined : onClose())}
      error={errorFor("activate")}
      title={`Activate ${label.toLowerCase()}`}
      description="You get the role on your next request. Halo removes it when the time ends, and records the activation and your reason in the audit log."
      footer={
        <>
          <Button variant="secondary" disabled={pending === "activate"} onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" type="submit" form={formId} loading={pending === "activate"}>
            Activate role
          </Button>
        </>
      }
    >
      <form id={formId} noValidate onSubmit={submit} className="flex flex-col gap-6">
        <Field label="Justification" hint="What you'll do with the role. One or two sentences is enough." error={error}>
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
        <Field label="Duration" hint="You can deactivate the role sooner when you're done.">
          {(field) => (
            <Select {...field} value={hours} onChange={(event) => setHours(event.target.value)}>
              {ACTIVATION_HOURS.map((value) => (
                <option key={value} value={value}>
                  {pluralize(value, "hour")}
                </option>
              ))}
            </Select>
          )}
        </Field>
      </form>
    </Dialog>
  );
}

export function DeactivateRole({ activation }: { activation: RoleActivation }) {
  const { pending, run, toast } = useMutation();
  const label = ROLES[activation.role].label;
  return (
    <Button
      size="sm"
      variant="quiet"
      loading={pending === "deactivate"}
      onClick={() =>
        run(
          "deactivate",
          () => api(`/me/role-activations/${activation.id}/deactivate`, { method: "POST" }),
          () => toast({ title: `${label} deactivated`, description: "You no longer hold the role. Recorded in the audit log." }),
        )
      }
    >
      Deactivate
    </Button>
  );
}
