"use client";

import Link from "next/link";
import type { ColumnDef } from "@tanstack/react-table";
import { CircleCheck, CircleSlash, UserRound } from "lucide-react";
import { useMutation } from "@/components/console/use-mutation";
import { Badge, StatusDot, type Tone } from "@/components/ui/badge";
import { DataTable, facetFilter, type SavedView } from "@/components/ui/data-table";
import { RelativeTime } from "@/components/ui/relative-time";
import { api } from "@/lib/api/client";
import { formatDateTime } from "@/lib/format";
import { useLocale } from "@/lib/i18n/client";
import { policyMessages } from "@/lib/i18n/console/policy";
import type { Locale } from "@/lib/i18n/locale";
import { RISK } from "@/lib/labels";
import type { RiskEvent, RiskEventType } from "@/lib/policy-types";
import type { PolicyCopy } from "./shared";

type Row = RiskEvent & { user: string };

const STATUS: Record<RiskEvent["status"], Tone> = {
  open: "warning",
  resolved: "success",
  dismissed: "neutral",
};

function riskColumns(copy: PolicyCopy, locale: Locale): ColumnDef<Row, any>[] {
  const t = copy.risk;
  return [
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
      id: "user",
      accessorFn: (e) => e.user,
      header: t.user,
      meta: { label: t.user },
      cell: ({ row }) =>
        row.original.userId ? (
          <Link href={`/admin/users/${row.original.userId}`} className="font-medium whitespace-nowrap text-fg underline-offset-3 hover:underline">
            {row.original.user}
          </Link>
        ) : (
          <span className="text-fg-3">{copy.unknownUser}</span>
        ),
    },
    {
      id: "type",
      accessorFn: (e) => e.type,
      header: t.event,
      filterFn: facetFilter,
      meta: { label: t.event },
      cell: ({ row }) => (
        <span className="flex min-w-64 flex-col py-2">
          <span className="text-fg">{copy.labels.riskType[row.original.type]}</span>
          <span className="text-caption text-fg-3">
            {row.original.detail}
            {row.original.signInId ? (
              <>
                {" "}
                <Link href={`/admin/sign-ins?event=${row.original.signInId}`} className="text-link underline underline-offset-3 hover:text-ember">
                  {t.viewSignIn}
                </Link>
              </>
            ) : null}
          </span>
        </span>
      ),
    },
    {
      id: "level",
      accessorFn: (e) => e.level,
      header: t.risk,
      filterFn: facetFilter,
      meta: { label: t.risk },
      cell: ({ row }) => <Badge tone={RISK[row.original.level].tone}>{copy.labels.risk[row.original.level]}</Badge>,
    },
    {
      id: "ip",
      accessorFn: (e) => e.ip,
      header: t.ip,
      meta: { label: t.ip },
      cell: ({ row }) => <span className="font-mono text-code-sm whitespace-nowrap text-fg-3">{row.original.ip || "—"}</span>,
    },
    {
      id: "device",
      accessorFn: (e) => e.device,
      header: t.device,
      meta: { label: t.device },
      cell: ({ row }) => <span className="whitespace-nowrap text-fg-3">{row.original.device || "—"}</span>,
    },
    {
      id: "status",
      accessorFn: (e) => e.status,
      header: t.statusLabel,
      filterFn: facetFilter,
      meta: { label: t.statusLabel },
      cell: ({ row }) => (
        <span className="inline-flex items-center gap-2 whitespace-nowrap">
          <StatusDot tone={STATUS[row.original.status]} />
          {t.status[row.original.status]}
        </span>
      ),
    },
  ];
}

export function RiskTable({ events, userNames }: { events: RiskEvent[]; userNames: Record<string, string> }) {
  const locale = useLocale();
  const copy = policyMessages[locale];
  const t = copy.risk;
  const { run, toast, router } = useMutation();
  const rows = events.map((e) => ({ ...e, user: (e.userId && userNames[e.userId]) || copy.unknownUser }));
  const views: SavedView[] = [
    { id: "open", label: t.status.open, filters: [{ id: "status", value: ["open"] }] },
    { id: "resolved", label: t.status.resolved, filters: [{ id: "status", value: ["resolved"] }] },
    { id: "dismissed", label: t.status.dismissed, filters: [{ id: "status", value: ["dismissed"] }] },
    { id: "all", label: t.all, filters: [] },
  ];

  function close(selected: Row[], action: "resolve" | "dismiss") {
    return run(
      action,
      () => Promise.all(selected.map((e) => api(`/risk-events/${e.id}/${action}`, { method: "POST" }))),
      () =>
        toast({
          title: t.closed(selected.length, action),
          description: action === "resolve" ? t.resolvedDescription : t.dismissedDescription,
        }),
    );
  }

  return (
    <DataTable
      data={rows}
      columns={riskColumns(copy, locale)}
      getRowId={(e) => e.id}
      label={t.label}
      noun={t.noun}
      storageKey="risk-events"
      searchText={(e) => `${e.user} ${copy.labels.riskType[e.type]} ${e.detail} ${e.ip} ${e.device}`}
      searchPlaceholder={t.search}
      views={views}
      facets={[
        { column: "type", label: t.event, options: (Object.keys(copy.labels.riskType) as RiskEventType[]).map((value) => ({ value, label: copy.labels.riskType[value] })) },
        { column: "level", label: t.risk, options: (["high", "medium"] as const).map((value) => ({ value, label: copy.labels.risk[value] })) },
      ]}
      rowActions={[
        { label: t.resolve, icon: CircleCheck, onSelect: (e) => close([e], "resolve") },
        { label: t.dismiss, icon: CircleSlash, onSelect: (e) => close([e], "dismiss") },
        { label: t.openUser, icon: UserRound, separatorBefore: true, onSelect: (e) => (e.userId ? router.push(`/admin/users/${e.userId}`) : undefined) },
      ]}
      bulkActions={[
        { label: t.resolve, icon: CircleCheck, onSelect: (selected) => close(selected, "resolve") },
        { label: t.dismiss, icon: CircleSlash, onSelect: (selected) => close(selected, "dismiss") },
      ]}
    />
  );
}
