"use client";

import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { DescriptionList } from "@/components/ui/description-list";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import type { PosixAccount, User } from "@/lib/types";

export function LinuxAccountCard({ user, editable }: { user: User; editable: boolean }) {
  const [open, setOpen] = useState(false);
  const posix = user.posix;
  return (
    <Card>
      <CardHeader
        title="Linux account"
        description="The account this person logs in to on hosts that run halo-hostd, and that the LDAP server shows as posixAccount."
        actions={
          posix && editable ? (
            <Button size="sm" variant="quiet" onClick={() => setOpen(true)}>
              Edit
            </Button>
          ) : undefined
        }
      />
      <CardBody>
        {posix ? (
          <DescriptionList
            columns={2}
            items={[
              { label: "User name", value: posix.username, mono: true },
              { label: "User and group number", value: String(posix.uidNumber), mono: true },
              { label: "Login shell", value: posix.loginShell, mono: true },
              { label: "Home directory", value: posix.homeDirectory, mono: true },
            ]}
          />
        ) : (
          <p className="text-body-sm text-fg-3">No Linux account yet. The user number range under Infrastructure access is used up; widen it and Halo assigns one.</p>
        )}
      </CardBody>
      {open && posix ? <LinuxAccountDialog user={user} posix={posix} onClose={() => setOpen(false)} /> : null}
    </Card>
  );
}

function LinuxAccountDialog({ user, posix, onClose }: { user: User; posix: PosixAccount; onClose: () => void }) {
  const { pending, run, errorFor, toast } = useMutation();
  const [draft, setDraft] = useState({ username: posix.username, loginShell: posix.loginShell, homeDirectory: posix.homeDirectory });

  function save() {
    void run(
      "posix",
      () => api<User>(`/users/${user.id}/posix`, { method: "PUT", body: draft }),
      (saved) => {
        toast({ title: `Linux account saved as ${saved.posix?.username ?? draft.username}`, description: "Hosts pick it up at their next refresh. Recorded in the audit log." });
        onClose();
      },
      "inline",
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      error={errorFor("posix")}
      title={`Edit ${user.name}'s Linux account`}
      description={`The user and group number ${posix.uidNumber} never changes. Renaming doesn't move the home directory on hosts that already have one.`}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={pending === "posix"} onClick={save}>
            Save account
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
        <Field label="User name" hint="Up to 32 lowercase letters, digits, _ . or -, starting with a letter or _. Unique across people and groups.">
          {(props) => <Input {...props} autoFocus mono autoComplete="off" spellCheck={false} value={draft.username} onChange={(event) => setDraft({ ...draft, username: event.target.value })} />}
        </Field>
        <Field label="Login shell">
          {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={draft.loginShell} onChange={(event) => setDraft({ ...draft, loginShell: event.target.value })} />}
        </Field>
        <Field label="Home directory">
          {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={draft.homeDirectory} onChange={(event) => setDraft({ ...draft, homeDirectory: event.target.value })} />}
        </Field>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}
