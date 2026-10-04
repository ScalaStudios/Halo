"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { BookOpen, Copy, Ellipsis, ExternalLink, Pencil, Power, RefreshCw, Trash2 } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Button, buttonClasses } from "@/components/ui/button";
import { Card, CardHeader } from "@/components/ui/card";
import { HoldToConfirm } from "@/components/ui/hold-to-confirm";
import { Menu, MenuContent, MenuItem, MenuSeparator, MenuTrigger } from "@/components/ui/menu";
import { useToast } from "@/components/ui/toast";
import { api } from "@/lib/api/client";
import { cn } from "@/lib/cn";
import type { User } from "@/lib/types";
import { RotateSecretDialog } from "./credentials";
import { EditDetailsDialog } from "./edit-application";

type AppSummary = {
  id: string;
  name: string;
  description: string;
  owner: string | null;
  clientId: string;
  homepage: string;
  saml: boolean;
  disabled: boolean;
  hasSecret: boolean;
};

export function AppActions({ app, directory }: { app: AppSummary; directory: User[] }) {
  const router = useRouter();
  const toast = useToast();
  const [rotate, setRotate] = useState(false);
  const [editing, setEditing] = useState(false);
  const idLabel = app.saml ? "entity ID" : "client ID";

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Link href={`/admin/applications/${app.id}#setup-guide`} className={buttonClasses("secondary", "sm")}>
        <BookOpen aria-hidden="true" size={16} strokeWidth={1.75} />
        View setup guide
      </Link>
      {app.hasSecret ? (
        <Button size="sm" variant="secondary" onClick={() => setRotate(true)}>
          <RefreshCw aria-hidden="true" size={16} strokeWidth={1.75} />
          Rotate secret
        </Button>
      ) : null}
      <Menu>
        <MenuTrigger asChild>
          <Button size="icon-sm" variant="quiet" aria-label="More actions">
            <Ellipsis aria-hidden="true" size={16} strokeWidth={1.75} />
          </Button>
        </MenuTrigger>
        <MenuContent>
          <MenuItem icon={Pencil} onSelect={() => setEditing(true)}>
            Edit details
          </MenuItem>
          {app.homepage ? (
            <MenuItem icon={ExternalLink} onSelect={() => window.open(app.homepage, "_blank", "noopener,noreferrer")}>
              Open {new URL(app.homepage).host}
            </MenuItem>
          ) : null}
          <MenuItem
            icon={Copy}
            onSelect={() => {
              void navigator.clipboard?.writeText(app.clientId);
              toast({ title: app.saml ? "Entity ID copied" : "Client ID copied", description: app.clientId });
            }}
          >
            Copy {idLabel}
          </MenuItem>
          <MenuSeparator />
          <MenuItem icon={Power} onSelect={() => router.push(`/admin/applications/${app.id}#danger-zone`)}>
            {app.disabled ? "Enable application" : "Disable application"}
          </MenuItem>
          <MenuItem icon={Trash2} danger onSelect={() => router.push(`/admin/applications/${app.id}#danger-zone`)}>
            Delete application
          </MenuItem>
        </MenuContent>
      </Menu>
      {app.hasSecret ? <RotateSecretDialog open={rotate} onOpenChange={setRotate} appId={app.id} appName={app.name} /> : null}
      <EditDetailsDialog app={app} directory={directory} open={editing} onOpenChange={setEditing} />
    </div>
  );
}

export function DangerZone({ id, name, disabled, className }: { id: string; name: string; disabled: boolean; className?: string }) {
  const { pending, run, toast, router } = useMutation();

  function toggle() {
    void run(
      "toggle",
      () => api(`/applications/${id}/${disabled ? "enable" : "disable"}`, { method: "POST" }),
      () =>
        toast(
          disabled
            ? { title: `${name} enabled`, description: "Assigned people can sign in again." }
            : { title: `${name} disabled`, description: "New sign-ins are blocked and tokens already issued are revoked." },
        ),
    );
  }

  function remove() {
    void run(
      "delete",
      () => api(`/applications/${id}`, { method: "DELETE" }),
      () => {
        toast({ title: `${name} deleted`, description: "Recorded in the audit log." });
        router.push("/admin/applications");
      },
    );
  }

  return (
    <div id="danger-zone" className={cn("scroll-mt-24", className)}>
      <Card>
        <CardHeader title="Danger zone" />
        <div className="flex flex-col gap-4 border-t border-border px-6 py-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex flex-col gap-1">
            <p className="text-body-sm font-semibold text-fg">{disabled ? "Enable application" : "Disable application"}</p>
            <p className="text-body-sm text-fg-3">
              {disabled
                ? `Lets people assigned to ${name} sign in again with the existing credentials.`
                : `Blocks new sign-ins to ${name} and revokes tokens already issued. You can enable it again at any time.`}
            </p>
          </div>
          <Button size="sm" variant="secondary" loading={pending === "toggle"} onClick={toggle} className="self-start sm:self-center">
            <Power aria-hidden="true" size={16} strokeWidth={1.75} />
            {disabled ? "Enable application" : "Disable application"}
          </Button>
        </div>
        <div className="flex flex-col gap-4 border-t border-border px-6 py-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex flex-col gap-1">
            <p className="text-body-sm font-semibold text-fg">Delete application</p>
            <p className="text-body-sm text-fg-3">Removes {name}, its credentials and group assignments. Sign-in stops immediately and cannot be restored.</p>
          </div>
          <HoldToConfirm
            size="sm"
            doneLabel="Deleted"
            className="self-start sm:self-center"
            onConfirm={remove}
          >
            Hold to delete
          </HoldToConfirm>
        </div>
      </Card>
    </div>
  );
}
