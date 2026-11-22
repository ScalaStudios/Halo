"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import type { ColumnDef } from "@tanstack/react-table";
import { Ban, MonitorX } from "lucide-react";
import { useState, useTransition } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { SuspendDialog } from "@/components/directory/user-actions";
import { ExportButton } from "@/components/settings/export-button";
import { Badge, StatusDot } from "@/components/ui/badge";
import { DataTable, facetFilter, type BulkAction } from "@/components/ui/data-table";
import { Select } from "@/components/ui/input";
import { NavigationProgress } from "@/components/ui/navigation-progress";
import { RelativeTime } from "@/components/ui/relative-time";
import { api } from "@/lib/api/client";
import { useMessages } from "@/lib/i18n/client";
import { monitoringMessages } from "@/lib/i18n/console/monitoring";
import type { Locale } from "@/lib/i18n/locale";
import { STATUS, STRENGTH } from "@/lib/labels";
import type { ApplicationUsageReport, InactivePeopleReport, MethodRegistrationReport } from "@/lib/types";

type Registration = MethodRegistrationReport["people"][number];
type Inactive = InactivePeopleReport["people"][number];
type AppUsage = ApplicationUsageReport["applications"][number];
type Messages = (typeof monitoringMessages)[Locale];

export function Bar({ value, max }: { value: number; max: number }) {
  return (
    <span aria-hidden="true" className="block h-2 w-full overflow-hidden rounded-full bg-n800">
      <span className="block h-full rounded-full bg-fg-2" style={{ width: `${max ? (value / max) * 100 : 0}%` }} />
    </span>
  );
}

function lastSignIn(iso: string | null, never: string) {
  return <span className="tnum whitespace-nowrap text-fg-3">{iso ? <RelativeTime iso={iso} /> : never}</span>;
}

function person(name: string, email: string, id: string) {
  return (
    <span className="flex min-w-48 flex-col">
      <Link href={`/admin/users/${id}`} onClick={(event) => event.stopPropagation()} className="truncate font-medium text-fg hover:underline">
        {name}
      </Link>
      <span className="truncate text-caption text-fg-3">{email}</span>
    </span>
  );
}

function lastSignInColumn<T extends { lastSignInAt: string | null }>(header: string, never: string): ColumnDef<T, any> {
  return {
    id: "lastSignIn",
    accessorFn: (p) => (p.lastSignInAt ? new Date(p.lastSignInAt).getTime() : 0),
    header,
    meta: { label: header, align: "right" },
    cell: ({ row }) => lastSignIn(row.original.lastSignInAt, never),
  };
}

const registrationColumns = (t: Messages): ColumnDef<Registration, any>[] => [
  { id: "name", accessorFn: (p) => p.name, header: t.reports.name, meta: { label: t.reports.name, hideable: false }, cell: ({ row }) => person(row.original.name, row.original.email, row.original.id) },
  {
    id: "methods",
    accessorFn: (p) => p.methods.length,
    header: t.reports.registeredMethods,
    meta: { label: t.reports.registeredMethods },
    cell: ({ row }) =>
      row.original.methods.length ? (
        <span className="whitespace-nowrap">{row.original.methods.map((m) => t.methods[m]).join(", ")}</span>
      ) : (
        <span className="text-fg-3">{t.reports.none}</span>
      ),
  },
  {
    id: "strength",
    accessorFn: (p) => p.strength,
    header: t.reports.authentication,
    meta: { label: t.reports.authentication },
    cell: ({ row }) => <Badge tone={STRENGTH[row.original.strength].tone}>{t.strengths[row.original.strength]}</Badge>,
  },
  { id: "department", accessorFn: (p) => p.department, header: t.reports.department, meta: { label: t.reports.department } },
  lastSignInColumn(t.reports.lastSignIn, t.reports.never),
];

export function RegistrationTable({ people, exportPath }: { people: Registration[]; exportPath: string }) {
  const t = useMessages(monitoringMessages);
  return (
    <DataTable
      data={people}
      columns={registrationColumns(t)}
      getRowId={(p) => p.id}
      label={t.reports.registrationLabel}
      noun={t.reports.person}
      storageKey="report-methods"
      searchText={(p) => `${p.name} ${p.email} ${p.department}`}
      searchPlaceholder={t.reports.registrationSearch}
      toolbarEnd={<ExportButton path={exportPath} label={t.exportCsv} variant="secondary" size="sm" recorded={false} />}
    />
  );
}

const inactiveColumns = (t: Messages): ColumnDef<Inactive, any>[] => [
  { id: "name", accessorFn: (p) => p.name, header: t.reports.name, meta: { label: t.reports.name, hideable: false }, cell: ({ row }) => person(row.original.name, row.original.email, row.original.id) },
  {
    id: "status",
    accessorFn: (p) => p.status,
    header: t.reports.status,
    meta: { label: t.reports.status },
    cell: ({ row }) => (
      <span className="inline-flex items-center gap-2 whitespace-nowrap">
        <StatusDot tone={STATUS[row.original.status].tone} />
        {t.statuses[row.original.status]}
      </span>
    ),
  },
  lastSignInColumn(t.reports.lastSignIn, t.reports.never),
  {
    id: "manager",
    accessorFn: (p) => p.manager?.name ?? "",
    header: t.reports.manager,
    meta: { label: t.reports.manager },
    cell: ({ row }) => (row.original.manager ? <span className="whitespace-nowrap">{row.original.manager.name}</span> : <span className="text-fg-3">—</span>),
  },
  {
    id: "groups",
    accessorFn: (p) => p.groups.map((g) => g.name).join(", "),
    header: t.reports.groups,
    meta: { label: t.reports.groups },
    cell: ({ getValue }) => <span className="block max-w-64 truncate text-fg-3">{getValue<string>() || "—"}</span>,
  },
];

export function InactiveTable({ people, exportPath, canRevoke, canSuspend }: { people: Inactive[]; exportPath: string; canRevoke: boolean; canSuspend: boolean }) {
  const t = useMessages(monitoringMessages);
  const { run, toast, router } = useMutation();
  const [suspending, setSuspending] = useState<Inactive[]>([]);

  function revokeSessions(rows: Inactive[]) {
    return run(
      "revoke",
      () => api<{ succeeded: number; failed: number }>("/users/bulk/revoke-sessions", { body: { userIds: rows.map((p) => p.id) } }),
      ({ succeeded, failed }) =>
        toast({
          title: failed ? t.reports.notRevoked : t.reports.revoked,
          description: t.reports.revokedHint(succeeded, failed),
          tone: failed ? "danger" : undefined,
        }),
    );
  }

  const bulkActions: BulkAction<Inactive>[] = [
    ...(canRevoke ? [{ label: t.revokeSessions, icon: MonitorX, onSelect: revokeSessions }] : []),
    ...(canSuspend ? [{ label: t.reports.suspend, icon: Ban, danger: true, onSelect: setSuspending }] : []),
  ];

  return (
    <>
      <DataTable
        data={people}
        columns={inactiveColumns(t)}
        getRowId={(p) => p.id}
        label={t.reports.inactiveLabel}
        noun={t.reports.person}
        storageKey="report-inactive"
        searchText={(p) => `${p.name} ${p.email} ${p.department} ${p.manager?.name ?? ""} ${p.groups.map((g) => g.name).join(" ")}`}
        searchPlaceholder={t.reports.inactiveSearch}
        onRowOpen={(p) => router.push(`/admin/users/${p.id}`)}
        bulkActions={bulkActions.length ? bulkActions : undefined}
        toolbarEnd={<ExportButton path={exportPath} label={t.exportCsv} variant="secondary" size="sm" recorded={false} />}
      />
      <SuspendDialog users={suspending} onClose={() => setSuspending([])} />
    </>
  );
}

function applicationColumns(max: number, t: Messages): ColumnDef<AppUsage, any>[] {
  return [
    {
      id: "name",
      accessorFn: (a) => a.name,
      header: t.application,
      meta: { label: t.application, hideable: false },
      cell: ({ row }) => (
        <span className="flex min-w-40 items-center gap-2">
          <Link href={`/admin/applications/${row.original.id}`} onClick={(event) => event.stopPropagation()} className="truncate font-medium text-fg hover:underline">
            {row.original.name}
          </Link>
          {row.original.status === "disabled" ? <Badge>{t.reports.disabled}</Badge> : null}
        </span>
      ),
    },
    {
      id: "signIns",
      accessorFn: (a) => a.signIns,
      header: t.signIns,
      meta: { label: t.signIns },
      cell: ({ row }) => (
        <span className="flex min-w-40 items-center gap-3">
          <span className="tnum w-12 shrink-0 text-right">{row.original.signIns}</span>
          <Bar value={row.original.signIns} max={max} />
        </span>
      ),
    },
    {
      id: "people",
      accessorFn: (a) => a.people,
      header: t.reports.people,
      meta: { label: t.reports.people, align: "right" },
      cell: ({ row }) => (
        <span className="tnum whitespace-nowrap">
          {row.original.people} <span className="text-fg-3">{t.reports.ofAssigned(row.original.assigned)}</span>
        </span>
      ),
    },
    lastSignInColumn(t.reports.lastSignInInPeriod, t.reports.never),
    { id: "used", accessorFn: (a) => (a.signIns ? "used" : "unused"), filterFn: facetFilter, header: t.reports.used, meta: { label: t.reports.used, hideable: false } },
  ];
}

export function ApplicationsTable({ applications, exportPath }: { applications: AppUsage[]; exportPath: string }) {
  const router = useRouter();
  const t = useMessages(monitoringMessages);
  return (
    <DataTable
      data={applications}
      columns={applicationColumns(Math.max(0, ...applications.map((a) => a.signIns)), t)}
      getRowId={(a) => a.id}
      label={t.reports.applicationsLabel}
      noun={t.reports.application}
      storageKey="report-applications"
      searchText={(a) => a.name}
      searchPlaceholder={t.reports.applicationsSearch}
      views={[
        { id: "all", label: t.reports.allApplications, filters: [] },
        { id: "unused", label: t.reports.noSignIns, filters: [{ id: "used", value: ["unused"] }] },
      ]}
      initialVisibility={{ used: false }}
      onRowOpen={(a) => router.push(`/admin/applications/${a.id}`)}
      toolbarEnd={<ExportButton path={exportPath} label={t.exportCsv} variant="secondary" size="sm" recorded={false} />}
    />
  );
}

export function GroupFilter({ groups, value, query }: { groups: { id: string; name: string }[]; value: string; query: Record<string, string> }) {
  const router = useRouter();
  const t = useMessages(monitoringMessages).reports;
  const [pending, start] = useTransition();
  return (
    <div className="w-full sm:w-56">
      {pending ? <NavigationProgress /> : null}
      <Select
        size="sm"
        aria-label={t.group}
        value={value}
        onChange={(event) => {
          const next = new URLSearchParams(query);
          if (event.target.value) next.set("group", event.target.value);
          start(() => router.push(`/admin/reports?${next}`, { scroll: false }));
        }}
      >
        <option value="">{t.allGroups}</option>
        {groups.map((g) => (
          <option key={g.id} value={g.id}>
            {g.name}
          </option>
        ))}
      </Select>
    </div>
  );
}
