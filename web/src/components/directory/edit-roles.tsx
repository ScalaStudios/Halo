"use client";

import { ShieldAlert } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { RoleCheckboxes } from "@/components/provisioning/service-accounts-table";
import { ROLES } from "@/components/provisioning/shared";
import { keysOf } from "@/components/settings/roles-board";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { api } from "@/lib/api/client";
import type { RoleKey } from "@/lib/provisioning-types";
import type { User } from "@/lib/types";

export function PasskeyWarning({ role }: { role: string }) {
  return (
    <div className="flex gap-3 rounded-md border border-warning/20 bg-warning-container p-4">
      <ShieldAlert aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0 text-warning" />
      <p className="text-body-sm text-fg-2">{role} should sign in with a passkey or security key. Until then this account can be phished.</p>
    </div>
  );
}

export function EditRolesButton({ user }: { user: User }) {
  const [open, setOpen] = useState(false);
  const [roles, setRoles] = useState<RoleKey[]>([]);
  const { pending, run, toast } = useMutation();
  const given = roles.find((role) => !keysOf(user).includes(role));

  function save() {
    void run("roles", () => api(`/users/${user.id}/roles`, { method: "PUT", body: { roles } }), () => {
      setOpen(false);
      toast({ title: "Roles saved", description: "Takes effect on their next request. Recorded in the audit log." });
    });
  }

  return (
    <>
      <button
        type="button"
        aria-label={`Edit ${user.name}'s roles`}
        className="rounded-sm text-link transition-colors duration-fast hover:text-ember"
        onClick={() => {
          setRoles(keysOf(user));
          setOpen(true);
        }}
      >
        Edit
      </button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        title={`Edit ${user.name}'s roles`}
        description="Every role can read every administration page. Changes take effect on their next request."
        footer={
          <>
            <Button variant="secondary" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button variant="primary" loading={pending === "roles"} onClick={save}>
              Save roles
            </Button>
          </>
        }
      >
        <div className="flex flex-col gap-6">
          <RoleCheckboxes selected={roles} onChange={setRoles} />
          {given && user.strength !== "phishing-resistant" ? <PasskeyWarning role={ROLES[given].label} /> : null}
        </div>
      </Dialog>
    </>
  );
}
