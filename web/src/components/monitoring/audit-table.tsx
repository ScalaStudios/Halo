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
import { useLocale } from "@/lib/i18n/client";
import { frameMessages } from "@/lib/i18n/console/frame";
import { monitoringMessages } from "@/lib/i18n/console/monitoring";
import type { Locale } from "@/lib/i18n/locale";
import type { AuditChange, AuditEvent } from "@/lib/types";

type Row = AuditEvent & { actor: string };

type Messages = (typeof monitoringMessages)[Locale];

const columns = (t: Messages, locale: Locale): ColumnDef<Row, any>[] => [
  {
    id: "time",
    accessorFn: (e) => new Date(e.time).getTime(),
    header: t.time,
    meta: { label: t.time, hideable: false },
    cell: ({ row }) => (
      <span className="tnum whitespace-nowrap text-fg-3" title={formatDateTime(row.original.time, locale)}>
        <RelativeTime iso={row.original.time} />
      </span>
    ),
  },
  {
    id: "actor",
    accessorFn: (e) => e.actor,
    header: t.actor,
    filterFn: facetFilter,
    meta: { label: t.actor, hideable: false },
    cell: ({ row }) => <span className="font-medium whitespace-nowrap text-fg">{row.original.actor}</span>,
  },
  {
    id: "action",
    accessorFn: (e) => e.action.split(".")[0],
    header: t.action,
    filterFn: facetFilter,
    meta: { label: t.action },
    cell: ({ row }) => <Tag>{row.original.action}</Tag>,
  },
  {
    id: "summary",
    accessorFn: (e) => e.summary,
    header: t.audit.summary,
    enableSorting: false,
    meta: { label: t.audit.summary },
    cell: ({ row }) => <span className="block min-w-64">{row.original.summary}</span>,
  },
  {
    id: "target",
    accessorFn: (e) => e.target,
    header: t.audit.target,
    meta: { label: t.audit.target },
    cell: ({ row }) => <span className="whitespace-nowrap text-fg">{row.original.target}</span>,
  },
  {
    id: "ip",
    accessorFn: (e) => e.ip,
    header: t.ipAddress,
    meta: { label: t.ipAddress },
    cell: ({ row }) => <span className="font-mono text-code-sm whitespace-nowrap text-fg-3">{row.original.ip}</span>,
  },
];

export function AuditTable({ events, userNames, initialEventId }: { events: AuditEvent[]; userNames: Record<string, string>; initialEventId?: string }) {
  const locale = useLocale();
  const t = monitoringMessages[locale];
  const [openId, setOpenId] = useState(initialEventId);
  const rows = events.map((e) => ({ ...e, actor: e.actorId ? (userNames[e.actorId] ?? e.actorId) : "Halo" }));
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
        columns={columns(t, locale)}
        getRowId={(e) => e.id}
        label={frameMessages[locale].nav.items.audit.label}
        noun={t.audit.noun}
        storageKey="audit"
        searchText={(e) => `${e.actor} ${e.action} ${e.summary} ${e.target} ${e.ip}`}
        searchPlaceholder={t.audit.searchPlaceholder}
        facets={[
          { column: "actor", label: t.actor, options: actors.map((actor) => ({ value: actor, label: actor })) },
          { column: "action", label: t.action, options: prefixes.map((p) => ({ value: p, label: `${p}.*` })) },
        ]}
        onRowOpen={(e) => open(e.id)}
      />
      <Dialog
        open={Boolean(selected)}
        onOpenChange={(next) => {
          if (!next) open(undefined);
        }}
        width="wide"
        title={selected?.summary ?? t.audit.event}
        description={selected ? `${selected.actor} · ${formatDateTime(selected.time, locale)}` : undefined}
      >
        {selected ? <AuditDetail event={selected} t={t} /> : null}
      </Dialog>
    </>
  );
}

function AuditDetail({ event, t }: { event: Row; t: Messages }) {
  return (
    <div className="flex flex-col gap-6 pb-4">
      <DescriptionList
        columns={2}
        items={[
          { label: t.action, value: <Tag>{event.action}</Tag> },
          { label: t.audit.target, value: event.target || "—" },
          { label: t.actor, value: event.actor },
          {
            label: t.ipAddress,
            value: event.ip ? (
              <span className="-my-2 flex items-center gap-2">
                <span className="font-mono text-code-sm">{event.ip}</span>
                <CopyButton value={event.ip} label={t.copyIp} />
              </span>
            ) : (
              "—"
            ),
          },
          {
            label: t.eventId,
            value: (
              <span className="-my-2 flex items-center gap-2">
                <span className="font-mono text-code-sm break-all">{event.id}</span>
                <CopyButton value={event.id} label={t.copyId} />
              </span>
            ),
          },
        ]}
      />
      <section aria-labelledby="audit-changes" className="flex flex-col gap-3">
        <h3 id="audit-changes" className="text-h4 text-fg">
          {t.audit.changes}
        </h3>
        {event.changes.length === 0 ? (
          <p className="text-body-sm text-fg-3">{t.audit.noChanges}</p>
        ) : (
          <ul className="divide-y divide-border overflow-hidden rounded-md border border-border bg-sunken">
            {event.changes.map((change) => (
              <li key={change.field} className="flex flex-col gap-2 p-4">
                <span className="font-mono text-code-sm break-all text-fg">{change.field}</span>
                <ChangeLines change={change} t={t} />
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}

function ChangeLines({ change, t }: { change: AuditChange; t: Messages }) {
  const { before, after } = change;
  if (change.redacted) {
    return (
      <p className="flex items-center gap-2 text-body-sm text-fg-3">
        <LockKeyhole aria-hidden="true" size={16} strokeWidth={1.75} className="shrink-0" />
        {t.audit.redacted}
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
      return <p className="text-body-sm text-fg-3">{t.audit.reordered}</p>;
    }
    return (
      <div className="flex flex-col gap-1">
        {removed.map((item, i) => (
          <Line key={`removed-${i}`} label={t.audit.removed} value={item} t={t} />
        ))}
        {added.map((item, i) => (
          <Line key={`added-${i}`} added label={t.audit.added} value={item} t={t} />
        ))}
      </div>
    );
  }
  return (
    <div className="flex flex-col gap-1">
      <Line label={t.audit.before} value={before} t={t} />
      <Line added label={t.audit.after} value={after} t={t} />
    </div>
  );
}

function Line({ added = false, label, value, t }: { added?: boolean; label: string; value: unknown; t: Messages }) {
  return (
    <div className={cn("flex gap-2 rounded-xs px-2 py-1 font-mono text-code-sm", added ? "bg-success-container text-fg" : "bg-danger-container text-fg-2")}>
      <span aria-hidden="true" className={cn("select-none", added ? "text-success" : "text-danger")}>
        {added ? "+" : "−"}
      </span>
      <span className="sr-only">{label}: </span>
      <span className="min-w-0 break-all whitespace-pre-wrap">
        {value == null || value === "" ? (
          <span className="font-sans text-fg-3">{value === "" ? t.audit.empty : t.audit.notSet}</span>
        ) : typeof value === "string" ? (
          value
        ) : (
          JSON.stringify(value)
        )}
      </span>
    </div>
  );
}
