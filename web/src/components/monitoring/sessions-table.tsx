"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { MonitorX } from "lucide-react";
import { useMutation } from "@/components/console/use-mutation";
import { Badge } from "@/components/ui/badge";
import { DataTable, facetFilter } from "@/components/ui/data-table";
import { RelativeTime } from "@/components/ui/relative-time";
import { api } from "@/lib/api/client";
import { formatDateTime, pluralize } from "@/lib/format";
import { METHOD } from "@/lib/labels";
import type { Session } from "@/lib/types";

type Row = Session & { userName: string };

const columns: ColumnDef<Row, any>[] = [
  {
    id: "user",
    accessorFn: (s) => s.userName,
    header: "User",
    meta: { label: "User", hideable: false },
    cell: ({ getValue }) => <span className="font-medium whitespace-nowrap text-fg">{getValue<string>()}</span>,
  },
  {
    id: "device",
    accessorFn: (s) => `${s.device} · ${s.browser}`,
    header: "Device",
    meta: { label: "Device" },
    cell: ({ row, getValue }) => (
      <span className="inline-flex items-center gap-2 whitespace-nowrap">
        {getValue<string>()}
        {row.original.current ? <Badge tone="success">This browser</Badge> : null}
      </span>
    ),
  },
  {
    id: "ip",
    accessorFn: (s) => s.ip,
    header: "IP address",
    meta: { label: "IP address" },
    cell: ({ getValue }) => <span className="font-mono text-code-sm whitespace-nowrap text-fg-3">{getValue<string>()}</span>,
  },
  {
    id: "location",
    accessorFn: (s) => s.location,
    header: "Location",
    meta: { label: "Location" },
    cell: ({ getValue }) => <span className="whitespace-nowrap text-fg-3">{getValue<string>()}</span>,
  },
  {
    id: "method",
    accessorFn: (s) => s.method,
    header: "Method",
    filterFn: facetFilter,
    meta: { label: "Method" },
    cell: ({ row }) => <span className="whitespace-nowrap">{METHOD[row.original.method].label}</span>,
  },
  {
    id: "created",
    accessorFn: (s) => new Date(s.createdAt).getTime(),
    header: "Signed in",
    meta: { label: "Signed in", align: "right" },
    cell: ({ row }) => (
      <span className="tnum whitespace-nowrap text-fg-3" title={formatDateTime(row.original.createdAt)}>
        <RelativeTime iso={row.original.createdAt} />
      </span>
    ),
  },
  {
    id: "lastActive",
    accessorFn: (s) => new Date(s.lastActiveAt).getTime(),
    header: "Last active",
    meta: { label: "Last active", align: "right" },
    cell: ({ row }) => (
      <span className="tnum whitespace-nowrap text-fg-3" title={formatDateTime(row.original.lastActiveAt)}>
        <RelativeTime iso={row.original.lastActiveAt} />
      </span>
    ),
  },
];

export function SessionsTable({ sessions, userNames }: { sessions: Session[]; userNames: Record<string, string> }) {
  const { run, toast } = useMutation();
  const rows = sessions.map((s) => ({ ...s, userName: userNames[s.userId] ?? "Unknown user" }));
  const methods = [...new Set(sessions.map((s) => s.method))];

  function revoke(selected: Row[]) {
    const targets = selected.filter((s) => !s.current);
    if (targets.length === 0) {
      toast({ title: "This is your current session", description: "Sign out from the account menu instead." });
      return;
    }
    void run(
      "revoke",
      () => Promise.all(targets.map((s) => api(`/sessions/${s.id}`, { method: "DELETE" }))),
      () =>
        toast({
          title: `${pluralize(targets.length, "session")} revoked`,
          description: targets.length < selected.length ? "Your current session was skipped. Sign out from the account menu instead." : "Those devices are signed out of Halo.",
        }),
    );
  }

  return (
    <DataTable
      data={rows}
      columns={columns}
      getRowId={(s) => s.id}
      label="Active sessions"
      noun={["session", "sessions"]}
      storageKey="sessions"
      searchText={(s) => `${s.userName} ${s.device} ${s.browser} ${s.ip} ${s.location}`}
      searchPlaceholder="Search user, device, IP or location"
      facets={[{ column: "method", label: "Method", options: methods.map((m) => ({ value: m, label: METHOD[m].label })) }]}
      rowActions={[{ label: "Revoke", icon: MonitorX, danger: true, onSelect: (s) => revoke([s]) }]}
      bulkActions={[{ label: "Revoke sessions", icon: MonitorX, danger: true, onSelect: revoke }]}
    />
  );
}
