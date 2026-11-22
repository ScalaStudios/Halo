"use client";

import { Plus } from "lucide-react";
import { useId, useState, type FormEvent } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ROLES } from "@/components/provisioning/shared";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input, Select, Textarea } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import { formatDateTime } from "@/lib/format";
import type { AccessPackage, AccessRequest, NamedRef } from "@/lib/governance-types";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { governanceMessages } from "@/lib/i18n/console/governance";
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
  const t = useMessages(governanceMessages);
  return (
    <>
      <Button variant="primary" onClick={() => setOpen(true)}>
        <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
        {t.request.requestAccess}
      </Button>
      {open ? <RequestDialog packages={packages} pendingIds={pendingIds} reports={reports} onClose={() => setOpen(false)} /> : null}
    </>
  );
}

export function ExtendAccess({ grant, pkg }: { grant: AccessRequest; pkg: AccessPackage }) {
  const [open, setOpen] = useState(false);
  const t = useMessages(governanceMessages);
  return (
    <>
      <Button size="sm" variant="secondary" onClick={() => setOpen(true)}>
        {t.request.requestExtension}
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
  const locale = useLocale();
  const t = governanceMessages[locale];
  const copy = t.request;
  const manager = beneficiary ? t.managerOf(beneficiary.name) : t.yourManager;

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
    if (pkg.requireJustification && !justification.trim()) next.justification = copy.justificationRequired;
    for (const q of pkg.questions) if (q.required && !answers[q.id]?.trim()) next[q.id] = copy.answerRequired(pkg.name);
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
          title: extending ? copy.extensionRequested : copy.sentTo(stagesLabel(pkg.stages.slice(0, 1), locale, manager)),
          description: beneficiary
            ? copy.forBeneficiary(pkg.name, beneficiary.name)
            : copy.forSelf(pkg.name, extending && days !== null ? t.moreDays(days) : durationLabel(days, locale).toLowerCase()),
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
      title={extending ? copy.extendTitle(extending.package.name) : copy.requestAccess}
      description={extending?.expiresAt ? copy.extendDescription(formatDateTime(extending.expiresAt, locale)) : copy.description}
      footer={
        <>
          <Button variant="secondary" disabled={pending === "request"} onClick={onClose}>
            {t.cancel}
          </Button>
          <Button variant="primary" type="submit" form={formId} loading={pending === "request"} disabled={!pkg}>
            {extending ? copy.requestExtension : copy.send}
          </Button>
        </>
      }
    >
      <form id={formId} noValidate onSubmit={submit} className="flex flex-col gap-6">
        {reports.length ? (
          <Field label={copy.requestFor} hint={beneficiary ? copy.beneficiaryHint(beneficiary.name) : copy.onBehalfHint}>
            {(field) => (
              <Select {...field} value={beneficiaryId} onChange={(event) => setBeneficiaryId(event.target.value)}>
                <option value="">{copy.myself}</option>
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
          <legend className="text-label text-fg">{copy.accessPackage}</legend>
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
                        ? copy.alreadyAsked
                        : `${t.requests.grants(joinNames(option.groups, locale))} · ${option.maxDays === null ? t.untilRevoked : copy.upTo(durationLabel(option.maxDays, locale).toLowerCase())} · ${copy.approvedBy(stagesLabel(option.stages, locale, manager))}`}
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
              label={t.justification}
              hint={pkg.requireJustification ? copy.justificationHint : copy.justificationOptional}
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
              <Field key={q.id} label={q.text} hint={q.required ? undefined : t.optional} error={errors[q.id]}>
                {(field) => {
                  const change = (value: string) => {
                    setAnswers((current) => ({ ...current, [q.id]: value }));
                    setErrors(({ [q.id]: _, ...rest }) => rest);
                  };
                  return q.kind === "choice" ? (
                    <Select {...field} value={answers[q.id] ?? ""} onChange={(event) => change(event.target.value)}>
                      <option value="">{copy.chooseAnswer}</option>
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
            <Field label={extending ? copy.extraTime : t.duration} hint={extending ? copy.extraTimeHint : copy.durationHint}>
              {(field) => (
                <Select {...field} value={duration} onChange={(event) => setDuration(event.target.value)}>
                  {durationOptions(pkg.maxDays).map((value) => (
                    <option key={value} value={value}>
                      {durationLabel(value === "none" ? null : Number(value), locale)}
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
  const t = useMessages(governanceMessages).request;
  return (
    <Button
      size="sm"
      variant="quiet"
      loading={pending === "cancel"}
      onClick={() =>
        run(
          "cancel",
          () => api(`/me/access-requests/${request.id}/cancel`, { method: "POST" }),
          () => toast({ title: t.cancelled, description: t.cancelledHint(request.package.name) }),
        )
      }
    >
      {t.cancelRequest}
    </Button>
  );
}

const ACTIVATION_HOURS = [1, 2, 3, 4, 5, 6, 7, 8];

export function ActivateRole({ role }: { role: RoleKey }) {
  const [open, setOpen] = useState(false);
  const t = useMessages(governanceMessages);
  return (
    <>
      <Button size="sm" variant="secondary" onClick={() => setOpen(true)}>
        {t.request.activate}
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
  const locale = useLocale();
  const t = governanceMessages[locale];
  const copy = t.request;
  const label = ROLES[role].label;

  function submit(event: FormEvent) {
    event.preventDefault();
    if (!justification.trim()) {
      setError(copy.activationReasonRequired);
      return;
    }
    void run(
      "activate",
      () => api<RoleActivation>("/me/role-activations", { body: { role, durationHours: Number(hours), justification: justification.trim() } }),
      (activation) => {
        onClose();
        toast({ title: copy.active(label), description: copy.activeHint(formatDateTime(activation.expiresAt, locale)) });
      },
      "inline",
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(next) => (next ? undefined : onClose())}
      error={errorFor("activate")}
      title={copy.activateTitle(label)}
      description={copy.activateDescription}
      footer={
        <>
          <Button variant="secondary" disabled={pending === "activate"} onClick={onClose}>
            {t.cancel}
          </Button>
          <Button variant="primary" type="submit" form={formId} loading={pending === "activate"}>
            {copy.activateRole}
          </Button>
        </>
      }
    >
      <form id={formId} noValidate onSubmit={submit} className="flex flex-col gap-6">
        <Field label={t.justification} hint={copy.activationHint} error={error}>
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
        <Field label={t.duration} hint={copy.activationDurationHint}>
          {(field) => (
            <Select {...field} value={hours} onChange={(event) => setHours(event.target.value)}>
              {ACTIVATION_HOURS.map((value) => (
                <option key={value} value={value}>
                  {t.hours(value)}
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
  const t = useMessages(governanceMessages).request;
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
          () => toast({ title: t.deactivated(label), description: t.deactivatedHint }),
        )
      }
    >
      {t.deactivate}
    </Button>
  );
}
