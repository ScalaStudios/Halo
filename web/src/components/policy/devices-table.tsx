"use client";

import Link from "next/link";
import type { ColumnDef } from "@tanstack/react-table";
import { Ban, Link2, RotateCcw, ShieldCheck, UserRound } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { StatusDot } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DataTable, facetFilter, type SavedView } from "@/components/ui/data-table";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input, Select } from "@/components/ui/input";
import { RelativeTime } from "@/components/ui/relative-time";
import { Spinner } from "@/components/ui/spinner";
import { ApiError, api } from "@/lib/api/client";
import { formatDateTime, pluralize } from "@/lib/format";
import type { ComplianceRecord, ComplianceSource, ComplianceState, Device, DeviceTrust } from "@/lib/policy-types";
import { COMPLIANCE, TRUST } from "./shared";

type Row = Device & { owner: string; email: string };

const UNLINKED = { label: "Not linked", tone: "neutral" } as const;

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
    id: "compliance",
    accessorFn: (d) => d.compliance?.state ?? "unlinked",
    header: "Compliance",
    filterFn: facetFilter,
    meta: { label: "Compliance" },
    cell: ({ row }) => {
      const c = row.original.compliance;
      if (!c) return <span className="whitespace-nowrap text-fg-3">{UNLINKED.label}</span>;
      return (
        <span className="flex flex-col">
          <span className="inline-flex items-center gap-2 whitespace-nowrap">
            <StatusDot tone={COMPLIANCE[c.state].tone} />
            {COMPLIANCE[c.state].label}
          </span>
          <span className="text-caption whitespace-nowrap text-fg-3">
            {c.source} · {c.stale ? "stale, " : ""}
            <RelativeTime iso={c.reportedAt} lowercase />
          </span>
        </span>
      );
    },
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
  { id: "not-compliant", label: "Not compliant", filters: [{ id: "compliance", value: ["non-compliant", "unknown", "unlinked"] }] },
];

const RESULT: Record<DeviceTrust, { title: string; description: string }> = {
  trusted: { title: "trusted", description: "Policies that need a trusted device now let it sign in." },
  blocked: { title: "blocked", description: "New sign-ins from it are refused, and its open sessions and their application tokens were signed out." },
  unknown: { title: "reset to unknown", description: "Halo treats it like any device it hasn't been told about." },
};

export function DevicesTable({ devices, owners, sources }: { devices: Device[]; owners: Record<string, { name: string; email: string }>; sources: ComplianceSource[] }) {
  const { run, toast, router } = useMutation();
  const [linking, setLinking] = useState<Row | null>(null);
  const rows = devices.map((d) => ({ ...d, owner: owners[d.userId]?.name ?? "Unknown user", email: owners[d.userId]?.email ?? "" }));

  function setTrust(selected: Row[], trust: DeviceTrust) {
    const targets = selected.filter((d) => d.trust !== trust);
    const label = selected.length === 1 ? selected[0]!.name : pluralize(selected.length, "device");
    if (targets.length === 0) {
      toast({ title: `${label} ${selected.length === 1 ? "is" : "are"} already ${TRUST[trust].label.toLowerCase()}`, description: "Nothing changed." });
      return;
    }
    return run(
      "trust",
      () => Promise.all(targets.map((d) => api(`/devices/${d.id}`, { method: "PUT", body: { trust } }))),
      () => toast({ title: `${targets.length === 1 ? targets[0]!.name : pluralize(targets.length, "device")} ${RESULT[trust].title}`, description: RESULT[trust].description }),
    );
  }

  return (
    <>
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
        facets={[
          { column: "trust", label: "Trust", options: (Object.keys(TRUST) as DeviceTrust[]).map((value) => ({ value, label: TRUST[value].label })) },
          {
            column: "compliance",
            label: "Compliance",
            options: [...(Object.keys(COMPLIANCE) as ComplianceState[]).map((value) => ({ value, label: COMPLIANCE[value].label })), { value: "unlinked", label: UNLINKED.label }],
          },
        ]}
        rowActions={[
          { label: "Mark as trusted", icon: ShieldCheck, onSelect: (d) => setTrust([d], "trusted") },
          { label: "Reset to unknown", icon: RotateCcw, onSelect: (d) => setTrust([d], "unknown") },
          { label: "Link compliance record", icon: Link2, onSelect: (d) => setLinking(d) },
          { label: "Open owner", icon: UserRound, onSelect: (d) => router.push(`/admin/users/${d.userId}`) },
          { label: "Block device", icon: Ban, danger: true, separatorBefore: true, onSelect: (d) => setTrust([d], "blocked") },
        ]}
        bulkActions={[
          { label: "Mark as trusted", icon: ShieldCheck, onSelect: (selected) => setTrust(selected, "trusted") },
          { label: "Block", icon: Ban, danger: true, onSelect: (selected) => setTrust(selected, "blocked") },
        ]}
      />
      {linking ? <LinkDialog key={linking.id} device={linking} sources={sources} onClose={() => setLinking(null)} /> : null}
    </>
  );
}

function LinkDialog({ device, sources, onClose }: { device: Row; sources: ComplianceSource[]; onClose: () => void }) {
  const { pending, run, errorFor, toast } = useMutation();
  const [sourceId, setSourceId] = useState(device.compliance?.sourceId ?? sources[0]?.id ?? "");
  const [query, setQuery] = useState(device.compliance?.externalId ?? device.email);
  const [selected, setSelected] = useState(device.compliance?.externalId ?? "");
  const [records, setRecords] = useState<ComplianceRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [problem, setProblem] = useState<string>();
  const latest = useRef(0);
  const source = sources.find((s) => s.id === sourceId);

  useEffect(() => {
    if (!sourceId) return;
    const request = ++latest.current;
    setLoading(true);
    const timer = setTimeout(async () => {
      let found: ComplianceRecord[] = [];
      let failure: string | undefined;
      try {
        found = await api<ComplianceRecord[]>(`/compliance-sources/${sourceId}/devices?q=${encodeURIComponent(query.trim())}`);
      } catch (error) {
        failure = error instanceof ApiError ? error.message : "Halo could not reach the server. Try again.";
      }
      if (request !== latest.current) return;
      setRecords(found);
      setProblem(failure);
      setLoading(false);
    }, 250);
    return () => clearTimeout(timer);
  }, [sourceId, query]);

  function save(link: boolean) {
    const body = link ? { sourceId, externalId: selected } : { sourceId: null };
    void run(
      link ? "link" : "unlink",
      () => api<Device>(`/devices/${device.id}/compliance`, { method: "PUT", body }),
      (saved) => {
        toast(
          saved.compliance
            ? { title: `${device.name} linked`, description: `${saved.compliance.source} reports it ${COMPLIANCE[saved.compliance.state].label.toLowerCase()}. Policies use this state from its next sign-in.` }
            : { title: `${device.name} unlinked`, description: "Its compliance is unknown until you link it again." },
        );
        onClose();
      },
      "inline",
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      error={errorFor("link") ?? errorFor("unlink")}
      width="wide"
      title={`Link ${device.name}`}
      description={`Choose the record ${source?.name ?? "a compliance source"} keeps for ${device.owner}'s device. Halo then uses the compliance state it reports.`}
      footer={
        <>
          {device.compliance ? (
            <Button variant="secondary" className="mr-auto" loading={pending === "unlink"} onClick={() => save(false)}>
              Unlink
            </Button>
          ) : null}
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" disabled={!selected || !sourceId} loading={pending === "link"} onClick={() => save(true)}>
            Link device
          </Button>
        </>
      }
    >
      {sources.length === 0 ? (
        <p className="text-body-sm text-fg-3">
          There are no compliance sources yet.{" "}
          <Link href="/admin/devices?tab=sources" className="text-link underline underline-offset-3 hover:text-ember">
            Add one on the Compliance sources tab
          </Link>
          .
        </p>
      ) : (
        <form
          className="flex flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            if (selected) save(true);
          }}
        >
          <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
            <Field label="Source">
              {(props) => (
                <Select
                  {...props}
                  value={sourceId}
                  onChange={(event) => {
                    setSourceId(event.target.value);
                    setSelected("");
                  }}
                >
                  {sources.map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.name}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
            <Field label="Search">
              {(props) => <Input {...props} type="search" autoComplete="off" spellCheck={false} value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Name, owner or ID" />}
            </Field>
          </div>
          <fieldset className="flex flex-col gap-2" aria-busy={loading}>
            <legend className="mb-2 flex items-center gap-2 text-label text-fg">
              Device record
              {loading ? <Spinner /> : null}
            </legend>
            {problem ? (
              <p role="alert" className="text-caption text-danger">
                {problem}
              </p>
            ) : records.length === 0 && !loading ? (
              <p className="rounded-md border border-border bg-sunken p-4 text-body-sm text-fg-3">
                {query.trim() ? `No device from ${source?.name ?? "this source"} matches “${query.trim()}”.` : `${source?.name ?? "This source"} hasn't reported any devices yet.`}
              </p>
            ) : (
              <div className={`flex max-h-72 flex-col gap-2 overflow-y-auto transition-opacity duration-fast ${loading ? "opacity-60" : ""}`}>
                {records.map((r) => (
                  <label
                    key={r.id}
                    className="flex cursor-pointer items-start gap-3 rounded-md border border-border p-4 transition-colors duration-fast ease-brand hover:border-border-strong has-checked:border-ember"
                  >
                    <input type="radio" name="record" value={r.id} checked={selected === r.id} onChange={() => setSelected(r.id)} className="mt-1 size-4 shrink-0 accent-ember" />
                    <span className="flex min-w-0 flex-col gap-1">
                      <span className="flex flex-wrap items-center gap-x-3 gap-y-1">
                        <span className="text-body-sm font-semibold text-fg">{r.name || r.id}</span>
                        <span className="inline-flex items-center gap-2 text-caption text-fg-3">
                          <StatusDot tone={COMPLIANCE[r.state].tone} />
                          {COMPLIANCE[r.state].label}
                        </span>
                      </span>
                      <span className="text-caption break-all text-fg-3">
                        {[r.owner, r.osVersion, r.encrypted === null ? "" : r.encrypted ? "Disk encrypted" : "Disk not encrypted", r.patchLevel && `Patched ${r.patchLevel}`, `ID ${r.id}`].filter(Boolean).join(" · ")}
                      </span>
                    </span>
                  </label>
                ))}
              </div>
            )}
          </fieldset>
          <button type="submit" hidden />
        </form>
      )}
    </Dialog>
  );
}
