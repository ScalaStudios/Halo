"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { MonitorX } from "lucide-react";
import { useMutation } from "@/components/console/use-mutation";
import { Badge } from "@/components/ui/badge";
import { DataTable, facetFilter } from "@/components/ui/data-table";
import { RelativeTime } from "@/components/ui/relative-time";
import { api } from "@/lib/api/client";
import { formatDateTime } from "@/lib/format";
import { useLocale } from "@/lib/i18n/client";
import { monitoringMessages } from "@/lib/i18n/console/monitoring";
import type { Locale } from "@/lib/i18n/locale";
import type { Session } from "@/lib/types";

type Row = Session & { userName: string };

const columns = (t: (typeof monitoringMessages)[Locale], locale: Locale): ColumnDef<Row, any>[] => [
  {
    id: "user",
    accessorFn: (s) => s.userName,
    header: t.user,
    meta: { label: t.user, hideable: false },
    cell: ({ getValue }) => <span className="font-medium whitespace-nowrap text-fg">{getValue<string>()}</span>,
  },
  {
    id: "device",
    accessorFn: (s) => `${s.device} · ${s.browser}`,
    header: t.device,
    meta: { label: t.device },
    cell: ({ row, getValue }) => (
      <span className="inline-flex items-center gap-2 whitespace-nowrap">
        {getValue<string>()}
        {row.original.current ? <Badge tone="success">{t.sessions.thisBrowser}</Badge> : null}
        {row.original.impersonationId ? <Badge tone="warning">{t.impersonation}</Badge> : null}
      </span>
    ),
  },
  {
    id: "ip",
    accessorFn: (s) => s.ip,
    header: t.ipAddress,
    meta: { label: t.ipAddress },
    cell: ({ getValue }) => <span className="font-mono text-code-sm whitespace-nowrap text-fg-3">{getValue<string>()}</span>,
  },
  {
    id: "location",
    accessorFn: (s) => s.location,
    header: t.location,
    meta: { label: t.location },
    cell: ({ getValue }) => <span className="whitespace-nowrap text-fg-3">{getValue<string>()}</span>,
  },
  {
    id: "method",
    accessorFn: (s) => s.method,
    header: t.method,
    filterFn: facetFilter,
    meta: { label: t.method },
    cell: ({ row }) => <span className="whitespace-nowrap">{t.methods[row.original.method]}</span>,
  },
  {
    id: "created",
    accessorFn: (s) => new Date(s.createdAt).getTime(),
    header: t.sessions.signedIn,
    meta: { label: t.sessions.signedIn, align: "right" },
    cell: ({ row }) => (
      <span className="tnum whitespace-nowrap text-fg-3" title={formatDateTime(row.original.createdAt, locale)}>
        <RelativeTime iso={row.original.createdAt} />
      </span>
    ),
  },
  {
    id: "lastActive",
    accessorFn: (s) => new Date(s.lastActiveAt).getTime(),
    header: t.sessions.lastActive,
    meta: { label: t.sessions.lastActive, align: "right" },
    cell: ({ row }) => (
      <span className="tnum whitespace-nowrap text-fg-3" title={formatDateTime(row.original.lastActiveAt, locale)}>
        <RelativeTime iso={row.original.lastActiveAt} />
      </span>
    ),
  },
];

export function SessionsTable({ sessions, userNames }: { sessions: Session[]; userNames: Record<string, string> }) {
  const locale = useLocale();
  const t = monitoringMessages[locale];
  const { run, toast } = useMutation();
  const rows = sessions.map((s) => ({ ...s, userName: userNames[s.userId] ?? t.unknownUser }));
  const methods = [...new Set(sessions.map((s) => s.method))];

  function revoke(selected: Row[]) {
    const targets = selected.filter((s) => !s.current);
    if (targets.length === 0) {
      toast({ title: t.sessions.current, description: t.sessions.currentHint });
      return;
    }
    return run(
      "revoke",
      () => Promise.all(targets.map((s) => api(`/sessions/${s.id}`, { method: "DELETE" }))),
      () =>
        toast({
          title: t.sessions.revoked(targets.length),
          description: targets.length < selected.length ? t.sessions.skipped : t.sessions.signedOut,
        }),
    );
  }

  return (
    <DataTable
      data={rows}
      columns={columns(t, locale)}
      getRowId={(s) => s.id}
      label={t.sessions.label}
      noun={t.sessions.noun}
      storageKey="sessions"
      searchText={(s) => `${s.userName} ${s.device} ${s.browser} ${s.ip} ${s.location}`}
      searchPlaceholder={t.sessions.searchPlaceholder}
      facets={[{ column: "method", label: t.method, options: methods.map((m) => ({ value: m, label: t.methods[m] })) }]}
      rowActions={[{ label: t.sessions.revoke, icon: MonitorX, danger: true, onSelect: (s) => revoke([s]) }]}
      bulkActions={[{ label: t.revokeSessions, icon: MonitorX, danger: true, onSelect: revoke }]}
    />
  );
}
