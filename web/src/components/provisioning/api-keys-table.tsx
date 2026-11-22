"use client";

import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import { Bot, KeyRound } from "lucide-react";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { ExpiryText } from "@/components/applications/shared";
import { DataTable, facetFilter } from "@/components/ui/data-table";
import { RelativeTime } from "@/components/ui/relative-time";
import { useToast } from "@/components/ui/toast";
import { formatDate } from "@/lib/format";
import { useLocale } from "@/lib/i18n/client";
import { provisioningMessages } from "@/lib/i18n/console/infrastructure/provisioning";
import type { Locale } from "@/lib/i18n/locale";
import type { ApiKey } from "@/lib/provisioning-types";
import { RevokeKeyDialog } from "./account-actions";
import { KEY_STATE, RateLimitHits, ScopeTags, StatusLabel, keyState } from "./shared";

const columns = (t: (typeof provisioningMessages)[Locale], locale: Locale): ColumnDef<ApiKey, any>[] => [
  {
    id: "label",
    accessorFn: (k) => k.label,
    header: t.apiKeys.key,
    meta: { label: t.apiKeys.key, hideable: false },
    cell: ({ row }) => (
      <span className="flex min-w-48 flex-col py-2">
        <span className="font-medium text-fg">{row.original.label}</span>
        <span className="font-mono text-code-sm text-fg-3">{row.original.prefix}…</span>
      </span>
    ),
  },
  {
    id: "account",
    accessorFn: (k) => k.serviceAccountName,
    header: t.apiKeys.serviceAccount,
    meta: { label: t.apiKeys.serviceAccount },
    cell: ({ row }) => (
      <Link
        href={`/admin/service-accounts/${row.original.serviceAccountId}`}
        onClick={(event) => event.stopPropagation()}
        className="whitespace-nowrap text-link underline underline-offset-3 hover:text-ember"
      >
        {row.original.serviceAccountName}
      </Link>
    ),
  },
  {
    id: "scopes",
    accessorFn: (k) => k.scopes,
    getUniqueValues: (k) => k.scopes,
    header: t.scopes,
    filterFn: facetFilter,
    enableSorting: false,
    meta: { label: t.scopes },
    cell: ({ row }) => <ScopeTags scopes={row.original.scopes} />,
  },
  {
    id: "status",
    accessorFn: (k) => keyState(k),
    header: t.status,
    filterFn: facetFilter,
    meta: { label: t.status },
    cell: ({ row }) => <StatusLabel {...KEY_STATE[keyState(row.original)]} label={t.keyState[keyState(row.original)]} />,
  },
  {
    id: "created",
    accessorFn: (k) => new Date(k.createdAt).getTime(),
    header: t.created,
    meta: { label: t.created, align: "right" },
    cell: ({ row }) => <span className="tnum whitespace-nowrap text-fg-3">{formatDate(row.original.createdAt, locale)}</span>,
  },
  {
    id: "lastUsed",
    accessorFn: (k) => (k.lastUsedAt ? new Date(k.lastUsedAt).getTime() : 0),
    header: t.lastUsed,
    meta: { label: t.lastUsed, align: "right" },
    cell: ({ row }) => <span className="tnum whitespace-nowrap text-fg-3">{row.original.lastUsedAt ? <RelativeTime iso={row.original.lastUsedAt} /> : t.never}</span>,
  },
  {
    id: "rateLimitHits",
    accessorFn: (k) => k.rateLimitHits,
    header: t.apiKeys.rateLimited,
    meta: { label: t.apiKeys.rateLimited, align: "right" },
    cell: ({ row }) => <RateLimitHits hits={row.original.rateLimitHits} locale={locale} />,
  },
  {
    id: "expires",
    accessorFn: (k) => (k.expiresAt ? new Date(k.expiresAt).getTime() : Number.MAX_SAFE_INTEGER),
    header: t.apiKeys.expires,
    meta: { label: t.apiKeys.expires, align: "right" },
    cell: ({ row }) =>
      keyState(row.original) === "revoked" ? (
        <span className="text-fg-3">—</span>
      ) : row.original.expiresAt ? (
        <ExpiryText iso={row.original.expiresAt} />
      ) : (
        <span className="whitespace-nowrap text-fg-3">{t.never}</span>
      ),
  },
];

export function ApiKeysTable({ keys }: { keys: ApiKey[] }) {
  const router = useRouter();
  const toast = useToast();
  const [revoking, setRevoking] = useState<ApiKey | null>(null);
  const locale = useLocale();
  const t = provisioningMessages[locale];

  return (
    <>
      <DataTable
        data={keys}
        columns={columns(t, locale)}
        getRowId={(k) => k.id}
        label={t.apiKeys.label}
        noun={t.apiKeys.noun}
        storageKey="api-keys"
        initialVisibility={{ created: false }}
        searchText={(k) => `${k.label} ${k.prefix} ${k.serviceAccountName}`}
        searchPlaceholder={t.apiKeys.search}
        views={[
          { id: "active", label: t.apiKeys.activeView, filters: [{ id: "status", value: ["active"] }] },
          { id: "all", label: t.apiKeys.allView, filters: [] },
        ]}
        facets={[
          { column: "status", label: t.status, options: Object.entries(t.keyState).map(([value, label]) => ({ value, label })) },
          { column: "scopes", label: t.apiKeys.scopeFacet, options: Object.entries(t.scope).map(([value, s]) => ({ value, label: s.label })) },
        ]}
        onRowOpen={(k) => router.push(`/admin/service-accounts/${k.serviceAccountId}`)}
        rowActions={[
          { label: t.serviceAccounts.open, icon: Bot, onSelect: (k) => router.push(`/admin/service-accounts/${k.serviceAccountId}`) },
          { label: t.apiKeys.revoke, icon: KeyRound, danger: true, separatorBefore: true, onSelect: (k) => (keyState(k) === "revoked" ? toast({ title: t.apiKeys.alreadyRevoked(k.label), description: t.apiKeys.alreadyRevokedHint }) : setRevoking(k)) },
        ]}
      />
      <RevokeKeyDialog apiKey={revoking} onClose={() => setRevoking(null)} />
    </>
  );
}
