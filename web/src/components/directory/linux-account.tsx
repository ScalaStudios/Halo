"use client";

import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { DescriptionList } from "@/components/ui/description-list";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import { useMessages } from "@/lib/i18n/client";
import { directoryMessages } from "@/lib/i18n/console/directory";
import type { PosixAccount, User } from "@/lib/types";

export function LinuxAccountCard({ user, editable }: { user: User; editable: boolean }) {
  const [open, setOpen] = useState(false);
  const t = useMessages(directoryMessages);
  const posix = user.posix;
  return (
    <Card>
      <CardHeader
        title={t.linux.title}
        description={t.linux.description}
        actions={
          posix && editable ? (
            <Button size="sm" variant="quiet" onClick={() => setOpen(true)}>
              {t.edit}
            </Button>
          ) : undefined
        }
      />
      <CardBody>
        {posix ? (
          <DescriptionList
            columns={2}
            items={[
              { label: t.linux.userName, value: posix.username, mono: true },
              { label: t.linux.uid, value: String(posix.uidNumber), mono: true },
              { label: t.linux.shell, value: posix.loginShell, mono: true },
              { label: t.linux.home, value: posix.homeDirectory, mono: true },
            ]}
          />
        ) : (
          <p className="text-body-sm text-fg-3">{t.linux.none}</p>
        )}
      </CardBody>
      {open && posix ? <LinuxAccountDialog user={user} posix={posix} onClose={() => setOpen(false)} /> : null}
    </Card>
  );
}

function LinuxAccountDialog({ user, posix, onClose }: { user: User; posix: PosixAccount; onClose: () => void }) {
  const { pending, run, errorFor, toast } = useMutation();
  const t = useMessages(directoryMessages);
  const [draft, setDraft] = useState({ username: posix.username, loginShell: posix.loginShell, homeDirectory: posix.homeDirectory });

  function save() {
    void run(
      "posix",
      () => api<User>(`/users/${user.id}/posix`, { method: "PUT", body: draft }),
      (saved) => {
        toast({ title: t.linux.saved(saved.posix?.username ?? draft.username), description: t.linux.savedHint });
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
      title={t.linux.editTitle(user.name)}
      description={t.linux.editDescription(posix.uidNumber)}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t.cancel}
          </Button>
          <Button variant="primary" loading={pending === "posix"} onClick={save}>
            {t.linux.save}
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
        <Field label={t.linux.userName} hint={t.linux.userNameHint}>
          {(props) => <Input {...props} autoFocus mono autoComplete="off" spellCheck={false} value={draft.username} onChange={(event) => setDraft({ ...draft, username: event.target.value })} />}
        </Field>
        <Field label={t.linux.shell}>
          {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={draft.loginShell} onChange={(event) => setDraft({ ...draft, loginShell: event.target.value })} />}
        </Field>
        <Field label={t.linux.home}>
          {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={draft.homeDirectory} onChange={(event) => setDraft({ ...draft, homeDirectory: event.target.value })} />}
        </Field>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}
