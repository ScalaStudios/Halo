"use client";

import Link from "next/link";
import type { ColumnDef } from "@tanstack/react-table";
import { Ban, RotateCcw, ShieldCheck, UserRound } from "lucide-react";
import { useMutation } from "@/components/console/use-mutation";
import { StatusDot } from "@/components/ui/badge";
import { DataTable, facetFilter, type SavedView } from "@/components/ui/data-table";
import { RelativeTime } from "@/components/ui/relative-time";
import { api } from "@/lib/api/client";
import { formatDateTime, pluralize } from "@/lib/format";
import type { Device, DeviceTrust } from "@/lib/policy-types";
import { TRUST } from "./shared";

type Row = Device & { owner: string; email: string };

const columns: ColumnDef<Row, any>[] = [
  {
    id: "device",
    accessorFn: (d) => d.name,
    header: "Device",
    meta: { label: "Device", hideable: false },
    cell: ({ row }) => (
      <span className="flex min-w-48 flex-col">
        <span className="font-medium whitespace-nowrap text-fg">{row.original.name}</span>
        <span className="text-caption whitespace-nowrap text-fg-3">
          {row.original.browser} · {row.original.os}
        </span>
      </span>
    ),
  },
  {
    id: "owner",
    accessorFn: (d) => d.owner,
    header: "Owner",
    meta: { label: "Owner" },
    cell: ({ row }) => (
      <span className="flex flex-col">
        <Link href={`/admin/users/${row.original.userId}`} className="w-fit whitespace-nowrap text-fg underline-offset-3 hover:underline">
          {row.original.owner}
        </Link>
        <span className="text-caption whitespace-nowrap text-fg-3">{row.original.email}</span>
      </span>
    ),
  },
  {
    id: "trust",
    accessorFn: (d) => d.trust,
    header: "Trust",
    filterFn: facetFilter,
    meta: { label: "Trust" },
    cell: ({ row }) => (
      <span className="inline-flex items-center gap-2 whitespace-nowrap">
        <StatusDot tone={TRUST[row.original.trust].tone} />
        {TRUST[row.original.trust].label}
      </span>
    ),
  },
  {
    id: "lastSeen",
    accessorFn: (d) => new Date(d.lastSeenAt).getTime(),
    header: "Last seen",
    meta: { label: "Last seen", align: "right" },
    cell: ({ row }) => (
      <span className="tnum whitespace-nowrap text-fg-3" title={formatDateTime(row.original.lastSeenAt)}>
        <RelativeTime iso={row.original.lastSeenAt} />
      </span>
    ),
  },
  {
    id: "ip",
    accessorFn: (d) => d.lastIp,
    header: "Last IP address",
    meta: { label: "Last IP address" },
    cell: ({ row }) => <span className="font-mono text-code-sm whitespace-nowrap text-fg-3">{row.original.lastIp || "—"}</span>,
  },
  {
    id: "signIns",
    accessorFn: (d) => d.signIns,
    header: "Sign-ins",
    meta: { label: "Sign-ins", align: "right" },
    cell: ({ row }) => <span className="tnum">{row.original.signIns}</span>,
  },
  {
    id: "firstSeen",
    accessorFn: (d) => new Date(d.firstSeenAt).getTime(),
    header: "First seen",
    meta: { label: "First seen", align: "right" },
    cell: ({ row }) => (
      <span className="tnum whitespace-nowrap text-fg-3" title={formatDateTime(row.original.firstSeenAt)}>
        <RelativeTime iso={row.original.firstSeenAt} />
      </span>
    ),
  },
];

const views: SavedView[] = [
  { id: "all", label: "All devices", filters: [] },
  { id: "unknown", label: "Unknown", filters: [{ id: "trust", value: ["unknown"] }] },
  { id: "trusted", label: "Trusted", filters: [{ id: "trust", value: ["trusted"] }] },
  { id: "blocked", label: "Blocked", filters: [{ id: "trust", value: ["blocked"] }] },
];

const RESULT: Record<DeviceTrust, { title: string; description: string }> = {
  trusted: { title: "trusted", description: "Policies that need a trusted device now let it sign in." },
  blocked: { title: "blocked", description: "New sign-ins from it are refused, and its open sessions and their application tokens were signed out." },
  unknown: { title: "reset to unknown", description: "Halo treats it like any device it hasn't been told about." },
};

export function DevicesTable({ devices, owners }: { devices: Device[]; owners: Record<string, { name: string; email: string }> }) {
  const { run, toast, router } = useMutation();
  const rows = devices.map((d) => ({ ...d, owner: owners[d.userId]?.name ?? "Unknown user", email: owners[d.userId]?.email ?? "" }));

  function setTrust(selected: Row[], trust: DeviceTrust) {
    const targets = selected.filter((d) => d.trust !== trust);
    const label = selected.length === 1 ? selected[0]!.name : pluralize(selected.length, "device");
    if (targets.length === 0) {
      toast({ title: `${label} ${selected.length === 1 ? "is" : "are"} already ${TRUST[trust].label.toLowerCase()}`, description: "Nothing changed." });
      return;
    }
    void run(
      "trust",
      () => Promise.all(targets.map((d) => api(`/devices/${d.id}`, { method: "PUT", body: { trust } }))),
      () => toast({ title: `${targets.length === 1 ? targets[0]!.name : pluralize(targets.length, "device")} ${RESULT[trust].title}`, description: RESULT[trust].description }),
    );
  }

  return (
    <DataTable
      data={rows}
      columns={columns}
      getRowId={(d) => d.id}
      label="Devices"
      noun={["device", "devices"]}
      storageKey="devices"
      searchText={(d) => `${d.name} ${d.browser} ${d.os} ${d.owner} ${d.email} ${d.lastIp}`}
      searchPlaceholder="Search device, owner or IP"
      views={views}
      initialVisibility={{ firstSeen: false }}
      facets={[{ column: "trust", label: "Trust", options: (Object.keys(TRUST) as DeviceTrust[]).map((value) => ({ value, label: TRUST[value].label })) }]}
      rowActions={[
        { label: "Mark as trusted", icon: ShieldCheck, onSelect: (d) => setTrust([d], "trusted") },
        { label: "Reset to unknown", icon: RotateCcw, onSelect: (d) => setTrust([d], "unknown") },
        { label: "Open owner", icon: UserRound, onSelect: (d) => router.push(`/admin/users/${d.userId}`) },
        { label: "Block device", icon: Ban, danger: true, separatorBefore: true, onSelect: (d) => setTrust([d], "blocked") },
      ]}
      bulkActions={[
        { label: "Mark as trusted", icon: ShieldCheck, onSelect: (selected) => setTrust(selected, "trusted") },
        { label: "Block", icon: Ban, danger: true, onSelect: (selected) => setTrust(selected, "blocked") },
      ]}
    />
  );
}
