"use client";

import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { PeoplePicker } from "@/components/governance/people-picker";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import { formatDate, isoDay } from "@/lib/format";
import type { User } from "@/lib/types";

export type GuestTerms = { sponsorId: string; ends: string };
type GuestErrors = { sponsor?: string; ends?: string };

export function checkGuest(terms: GuestTerms, withSponsor: boolean): GuestErrors {
  return {
    sponsor: !withSponsor || terms.sponsorId ? undefined : "Choose the person responsible for this guest.",
    ends: terms.ends >= isoDay(0) && terms.ends <= isoDay(364) ? undefined : "Choose a day within the next year.",
  };
}

export function guestEnd(day: string) {
  return new Date(`${day}T23:59:59Z`).toISOString();
}

export function GuestFields({ value, onChange, directory, errors }: { value: GuestTerms; onChange: (value: GuestTerms) => void; directory?: User[]; errors: GuestErrors }) {
  const sponsors = directory?.filter((u) => !u.guest && u.status === "active");
  return (
    <>
      {sponsors ? (
        <Field label="Sponsor" error={errors.sponsor} hint="The person at your organization who is responsible for this guest. They're emailed two weeks before access ends and can extend it.">
          {(props) => <PeoplePicker {...props} people={sponsors} value={value.sponsorId ? [value.sponsorId] : []} onChange={(ids) => onChange({ ...value, sponsorId: ids.at(-1) ?? "" })} />}
        </Field>
      ) : null}
      <Field label="Access ends" error={errors.ends} hint="Halo suspends the account at the end of this day, UTC, unless the sponsor extends it. Up to a year ahead.">
        {(props) => <Input {...props} type="date" min={isoDay(0)} max={isoDay(364)} value={value.ends} onChange={(event) => onChange({ ...value, ends: event.target.value })} />}
      </Field>
    </>
  );
}

export function GuestAccessButton({ guest, directory }: { guest: Pick<User, "id" | "name" | "sponsor" | "expiresAt">; directory?: User[] }) {
  const current = { sponsorId: guest.sponsor?.id ?? "", ends: guest.expiresAt && guest.expiresAt.slice(0, 10) >= isoDay(0) ? guest.expiresAt.slice(0, 10) : isoDay(90) };
  const [open, setOpen] = useState(false);
  const [terms, setTerms] = useState<GuestTerms>(current);
  const [errors, setErrors] = useState<GuestErrors>({});
  const { pending, run, errorFor, clearError, toast } = useMutation();

  function save() {
    const next = checkGuest(terms, Boolean(directory));
    setErrors(next);
    if (next.sponsor || next.ends) return;
    const body = directory && terms.sponsorId !== guest.sponsor?.id ? { expiresAt: guestEnd(terms.ends), sponsorId: terms.sponsorId } : { expiresAt: guestEnd(terms.ends) };
    void run(
      "guest",
      () => api(`/users/${guest.id}/guest`, { method: "PUT", body }),
      () => {
        setOpen(false);
        toast({ title: "Guest access saved", description: `${guest.name} keeps access until ${formatDate(guestEnd(terms.ends))}. Recorded in the audit log.` });
      },
      "inline",
    );
  }

  return (
    <>
      <Button
        size="sm"
        variant={directory ? "quiet" : "secondary"}
        onClick={() => {
          setTerms(current);
          setErrors({});
          clearError();
          setOpen(true);
        }}
      >
        {directory ? "Edit" : "Extend"}
      </Button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        error={errorFor("guest")}
        title={directory ? `Guest access for ${guest.name}` : `Extend ${guest.name}'s access`}
        description="Choose when their access ends. Halo emails the sponsor again two weeks before the new date."
        footer={
          <>
            <Button variant="secondary" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button variant="primary" loading={pending === "guest"} onClick={save}>
              Save
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
          <GuestFields value={terms} onChange={setTerms} directory={directory?.filter((u) => u.id !== guest.id)} errors={errors} />
          <button type="submit" hidden />
        </form>
      </Dialog>
    </>
  );
}
