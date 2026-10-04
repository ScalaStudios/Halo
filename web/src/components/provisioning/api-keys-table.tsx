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
import type { ApiKey } from "@/lib/provisioning-types";
import { RevokeKeyDialog } from "./account-actions";
import { KEY_STATE, SCOPES, ScopeTags, StatusLabel, keyState } from "./shared";

const columns: ColumnDef<ApiKey, any>[] = [
  {
    id: "label",
    accessorFn: (k) => k.label,
    header: "Key",
    meta: { label: "Key", hideable: false },
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
    header: "Service account",
    meta: { label: "Service account" },
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
    header: "Scopes",
    filterFn: facetFilter,
    enableSorting: false,
    meta: { label: "Scopes" },
    cell: ({ row }) => <ScopeTags scopes={row.original.scopes} />,
  },
  {
    id: "status",
    accessorFn: (k) => keyState(k),
    header: "Status",
    filterFn: facetFilter,
    meta: { label: "Status" },
    cell: ({ row }) => <StatusLabel {...KEY_STATE[keyState(row.original)]} />,
  },
  {
    id: "created",
    accessorFn: (k) => new Date(k.createdAt).getTime(),
    header: "Created",
    meta: { label: "Created", align: "right" },
    cell: ({ row }) => <span className="tnum whitespace-nowrap text-fg-3">{formatDate(row.original.createdAt)}</span>,
  },
  {
    id: "lastUsed",
    accessorFn: (k) => (k.lastUsedAt ? new Date(k.lastUsedAt).getTime() : 0),
    header: "Last used",
    meta: { label: "Last used", align: "right" },
    cell: ({ row }) => <span className="tnum whitespace-nowrap text-fg-3">{row.original.lastUsedAt ? <RelativeTime iso={row.original.lastUsedAt} /> : "Never"}</span>,
  },
  {
    id: "expires",
    accessorFn: (k) => (k.expiresAt ? new Date(k.expiresAt).getTime() : Number.MAX_SAFE_INTEGER),
    header: "Expires",
    meta: { label: "Expires", align: "right" },
    cell: ({ row }) =>
      keyState(row.original) === "revoked" ? (
        <span className="text-fg-3">—</span>
      ) : row.original.expiresAt ? (
        <ExpiryText iso={row.original.expiresAt} />
      ) : (
        <span className="whitespace-nowrap text-fg-3">Never</span>
      ),
  },
];

export function ApiKeysTable({ keys }: { keys: ApiKey[] }) {
  const router = useRouter();
  const toast = useToast();
  const [revoking, setRevoking] = useState<ApiKey | null>(null);

  return (
    <>
      <DataTable
        data={keys}
        columns={columns}
        getRowId={(k) => k.id}
        label="API keys"
        noun={["key", "keys"]}
        storageKey="api-keys"
        searchText={(k) => `${k.label} ${k.prefix} ${k.serviceAccountName}`}
        searchPlaceholder="Search label, prefix or service account"
        views={[
          { id: "active", label: "Active keys", filters: [{ id: "status", value: ["active"] }] },
          { id: "all", label: "All keys", filters: [] },
        ]}
        facets={[
          { column: "status", label: "Status", options: Object.entries(KEY_STATE).map(([value, s]) => ({ value, label: s.label })) },
          { column: "scopes", label: "Scope", options: Object.entries(SCOPES).map(([value, s]) => ({ value, label: s.label })) },
        ]}
        onRowOpen={(k) => router.push(`/admin/service-accounts/${k.serviceAccountId}`)}
        rowActions={[
          { label: "Open service account", icon: Bot, onSelect: (k) => router.push(`/admin/service-accounts/${k.serviceAccountId}`) },
          { label: "Revoke key", icon: KeyRound, danger: true, separatorBefore: true, onSelect: (k) => (keyState(k) === "revoked" ? toast({ title: `“${k.label}” is already revoked`, description: "Requests that use it are rejected." }) : setRevoking(k)) },
        ]}
      />
      <RevokeKeyDialog apiKey={revoking} onClose={() => setRevoking(null)} />
    </>
  );
}
