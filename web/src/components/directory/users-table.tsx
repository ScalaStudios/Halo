"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Ban, Copy, FileUp, MonitorX, RotateCcw, ShieldAlert, ShieldCheck, ShieldX, UserPlus, UserRound } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Avatar } from "@/components/ui/avatar";
import { Badge, StatusDot } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DataTable, facetFilter, type SavedView } from "@/components/ui/data-table";
import { RelativeTime } from "@/components/ui/relative-time";
import { api } from "@/lib/api/client";
import { formatDate } from "@/lib/format";
import { useLocale } from "@/lib/i18n/client";
import { directoryMessages } from "@/lib/i18n/console/directory";
import type { Locale } from "@/lib/i18n/locale";
import { STATUS } from "@/lib/labels";
import type { Group, User } from "@/lib/types";
import { ImportDialog } from "./import-dialog";
import { InviteDialog } from "./invite-dialog";
import { ResetAuthenticationDialog, SuspendDialog } from "./user-actions";

const strengthIcon = { "phishing-resistant": ShieldCheck, "multi-factor": ShieldAlert, "single-factor": ShieldX } as const;
const strengthColor = { "phishing-resistant": "text-success", "multi-factor": "text-warning", "single-factor": "text-danger" } as const;

export function userColumns(locale: Locale): ColumnDef<User, any>[] {
  const t = directoryMessages[locale];
  return [
    {
      id: "name",
      accessorFn: (u) => u.name,
      header: t.fields.name,
      meta: { label: t.fields.name, hideable: false },
      cell: ({ row }) => (
        <span className="flex min-w-56 items-center gap-3">
          <Avatar name={row.original.name} src={row.original.avatarUrl} size="md" />
          <span className="flex min-w-0 flex-col">
            <span className="flex min-w-0 items-center gap-2">
              <span className="truncate font-medium text-fg">{row.original.name}</span>
              {row.original.guest ? <Badge tone="info">{t.guest}</Badge> : null}
            </span>
            <span className="truncate text-caption text-fg-3">{row.original.email}</span>
          </span>
        </span>
      ),
    },
    {
      id: "type",
      accessorFn: (u) => (u.guest ? "guest" : "member"),
      header: t.fields.type,
      filterFn: facetFilter,
      meta: { label: t.fields.type },
      cell: ({ row }) =>
        row.original.guest && row.original.expiresAt ? (
          <span className="tnum whitespace-nowrap">{t.users.guestUntil(formatDate(row.original.expiresAt, locale))}</span>
        ) : (
          <span className="text-fg-3">{t.member}</span>
        ),
    },
    {
      id: "status",
      accessorFn: (u) => u.status,
      header: t.fields.status,
      filterFn: facetFilter,
      meta: { label: t.fields.status },
      cell: ({ row }) => (
        <span className="inline-flex items-center gap-2 whitespace-nowrap">
          <StatusDot tone={STATUS[row.original.status].tone} />
          {t.status[row.original.status]}
        </span>
      ),
    },
    {
      id: "strength",
      accessorFn: (u) => u.strength,
      header: t.fields.authentication,
      filterFn: facetFilter,
      sortingFn: (a, b) => ["single-factor", "multi-factor", "phishing-resistant"].indexOf(a.original.strength) - ["single-factor", "multi-factor", "phishing-resistant"].indexOf(b.original.strength),
      meta: { label: t.fields.authentication },
      cell: ({ row }) => {
        const Icon = strengthIcon[row.original.strength];
        return (
          <span className="inline-flex items-center gap-2 whitespace-nowrap">
            <Icon aria-hidden="true" size={16} strokeWidth={1.75} className={strengthColor[row.original.strength]} />
            {t.strength[row.original.strength].label}
          </span>
        );
      },
    },
    {
      id: "department",
      accessorFn: (u) => u.department,
      header: t.fields.department,
      filterFn: facetFilter,
      meta: { label: t.fields.department },
      cell: ({ row }) => <span className="whitespace-nowrap">{row.original.department}</span>,
    },
    {
      id: "role",
      accessorFn: (u) => (u.roles.length ? "admin" : "none"),
      header: t.fields.adminRole,
      filterFn: facetFilter,
      meta: { label: t.fields.adminRole },
      cell: ({ row }) => (row.original.roles.length ? <span className="whitespace-nowrap text-fg">{row.original.roles[0]}</span> : <span className="text-fg-3">—</span>),
    },
    {
      id: "groups",
      accessorFn: (u) => u.groupIds.length,
      header: t.fields.groups,
      meta: { label: t.fields.groups, align: "right" },
      cell: ({ getValue }) => <span className="tnum">{getValue<number>()}</span>,
    },
    {
      id: "source",
      accessorFn: (u) => u.source,
      header: t.fields.source,
      filterFn: facetFilter,
      meta: { label: t.fields.source },
      cell: ({ row }) => <span className="whitespace-nowrap text-fg-3">{row.original.source}</span>,
    },
    {
      id: "lastSignIn",
      accessorFn: (u) => (u.lastSignInAt ? new Date(u.lastSignInAt).getTime() : 0),
      header: t.fields.lastSignIn,
      meta: { label: t.fields.lastSignIn, align: "right" },
      cell: ({ row }) => (
        <span className="tnum whitespace-nowrap text-fg-3">{row.original.lastSignInAt ? <RelativeTime iso={row.original.lastSignInAt} /> : t.never}</span>
      ),
    },
    {
      id: "created",
      accessorFn: (u) => new Date(u.createdAt).getTime(),
      header: t.fields.created,
      meta: { label: t.fields.created, align: "right" },
      cell: ({ row }) => <span className="tnum whitespace-nowrap text-fg-3">{formatDate(row.original.createdAt, locale)}</span>,
    },
  ];
}

function savedViews(t: (typeof directoryMessages)["en"]): SavedView[] {
  return [
    { id: "all", label: t.users.views.all, filters: [] },
    { id: "admins", label: t.users.views.admins, filters: [{ id: "role", value: ["admin"] }] },
    {
      id: "weak-admins",
      label: t.users.views.weakAdmins,
      filters: [
        { id: "role", value: ["admin"] },
        { id: "strength", value: ["multi-factor", "single-factor"] },
        { id: "status", value: ["active"] },
      ],
    },
    { id: "weak", label: t.users.views.weak, filters: [{ id: "strength", value: ["multi-factor", "single-factor"] }] },
    { id: "guests", label: t.users.views.guests, filters: [{ id: "type", value: ["guest"] }] },
    { id: "suspended", label: t.users.views.suspended, filters: [{ id: "status", value: ["suspended"] }] },
    { id: "invited", label: t.users.views.invited, filters: [{ id: "status", value: ["invited"] }] },
  ];
}

export function UsersTable({ users, groups, initialView, inviteOpen, canGrantRoles }: { users: User[]; groups: Group[]; initialView?: string; inviteOpen?: boolean; canGrantRoles: boolean }) {
  const { run, toast, router } = useMutation();
  const locale = useLocale();
  const t = directoryMessages[locale];
  const [invite, setInvite] = useState(Boolean(inviteOpen));
  const [importing, setImporting] = useState(false);
  const [resetting, setResetting] = useState<User | null>(null);
  const [suspending, setSuspending] = useState<User[]>([]);

  function revokeSessions(rows: User[]) {
    return run(
      "revoke",
      () => Promise.all(rows.map((u) => api<{ revoked: number }>(`/users/${u.id}/revoke-sessions`, { method: "POST" }))),
      (results) =>
        toast({
          title: t.sessionsRevoked,
          description: t.sessionsRevokedHint(results.reduce((sum, r) => sum + r.revoked, 0), rows[0]?.name, rows.length),
        }),
    );
  }

  function suspend(rows: User[]) {
    const active = rows.filter((u) => u.status !== "suspended");
    if (active.length === 0) {
      toast({ title: t.users.alreadySuspended(rows.length === 1 ? rows[0]?.name : undefined), description: t.users.alreadySuspendedHint });
      return;
    }
    setSuspending(active);
  }
  const departments = [...new Set(users.map((u) => u.department).filter(Boolean))].sort();

  return (
    <>
      <DataTable
        data={users}
        columns={userColumns(locale)}
        getRowId={(u) => u.id}
        label={t.fields.users}
        noun={t.users.noun}
        storageKey="users"
        searchText={(u) => `${u.name} ${u.email} ${u.title} ${u.department} ${u.location}`}
        searchPlaceholder={t.searchPeople}
        views={savedViews(t)}
        initialViewId={initialView}
        initialVisibility={{ type: false, groups: false, source: false, created: false }}
        facets={[
          { column: "status", label: t.fields.status, options: Object.entries(t.status).map(([value, label]) => ({ value, label })) },
          { column: "type", label: t.fields.type, options: [{ value: "member", label: t.member }, { value: "guest", label: t.guest }] },
          { column: "strength", label: t.fields.authentication, options: Object.entries(t.strength).map(([value, s]) => ({ value, label: s.label })) },
          { column: "department", label: t.fields.department, options: departments.map((d) => ({ value: d, label: d })) },
          { column: "role", label: t.fields.adminRole, options: [{ value: "admin", label: t.users.hasAdminRole }, { value: "none", label: t.users.noAdminRole }] },
        ]}
        onRowOpen={(u) => router.push(`/admin/users/${u.id}`)}
        rowActions={[
          { label: t.openProfile, icon: UserRound, onSelect: (u) => router.push(`/admin/users/${u.id}`) },
          {
            label: t.copyUserId,
            icon: Copy,
            onSelect: (u) => {
              void navigator.clipboard?.writeText(u.id);
              toast({ title: t.userIdCopied, description: u.id });
            },
          },
          { label: t.revokeSessions, icon: MonitorX, separatorBefore: true, onSelect: (u) => revokeSessions([u]) },
          { label: t.resetAuthentication, icon: RotateCcw, onSelect: setResetting },
          { label: t.suspend, icon: Ban, danger: true, separatorBefore: true, onSelect: (u) => suspend([u]) },
        ]}
        bulkActions={[
          { label: t.revokeSessions, icon: MonitorX, onSelect: revokeSessions },
          { label: t.suspend, icon: Ban, danger: true, onSelect: suspend },
        ]}
        toolbarEnd={
          <>
            <Button size="sm" variant="secondary" onClick={() => setImporting(true)}>
              <FileUp aria-hidden="true" size={16} strokeWidth={1.75} />
              {t.users.importUsers}
            </Button>
            <Button size="sm" variant="primary" onClick={() => setInvite(true)}>
              <UserPlus aria-hidden="true" size={16} strokeWidth={1.75} />
              {t.users.inviteUser}
            </Button>
          </>
        }
      />
      <ImportDialog open={importing} onOpenChange={setImporting} />
      <InviteDialog open={invite} onOpenChange={setInvite} groups={groups} directory={users} canGrantRoles={canGrantRoles} />
      <ResetAuthenticationDialog user={resetting} onClose={() => setResetting(null)} />
      <SuspendDialog users={suspending} onClose={() => setSuspending([])} />
    </>
  );
}
