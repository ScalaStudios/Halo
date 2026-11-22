"use client";

import Link from "next/link";
import type { ColumnDef } from "@tanstack/react-table";
import { AppWindow, Copy, Plus, Power, Trash2 } from "lucide-react";
import { useMutation } from "@/components/console/use-mutation";
import { Avatar } from "@/components/ui/avatar";
import { Badge, StatusDot } from "@/components/ui/badge";
import { buttonClasses } from "@/components/ui/button";
import { DataTable, facetFilter, type SavedView } from "@/components/ui/data-table";
import { api } from "@/lib/api/client";
import { useLocale } from "@/lib/i18n/client";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import type { Locale } from "@/lib/i18n/locale";
import type { Application } from "@/lib/types";
import { APP_STATUS, credentialState, ExpiryText, soonestExpiry, statusKey } from "./shared";

const columns = (t: (typeof applicationsMessages)[Locale], locale: Locale): ColumnDef<Application, any>[] => [
  {
    id: "name",
    accessorFn: (a) => a.name,
    header: t.common.name,
    meta: { label: t.common.name, hideable: false },
    cell: ({ row }) => (
      <span className="flex max-w-72 min-w-60 items-center gap-3">
        <Avatar name={row.original.name} src={row.original.logoUrl} size="md" square />
        <span className="flex min-w-0 flex-col">
          <span className="flex min-w-0 items-center gap-2">
            <span className="truncate font-medium text-fg">{row.original.name}</span>
            {row.original.registeredDynamically ? <Badge tone="info">{t.list.dynamic}</Badge> : null}
          </span>
          <span className="truncate text-caption text-fg-3">{row.original.description}</span>
        </span>
      </span>
    ),
  },
  {
    id: "protocol",
    accessorFn: (a) => a.protocol,
    header: t.list.protocol,
    filterFn: facetFilter,
    meta: { label: t.list.protocol },
    cell: ({ row }) => <span className="whitespace-nowrap">{t.protocols[row.original.protocol]}</span>,
  },
  {
    id: "type",
    accessorFn: (a) => a.type,
    header: t.list.type,
    filterFn: facetFilter,
    meta: { label: t.list.type },
    cell: ({ row }) =>
      ["bookmark", "forward-auth", "radius", "ldap", "remote"].includes(row.original.protocol) ? <span className="text-fg-3">—</span> : <span className="whitespace-nowrap">{t.types[row.original.type]}</span>,
  },
  {
    id: "status",
    accessorFn: statusKey,
    header: t.common.status,
    filterFn: facetFilter,
    meta: { label: t.common.status },
    cell: ({ row }) => (
      <span className="inline-flex items-center gap-2 whitespace-nowrap">
        <StatusDot tone={APP_STATUS[statusKey(row.original)].tone} />
        {t.status[statusKey(row.original)]}
      </span>
    ),
  },
  {
    id: "users",
    accessorFn: (a) => a.userCount,
    header: t.list.users,
    meta: { label: t.list.users, align: "right" },
    cell: ({ getValue }) => <span className="tnum">{getValue<number>()}</span>,
  },
  {
    id: "signIns",
    accessorFn: (a) => a.signIns7d,
    header: t.list.signIns,
    meta: { label: t.list.signIns, align: "right" },
    cell: ({ getValue }) => <span className="tnum">{getValue<number>().toLocaleString(locale)}</span>,
  },
  {
    id: "credentials",
    accessorFn: credentialState,
    header: t.list.credentials,
    filterFn: facetFilter,
    sortingFn: (a, b) => (soonestExpiry(a.original) ?? "9999").localeCompare(soonestExpiry(b.original) ?? "9999"),
    meta: { label: t.list.credentials },
    cell: ({ row }) => {
      const iso = soonestExpiry(row.original);
      return iso ? <ExpiryText iso={iso} /> : <span className="text-fg-3">—</span>;
    },
  },
];

const views = (t: (typeof applicationsMessages)[Locale]): SavedView[] => [
  { id: "all", label: t.list.allApplications, filters: [] },
  { id: "oidc", label: "OpenID Connect", filters: [{ id: "protocol", value: ["oidc"] }] },
  { id: "saml", label: "SAML", filters: [{ id: "protocol", value: ["saml"] }] },
  { id: "expiring", label: t.list.expiring, filters: [{ id: "credentials", value: ["expiring"] }] },
  { id: "disabled", label: t.status.disabled, filters: [{ id: "status", value: ["disabled"] }] },
  { id: "awaiting", label: t.status.awaiting, filters: [{ id: "status", value: ["awaiting"] }] },
];

export function ApplicationsTable({ applications, initialView }: { applications: Application[]; initialView?: string }) {
  const { run, toast, router } = useMutation();
  const locale = useLocale();
  const t = applicationsMessages[locale];

  function disable(rows: Application[]) {
    const changing = rows.filter((a) => a.status !== "disabled");
    if (changing.length === 0) {
      toast({ title: rows.length === 1 ? t.list.alreadyDisabled(rows[0]?.name ?? "") : t.list.alreadyDisabledMany, description: t.list.enableHint });
      return;
    }
    return run(
      "status",
      () => Promise.all(changing.map((a) => api(`/applications/${a.id}/disable`, { method: "POST" }))),
      () =>
        toast({
          title: changing.length === 1 ? t.list.disabled(changing[0]?.name ?? "") : t.list.disabledMany(changing.length),
          description: t.actions.application.disabled,
        }),
    );
  }

  return (
    <DataTable
      data={applications}
      columns={columns(t, locale)}
      getRowId={(a) => a.id}
      label={t.common.applications}
      noun={t.list.noun}
      storageKey="applications"
      searchText={(a) => `${a.name} ${a.description} ${a.clientId} ${a.homepage}`}
      searchPlaceholder={t.list.search}
      views={views(t)}
      initialViewId={initialView}
      facets={[
        { column: "protocol", label: t.list.protocol, options: Object.entries(t.protocols).map(([value, label]) => ({ value, label })) },
        { column: "type", label: t.list.type, options: Object.entries(t.types).map(([value, label]) => ({ value, label })) },
        { column: "status", label: t.common.status, options: Object.entries(t.status).map(([value, label]) => ({ value, label })) },
      ]}
      onRowOpen={(a) => router.push(`/admin/applications/${a.id}`)}
      rowActions={[
        { label: t.list.open, icon: AppWindow, onSelect: (a) => router.push(`/admin/applications/${a.id}`) },
        {
          label: t.list.copyClientId,
          icon: Copy,
          onSelect: (a) => {
            void navigator.clipboard?.writeText(a.clientId);
            toast({ title: a.protocol === "saml" ? t.list.entityIdCopied : t.list.clientIdCopied, description: a.clientId });
          },
        },
        { label: t.list.disable, icon: Power, separatorBefore: true, onSelect: (a) => disable([a]) },
        { label: t.list.delete, icon: Trash2, danger: true, onSelect: (a) => router.push(`/admin/applications/${a.id}#danger-zone`) },
      ]}
      bulkActions={[{ label: t.list.disable, icon: Power, onSelect: disable }]}
      toolbarEnd={
        <Link href="/admin/applications/new" className={buttonClasses("primary", "sm")}>
          <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
          {t.list.add}
        </Link>
      }
    />
  );
}
