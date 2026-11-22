"use client";

import { ShieldAlert } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { RoleCheckboxes } from "@/components/provisioning/service-accounts-table";
import { rolesFor } from "@/components/provisioning/shared";
import { keysOf } from "@/components/settings/roles";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { api } from "@/lib/api/client";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { directoryMessages } from "@/lib/i18n/console/directory";
import type { RoleKey } from "@/lib/provisioning-types";
import type { User } from "@/lib/types";

export function PasskeyWarning({ role }: { role: string }) {
  const t = useMessages(directoryMessages);
  return (
    <div className="flex gap-3 rounded-md border border-warning/20 bg-warning-container p-4">
      <ShieldAlert aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0 text-warning" />
      <p className="text-body-sm text-fg-2">{t.roles.passkeyWarning(role)}</p>
    </div>
  );
}

export function EditRolesButton({ user }: { user: User }) {
  const [open, setOpen] = useState(false);
  const [roles, setRoles] = useState<RoleKey[]>([]);
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const t = useMessages(directoryMessages);
  const locale = useLocale();
  const given = roles.find((role) => !keysOf(user).includes(role));

  function save() {
    void run("roles", () => api(`/users/${user.id}/roles`, { method: "PUT", body: { roles } }), () => {
      setOpen(false);
      toast({ title: t.roles.saved, description: t.roles.savedHint });
    }, "inline");
  }

  return (
    <>
      <button
        type="button"
        aria-label={t.roles.title(user.name)}
        className="rounded-sm text-link transition-colors duration-fast hover:text-ember"
        onClick={() => {
          setRoles(keysOf(user));
          clearError();
          setOpen(true);
        }}
      >
        {t.edit}
      </button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        error={errorFor("roles")}
        title={t.roles.title(user.name)}
        description={t.roles.description(Object.keys(user.roleScopes).length > 0)}
        footer={
          <>
            <Button variant="secondary" onClick={() => setOpen(false)}>
              {t.cancel}
            </Button>
            <Button variant="primary" loading={pending === "roles"} onClick={save}>
              {t.roles.save}
            </Button>
          </>
        }
      >
        <div className="flex flex-col gap-6">
          <RoleCheckboxes selected={roles} onChange={setRoles} />
          {given && user.strength !== "phishing-resistant" ? <PasskeyWarning role={rolesFor(locale)[given].label} /> : null}
        </div>
      </Dialog>
    </>
  );
}
