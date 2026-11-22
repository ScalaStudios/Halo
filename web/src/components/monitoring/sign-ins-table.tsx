"use client";

import Link from "next/link";
import type { ColumnDef } from "@tanstack/react-table";
import { CircleCheck, Download, TriangleAlert } from "lucide-react";
import { useState } from "react";
import { Badge, StatusDot } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CopyButton } from "@/components/ui/copy-button";
import { DataTable, facetFilter, type SavedView } from "@/components/ui/data-table";
import { DescriptionList } from "@/components/ui/description-list";
import { Dialog } from "@/components/ui/dialog";
import { RelativeTime } from "@/components/ui/relative-time";
import { cn } from "@/lib/cn";
import { formatDateTime } from "@/lib/format";
import { useLocale } from "@/lib/i18n/client";
import { frameMessages } from "@/lib/i18n/console/frame";
import { monitoringMessages } from "@/lib/i18n/console/monitoring";
import type { Locale } from "@/lib/i18n/locale";
import { RESULT, RISK, signInApp, signInUser } from "@/lib/labels";
import type { SignInEvent } from "@/lib/types";

type Row = SignInEvent & { user: string; app: string };

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
    id: "user",
    accessorFn: (e) => e.user,
    header: t.user,
    meta: { label: t.user, hideable: false },
    cell: ({ row }) =>
      row.original.userId ? (
        <Link
          href={`/admin/users/${row.original.userId}`}
          onClick={(event) => event.stopPropagation()}
          className="font-medium whitespace-nowrap text-fg hover:underline"
        >
          {row.original.user}
        </Link>
      ) : (
        <span className="font-medium whitespace-nowrap text-fg">{row.original.user}</span>
      ),
  },
  {
    id: "application",
    accessorFn: (e) => e.app,
    header: t.application,
    filterFn: facetFilter,
    meta: { label: t.application },
    cell: ({ row }) => <span className="whitespace-nowrap">{row.original.app}</span>,
  },
  {
    id: "result",
    accessorFn: (e) => e.result,
    header: t.result,
    filterFn: facetFilter,
    meta: { label: t.result },
    cell: ({ row }) => (
      <span className="inline-flex items-center gap-2 whitespace-nowrap">
        <StatusDot tone={RESULT[row.original.result].tone} />
        {t.results[row.original.result]}
      </span>
    ),
  },
  {
    id: "method",
    accessorFn: (e) => e.method,
    header: t.method,
    filterFn: facetFilter,
    meta: { label: t.method },
    cell: ({ row }) => (
      <span className="inline-flex items-center gap-2 whitespace-nowrap">
        {t.methods[row.original.method]}
        {row.original.impersonatorId ? <Badge tone="warning">{t.impersonation}</Badge> : null}
      </span>
    ),
  },
  {
    id: "risk",
    accessorFn: (e) => e.risk,
    header: t.risk,
    filterFn: facetFilter,
    sortingFn: (a, b) => Object.keys(RISK).indexOf(a.original.risk) - Object.keys(RISK).indexOf(b.original.risk),
    meta: { label: t.risk },
    cell: ({ row }) =>
      row.original.risk === "none" ? <span className="text-fg-3">—</span> : <Badge tone={RISK[row.original.risk].tone}>{t.risks[row.original.risk]}</Badge>,
  },
  {
    id: "location",
    accessorFn: (e) => e.location,
    header: t.location,
    meta: { label: t.location },
    cell: ({ row }) => <span className="whitespace-nowrap text-fg-3">{row.original.location}</span>,
  },
  {
    id: "ip",
    accessorFn: (e) => e.ip,
    header: t.ipAddress,
    meta: { label: t.ipAddress },
    cell: ({ row }) => <span className="font-mono text-code-sm whitespace-nowrap text-fg-3">{row.original.ip}</span>,
  },
  {
    id: "device",
    accessorFn: (e) => e.device,
    header: t.device,
    meta: { label: t.device },
    cell: ({ row }) => <span className="whitespace-nowrap text-fg-3">{row.original.device}</span>,
  },
];

const views = (t: Messages): SavedView[] => [
  { id: "all", label: t.signInLog.views.all, filters: [] },
  { id: "failures", label: t.signInLog.views.failures, filters: [{ id: "result", value: ["failure", "interrupted"] }] },
  { id: "risky", label: t.signInLog.views.risky, filters: [{ id: "risk", value: ["high", "medium"] }] },
];

function csv(events: Row[]) {
  const head = ["time", "user", "email", "application", "result", "reason", "method", "risk", "location", "ip", "device", "event_id"];
  const lines = events.map((e) =>
    [e.time, e.user, e.email, e.app, e.result, e.reason ?? "", e.method, e.risk, e.location, e.ip, e.device, e.id]
      .map((value) => `"${(/^[=+\-@\t\r]/.test(value) ? `'${value}` : value).replaceAll('"', '""')}"`)
      .join(","),
  );
  return [head.join(","), ...lines].join("\n");
}

function download(events: Row[]) {
  const url = URL.createObjectURL(new Blob([csv(events)], { type: "text/csv;charset=utf-8" }));
  const link = document.createElement("a");
  link.href = url;
  link.download = "halo-sign-ins.csv";
  link.click();
  URL.revokeObjectURL(url);
}

export function SignInsTable({
  events,
  userNames,
  appNames,
  initialEventId,
  initialView,
}: {
  events: SignInEvent[];
  userNames: Record<string, string>;
  appNames: Record<string, string>;
  initialEventId?: string;
  initialView?: string;
}) {
  const locale = useLocale();
  const t = monitoringMessages[locale];
  const [openId, setOpenId] = useState(initialEventId);
  const rows = events.map((e) => ({ ...e, user: signInUser(e, userNames), app: signInApp(e, appNames) }));
  const selected = rows.find((e) => e.id === openId);
  const apps = [...new Set(rows.map((e) => e.app))].sort();
  const methods = [...new Set(events.map((e) => e.method))];

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
        label={frameMessages[locale].nav.items.signIns.label}
        noun={t.signInLog.noun}
        storageKey="sign-ins"
        searchText={(e) => `${e.user} ${e.email} ${e.app} ${e.ip} ${e.location} ${e.id}`}
        searchPlaceholder={t.signInLog.searchPlaceholder}
        views={views(t)}
        initialViewId={initialView}
        initialVisibility={{ device: false }}
        facets={[
          { column: "result", label: t.result, options: Object.entries(t.results).map(([value, label]) => ({ value, label })) },
          { column: "risk", label: t.risk, options: Object.entries(t.risks).map(([value, label]) => ({ value, label })) },
          { column: "application", label: t.application, options: apps.map((app) => ({ value: app, label: app })) },
          { column: "method", label: t.method, options: methods.map((m) => ({ value: m, label: t.methods[m] })) },
        ]}
        onRowOpen={(e) => open(e.id)}
        toolbarEnd={(rows) => (
          <Button size="sm" variant="secondary" onClick={() => download(rows)}>
            <Download aria-hidden="true" size={16} strokeWidth={1.75} />
            {t.exportCsv}
          </Button>
        )}
      />
      <Dialog
        open={Boolean(selected)}
        onOpenChange={(next) => {
          if (!next) open(undefined);
        }}
        width="wide"
        title={selected?.result === "ended" ? t.results.ended : selected ? t.signInLog.signInTo(selected.app) : t.signInLog.signIn}
        description={selected ? `${selected.user} · ${formatDateTime(selected.time, locale)}` : undefined}
      >
        {selected ? <SignInDetail event={selected} t={t} /> : null}
      </Dialog>
    </>
  );
}

function SignInDetail({ event, t }: { event: Row; t: Messages }) {
  const copy = t.signInLog;
  const guidance = event.reason ? (copy.guidance as Record<string, { why: string; next: string }>)[event.reason] : undefined;
  const failed = event.result === "failure";

  return (
    <div className="flex flex-col gap-6 pb-4">
      {event.result === "success" ? (
        <div className="flex items-center gap-3 rounded-md border border-success/20 bg-success-container p-4">
          <CircleCheck aria-hidden="true" size={20} strokeWidth={1.75} className="shrink-0 text-success" />
          <p className="text-body-sm text-fg">
            <span className="font-semibold">{copy.signedIn}</span>
            {copy.withMethod(t.methods[event.method])}
            {event.impersonatorId ? <span lang="en"> {event.reason}</span> : null}
          </p>
        </div>
      ) : (
        <div className={cn("flex gap-3 rounded-md border p-4", failed ? "border-danger/20 bg-danger-container" : "border-warning/20 bg-warning-container")}>
          <TriangleAlert aria-hidden="true" size={20} strokeWidth={1.75} className={cn("mt-px shrink-0", failed ? "text-danger" : "text-warning")} />
          <dl className="flex min-w-0 flex-col gap-3">
            <div className="flex flex-col gap-1">
              <dt className="text-label text-fg-3">{failed ? copy.failed : event.result === "ended" ? t.results.ended : copy.interrupted}</dt>
              <dd lang="en" className="text-body-sm font-semibold text-fg">{event.reason}</dd>
            </div>
            {guidance ? (
              <>
                <div className="flex flex-col gap-1">
                  <dt className="text-label text-fg-3">{copy.why}</dt>
                  <dd className="text-body-sm text-fg-2">{guidance.why}</dd>
                </div>
                <div className="flex flex-col gap-1">
                  <dt className="text-label text-fg-3">{copy.next}</dt>
                  <dd className="text-body-sm text-fg-2">{guidance.next}</dd>
                </div>
              </>
            ) : null}
          </dl>
        </div>
      )}

      <DescriptionList
        columns={2}
        items={[
          {
            label: t.user,
            value: event.userId ? (
              <Link href={`/admin/users/${event.userId}`} className="font-medium text-link hover:underline">
                {event.user}
              </Link>
            ) : (
              event.email
            ),
          },
          { label: t.application, value: event.app },
          { label: t.method, value: t.methods[event.method] },
          {
            label: t.risk,
            value: event.risk === "none" ? copy.noneDetected : <Badge tone={RISK[event.risk].tone}>{t.risks[event.risk]}</Badge>,
          },
          {
            label: t.ipAddress,
            value: (
              <span className="-my-2 flex items-center gap-2">
                <span className="font-mono text-code-sm">{event.ip}</span>
                <CopyButton value={event.ip} label={t.copyIp} />
              </span>
            ),
          },
          { label: t.location, value: event.location },
          { label: t.device, value: event.device },
          ...(event.challenge ? [{ label: copy.botCheck, value: copy.challenge[event.challenge] }] : []),
          ...(event.hook ? [{ label: copy.hook, value: copy.hooks[event.hook] }] : []),
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
    </div>
  );
}
