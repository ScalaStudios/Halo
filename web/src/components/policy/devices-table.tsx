"use client";

import Link from "next/link";
import type { ColumnDef } from "@tanstack/react-table";
import { Ban, Link2, RotateCcw, ShieldCheck, UserRound } from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { StatusDot } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DataTable, facetFilter, type SavedView } from "@/components/ui/data-table";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input, Select } from "@/components/ui/input";
import { RelativeTime } from "@/components/ui/relative-time";
import { Spinner } from "@/components/ui/spinner";
import { ApiError, api } from "@/lib/api/client";
import { formatDateTime } from "@/lib/format";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { frameMessages } from "@/lib/i18n/console/frame";
import { policyMessages } from "@/lib/i18n/console/policy";
import type { Locale } from "@/lib/i18n/locale";
import type { ComplianceRecord, ComplianceSource, ComplianceState, Device, DeviceTrust } from "@/lib/policy-types";
import { COMPLIANCE, TRUST, type PolicyCopy } from "./shared";

type Row = Device & { owner: string; email: string };

function deviceColumns(copy: PolicyCopy, locale: Locale): ColumnDef<Row, any>[] {
  const t = copy.devices;
  return [
    {
      id: "device",
      accessorFn: (d) => d.name,
      header: t.device,
      meta: { label: t.device, hideable: false },
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
      header: t.owner,
      meta: { label: t.owner },
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
      header: t.trust,
      filterFn: facetFilter,
      meta: { label: t.trust },
      cell: ({ row }) => (
        <span className="inline-flex items-center gap-2 whitespace-nowrap">
          <StatusDot tone={TRUST[row.original.trust].tone} />
          {copy.labels.trust[row.original.trust]}
        </span>
      ),
    },
    {
      id: "compliance",
      accessorFn: (d) => d.compliance?.state ?? "unlinked",
      header: t.compliance,
      filterFn: facetFilter,
      meta: { label: t.compliance },
      cell: ({ row }) => {
        const c = row.original.compliance;
        if (!c) return <span className="whitespace-nowrap text-fg-3">{t.unlinked}</span>;
        return (
          <span className="flex flex-col">
            <span className="inline-flex items-center gap-2 whitespace-nowrap">
              <StatusDot tone={COMPLIANCE[c.state].tone} />
              {copy.labels.compliance[c.state]}
            </span>
            <span className="text-caption whitespace-nowrap text-fg-3">
              {c.source} · {c.stale ? t.stale : ""}
              <RelativeTime iso={c.reportedAt} lowercase />
            </span>
          </span>
        );
      },
    },
    {
      id: "lastSeen",
      accessorFn: (d) => new Date(d.lastSeenAt).getTime(),
      header: t.lastSeen,
      meta: { label: t.lastSeen, align: "right" },
      cell: ({ row }) => (
        <span className="tnum whitespace-nowrap text-fg-3" title={formatDateTime(row.original.lastSeenAt, locale)}>
          <RelativeTime iso={row.original.lastSeenAt} />
        </span>
      ),
    },
    {
      id: "ip",
      accessorFn: (d) => d.lastIp,
      header: t.lastIp,
      meta: { label: t.lastIp },
      cell: ({ row }) => <span className="font-mono text-code-sm whitespace-nowrap text-fg-3">{row.original.lastIp || "—"}</span>,
    },
    {
      id: "signIns",
      accessorFn: (d) => d.signIns,
      header: t.signIns,
      meta: { label: t.signIns, align: "right" },
      cell: ({ row }) => <span className="tnum">{row.original.signIns}</span>,
    },
    {
      id: "firstSeen",
      accessorFn: (d) => new Date(d.firstSeenAt).getTime(),
      header: t.firstSeen,
      meta: { label: t.firstSeen, align: "right" },
      cell: ({ row }) => (
        <span className="tnum whitespace-nowrap text-fg-3" title={formatDateTime(row.original.firstSeenAt, locale)}>
          <RelativeTime iso={row.original.firstSeenAt} />
        </span>
      ),
    },
  ];
}

export function DevicesTable({ devices, owners, sources }: { devices: Device[]; owners: Record<string, { name: string; email: string }>; sources: ComplianceSource[] }) {
  const locale = useLocale();
  const copy = policyMessages[locale];
  const t = copy.devices;
  const { run, toast, router } = useMutation();
  const [linking, setLinking] = useState<Row | null>(null);
  const rows = devices.map((d) => ({ ...d, owner: owners[d.userId]?.name ?? copy.unknownUser, email: owners[d.userId]?.email ?? "" }));
  const views: SavedView[] = [
    { id: "all", label: t.views.all, filters: [] },
    { id: "unknown", label: t.views.unknown, filters: [{ id: "trust", value: ["unknown"] }] },
    { id: "trusted", label: t.views.trusted, filters: [{ id: "trust", value: ["trusted"] }] },
    { id: "blocked", label: t.views.blocked, filters: [{ id: "trust", value: ["blocked"] }] },
    { id: "not-compliant", label: t.views.notCompliant, filters: [{ id: "compliance", value: ["non-compliant", "unknown", "unlinked"] }] },
  ];

  function setTrust(selected: Row[], trust: DeviceTrust) {
    const targets = selected.filter((d) => d.trust !== trust);
    const label = selected.length === 1 ? selected[0]!.name : t.count(selected.length);
    if (targets.length === 0) {
      toast({ title: t.already(label, selected.length !== 1, trust), description: t.nothingChanged });
      return;
    }
    return run(
      "trust",
      () => Promise.all(targets.map((d) => api(`/devices/${d.id}`, { method: "PUT", body: { trust } }))),
      () => toast({ title: t.result[trust].title(targets.length === 1 ? targets[0]!.name : t.count(targets.length), targets.length !== 1), description: t.result[trust].description }),
    );
  }

  return (
    <>
      <DataTable
        data={rows}
        columns={deviceColumns(copy, locale)}
        getRowId={(d) => d.id}
        label={t.label}
        noun={t.noun}
        storageKey="devices"
        searchText={(d) => `${d.name} ${d.browser} ${d.os} ${d.owner} ${d.email} ${d.lastIp}`}
        searchPlaceholder={t.search}
        views={views}
        initialVisibility={{ firstSeen: false }}
        facets={[
          { column: "trust", label: t.trust, options: (Object.keys(TRUST) as DeviceTrust[]).map((value) => ({ value, label: copy.labels.trust[value] })) },
          {
            column: "compliance",
            label: t.compliance,
            options: [...(Object.keys(COMPLIANCE) as ComplianceState[]).map((value) => ({ value, label: copy.labels.compliance[value] })), { value: "unlinked", label: t.unlinked }],
          },
        ]}
        rowActions={[
          { label: t.markTrusted, icon: ShieldCheck, onSelect: (d) => setTrust([d], "trusted") },
          { label: t.resetUnknown, icon: RotateCcw, onSelect: (d) => setTrust([d], "unknown") },
          { label: t.linkRecord, icon: Link2, onSelect: (d) => setLinking(d) },
          { label: t.openOwner, icon: UserRound, onSelect: (d) => router.push(`/admin/users/${d.userId}`) },
          { label: t.block, icon: Ban, danger: true, separatorBefore: true, onSelect: (d) => setTrust([d], "blocked") },
        ]}
        bulkActions={[
          { label: t.markTrusted, icon: ShieldCheck, onSelect: (selected) => setTrust(selected, "trusted") },
          { label: t.blockBulk, icon: Ban, danger: true, onSelect: (selected) => setTrust(selected, "blocked") },
        ]}
      />
      {linking ? <LinkDialog key={linking.id} device={linking} sources={sources} onClose={() => setLinking(null)} /> : null}
    </>
  );
}

function LinkDialog({ device, sources, onClose }: { device: Row; sources: ComplianceSource[]; onClose: () => void }) {
  const copy = useMessages(policyMessages);
  const t = copy.devices;
  const unreachable = useMessages(frameMessages).mutation.unreachable;
  const { pending, run, errorFor, toast } = useMutation();
  const [sourceId, setSourceId] = useState(device.compliance?.sourceId ?? sources[0]?.id ?? "");
  const [query, setQuery] = useState(device.compliance?.externalId ?? device.email);
  const [selected, setSelected] = useState(device.compliance?.externalId ?? "");
  const [records, setRecords] = useState<ComplianceRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [problem, setProblem] = useState<ReactNode>();
  const latest = useRef(0);
  const source = sources.find((s) => s.id === sourceId);

  useEffect(() => {
    if (!sourceId) return;
    const request = ++latest.current;
    setLoading(true);
    const timer = setTimeout(async () => {
      let found: ComplianceRecord[] = [];
      let failure: ReactNode;
      try {
        found = await api<ComplianceRecord[]>(`/compliance-sources/${sourceId}/devices?q=${encodeURIComponent(query.trim())}`);
      } catch (error) {
        failure = error instanceof ApiError ? <span lang="en">{error.message}</span> : unreachable;
      }
      if (request !== latest.current) return;
      setRecords(found);
      setProblem(failure);
      setLoading(false);
    }, 250);
    return () => clearTimeout(timer);
  }, [sourceId, query, unreachable]);

  function save(link: boolean) {
    const body = link ? { sourceId, externalId: selected } : { sourceId: null };
    void run(
      link ? "link" : "unlink",
      () => api<Device>(`/devices/${device.id}/compliance`, { method: "PUT", body }),
      (saved) => {
        toast(
          saved.compliance
            ? { title: t.linked(device.name), description: t.linkedDescription(saved.compliance.source, saved.compliance.state) }
            : { title: t.unlinkedTitle(device.name), description: t.unlinkedDescription },
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
      title={t.linkTitle(device.name)}
      description={t.linkDescription(source?.name, device.owner)}
      footer={
        <>
          {device.compliance ? (
            <Button variant="secondary" className="mr-auto" loading={pending === "unlink"} onClick={() => save(false)}>
              {t.unlink}
            </Button>
          ) : null}
          <Button variant="secondary" onClick={onClose}>
            {copy.cancel}
          </Button>
          <Button variant="primary" disabled={!selected || !sourceId} loading={pending === "link"} onClick={() => save(true)}>
            {t.link}
          </Button>
        </>
      }
    >
      {sources.length === 0 ? (
        <p className="text-body-sm text-fg-3">
          {t.noSources}{" "}
          <Link href="/admin/devices?tab=sources" className="text-link underline underline-offset-3 hover:text-ember">
            {t.addSource}
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
            <Field label={t.source}>
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
            <Field label={t.searchLabel}>
              {(props) => <Input {...props} type="search" autoComplete="off" spellCheck={false} value={query} onChange={(event) => setQuery(event.target.value)} placeholder={t.searchPlaceholder} />}
            </Field>
          </div>
          <fieldset className="flex flex-col gap-2" aria-busy={loading}>
            <legend className="mb-2 flex items-center gap-2 text-label text-fg">
              {t.record}
              {loading ? <Spinner /> : null}
            </legend>
            {problem ? (
              <p role="alert" className="text-caption text-danger">
                {problem}
              </p>
            ) : records.length === 0 && !loading ? (
              <p className="rounded-md border border-border bg-sunken p-4 text-body-sm text-fg-3">
                {query.trim() ? t.noMatch(source?.name, query.trim()) : t.noRecords(source?.name)}
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
                          {copy.labels.compliance[r.state]}
                        </span>
                      </span>
                      <span className="text-caption break-all text-fg-3">
                        {[r.owner, r.osVersion, r.encrypted === null ? "" : r.encrypted ? t.encrypted : t.notEncrypted, r.patchLevel && t.patched(r.patchLevel), t.id(r.id)].filter(Boolean).join(" · ")}
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
