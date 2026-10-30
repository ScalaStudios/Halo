"use client";

import { Ban, Copy, MonitorX, MoreHorizontal, RectangleEllipsis, RotateCcw, Undo2, UserPen, UsersRound } from "lucide-react";
import { useId, useState } from "react";
import { SecretWarning } from "@/components/applications/shared";
import { useMutation } from "@/components/console/use-mutation";
import { Button } from "@/components/ui/button";
import { CopyButton } from "@/components/ui/copy-button";
import { CopyField } from "@/components/ui/copy-field";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input } from "@/components/ui/input";
import { Menu, MenuContent, MenuItem, MenuSeparator, MenuTrigger } from "@/components/ui/menu";
import { api } from "@/lib/api/client";
import { formatDateTime, pluralize } from "@/lib/format";
import type { SetupLink } from "@/lib/mail-types";
import { EditProfileDialog, type ProfileEdit } from "./edit-profile";

type UserRef = { id: string; name: string };

const PASSWORD_ALPHABET = "abcdefghijkmnpqrstuvwxyz23456789";

function randomPassword() {
  const chars = Array.from(crypto.getRandomValues(new Uint8Array(20)), (byte) => PASSWORD_ALPHABET[byte % PASSWORD_ALPHABET.length]);
  return [0, 5, 10, 15].map((start) => chars.slice(start, start + 5).join("")).join("-");
}

export function UserActions({ id, name, suspended, edit }: { id: string; name: string; suspended: boolean; edit?: ProfileEdit }) {
  const { pending, run, toast, router } = useMutation();
  const [editing, setEditing] = useState(false);
  const [resetting, setResetting] = useState(false);
  const [suspending, setSuspending] = useState(false);
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
            ({ revoked }) => toast({ title: "Sessions revoked", description: `Ended ${pluralize(revoked, "session")}. ${name} must sign in again everywhere.` }),
          )
        }
      >
        <MonitorX aria-hidden="true" size={16} strokeWidth={1.75} />
        Revoke sessions
      </Button>
      <Button size="sm" variant="secondary" onClick={() => setResetting(true)}>
        <RotateCcw aria-hidden="true" size={16} strokeWidth={1.75} />
        Reset authentication
      </Button>
      <Menu>
        <MenuTrigger asChild>
          <Button size="icon-sm" variant="quiet" aria-label="More actions" loading={pending === "restore"}>
            <MoreHorizontal aria-hidden="true" size={16} strokeWidth={1.75} />
          </Button>
        </MenuTrigger>
        <MenuContent>
          {edit ? (
            <MenuItem icon={UserPen} onSelect={() => setEditing(true)}>
              Edit profile
            </MenuItem>
          ) : null}
          <MenuItem icon={UsersRound} onSelect={() => router.push(`/admin/users/${id}?tab=groups`, { scroll: false })}>
            Modify groups
          </MenuItem>
          <MenuItem
            icon={Copy}
            onSelect={() => {
              void navigator.clipboard?.writeText(id);
              toast({ title: "User ID copied", description: id });
            }}
          >
            Copy user ID
          </MenuItem>
          <MenuSeparator />
          {suspended ? (
            <MenuItem
              icon={Undo2}
              onSelect={() =>
                run(
                  "restore",
                  () => api(`/users/${id}/restore`, { method: "POST" }),
                  () => toast({ title: `${name} restored`, description: "They can sign in again. Recorded in the audit log." }),
                )
              }
            >
              Restore user
            </MenuItem>
          ) : (
            <MenuItem icon={Ban} danger onSelect={() => setSuspending(true)}>
              Suspend user
            </MenuItem>
          )}
        </MenuContent>
      </Menu>
      <ResetAuthenticationDialog user={resetting ? user : null} onClose={() => setResetting(false)} />
      <SuspendDialog users={suspending ? [user] : []} onClose={() => setSuspending(false)} />
      {edit ? <EditProfileDialog {...edit} open={editing} onOpenChange={setEditing} /> : null}
    </div>
  );
}

export function ResetAuthenticationDialog({ user, onClose }: { user: UserRef | null; onClose: () => void }) {
  const { pending, run } = useMutation();
  const [link, setLink] = useState<SetupLink | null>(null);

  function close() {
    onClose();
    setLink(null);
  }

  return (
    <Dialog
      open={user !== null}
      onOpenChange={(next) => (next ? undefined : close())}
      title={link ? "New setup link" : `Reset ${user?.name ?? ""}'s authentication?`}
      description={
        link
          ? `${link.emailed ? `We emailed a setup link to ${user?.name}` : `Send this link to ${user?.name}`} so they can enroll a new passkey. It expires ${formatDateTime(link.expiresAt)}.`
          : "Removes every passkey, security key, authenticator app, password and recovery code, and signs them out everywhere. They enroll again from a new setup link."
      }
      footer={
        link ? (
          <Button variant="primary" onClick={close}>
            Done
          </Button>
        ) : (
          <>
            <Button variant="secondary" onClick={close}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              loading={pending === "reset"}
              onClick={() => user && run("reset", () => api<SetupLink>(`/users/${user.id}/reset-authentication`, { method: "POST" }), setLink)}
            >
              Reset authentication
            </Button>
          </>
        )
      }
    >
      {link ? (
        <div className="flex animate-enter flex-col gap-4">
          <CopyField label="Setup link" value={link.enrollUrl} />
          <SecretWarning>
            {link.emailed ? "You can also copy the link and share it yourself." : "Halo didn't email this link, so share it with them directly."} Anyone with it can enroll a
            passkey on {user?.name}&apos;s account.
          </SecretWarning>
        </div>
      ) : null}
    </Dialog>
  );
}

export function TemporaryPasswordButton({ user, warning }: { user: UserRef; warning?: string }) {
  const { pending, run, toast } = useMutation();
  const formId = useId();
  const [open, setOpen] = useState(false);
  const [password, setPassword] = useState("");

  return (
    <>
      <Button
        size="sm"
        onClick={() => {
          setPassword(randomPassword());
          setOpen(true);
        }}
      >
        <RectangleEllipsis aria-hidden="true" size={16} strokeWidth={1.75} />
        Set temporary password
      </Button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        title="Set a temporary password"
        description={`${user.name} signs in with it once and then chooses their own password. Share it in person or over another channel you trust.`}
        footer={
          <>
            <Button variant="secondary" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button variant="primary" type="submit" form={formId} loading={pending === "password"}>
              Set temporary password
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
              toast({ title: "Temporary password set", description: `${user.name} must choose a new password the next time they sign in.` });
            });
          }}
        >
          <Field label="Temporary password" hint="Halo generated a random one. You can replace it with your own of at least 15 characters.">
            {(props) => (
              <Input
                {...props}
                mono
                autoComplete="off"
                spellCheck={false}
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                trailing={<CopyButton value={password} label="Copy password" className="[&>span]:sr-only" />}
              />
            )}
          </Field>
          {warning ? <SecretWarning>{warning}</SecretWarning> : null}
        </form>
      </Dialog>
    </>
  );
}

export function SuspendDialog({ users, onClose }: { users: UserRef[]; onClose: () => void }) {
  const { pending, run, toast } = useMutation();
  const single = users.length === 1 ? users[0] : undefined;

  return (
    <Dialog
      open={users.length > 0}
      onOpenChange={(next) => (next ? undefined : onClose())}
      title={single ? `Suspend ${single.name}?` : `Suspend ${pluralize(users.length, "user")}?`}
      description="Sign-in is blocked immediately and every session is revoked. Group memberships and application assignments are kept, so you can restore access later."
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
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
                  toast({ title: single ? `${single.name} suspended` : `${pluralize(users.length, "user")} suspended`, description: "Recorded in the audit log." });
                },
              )
            }
          >
            Suspend
          </Button>
        </>
      }
    />
  );
}
