"use client";

import Link from "next/link";
import type { ColumnDef } from "@tanstack/react-table";
import { CircleCheck, CircleSlash, UserRound } from "lucide-react";
import { useMutation } from "@/components/console/use-mutation";
import { Badge, StatusDot, type Tone } from "@/components/ui/badge";
import { DataTable, facetFilter, type SavedView } from "@/components/ui/data-table";
import { RelativeTime } from "@/components/ui/relative-time";
import { api } from "@/lib/api/client";
import { formatDateTime, pluralize } from "@/lib/format";
import { RISK } from "@/lib/labels";
import type { RiskEvent, RiskEventType } from "@/lib/policy-types";
import { RISK_TYPE } from "./shared";

type Row = RiskEvent & { user: string };

const STATUS: Record<RiskEvent["status"], { label: string; tone: Tone }> = {
  open: { label: "Open", tone: "warning" },
  resolved: { label: "Resolved", tone: "success" },
  dismissed: { label: "Dismissed", tone: "neutral" },
};

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
    id: "user",
    accessorFn: (e) => e.user,
    header: "User",
    meta: { label: "User" },
    cell: ({ row }) =>
      row.original.userId ? (
        <Link href={`/admin/users/${row.original.userId}`} className="font-medium whitespace-nowrap text-fg underline-offset-3 hover:underline">
          {row.original.user}
        </Link>
      ) : (
        <span className="text-fg-3">Unknown user</span>
      ),
  },
  {
    id: "type",
    accessorFn: (e) => e.type,
    header: "Event",
    filterFn: facetFilter,
    meta: { label: "Event" },
    cell: ({ row }) => (
      <span className="flex min-w-64 flex-col py-2">
        <span className="text-fg">{RISK_TYPE[row.original.type]}</span>
        <span className="text-caption text-fg-3">
          {row.original.detail}
          {row.original.signInId ? (
            <>
              {" "}
              <Link href={`/admin/sign-ins?event=${row.original.signInId}`} className="text-link underline underline-offset-3 hover:text-ember">
                View sign-in
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
    header: "Risk",
    filterFn: facetFilter,
    meta: { label: "Risk" },
    cell: ({ row }) => <Badge tone={RISK[row.original.level].tone}>{RISK[row.original.level].label}</Badge>,
  },
  {
    id: "ip",
    accessorFn: (e) => e.ip,
    header: "IP address",
    meta: { label: "IP address" },
    cell: ({ row }) => <span className="font-mono text-code-sm whitespace-nowrap text-fg-3">{row.original.ip || "—"}</span>,
  },
  {
    id: "device",
    accessorFn: (e) => e.device,
    header: "Device",
    meta: { label: "Device" },
    cell: ({ row }) => <span className="whitespace-nowrap text-fg-3">{row.original.device || "—"}</span>,
  },
  {
    id: "status",
    accessorFn: (e) => e.status,
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
];

const views: SavedView[] = [
  { id: "open", label: "Open", filters: [{ id: "status", value: ["open"] }] },
  { id: "resolved", label: "Resolved", filters: [{ id: "status", value: ["resolved"] }] },
  { id: "dismissed", label: "Dismissed", filters: [{ id: "status", value: ["dismissed"] }] },
  { id: "all", label: "All risk events", filters: [] },
];

export function RiskTable({ events, userNames }: { events: RiskEvent[]; userNames: Record<string, string> }) {
  const { run, toast, router } = useMutation();
  const rows = events.map((e) => ({ ...e, user: (e.userId && userNames[e.userId]) || "Unknown user" }));

  function close(selected: Row[], action: "resolve" | "dismiss") {
    void run(
      action,
      () => Promise.all(selected.map((e) => api(`/risk-events/${e.id}/${action}`, { method: "POST" }))),
      () =>
        toast({
          title: `${pluralize(selected.length, "risk event")} ${action === "resolve" ? "resolved" : "dismissed"}`,
          description: action === "resolve" ? "Marked as handled. Recorded in the audit log." : "Marked as not a threat. Recorded in the audit log.",
        }),
    );
  }

  return (
    <DataTable
      data={rows}
      columns={columns}
      getRowId={(e) => e.id}
      label="Risk events"
      noun={["risk event", "risk events"]}
      storageKey="risk-events"
      searchText={(e) => `${e.user} ${RISK_TYPE[e.type]} ${e.detail} ${e.ip} ${e.device}`}
      searchPlaceholder="Search user, event or IP"
      views={views}
      facets={[
        { column: "type", label: "Event", options: (Object.keys(RISK_TYPE) as RiskEventType[]).map((value) => ({ value, label: RISK_TYPE[value] })) },
        { column: "level", label: "Risk", options: (["high", "medium"] as const).map((value) => ({ value, label: RISK[value].label })) },
      ]}
      rowActions={[
        { label: "Resolve", icon: CircleCheck, onSelect: (e) => close([e], "resolve") },
        { label: "Dismiss", icon: CircleSlash, onSelect: (e) => close([e], "dismiss") },
        { label: "Open user", icon: UserRound, separatorBefore: true, onSelect: (e) => (e.userId ? router.push(`/admin/users/${e.userId}`) : undefined) },
      ]}
      bulkActions={[
        { label: "Resolve", icon: CircleCheck, onSelect: (selected) => close(selected, "resolve") },
        { label: "Dismiss", icon: CircleSlash, onSelect: (selected) => close(selected, "dismiss") },
      ]}
    />
  );
}
