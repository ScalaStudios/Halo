"use client";

import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { PeoplePicker } from "@/components/governance/people-picker";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import { formatDate, isoDay } from "@/lib/format";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { directoryMessages } from "@/lib/i18n/console/directory";
import type { Locale } from "@/lib/i18n/locale";
import type { User } from "@/lib/types";

export type GuestTerms = { sponsorId: string; ends: string };
type GuestErrors = { sponsor?: string; ends?: string };

export function checkGuest(terms: GuestTerms, withSponsor: boolean, locale: Locale): GuestErrors {
  const t = directoryMessages[locale].guestAccess;
  return {
    sponsor: !withSponsor || terms.sponsorId ? undefined : t.sponsorRequired,
    ends: terms.ends >= isoDay(0) && terms.ends <= isoDay(364) ? undefined : t.endsInvalid,
  };
}

export function guestEnd(day: string) {
  return new Date(`${day}T23:59:59Z`).toISOString();
}

export function GuestFields({ value, onChange, directory, errors }: { value: GuestTerms; onChange: (value: GuestTerms) => void; directory?: User[]; errors: GuestErrors }) {
  const sponsors = directory?.filter((u) => !u.guest && u.status === "active");
  const t = useMessages(directoryMessages).guestAccess;
  return (
    <>
      {sponsors ? (
        <Field label={t.sponsor} error={errors.sponsor} hint={t.sponsorHint}>
          {(props) => <PeoplePicker {...props} people={sponsors} value={value.sponsorId ? [value.sponsorId] : []} onChange={(ids) => onChange({ ...value, sponsorId: ids.at(-1) ?? "" })} />}
        </Field>
      ) : null}
      <Field label={t.accessEnds} error={errors.ends} hint={t.accessEndsHint}>
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
  const locale = useLocale();
  const t = directoryMessages[locale];

  function save() {
    const next = checkGuest(terms, Boolean(directory), locale);
    setErrors(next);
    if (next.sponsor || next.ends) return;
    const body = directory && terms.sponsorId !== guest.sponsor?.id ? { expiresAt: guestEnd(terms.ends), sponsorId: terms.sponsorId } : { expiresAt: guestEnd(terms.ends) };
    void run(
      "guest",
      () => api(`/users/${guest.id}/guest`, { method: "PUT", body }),
      () => {
        setOpen(false);
        toast({ title: t.guestAccess.saved, description: t.guestAccess.savedHint(guest.name, formatDate(guestEnd(terms.ends), locale)) });
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
        {directory ? t.edit : t.guestAccess.extend}
      </Button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        error={errorFor("guest")}
        title={directory ? t.guestAccess.editTitle(guest.name) : t.guestAccess.extendTitle(guest.name)}
        description={t.guestAccess.description}
        footer={
          <>
            <Button variant="secondary" onClick={() => setOpen(false)}>
              {t.cancel}
            </Button>
            <Button variant="primary" loading={pending === "guest"} onClick={save}>
              {t.save}
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
