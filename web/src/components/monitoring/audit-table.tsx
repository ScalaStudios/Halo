"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Tag } from "@/components/ui/badge";
import { DataTable, facetFilter } from "@/components/ui/data-table";
import { RelativeTime } from "@/components/ui/relative-time";
import { formatDateTime } from "@/lib/format";
import type { AuditEvent } from "@/lib/types";

type Row = AuditEvent & { actor: string };

const columns: ColumnDef<Row, any>[] = [
  {
    id: "time",
    accessorFn: (e) => new Date(e.time).getTime(),
    header: "Time",
    meta: { label: "Time", hideable: false },
    cell: ({ row }) => (
      <span className="tnum whitespace-nowrap text-fg-3" title={formatDateTime(row.original.time)}>
        <RelativeTime iso={row.original.time} />
      </span>
    ),
  },
  {
    id: "actor",
    accessorFn: (e) => e.actor,
    header: "Actor",
    filterFn: facetFilter,
    meta: { label: "Actor", hideable: false },
    cell: ({ row }) => <span className="font-medium whitespace-nowrap text-fg">{row.original.actor}</span>,
  },
  {
    id: "action",
    accessorFn: (e) => e.action.split(".")[0],
    header: "Action",
    filterFn: facetFilter,
    meta: { label: "Action" },
    cell: ({ row }) => <Tag>{row.original.action}</Tag>,
  },
  {
    id: "summary",
    accessorFn: (e) => e.summary,
    header: "Summary",
    enableSorting: false,
    meta: { label: "Summary" },
    cell: ({ row }) => <span className="block min-w-64">{row.original.summary}</span>,
  },
  {
    id: "target",
    accessorFn: (e) => e.target,
    header: "Target",
    meta: { label: "Target" },
    cell: ({ row }) => <span className="whitespace-nowrap text-fg">{row.original.target}</span>,
  },
  {
    id: "ip",
    accessorFn: (e) => e.ip,
    header: "IP address",
    meta: { label: "IP address" },
    cell: ({ row }) => <span className="font-mono text-code-sm whitespace-nowrap text-fg-3">{row.original.ip}</span>,
  },
];

export function AuditTable({ events, userNames }: { events: AuditEvent[]; userNames: Record<string, string> }) {
  const rows = events.map((e) => ({ ...e, actor: e.actorId ? (userNames[e.actorId] ?? "Unknown user") : "Halo" }));
  const actors = [...new Set(rows.map((e) => e.actor))].sort();
  const prefixes = [...new Set(rows.map((e) => e.action.split(".")[0]))].sort();
  return (
    <DataTable
      data={rows}
      columns={columns}
      getRowId={(e) => e.id}
      label="Audit log"
      noun={["event", "events"]}
      storageKey="audit"
      searchText={(e) => `${e.actor} ${e.action} ${e.summary} ${e.target} ${e.ip}`}
      searchPlaceholder="Search actor, action or target"
      facets={[
        { column: "actor", label: "Actor", options: actors.map((actor) => ({ value: actor, label: actor })) },
        { column: "action", label: "Action", options: prefixes.map((p) => ({ value: p, label: `${p}.*` })) },
      ]}
    />
  );
}
