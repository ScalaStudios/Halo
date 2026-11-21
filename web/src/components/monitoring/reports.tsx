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
import { pluralize } from "@/lib/format";
import { METHOD, STATUS, STRENGTH } from "@/lib/labels";
import type { ApplicationUsageReport, InactivePeopleReport, MethodRegistrationReport } from "@/lib/types";

type Registration = MethodRegistrationReport["people"][number];
type Inactive = InactivePeopleReport["people"][number];
type AppUsage = ApplicationUsageReport["applications"][number];

export function Bar({ value, max }: { value: number; max: number }) {
  return (
    <span aria-hidden="true" className="block h-2 w-full overflow-hidden rounded-full bg-n800">
      <span className="block h-full rounded-full bg-fg-2" style={{ width: `${max ? (value / max) * 100 : 0}%` }} />
    </span>
  );
}

function lastSignIn(iso: string | null) {
  return <span className="tnum whitespace-nowrap text-fg-3">{iso ? <RelativeTime iso={iso} /> : "Never"}</span>;
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

function lastSignInColumn<T extends { lastSignInAt: string | null }>(header = "Last sign-in"): ColumnDef<T, any> {
  return {
    id: "lastSignIn",
    accessorFn: (p) => (p.lastSignInAt ? new Date(p.lastSignInAt).getTime() : 0),
    header,
    meta: { label: header, align: "right" },
    cell: ({ row }) => lastSignIn(row.original.lastSignInAt),
  };
}

const registrationColumns: ColumnDef<Registration, any>[] = [
  { id: "name", accessorFn: (p) => p.name, header: "Name", meta: { label: "Name", hideable: false }, cell: ({ row }) => person(row.original.name, row.original.email, row.original.id) },
  {
    id: "methods",
    accessorFn: (p) => p.methods.length,
    header: "Registered methods",
    meta: { label: "Registered methods" },
    cell: ({ row }) =>
      row.original.methods.length ? (
        <span className="whitespace-nowrap">{row.original.methods.map((m) => METHOD[m].label).join(", ")}</span>
      ) : (
        <span className="text-fg-3">None</span>
      ),
  },
  {
    id: "strength",
    accessorFn: (p) => p.strength,
    header: "Authentication",
    meta: { label: "Authentication" },
    cell: ({ row }) => <Badge tone={STRENGTH[row.original.strength].tone}>{STRENGTH[row.original.strength].label}</Badge>,
  },
  { id: "department", accessorFn: (p) => p.department, header: "Department", meta: { label: "Department" } },
  lastSignInColumn(),
];

export function RegistrationTable({ people, exportPath }: { people: Registration[]; exportPath: string }) {
  return (
    <DataTable
      data={people}
      columns={registrationColumns}
      getRowId={(p) => p.id}
      label="People and their sign-in methods"
      noun={["person", "people"]}
      storageKey="report-methods"
      searchText={(p) => `${p.name} ${p.email} ${p.department}`}
      searchPlaceholder="Search name, email or department"
      toolbarEnd={<ExportButton path={exportPath} label="Export CSV" variant="secondary" size="sm" recorded={false} />}
    />
  );
}

const inactiveColumns: ColumnDef<Inactive, any>[] = [
  { id: "name", accessorFn: (p) => p.name, header: "Name", meta: { label: "Name", hideable: false }, cell: ({ row }) => person(row.original.name, row.original.email, row.original.id) },
  {
    id: "status",
    accessorFn: (p) => p.status,
    header: "Status",
    meta: { label: "Status" },
    cell: ({ row }) => (
      <span className="inline-flex items-center gap-2 whitespace-nowrap">
        <StatusDot tone={STATUS[row.original.status].tone} />
        {STATUS[row.original.status].label}
      </span>
    ),
  },
  lastSignInColumn(),
  {
    id: "manager",
    accessorFn: (p) => p.manager?.name ?? "",
    header: "Manager",
    meta: { label: "Manager" },
    cell: ({ row }) => (row.original.manager ? <span className="whitespace-nowrap">{row.original.manager.name}</span> : <span className="text-fg-3">—</span>),
  },
  {
    id: "groups",
    accessorFn: (p) => p.groups.map((g) => g.name).join(", "),
    header: "Groups",
    meta: { label: "Groups" },
    cell: ({ getValue }) => <span className="block max-w-64 truncate text-fg-3">{getValue<string>() || "—"}</span>,
  },
];

export function InactiveTable({ people, exportPath, canRevoke, canSuspend }: { people: Inactive[]; exportPath: string; canRevoke: boolean; canSuspend: boolean }) {
  const { run, toast, router } = useMutation();
  const [suspending, setSuspending] = useState<Inactive[]>([]);

  function revokeSessions(rows: Inactive[]) {
    return run(
      "revoke",
      () => api<{ succeeded: number; failed: number }>("/users/bulk/revoke-sessions", { body: { userIds: rows.map((p) => p.id) } }),
      ({ succeeded, failed }) =>
        toast({
          title: failed ? "Some sessions were not revoked" : "Sessions revoked",
          description: `${pluralize(succeeded, "person", "people")} must sign in again everywhere.${failed ? ` ${pluralize(failed, "person", "people")} could not be changed.` : ""}`,
          tone: failed ? "danger" : undefined,
        }),
    );
  }

  const bulkActions: BulkAction<Inactive>[] = [
    ...(canRevoke ? [{ label: "Revoke sessions", icon: MonitorX, onSelect: revokeSessions }] : []),
    ...(canSuspend ? [{ label: "Suspend", icon: Ban, danger: true, onSelect: setSuspending }] : []),
  ];

  return (
    <>
      <DataTable
        data={people}
        columns={inactiveColumns}
        getRowId={(p) => p.id}
        label="Inactive people"
        noun={["person", "people"]}
        storageKey="report-inactive"
        searchText={(p) => `${p.name} ${p.email} ${p.department} ${p.manager?.name ?? ""} ${p.groups.map((g) => g.name).join(" ")}`}
        searchPlaceholder="Search name, email, manager or group"
        onRowOpen={(p) => router.push(`/admin/users/${p.id}`)}
        bulkActions={bulkActions.length ? bulkActions : undefined}
        toolbarEnd={<ExportButton path={exportPath} label="Export CSV" variant="secondary" size="sm" recorded={false} />}
      />
      <SuspendDialog users={suspending} onClose={() => setSuspending([])} />
    </>
  );
}

function applicationColumns(max: number): ColumnDef<AppUsage, any>[] {
  return [
    {
      id: "name",
      accessorFn: (a) => a.name,
      header: "Application",
      meta: { label: "Application", hideable: false },
      cell: ({ row }) => (
        <span className="flex min-w-40 items-center gap-2">
          <Link href={`/admin/applications/${row.original.id}`} onClick={(event) => event.stopPropagation()} className="truncate font-medium text-fg hover:underline">
            {row.original.name}
          </Link>
          {row.original.status === "disabled" ? <Badge>Disabled</Badge> : null}
        </span>
      ),
    },
    {
      id: "signIns",
      accessorFn: (a) => a.signIns,
      header: "Sign-ins",
      meta: { label: "Sign-ins" },
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
      header: "People",
      meta: { label: "People", align: "right" },
      cell: ({ row }) => (
        <span className="tnum whitespace-nowrap">
          {row.original.people} <span className="text-fg-3">of {row.original.assigned} assigned</span>
        </span>
      ),
    },
    lastSignInColumn("Last sign-in in period"),
    { id: "used", accessorFn: (a) => (a.signIns ? "used" : "unused"), filterFn: facetFilter, header: "Used", meta: { label: "Used", hideable: false } },
  ];
}

export function ApplicationsTable({ applications, exportPath }: { applications: AppUsage[]; exportPath: string }) {
  const router = useRouter();
  return (
    <DataTable
      data={applications}
      columns={applicationColumns(Math.max(0, ...applications.map((a) => a.signIns)))}
      getRowId={(a) => a.id}
      label="Application usage"
      noun={["application", "applications"]}
      storageKey="report-applications"
      searchText={(a) => a.name}
      searchPlaceholder="Search applications"
      views={[
        { id: "all", label: "All applications", filters: [] },
        { id: "unused", label: "No sign-ins", filters: [{ id: "used", value: ["unused"] }] },
      ]}
      initialVisibility={{ used: false }}
      onRowOpen={(a) => router.push(`/admin/applications/${a.id}`)}
      toolbarEnd={<ExportButton path={exportPath} label="Export CSV" variant="secondary" size="sm" recorded={false} />}
    />
  );
}

export function GroupFilter({ groups, value, query }: { groups: { id: string; name: string }[]; value: string; query: Record<string, string> }) {
  const router = useRouter();
  const [pending, start] = useTransition();
  return (
    <div className="w-full sm:w-56">
      {pending ? <NavigationProgress /> : null}
      <Select
        size="sm"
        aria-label="Group"
        value={value}
        onChange={(event) => {
          const next = new URLSearchParams(query);
          if (event.target.value) next.set("group", event.target.value);
          start(() => router.push(`/admin/reports?${next}`, { scroll: false }));
        }}
      >
        <option value="">All groups</option>
        {groups.map((g) => (
          <option key={g.id} value={g.id}>
            {g.name}
          </option>
        ))}
      </Select>
    </div>
  );
}
