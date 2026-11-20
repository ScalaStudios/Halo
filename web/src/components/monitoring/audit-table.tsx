"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { LockKeyhole } from "lucide-react";
import { useState } from "react";
import { Tag } from "@/components/ui/badge";
import { CopyButton } from "@/components/ui/copy-button";
import { DataTable, facetFilter } from "@/components/ui/data-table";
import { DescriptionList } from "@/components/ui/description-list";
import { Dialog } from "@/components/ui/dialog";
import { RelativeTime } from "@/components/ui/relative-time";
import { cn } from "@/lib/cn";
import { formatDateTime } from "@/lib/format";
import type { AuditChange, AuditEvent } from "@/lib/types";

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

export function AuditTable({ events, userNames, initialEventId }: { events: AuditEvent[]; userNames: Record<string, string>; initialEventId?: string }) {
  const [openId, setOpenId] = useState(initialEventId);
  const rows = events.map((e) => ({ ...e, actor: e.actorId ? (userNames[e.actorId] ?? "Unknown user") : "Halo" }));
  const selected = rows.find((e) => e.id === openId);
  const actors = [...new Set(rows.map((e) => e.actor))].sort();
  const prefixes = [...new Set(rows.map((e) => e.action.split(".")[0]))].sort();

  function open(id: string | undefined) {
    setOpenId(id);
    window.history.replaceState(null, "", id ? `?event=${id}` : window.location.pathname);
  }

  return (
    <>
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
        onRowOpen={(e) => open(e.id)}
      />
      <Dialog
        open={Boolean(selected)}
        onOpenChange={(next) => {
          if (!next) open(undefined);
        }}
        width="wide"
        title={selected?.summary ?? "Audit event"}
        description={selected ? `${selected.actor} · ${formatDateTime(selected.time)}` : undefined}
      >
        {selected ? <AuditDetail event={selected} /> : null}
      </Dialog>
    </>
  );
}

function AuditDetail({ event }: { event: Row }) {
  return (
    <div className="flex flex-col gap-6 pb-4">
      <DescriptionList
        columns={2}
        items={[
          { label: "Action", value: <Tag>{event.action}</Tag> },
          { label: "Target", value: event.target || "—" },
          { label: "Actor", value: event.actor },
          {
            label: "IP address",
            value: event.ip ? (
              <span className="-my-2 flex items-center gap-2">
                <span className="font-mono text-code-sm">{event.ip}</span>
                <CopyButton value={event.ip} label="Copy IP" />
              </span>
            ) : (
              "—"
            ),
          },
          {
            label: "Event ID",
            value: (
              <span className="-my-2 flex items-center gap-2">
                <span className="font-mono text-code-sm break-all">{event.id}</span>
                <CopyButton value={event.id} label="Copy ID" />
              </span>
            ),
          },
        ]}
      />
      <section aria-labelledby="audit-changes" className="flex flex-col gap-3">
        <h3 id="audit-changes" className="text-h4 text-fg">
          Changes
        </h3>
        {event.changes.length === 0 ? (
          <p className="text-body-sm text-fg-3">No field changes were recorded for this event.</p>
        ) : (
          <ul className="divide-y divide-border overflow-hidden rounded-md border border-border bg-sunken">
            {event.changes.map((change) => (
              <li key={change.field} className="flex flex-col gap-2 p-4">
                <span className="font-mono text-code-sm break-all text-fg">{change.field}</span>
                <ChangeLines change={change} />
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}

function ChangeLines({ change }: { change: AuditChange }) {
  const { before, after } = change;
  if (change.redacted) {
    return (
      <p className="flex items-center gap-2 text-body-sm text-fg-3">
        <LockKeyhole aria-hidden="true" size={16} strokeWidth={1.75} className="shrink-0" />
        Changed. Halo never records secret values.
      </p>
    );
  }
  const scalars = (value: unknown) => value == null || (Array.isArray(value) && value.every((item) => item === null || typeof item !== "object"));
  if ((Array.isArray(before) || Array.isArray(after)) && scalars(before) && scalars(after)) {
    const old: unknown[] = Array.isArray(before) ? before : [];
    const next: unknown[] = Array.isArray(after) ? after : [];
    const removed = old.filter((item) => !next.includes(item));
    const added = next.filter((item) => !old.includes(item));
    if (removed.length === 0 && added.length === 0) {
      return <p className="text-body-sm text-fg-3">Same items in a new order.</p>;
    }
    return (
      <div className="flex flex-col gap-1">
        {removed.map((item, i) => (
          <Line key={`removed-${i}`} label="Removed" value={item} />
        ))}
        {added.map((item, i) => (
          <Line key={`added-${i}`} added label="Added" value={item} />
        ))}
      </div>
    );
  }
  return (
    <div className="flex flex-col gap-1">
      <Line label="Before" value={before} />
      <Line added label="After" value={after} />
    </div>
  );
}

function Line({ added = false, label, value }: { added?: boolean; label: string; value: unknown }) {
  return (
    <div className={cn("flex gap-2 rounded-xs px-2 py-1 font-mono text-code-sm", added ? "bg-success-container text-fg" : "bg-danger-container text-fg-2")}>
      <span aria-hidden="true" className={cn("select-none", added ? "text-success" : "text-danger")}>
        {added ? "+" : "−"}
      </span>
      <span className="sr-only">{label}: </span>
      <span className="min-w-0 break-all whitespace-pre-wrap">
        {value == null || value === "" ? (
          <span className="font-sans text-fg-3">{value === "" ? "Empty" : "Not set"}</span>
        ) : typeof value === "string" ? (
          value
        ) : (
          JSON.stringify(value)
        )}
      </span>
    </div>
  );
}
