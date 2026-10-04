"use client";

import { useId, useState, type FormEvent } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { PeoplePicker } from "@/components/governance/people-picker";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import type { User } from "@/lib/types";

export type ProfileAccess = "all" | "org";
export type Org = { title: string; department: string; location: string; managerId: string };
export type ProfileEdit = { user: User; directory: User[]; access: ProfileAccess };

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
  const people = directory.filter((u) => u.id !== self && u.status !== "deprovisioned");
  const manager = directory.find((u) => u.id === value.managerId);
  const field = (key: "title" | "department" | "location") => (props: { id: string }) => (
    <Input {...props} list={`${id}-${key}`} autoComplete="off" className="[&::-webkit-calendar-picker-indicator]:hidden!" value={value[key]} onChange={(event) => onChange({ ...value, [key]: event.target.value })} />
  );

  return (
    <>
      <Field label="Title">{field("title")}</Field>
      <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
        <Field label="Department" hint="Rule-based groups match this exactly.">
          {field("department")}
        </Field>
        <Field label="Location">{field("location")}</Field>
      </div>
      <Field label="Manager" hint={managerLocked ? undefined : "Optional. Search for the person they report to."}>
        {(props) =>
          managerLocked ? (
            <Input {...props} disabled value={manager?.name ?? "None"} />
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

export function EditProfileDialog({ user, directory, access, open, onOpenChange }: ProfileEdit & { open: boolean; onOpenChange: (open: boolean) => void }) {
  const formId = useId();
  const { pending, run, toast } = useMutation();

  function save(body: Record<string, string>) {
    void run(
      "profile",
      () => api(`/users/${user.id}`, { method: "PATCH", body }),
      () => {
        onOpenChange(false);
        toast({ title: "Profile saved", description: `${user.name}'s profile is updated. Recorded in the audit log.` });
      },
    );
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title="Edit profile"
      description={
        access === "org"
          ? "Helpdesk administrators can change title, department and location. Ask a user administrator to change the name, email or manager."
          : "Rule-based group membership follows title, department and location, so changes here can add or remove access."
      }
      footer={
        <>
          <Button variant="secondary" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button type="submit" form={formId} variant="primary" loading={pending === "profile"}>
            Save profile
          </Button>
        </>
      }
    >
      <ProfileForm id={formId} user={user} directory={directory} access={access} onSave={save} />
    </Dialog>
  );
}

function ProfileForm({ id, user, directory, access, onSave }: ProfileEdit & { id: string; onSave: (body: Record<string, string>) => void }) {
  const [name, setName] = useState(user.name);
  const [email, setEmail] = useState(user.email);
  const [org, setOrg] = useState<Org>({ title: user.title, department: user.department, location: user.location, managerId: user.managerId ?? "" });
  const [errors, setErrors] = useState<{ name?: string; email?: string }>({});
  const locked = access === "org";

  function submit(event: FormEvent) {
    event.preventDefault();
    const next = {
      name: name.trim() ? undefined : "Enter their name so other administrators can recognise them.",
      email: EMAIL.test(email.trim()) ? undefined : "Enter a full email address, like sam@example.com.",
    };
    setErrors(next);
    if (next.name || next.email) return;
    const { managerId, ...profile } = org;
    onSave(locked ? profile : { name: name.trim(), email: email.trim(), managerId, ...profile });
  }

  return (
    <form id={id} className="flex flex-col gap-6" onSubmit={submit} noValidate>
      <Field label="Name" error={errors.name}>
        {(props) => <Input {...props} autoComplete="off" disabled={locked} value={name} onChange={(event) => setName(event.target.value)} />}
      </Field>
      <Field label="Email" error={errors.email}>
        {(props) => <Input {...props} type="email" autoComplete="off" disabled={locked} value={email} onChange={(event) => setEmail(event.target.value)} />}
      </Field>
      <OrgFields value={org} onChange={setOrg} directory={directory} self={user.id} managerLocked={locked} />
    </form>
  );
}

export function EditProfileButton(props: ProfileEdit) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button size="sm" variant="quiet" onClick={() => setOpen(true)}>
        Edit
      </Button>
      <EditProfileDialog {...props} open={open} onOpenChange={setOpen} />
    </>
  );
}
