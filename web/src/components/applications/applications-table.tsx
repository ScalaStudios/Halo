"use client";

import Link from "next/link";
import type { ColumnDef } from "@tanstack/react-table";
import { AppWindow, Copy, Plus, Power, Trash2 } from "lucide-react";
import { useMutation } from "@/components/console/use-mutation";
import { StatusDot } from "@/components/ui/badge";
import { buttonClasses } from "@/components/ui/button";
import { DataTable, facetFilter, type SavedView } from "@/components/ui/data-table";
import { api } from "@/lib/api/client";
import { pluralize } from "@/lib/format";
import type { Application } from "@/lib/types";
import { APP_STATUS, credentialState, ExpiryText, PROTOCOL_LABEL, soonestExpiry, TYPE_LABEL } from "./shared";

const columns: ColumnDef<Application, any>[] = [
  {
    id: "name",
    accessorFn: (a) => a.name,
    header: "Name",
    meta: { label: "Name", hideable: false },
    cell: ({ row }) => (
      <span className="flex max-w-60 min-w-48 flex-col">
        <span className="truncate font-medium text-fg">{row.original.name}</span>
        <span className="truncate text-caption text-fg-3">{row.original.description}</span>
      </span>
    ),
  },
  {
    id: "protocol",
    accessorFn: (a) => a.protocol,
    header: "Protocol",
    filterFn: facetFilter,
    meta: { label: "Protocol" },
    cell: ({ row }) => <span className="whitespace-nowrap">{PROTOCOL_LABEL[row.original.protocol]}</span>,
  },
  {
    id: "type",
    accessorFn: (a) => a.type,
    header: "Type",
    filterFn: facetFilter,
    meta: { label: "Type" },
    cell: ({ row }) => <span className="whitespace-nowrap">{TYPE_LABEL[row.original.type]}</span>,
  },
  {
    id: "status",
    accessorFn: (a) => a.status,
    header: "Status",
    filterFn: facetFilter,
    meta: { label: "Status" },
    cell: ({ row }) => (
      <span className="inline-flex items-center gap-2 whitespace-nowrap">
        <StatusDot tone={APP_STATUS[row.original.status].tone} />
        {APP_STATUS[row.original.status].label}
      </span>
    ),
  },
  {
    id: "users",
    accessorFn: (a) => a.userCount,
    header: "Users",
    meta: { label: "Users", align: "right" },
    cell: ({ getValue }) => <span className="tnum">{getValue<number>()}</span>,
  },
  {
    id: "signIns",
    accessorFn: (a) => a.signIns7d,
    header: "Sign-ins, 7 days",
    meta: { label: "Sign-ins, 7 days", align: "right" },
    cell: ({ getValue }) => <span className="tnum">{getValue<number>().toLocaleString("en-US")}</span>,
  },
  {
    id: "credentials",
    accessorFn: credentialState,
    header: "Credentials",
    filterFn: facetFilter,
    sortingFn: (a, b) => (soonestExpiry(a.original) ?? "9999").localeCompare(soonestExpiry(b.original) ?? "9999"),
    meta: { label: "Credentials" },
    cell: ({ row }) => {
      const iso = soonestExpiry(row.original);
      return iso ? <ExpiryText iso={iso} /> : <span className="text-fg-3">—</span>;
    },
  },
];

const views: SavedView[] = [
  { id: "all", label: "All applications", filters: [] },
  { id: "oidc", label: "OpenID Connect", filters: [{ id: "protocol", value: ["oidc"] }] },
  { id: "saml", label: "SAML", filters: [{ id: "protocol", value: ["saml"] }] },
  { id: "expiring", label: "Expiring credentials", filters: [{ id: "credentials", value: ["expiring"] }] },
  { id: "disabled", label: "Disabled", filters: [{ id: "status", value: ["disabled"] }] },
];

export function ApplicationsTable({ applications, initialView }: { applications: Application[]; initialView?: string }) {
  const { run, toast, router } = useMutation();

  function disable(rows: Application[]) {
    const changing = rows.filter((a) => a.status !== "disabled");
    if (changing.length === 0) {
      toast({ title: rows.length === 1 ? `${rows[0]?.name} is already disabled` : "Already disabled", description: "Enable an application from its danger zone." });
      return;
    }
    void run(
      "status",
      () => Promise.all(changing.map((a) => api(`/applications/${a.id}/disable`, { method: "POST" }))),
      () =>
        toast({
          title: changing.length === 1 ? `${changing[0]?.name} disabled` : `${pluralize(changing.length, "application")} disabled`,
          description: "New sign-ins are blocked and tokens already issued are revoked.",
        }),
    );
  }

  return (
    <DataTable
      data={applications}
      columns={columns}
      getRowId={(a) => a.id}
      label="Applications"
      noun={["application", "applications"]}
      storageKey="applications"
      searchText={(a) => `${a.name} ${a.description} ${a.clientId} ${a.homepage}`}
      searchPlaceholder="Search name, client ID or URL"
      views={views}
      initialViewId={initialView}
      facets={[
        { column: "protocol", label: "Protocol", options: Object.entries(PROTOCOL_LABEL).map(([value, label]) => ({ value, label })) },
        { column: "type", label: "Type", options: Object.entries(TYPE_LABEL).map(([value, label]) => ({ value, label })) },
        { column: "status", label: "Status", options: Object.entries(APP_STATUS).map(([value, s]) => ({ value, label: s.label })) },
      ]}
      onRowOpen={(a) => router.push(`/admin/applications/${a.id}`)}
      rowActions={[
        { label: "Open application", icon: AppWindow, onSelect: (a) => router.push(`/admin/applications/${a.id}`) },
        {
          label: "Copy client ID",
          icon: Copy,
          onSelect: (a) => {
            void navigator.clipboard?.writeText(a.clientId);
            toast({ title: a.protocol === "saml" ? "Entity ID copied" : "Client ID copied", description: a.clientId });
          },
        },
        { label: "Disable", icon: Power, separatorBefore: true, onSelect: (a) => disable([a]) },
        { label: "Delete", icon: Trash2, danger: true, onSelect: (a) => router.push(`/admin/applications/${a.id}#danger-zone`) },
      ]}
      bulkActions={[{ label: "Disable", icon: Power, onSelect: disable }]}
      toolbarEnd={
        <Link href="/admin/applications/new" className={buttonClasses("primary", "sm")}>
          <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
          Add application
        </Link>
      }
    />
  );
}
