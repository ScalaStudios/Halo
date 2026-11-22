"use client";

import { useId, useState, type FormEvent } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { PeoplePicker } from "@/components/governance/people-picker";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input } from "@/components/ui/input";
import { AttributeFields, draftsOf, parseDrafts } from "@/components/directory/attribute-fields";
import { api } from "@/lib/api/client";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { directoryMessages } from "@/lib/i18n/console/directory";
import type { Attribute, User } from "@/lib/types";

export type ProfileAccess = "all" | "org";
export type Org = { title: string; department: string; location: string; managerId: string };
export type ProfileEdit = { user: User; directory: User[]; access: ProfileAccess; attributes: Attribute[] };

export const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export function OrgFields({
  value,
  onChange,
  directory,
  self,
  managerLocked = false,
}: {
  value: Org;
  onChange: (value: Org) => void;
  directory: User[];
  self?: string;
  managerLocked?: boolean;
}) {
  const id = useId();
  const t = useMessages(directoryMessages);
  const people = directory.filter((u) => u.id !== self && u.status !== "deprovisioned");
  const manager = directory.find((u) => u.id === value.managerId);
  const field = (key: "title" | "department" | "location") => (props: { id: string }) => (
    <Input {...props} list={`${id}-${key}`} autoComplete="off" className="[&::-webkit-calendar-picker-indicator]:hidden!" value={value[key]} onChange={(event) => onChange({ ...value, [key]: event.target.value })} />
  );

  return (
    <>
      <Field label={t.fields.title}>{field("title")}</Field>
      <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
        <Field label={t.fields.department} hint={t.profile.departmentHint}>
          {field("department")}
        </Field>
        <Field label={t.fields.location}>{field("location")}</Field>
      </div>
      <Field label={t.fields.manager} hint={managerLocked ? undefined : t.profile.managerHint}>
        {(props) =>
          managerLocked ? (
            <Input {...props} disabled value={manager?.name ?? t.none} />
          ) : (
            <PeoplePicker {...props} people={people} value={value.managerId ? [value.managerId] : []} onChange={(ids) => onChange({ ...value, managerId: ids.at(-1) ?? "" })} />
          )
        }
      </Field>
      {(["title", "department", "location"] as const).map((key) => (
        <datalist key={key} id={`${id}-${key}`}>
          {[...new Set(directory.map((u) => u[key]).filter(Boolean))].sort().map((option) => (
            <option key={option} value={option} />
          ))}
        </datalist>
      ))}
    </>
  );
}

export function EditProfileDialog({ user, directory, access, attributes, open, onOpenChange }: ProfileEdit & { open: boolean; onOpenChange: (open: boolean) => void }) {
  const formId = useId();
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const t = useMessages(directoryMessages);

  function save(body: Record<string, unknown>) {
    void run(
      "profile",
      () => api(`/users/${user.id}`, { method: "PATCH", body }),
      () => {
        onOpenChange(false);
        toast({ title: t.profile.saved, description: t.profile.savedHint(user.name) });
      },
      "inline",
    );
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        clearError();
        onOpenChange(next);
      }}
      error={errorFor("profile")}
      title={t.actions.editProfile}
      description={access === "org" ? t.profile.helpdeskDescription : t.profile.description}
      footer={
        <>
          <Button variant="secondary" onClick={() => onOpenChange(false)}>
            {t.cancel}
          </Button>
          <Button type="submit" form={formId} variant="primary" loading={pending === "profile"}>
            {t.profile.save}
          </Button>
        </>
      }
    >
      <ProfileForm id={formId} user={user} directory={directory} access={access} attributes={attributes} onSave={save} />
    </Dialog>
  );
}

function ProfileForm({ id, user, directory, access, attributes, onSave }: ProfileEdit & { id: string; onSave: (body: Record<string, unknown>) => void }) {
  const [name, setName] = useState(user.name);
  const [email, setEmail] = useState(user.email);
  const [org, setOrg] = useState<Org>({ title: user.title, department: user.department, location: user.location, managerId: user.managerId ?? "" });
  const [errors, setErrors] = useState<{ name?: string; email?: string }>({});
  const [drafts, setDrafts] = useState(() => draftsOf(attributes, user.attributes));
  const [attributeErrors, setAttributeErrors] = useState<Record<string, string>>({});
  const locked = access === "org";
  const locale = useLocale();
  const t = directoryMessages[locale];

  function submit(event: FormEvent) {
    event.preventDefault();
    const next = {
      name: name.trim() ? undefined : t.validation.name,
      email: EMAIL.test(email.trim()) ? undefined : t.validation.email,
    };
    const parsed = parseDrafts(attributes, drafts, locale);
    setErrors(next);
    setAttributeErrors(parsed.errors);
    if (next.name || next.email || Object.keys(parsed.errors).length > 0) return;
    const { managerId, ...profile } = org;
    const extra = attributes.length > 0 ? { attributes: parsed.values } : {};
    onSave(locked ? { ...profile, ...extra } : { name: name.trim(), email: email.trim(), managerId, ...profile, ...extra });
  }

  return (
    <form id={id} className="flex flex-col gap-6" onSubmit={submit} noValidate>
      <Field label={t.fields.name} error={errors.name}>
        {(props) => <Input {...props} autoComplete="off" disabled={locked} value={name} onChange={(event) => setName(event.target.value)} />}
      </Field>
      <Field label={t.fields.email} error={errors.email}>
        {(props) => <Input {...props} type="email" autoComplete="off" disabled={locked} value={email} onChange={(event) => setEmail(event.target.value)} />}
      </Field>
      <OrgFields value={org} onChange={setOrg} directory={directory} self={user.id} managerLocked={locked} />
      <AttributeFields attributes={attributes} drafts={drafts} errors={attributeErrors} onChange={setDrafts} />
    </form>
  );
}

export function EditProfileButton(props: ProfileEdit) {
  const [open, setOpen] = useState(false);
  const t = useMessages(directoryMessages);
  return (
    <>
      <Button size="sm" variant="quiet" onClick={() => setOpen(true)}>
        {t.edit}
      </Button>
      <EditProfileDialog {...props} open={open} onOpenChange={setOpen} />
    </>
  );
}
