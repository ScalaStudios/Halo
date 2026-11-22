"use client";

import { Ban, Copy, MonitorX, MoreHorizontal, RectangleEllipsis, RotateCcw, TicketCheck, Undo2, UserPen, UsersRound, VenetianMask } from "lucide-react";
import { useId, useState } from "react";
import { SecretWarning } from "@/components/applications/shared";
import { useMutation } from "@/components/console/use-mutation";
import { Button } from "@/components/ui/button";
import { CopyButton } from "@/components/ui/copy-button";
import { CopyField } from "@/components/ui/copy-field";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input, Select } from "@/components/ui/input";
import { Menu, MenuContent, MenuItem, MenuSeparator, MenuTrigger } from "@/components/ui/menu";
import { api } from "@/lib/api/client";
import { formatDateTime } from "@/lib/format";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { directoryMessages } from "@/lib/i18n/console/directory";
import type { SetupLink } from "@/lib/mail-types";
import { EditProfileDialog, type ProfileEdit } from "./edit-profile";
import { ImpersonateDialog } from "./impersonation";

type UserRef = { id: string; name: string };

const PASSWORD_ALPHABET = "abcdefghijkmnpqrstuvwxyz23456789";

function randomPassword() {
  const chars = Array.from(crypto.getRandomValues(new Uint8Array(20)), (byte) => PASSWORD_ALPHABET[byte % PASSWORD_ALPHABET.length]);
  return [0, 5, 10, 15].map((start) => chars.slice(start, start + 5).join("")).join("-");
}

export function UserActions({ id, name, suspended, edit, impersonate }: { id: string; name: string; suspended: boolean; edit?: ProfileEdit; impersonate?: boolean }) {
  const { pending, run, toast, router } = useMutation();
  const [editing, setEditing] = useState(false);
  const [impersonating, setImpersonating] = useState(false);
  const [resetting, setResetting] = useState(false);
  const [suspending, setSuspending] = useState(false);
  const t = useMessages(directoryMessages);
  const user = { id, name };

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button
        size="sm"
        variant="secondary"
        loading={pending === "revoke"}
        onClick={() =>
          run(
            "revoke",
            () => api<{ revoked: number }>(`/users/${id}/revoke-sessions`, { method: "POST" }),
            ({ revoked }) => toast({ title: t.sessionsRevoked, description: t.sessionsRevokedHint(revoked, name, 1) }),
          )
        }
      >
        <MonitorX aria-hidden="true" size={16} strokeWidth={1.75} />
        {t.revokeSessions}
      </Button>
      <Button size="sm" variant="secondary" onClick={() => setResetting(true)}>
        <RotateCcw aria-hidden="true" size={16} strokeWidth={1.75} />
        {t.resetAuthentication}
      </Button>
      <Menu>
        <MenuTrigger asChild>
          <Button size="icon-sm" variant="quiet" aria-label={t.actions.moreActions} loading={pending === "restore"}>
            <MoreHorizontal aria-hidden="true" size={16} strokeWidth={1.75} />
          </Button>
        </MenuTrigger>
        <MenuContent>
          {edit ? (
            <MenuItem icon={UserPen} onSelect={() => setEditing(true)}>
              {t.actions.editProfile}
            </MenuItem>
          ) : null}
          <MenuItem icon={UsersRound} onSelect={() => router.push(`/admin/users/${id}?tab=groups`, { scroll: false })}>
            {t.actions.modifyGroups}
          </MenuItem>
          {impersonate ? (
            <MenuItem icon={VenetianMask} onSelect={() => setImpersonating(true)}>
              {t.actions.impersonate}
            </MenuItem>
          ) : null}
          <MenuItem
            icon={Copy}
            onSelect={() => {
              void navigator.clipboard?.writeText(id);
              toast({ title: t.userIdCopied, description: id });
            }}
          >
            {t.copyUserId}
          </MenuItem>
          <MenuSeparator />
          {suspended ? (
            <MenuItem
              icon={Undo2}
              onSelect={() =>
                run(
                  "restore",
                  () => api(`/users/${id}/restore`, { method: "POST" }),
                  () => toast({ title: t.actions.restored(name), description: t.actions.restoredHint }),
                )
              }
            >
              {t.actions.restoreUser}
            </MenuItem>
          ) : (
            <MenuItem icon={Ban} danger onSelect={() => setSuspending(true)}>
              {t.actions.suspendUser}
            </MenuItem>
          )}
        </MenuContent>
      </Menu>
      <ResetAuthenticationDialog user={resetting ? user : null} onClose={() => setResetting(false)} />
      <SuspendDialog users={suspending ? [user] : []} onClose={() => setSuspending(false)} />
      {edit ? <EditProfileDialog {...edit} open={editing} onOpenChange={setEditing} /> : null}
      {impersonate ? <ImpersonateDialog user={user} open={impersonating} onOpenChange={setImpersonating} /> : null}
    </div>
  );
}

export function ResetAuthenticationDialog({ user, onClose }: { user: UserRef | null; onClose: () => void }) {
  const { pending, run, errorFor, clearError } = useMutation();
  const locale = useLocale();
  const t = directoryMessages[locale];
  const [link, setLink] = useState<SetupLink | null>(null);

  function close() {
    onClose();
    setLink(null);
    clearError();
  }

  return (
    <Dialog
      open={user !== null}
      onOpenChange={(next) => (next ? undefined : close())}
      error={errorFor("reset")}
      title={link ? t.actions.newSetupLink : t.actions.resetTitle(user?.name ?? "")}
      description={link ? t.actions.linkDescription(user?.name, link.emailed, formatDateTime(link.expiresAt, locale)) : t.actions.resetDescription}
      footer={
        link ? (
          <Button variant="primary" onClick={close}>
            {t.done}
          </Button>
        ) : (
          <>
            <Button variant="secondary" onClick={close}>
              {t.cancel}
            </Button>
            <Button
              variant="destructive"
              loading={pending === "reset"}
              onClick={() => user && run("reset", () => api<SetupLink>(`/users/${user.id}/reset-authentication`, { method: "POST" }), setLink, "inline")}
            >
              {t.resetAuthentication}
            </Button>
          </>
        )
      }
    >
      {link ? (
        <div className="flex animate-enter flex-col gap-4">
          <CopyField label={t.setupLink} value={link.enrollUrl} />
          <SecretWarning>{t.actions.linkWarning(user?.name, link.emailed)}</SecretWarning>
        </div>
      ) : null}
    </Dialog>
  );
}

export function TemporaryPasswordButton({ user, warning }: { user: UserRef; warning?: string }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const t = useMessages(directoryMessages);
  const formId = useId();
  const [open, setOpen] = useState(false);
  const [password, setPassword] = useState("");

  return (
    <>
      <Button
        size="sm"
        onClick={() => {
          setPassword(randomPassword());
          clearError();
          setOpen(true);
        }}
      >
        <RectangleEllipsis aria-hidden="true" size={16} strokeWidth={1.75} />
        {t.actions.setTemporaryPassword}
      </Button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        error={errorFor("password")}
        title={t.actions.temporaryPasswordTitle}
        description={t.actions.temporaryPasswordDescription(user.name)}
        footer={
          <>
            <Button variant="secondary" onClick={() => setOpen(false)}>
              {t.cancel}
            </Button>
            <Button variant="primary" type="submit" form={formId} loading={pending === "password"}>
              {t.actions.setTemporaryPassword}
            </Button>
          </>
        }
      >
        <form
          id={formId}
          noValidate
          className="flex flex-col gap-4"
          onSubmit={(event) => {
            event.preventDefault();
            void run("password", () => api(`/users/${user.id}/password`, { method: "PUT", body: { password } }), () => {
              setOpen(false);
              toast({ title: t.actions.temporaryPasswordSet, description: t.actions.temporaryPasswordSetHint(user.name) });
            }, "inline");
          }}
        >
          <Field label={t.actions.temporaryPassword} hint={t.actions.temporaryPasswordHint}>
            {(props) => (
              <Input
                {...props}
                mono
                autoComplete="off"
                spellCheck={false}
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                trailing={<CopyButton value={password} label={t.actions.copyPassword} className="[&>span]:sr-only" />}
              />
            )}
          </Field>
          {warning ? <SecretWarning>{warning}</SecretWarning> : null}
        </form>
      </Dialog>
    </>
  );
}

type AccessPass = { pass: string; expiresAt: string; uses: number };

export function AccessPassButton({ user }: { user: UserRef }) {
  const { pending, run, errorFor, clearError } = useMutation();
  const locale = useLocale();
  const t = directoryMessages[locale];
  const formId = useId();
  const [open, setOpen] = useState(false);
  const [lifetime, setLifetime] = useState(60);
  const [uses, setUses] = useState(1);
  const [created, setCreated] = useState<AccessPass | null>(null);

  function close() {
    setOpen(false);
    setCreated(null);
  }

  return (
    <>
      <Button
        size="sm"
        onClick={() => {
          clearError();
          setOpen(true);
        }}
      >
        <TicketCheck aria-hidden="true" size={16} strokeWidth={1.75} />
        {t.actions.createAccessPass}
      </Button>
      <Dialog
        open={open}
        onOpenChange={close}
        error={errorFor("pass")}
        title={created ? t.actions.accessPass : t.actions.createAccessPassTitle}
        description={created ? t.actions.accessPassCreated(user.name, created.uses, formatDateTime(created.expiresAt, locale)) : t.actions.accessPassDescription(user.name)}
        footer={
          created ? (
            <Button variant="primary" onClick={close}>
              {t.done}
            </Button>
          ) : (
            <>
              <Button variant="secondary" onClick={close}>
                {t.cancel}
              </Button>
              <Button variant="primary" type="submit" form={formId} loading={pending === "pass"}>
                {t.actions.createPass}
              </Button>
            </>
          )
        }
      >
        {created ? (
          <div className="flex animate-enter flex-col gap-4">
            <CopyField label={t.actions.accessPass} value={created.pass} />
            <SecretWarning>{t.actions.accessPassWarning(user.name)}</SecretWarning>
          </div>
        ) : (
          <form
            id={formId}
            noValidate
            className="flex flex-col gap-4 sm:flex-row"
            onSubmit={(event) => {
              event.preventDefault();
              void run("pass", () => api<AccessPass>(`/users/${user.id}/access-pass`, { body: { lifetimeMinutes: lifetime, uses } }), setCreated, "inline");
            }}
          >
            <Field label={t.actions.validFor} className="flex-1">
              {(props) => (
                <Select {...props} value={lifetime} onChange={(event) => setLifetime(Number(event.target.value))}>
                  {[60, 480, 1440, 10080].map((minutes) => (
                    <option key={minutes} value={minutes}>
                      {t.actions.duration(minutes)}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
            <Field label={t.actions.uses} className="flex-1">
              {(props) => (
                <Select {...props} value={uses} onChange={(event) => setUses(Number(event.target.value))}>
                  {[1, 3, 5, 10].map((count) => (
                    <option key={count} value={count}>
                      {t.actions.usesOption(count)}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
          </form>
        )}
      </Dialog>
    </>
  );
}

export function SuspendDialog({ users, onClose }: { users: UserRef[]; onClose: () => void }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const t = useMessages(directoryMessages);
  const single = users.length === 1 ? users[0] : undefined;

  function close() {
    clearError();
    onClose();
  }

  return (
    <Dialog
      open={users.length > 0}
      onOpenChange={(next) => (next ? undefined : close())}
      error={errorFor("suspend")}
      title={t.actions.suspendTitle(single?.name, users.length)}
      description={t.actions.suspendDescription}
      footer={
        <>
          <Button variant="secondary" onClick={close}>
            {t.cancel}
          </Button>
          <Button
            variant="destructive"
            loading={pending === "suspend"}
            onClick={() =>
              run(
                "suspend",
                () => Promise.all(users.map((u) => api(`/users/${u.id}/suspend`, { method: "POST" }))),
                () => {
                  onClose();
                  toast({ title: t.actions.suspended(single?.name, users.length), description: t.recorded });
                },
                "inline",
              )
            }
          >
            {t.suspend}
          </Button>
        </>
      }
    />
  );
}
