"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Ban, Copy, MonitorX, RotateCcw, ShieldAlert, ShieldCheck, ShieldX, UserPlus, UserRound } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Avatar } from "@/components/ui/avatar";
import { StatusDot } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DataTable, facetFilter, type SavedView } from "@/components/ui/data-table";
import { RelativeTime } from "@/components/ui/relative-time";
import { api } from "@/lib/api/client";
import { formatDate, pluralize } from "@/lib/format";
import { STATUS, STRENGTH } from "@/lib/labels";
import type { Group, User } from "@/lib/types";
import { InviteDialog } from "./invite-dialog";
import { ResetAuthenticationDialog, SuspendDialog } from "./user-actions";

const strengthIcon = { "phishing-resistant": ShieldCheck, "multi-factor": ShieldAlert, "single-factor": ShieldX } as const;
const strengthColor = { "phishing-resistant": "text-success", "multi-factor": "text-warning", "single-factor": "text-danger" } as const;

export const userColumns: ColumnDef<User, any>[] = [
  {
    id: "name",
    accessorFn: (u) => u.name,
    header: "Name",
    meta: { label: "Name", hideable: false },
    cell: ({ row }) => (
      <span className="flex min-w-56 items-center gap-3">
        <Avatar name={row.original.name} src={row.original.avatarUrl} size="md" />
        <span className="flex min-w-0 flex-col">
          <span className="truncate font-medium text-fg">{row.original.name}</span>
          <span className="truncate text-caption text-fg-3">{row.original.email}</span>
        </span>
      </span>
    ),
  },
  {
    id: "status",
    accessorFn: (u) => u.status,
    header: "Status",
    filterFn: facetFilter,
    meta: { label: "Status" },
    cell: ({ row }) => (
      <span className="inline-flex items-center gap-2 whitespace-nowrap">
        <StatusDot tone={STATUS[row.original.status].tone} />
        {STATUS[row.original.status].label}
      </span>
    ),
  },
  {
    id: "strength",
    accessorFn: (u) => u.strength,
    header: "Authentication",
    filterFn: facetFilter,
    sortingFn: (a, b) => ["single-factor", "multi-factor", "phishing-resistant"].indexOf(a.original.strength) - ["single-factor", "multi-factor", "phishing-resistant"].indexOf(b.original.strength),
    meta: { label: "Authentication" },
    cell: ({ row }) => {
      const Icon = strengthIcon[row.original.strength];
      return (
        <span className="inline-flex items-center gap-2 whitespace-nowrap">
          <Icon aria-hidden="true" size={16} strokeWidth={1.75} className={strengthColor[row.original.strength]} />
          {STRENGTH[row.original.strength].label}
        </span>
      );
    },
  },
  {
    id: "department",
    accessorFn: (u) => u.department,
    header: "Department",
    filterFn: facetFilter,
    meta: { label: "Department" },
    cell: ({ row }) => <span className="whitespace-nowrap">{row.original.department}</span>,
  },
  {
    id: "role",
    accessorFn: (u) => (u.roles.length ? "admin" : "none"),
    header: "Admin role",
    filterFn: facetFilter,
    meta: { label: "Admin role" },
    cell: ({ row }) => (row.original.roles.length ? <span className="whitespace-nowrap text-fg">{row.original.roles[0]}</span> : <span className="text-fg-3">—</span>),
  },
  {
    id: "groups",
    accessorFn: (u) => u.groupIds.length,
    header: "Groups",
    meta: { label: "Groups", align: "right" },
    cell: ({ getValue }) => <span className="tnum">{getValue<number>()}</span>,
  },
  {
    id: "source",
    accessorFn: (u) => u.source,
    header: "Source",
    filterFn: facetFilter,
    meta: { label: "Source" },
    cell: ({ row }) => <span className="whitespace-nowrap text-fg-3">{row.original.source}</span>,
  },
  {
    id: "lastSignIn",
    accessorFn: (u) => (u.lastSignInAt ? new Date(u.lastSignInAt).getTime() : 0),
    header: "Last sign-in",
    meta: { label: "Last sign-in", align: "right" },
    cell: ({ row }) => (
      <span className="tnum whitespace-nowrap text-fg-3">{row.original.lastSignInAt ? <RelativeTime iso={row.original.lastSignInAt} /> : "Never"}</span>
    ),
  },
  {
    id: "created",
    accessorFn: (u) => new Date(u.createdAt).getTime(),
    header: "Created",
    meta: { label: "Created", align: "right" },
    cell: ({ row }) => <span className="tnum whitespace-nowrap text-fg-3">{formatDate(row.original.createdAt)}</span>,
  },
];

const views: SavedView[] = [
  { id: "all", label: "All users", filters: [] },
  { id: "admins", label: "Administrators", filters: [{ id: "role", value: ["admin"] }] },
  {
    id: "weak-admins",
    label: "Admins without phishing-resistant sign-in",
    filters: [
      { id: "role", value: ["admin"] },
      { id: "strength", value: ["multi-factor", "single-factor"] },
      { id: "status", value: ["active"] },
    ],
  },
  { id: "weak", label: "Weak authentication", filters: [{ id: "strength", value: ["multi-factor", "single-factor"] }] },
  { id: "suspended", label: "Suspended", filters: [{ id: "status", value: ["suspended"] }] },
  { id: "invited", label: "Invited", filters: [{ id: "status", value: ["invited"] }] },
];

export function UsersTable({ users, groups, initialView, inviteOpen }: { users: User[]; groups: Group[]; initialView?: string; inviteOpen?: boolean }) {
  const { run, toast, router } = useMutation();
  const [invite, setInvite] = useState(Boolean(inviteOpen));
  const [resetting, setResetting] = useState<User | null>(null);
  const [suspending, setSuspending] = useState<User[]>([]);

  function revokeSessions(rows: User[]) {
    void run(
      "revoke",
      () => Promise.all(rows.map((u) => api<{ revoked: number }>(`/users/${u.id}/revoke-sessions`, { method: "POST" }))),
      (results) =>
        toast({
          title: "Sessions revoked",
          description: `Ended ${pluralize(results.reduce((sum, r) => sum + r.revoked, 0), "session")}. ${rows.length === 1 ? rows[0]?.name : pluralize(rows.length, "user")} must sign in again everywhere.`,
        }),
    );
  }

  function suspend(rows: User[]) {
    const active = rows.filter((u) => u.status !== "suspended");
    if (active.length === 0) {
      toast({ title: rows.length === 1 ? `${rows[0]?.name} is already suspended` : "Already suspended", description: "Restore access from the person's profile." });
      return;
    }
    setSuspending(active);
  }
  const departments = [...new Set(users.map((u) => u.department).filter(Boolean))].sort();

  return (
    <>
      <DataTable
        data={users}
        columns={userColumns}
        getRowId={(u) => u.id}
        label="Users"
        noun={["user", "users"]}
        storageKey="users"
        searchText={(u) => `${u.name} ${u.email} ${u.title} ${u.department} ${u.location}`}
        searchPlaceholder="Search name, email or title"
        views={views}
        initialViewId={initialView}
        initialVisibility={{ groups: false, source: false, created: false }}
        facets={[
          { column: "status", label: "Status", options: Object.entries(STATUS).map(([value, s]) => ({ value, label: s.label })) },
          { column: "strength", label: "Authentication", options: Object.entries(STRENGTH).map(([value, s]) => ({ value, label: s.label })) },
          { column: "department", label: "Department", options: departments.map((d) => ({ value: d, label: d })) },
          { column: "role", label: "Admin role", options: [{ value: "admin", label: "Has an admin role" }, { value: "none", label: "No admin role" }] },
        ]}
        onRowOpen={(u) => router.push(`/admin/users/${u.id}`)}
        rowActions={[
          { label: "Open profile", icon: UserRound, onSelect: (u) => router.push(`/admin/users/${u.id}`) },
          {
            label: "Copy user ID",
            icon: Copy,
            onSelect: (u) => {
              void navigator.clipboard?.writeText(u.id);
              toast({ title: "User ID copied", description: u.id });
            },
          },
          { label: "Revoke sessions", icon: MonitorX, separatorBefore: true, onSelect: (u) => revokeSessions([u]) },
          { label: "Reset authentication", icon: RotateCcw, onSelect: setResetting },
          { label: "Suspend", icon: Ban, danger: true, separatorBefore: true, onSelect: (u) => suspend([u]) },
        ]}
        bulkActions={[
          { label: "Revoke sessions", icon: MonitorX, onSelect: revokeSessions },
          { label: "Suspend", icon: Ban, danger: true, onSelect: suspend },
        ]}
        toolbarEnd={
          <Button size="sm" variant="primary" onClick={() => setInvite(true)}>
            <UserPlus aria-hidden="true" size={16} strokeWidth={1.75} />
            Invite user
          </Button>
        }
      />
      <InviteDialog open={invite} onOpenChange={setInvite} groups={groups} directory={users} />
      <ResetAuthenticationDialog user={resetting} onClose={() => setResetting(null)} />
      <SuspendDialog users={suspending} onClose={() => setSuspending([])} />
    </>
  );
}
