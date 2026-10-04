"use client";

import { useState } from "react";
import { SecretWarning } from "@/components/applications/shared";
import { useMutation } from "@/components/console/use-mutation";
import { Button } from "@/components/ui/button";
import { CopyField } from "@/components/ui/copy-field";
import { Dialog } from "@/components/ui/dialog";
import { Checkbox, Field, Input } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import { formatDateTime } from "@/lib/format";
import type { SetupLink } from "@/lib/mail-types";
import type { Group, User } from "@/lib/types";
import { EMAIL, OrgFields, type Org } from "./edit-profile";

const NO_ORG: Org = { title: "", department: "", location: "", managerId: "" };

export function InviteDialog({
  open,
  onOpenChange,
  groups,
  directory,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  groups: Group[];
  directory: User[];
}) {
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [org, setOrg] = useState<Org>(NO_ORG);
  const [selected, setSelected] = useState<string[]>([]);
  const [errors, setErrors] = useState<{ email?: string; name?: string }>({});
  const [invitation, setInvitation] = useState<SetupLink | null>(null);
  const { pending, run } = useMutation();

  function reset() {
    setEmail("");
    setName("");
    setOrg(NO_ORG);
    setSelected([]);
    setErrors({});
    setInvitation(null);
  }

  function close() {
    onOpenChange(false);
    reset();
  }

  function submit() {
    const next = {
      email: EMAIL.test(email.trim()) ? undefined : "Enter a full email address, like sam@example.com.",
      name: name.trim() ? undefined : "Enter their name so other administrators can recognise them.",
    };
    setErrors(next);
    if (next.email || next.name) return;
    void run("invite", () => api<SetupLink>("/users", { body: { email: email.trim(), name: name.trim(), ...org, groupIds: selected } }), setInvitation);
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => (next ? onOpenChange(true) : close())}
      title={invitation ? "Invitation created" : "Invite user"}
      description={
        invitation
          ? `${invitation.emailed ? `We emailed a setup link to ${email.trim()}` : `Send this setup link to ${name.trim()}`}. It expires ${formatDateTime(invitation.expiresAt)}.`
          : "Halo creates a setup link they use to enroll a passkey. No password is ever created."
      }
      footer={
        invitation ? (
          <Button variant="primary" onClick={close}>
            Done
          </Button>
        ) : (
          <>
            <Button variant="secondary" onClick={close}>
              Cancel
            </Button>
            <Button variant="primary" loading={pending === "invite"} onClick={submit}>
              Create invitation
            </Button>
          </>
        )
      }
    >
      {invitation ? (
        <div className="flex animate-enter flex-col gap-4">
          <CopyField label="Setup link" value={invitation.enrollUrl} />
          <SecretWarning>
            {invitation.emailed ? "You can also copy the link and send it yourself." : "Halo didn't email this link, so send it yourself."} Anyone with it can enroll a passkey on
            this account.
          </SecretWarning>
        </div>
      ) : (
        <form
          className="flex flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <Field label="Email" error={errors.email}>
            {(props) => (
              <Input {...props} type="email" autoComplete="off" autoFocus value={email} onChange={(event) => setEmail(event.target.value)} placeholder="sam@example.com" />
            )}
          </Field>
          <Field label="Name" error={errors.name}>
            {(props) => <Input {...props} autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} />}
          </Field>
          <OrgFields value={org} onChange={setOrg} directory={directory} />
          {groups.length ? (
            <fieldset className="flex flex-col gap-3">
              <legend className="mb-3 text-label text-fg">Groups</legend>
              {groups.map((group) => (
                <label key={group.id} className="flex cursor-pointer items-start gap-3">
                  <Checkbox
                    className="mt-0.5"
                    checked={selected.includes(group.id)}
                    onChange={(event) =>
                      setSelected((current) => (event.target.checked ? [...current, group.id] : current.filter((id) => id !== group.id)))
                    }
                  />
                  <span className="flex flex-col">
                    <span className="text-body-sm text-fg">{group.name}</span>
                    <span className="text-caption text-fg-3">{group.description}</span>
                  </span>
                </label>
              ))}
            </fieldset>
          ) : null}
          <button type="submit" hidden />
        </form>
      )}
    </Dialog>
  );
}
