"use client";

import { useState } from "react";
import { SecretWarning } from "@/components/applications/shared";
import { useMutation } from "@/components/console/use-mutation";
import { rolesFor } from "@/components/provisioning/shared";
import { ROLE_ORDER } from "@/components/settings/roles";
import { Button } from "@/components/ui/button";
import { CopyField } from "@/components/ui/copy-field";
import { Dialog } from "@/components/ui/dialog";
import { Checkbox, Field, Input, Select } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import { formatDateTime } from "@/lib/format";
import { useLocale } from "@/lib/i18n/client";
import { directoryMessages } from "@/lib/i18n/console/directory";
import type { SetupLink } from "@/lib/mail-types";
import type { RoleKey } from "@/lib/provisioning-types";
import type { Group, User } from "@/lib/types";
import { EMAIL, OrgFields, type Org } from "./edit-profile";
import { PasskeyWarning } from "./edit-roles";
import { checkGuest, GuestFields, guestEnd, type GuestTerms } from "./guest-access";

const NO_ORG: Org = { title: "", department: "", location: "", managerId: "" };
const NO_GUEST: GuestTerms = { sponsorId: "", ends: "" };

export function InviteDialog({
  open,
  onOpenChange,
  groups,
  directory,
  canGrantRoles,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  groups: Group[];
  directory: User[];
  canGrantRoles: boolean;
}) {
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [org, setOrg] = useState<Org>(NO_ORG);
  const [selected, setSelected] = useState<string[]>([]);
  const [role, setRole] = useState<RoleKey | "">("");
  const [guest, setGuest] = useState<GuestTerms | null>(null);
  const [errors, setErrors] = useState<{ email?: string; name?: string; sponsor?: string; ends?: string }>({});
  const [invitation, setInvitation] = useState<SetupLink | null>(null);
  const { pending, run, errorFor, clearError } = useMutation();
  const locale = useLocale();
  const t = directoryMessages[locale];
  const roleLabels = rolesFor(locale);

  function reset() {
    setEmail("");
    setName("");
    setOrg(NO_ORG);
    setSelected([]);
    setRole("");
    setGuest(null);
    setErrors({});
    setInvitation(null);
  }

  function close() {
    onOpenChange(false);
    reset();
    clearError();
  }

  function submit() {
    const next = {
      email: EMAIL.test(email.trim()) ? undefined : t.validation.email,
      name: name.trim() ? undefined : t.validation.name,
      ...(guest ? checkGuest(guest, true, locale) : {}),
    };
    setErrors(next);
    if (Object.values(next).some(Boolean)) return;
    const terms = guest ? { guest: true, sponsorId: guest.sponsorId, expiresAt: guestEnd(guest.ends) } : { roles: role ? [role] : [] };
    void run("invite", () => api<SetupLink>("/users", { body: { email: email.trim(), name: name.trim(), ...org, groupIds: selected, ...terms } }), setInvitation, "inline");
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => (next ? onOpenChange(true) : close())}
      error={errorFor("invite")}
      title={invitation ? t.invite.created : t.users.inviteUser}
      description={invitation ? t.invite.linkDescription(invitation.emailed, email.trim(), name.trim(), formatDateTime(invitation.expiresAt, locale)) : t.invite.description}
      footer={
        invitation ? (
          <Button variant="primary" onClick={close}>
            {t.done}
          </Button>
        ) : (
          <>
            <Button variant="secondary" onClick={close}>
              {t.cancel}
            </Button>
            <Button variant="primary" loading={pending === "invite"} onClick={submit}>
              {t.invite.create}
            </Button>
          </>
        )
      }
    >
      {invitation ? (
        <div className="flex animate-enter flex-col gap-4">
          <CopyField label={t.setupLink} value={invitation.enrollUrl} />
          <SecretWarning>{t.invite.linkWarning(invitation.emailed)}</SecretWarning>
        </div>
      ) : (
        <form
          className="flex flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <Field label={t.fields.email} error={errors.email}>
            {(props) => (
              <Input {...props} type="email" autoComplete="off" autoFocus value={email} onChange={(event) => setEmail(event.target.value)} placeholder="sam@example.com" />
            )}
          </Field>
          <Field label={t.fields.name} error={errors.name}>
            {(props) => <Input {...props} autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} />}
          </Field>
          <Field label={t.invite.accountType} hint={guest ? t.invite.guestHint : t.invite.memberHint}>
            {(props) => (
              <Select
                {...props}
                value={guest ? "guest" : "member"}
                onChange={(event) => {
                  setGuest(event.target.value === "guest" ? NO_GUEST : null);
                  setRole("");
                }}
              >
                <option value="member">{t.member}</option>
                <option value="guest">{t.guest}</option>
              </Select>
            )}
          </Field>
          {guest ? <GuestFields value={guest} onChange={setGuest} directory={directory} errors={errors} /> : null}
          <OrgFields value={org} onChange={setOrg} directory={directory} />
          {canGrantRoles && !guest ? (
            <Field label={t.fields.adminRole} hint={role ? roleLabels[role].description : t.invite.roleHint}>
              {(props) => (
                <Select {...props} value={role} onChange={(event) => setRole(event.target.value as RoleKey | "")}>
                  <option value="">{t.users.noAdminRole}</option>
                  {ROLE_ORDER.map((key) => (
                    <option key={key} value={key}>
                      {roleLabels[key].label}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
          ) : null}
          {role ? <PasskeyWarning role={roleLabels[role].label} /> : null}
          {groups.length ? (
            <fieldset className="flex flex-col gap-3">
              <legend className="mb-3 text-label text-fg">{t.fields.groups}</legend>
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
