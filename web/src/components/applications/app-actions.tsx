"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { BookOpen, Check, Copy, Ellipsis, ExternalLink, Pencil, Power, RefreshCw, Trash2 } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ActionError } from "@/components/ui/action-error";
import { Button, buttonClasses } from "@/components/ui/button";
import { Card, CardHeader } from "@/components/ui/card";
import { HoldToConfirm } from "@/components/ui/hold-to-confirm";
import { Menu, MenuContent, MenuItem, MenuSeparator, MenuTrigger } from "@/components/ui/menu";
import { useToast } from "@/components/ui/toast";
import { api } from "@/lib/api/client";
import { cn } from "@/lib/cn";
import { useMessages } from "@/lib/i18n/client";
import { applicationsMessages } from "@/lib/i18n/console/applications";
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
  bookmark: boolean;
  radius: boolean;
  ldap?: boolean;
  remote?: boolean;
  disabled: boolean;
  awaitingApproval: boolean;
  redirectUris: string[];
  postLogoutUris: string[];
  hasSecret: boolean;
};

export function AppActions({ app, directory }: { app: AppSummary; directory: User[] }) {
  const router = useRouter();
  const toast = useToast();
  const { pending, run } = useMutation();
  const [rotate, setRotate] = useState(false);
  const [editing, setEditing] = useState(false);
  const t = useMessages(applicationsMessages);
  const a = t.actions;

  return (
    <div className="flex flex-wrap items-center gap-2">
      {app.awaitingApproval ? (
        <Button
          size="sm"
          variant="primary"
          loading={pending === "approve"}
          onClick={() =>
            void run(
              "approve",
              () => api(`/applications/${app.id}/enable`, { method: "POST", body: { redirectUris: app.redirectUris, postLogoutUris: app.postLogoutUris } }),
              () => toast({ title: a.approved(app.name), description: a.approvedHint }),
            )
          }
        >
          <Check aria-hidden="true" size={16} strokeWidth={1.75} />
          {a.approve}
        </Button>
      ) : null}
      {app.bookmark || app.radius || app.ldap || app.remote ? null : (
        <Link href={`/admin/applications/${app.id}#setup-guide`} className={buttonClasses("secondary", "sm")}>
          <BookOpen aria-hidden="true" size={16} strokeWidth={1.75} />
          {a.viewGuide}
        </Link>
      )}
      {app.hasSecret ? (
        <Button size="sm" variant="secondary" onClick={() => setRotate(true)}>
          <RefreshCw aria-hidden="true" size={16} strokeWidth={1.75} />
          {app.ldap ? a.rotateBind : a.rotateSecret}
        </Button>
      ) : null}
      <Menu>
        <MenuTrigger asChild>
          <Button size="icon-sm" variant="quiet" aria-label={a.more}>
            <Ellipsis aria-hidden="true" size={16} strokeWidth={1.75} />
          </Button>
        </MenuTrigger>
        <MenuContent>
          <MenuItem icon={Pencil} onSelect={() => setEditing(true)}>
            {a.editDetails}
          </MenuItem>
          {app.homepage ? (
            <MenuItem icon={ExternalLink} onSelect={() => window.open(app.homepage, "_blank", "noopener,noreferrer")}>
              {a.openHost(new URL(app.homepage).host)}
            </MenuItem>
          ) : null}
          {app.bookmark || app.radius || app.ldap || app.remote ? null : (
            <MenuItem
              icon={Copy}
              onSelect={() => {
                void navigator.clipboard?.writeText(app.clientId);
                toast({ title: app.saml ? t.list.entityIdCopied : t.list.clientIdCopied, description: app.clientId });
              }}
            >
              {app.saml ? a.copyEntityId : t.list.copyClientId}
            </MenuItem>
          )}
          <MenuSeparator />
          <MenuItem icon={Power} onSelect={() => router.push(`/admin/applications/${app.id}#danger-zone`)}>
            {app.disabled ? a.enable : a.disable}
          </MenuItem>
          <MenuItem icon={Trash2} danger onSelect={() => router.push(`/admin/applications/${app.id}#danger-zone`)}>
            {a.delete}
          </MenuItem>
        </MenuContent>
      </Menu>
      {app.hasSecret ? <RotateSecretDialog open={rotate} onOpenChange={setRotate} appId={app.id} appName={app.name} secret={app.radius ? "shared" : app.ldap ? "bind" : undefined} /> : null}
      <EditDetailsDialog app={app} directory={directory} open={editing} onOpenChange={setEditing} />
    </div>
  );
}

export function DangerZone({ id, name, disabled, bookmark = false, className }: { id: string; name: string; disabled: boolean; bookmark?: boolean; className?: string }) {
  const { pending, run, errorFor, toast, router } = useMutation();
  const t = useMessages(applicationsMessages);
  const a = t.actions;
  const copy = bookmark ? a.bookmark : a.application;

  function toggle() {
    void run(
      "toggle",
      () => api(`/applications/${id}/${disabled ? "enable" : "disable"}`, { method: "POST" }),
      () =>
        toast(
          disabled ? { title: a.enabledToast(name), description: copy.enabled } : { title: t.list.disabled(name), description: copy.disabled },
        ),
      "inline",
    );
  }

  function remove() {
    return run(
      "delete",
      () => api(`/applications/${id}`, { method: "DELETE" }),
      () => {
        toast({ title: t.common.deletedToast(name), description: t.common.audit });
        router.push("/admin/applications");
      },
      "inline",
    );
  }

  return (
    <div id="danger-zone" className={cn("scroll-mt-24", className)}>
      <Card>
        <CardHeader title={a.dangerZone} />
        <div className="flex flex-col gap-4 border-t border-border px-6 py-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex flex-col gap-1">
            <p className="text-body-sm font-semibold text-fg">{disabled ? a.enable : a.disable}</p>
            <p className="text-body-sm text-fg-3">{disabled ? copy.enable(name) : copy.disable(name)}</p>
            <ActionError message={errorFor("toggle")} />
          </div>
          <Button size="sm" variant="secondary" loading={pending === "toggle"} onClick={toggle} className="self-start sm:self-center">
            <Power aria-hidden="true" size={16} strokeWidth={1.75} />
            {disabled ? a.enable : a.disable}
          </Button>
        </div>
        <div className="flex flex-col gap-4 border-t border-border px-6 py-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex flex-col gap-1">
            <p className="text-body-sm font-semibold text-fg">{a.delete}</p>
            <p className="text-body-sm text-fg-3">{copy.remove(name)}</p>
            <ActionError message={errorFor("delete")} />
          </div>
          <HoldToConfirm
            size="sm"
            doneLabel={a.deleted}
            className="self-start sm:self-center"
            onConfirm={remove}
          >
            {a.holdToDelete}
          </HoldToConfirm>
        </div>
      </Card>
    </div>
  );
}
