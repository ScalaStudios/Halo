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
import { METHOD, RESULT, RISK, signInApp, signInUser } from "@/lib/labels";
import type { SignInEvent } from "@/lib/types";

type Row = SignInEvent & { user: string; app: string };

const GUIDANCE: Record<string, { why: string; next: string }> = {
  "User is not assigned to this application.": {
    why: "None of the user's groups is assigned to this application.",
    next: "Add the user to a group that is assigned to the application, or assign one of their groups to it.",
  },
  "The account is suspended.": {
    why: "An administrator suspended this account, so every sign-in is refused.",
    next: "Restore the account from the user's profile if they should have access again.",
  },
  "No account uses this email address.": {
    why: "Someone tried to sign in with an email address that doesn't belong to anyone in Halo.",
    next: "Check for a typo in the address. Repeated attempts for many addresses can mean someone is probing for accounts.",
  },
  "Too many failed attempts.": {
    why: "Halo locked code sign-in for this account after repeated wrong codes.",
    next: "The lock lifts on its own. If the user didn't make these attempts, revoke their sessions and reset their authentication.",
  },
  "The code did not match.": {
    why: "The authenticator code was wrong or had already expired, often because the phone clock is out of sync.",
    next: "Ask the user to turn on automatic time on their phone. Consider moving them to a passkey.",
  },
  "The code was already used.": {
    why: "Each authenticator code works once. This one had already been used to sign in.",
    next: "No action needed for a single attempt. If it repeats, the code may have been intercepted.",
  },
  "The recovery code did not match an unused code.": {
    why: "The recovery code was wrong or had already been used.",
    next: "If the user has lost their codes and methods, reset their authentication and send them a new setup link.",
  },
  "The passkey prompt expired or was already used.": {
    why: "The passkey prompt was dismissed or timed out before the user confirmed it.",
    next: "No action needed for a single attempt. If it repeats, ask the user whether the prompt appears on their device.",
  },
  "The passkey is not registered with Halo.": {
    why: "The passkey presented belongs to a different account or was removed.",
    next: "Ask the user to sign in with another method, or reset their authentication so they can enroll again.",
  },
  "The passkey's signature counter went backwards, so it may have been cloned.": {
    why: "Halo saw a counter value lower than the last one, which happens when a credential is copied to another device.",
    next: "Treat it as a possible compromise: revoke the user's sessions, remove the passkey and confirm with the user out of band.",
  },
  "Authenticator code did not match. Sign-in blocked by risk policy.": {
    why: "The attempt came from a Tor exit node with no prior history for this user, and the code was wrong.",
    next: "Treat it as a possible credential attack: confirm with the user out of band, revoke their sessions and reset their authenticator.",
  },
};

const columns: ColumnDef<Row, any>[] = [
  {
    id: "time",
    accessorFn: (e) => new Date(e.time).getTime(),
    header: "Time",
    meta: { label: "Time", hideable: false },
    cell: ({ row }) => (
      <span className="tnum whitespace-nowrap text-fg-3" title={formatDateTime(row.original.time)}>
        <RelativeTime iso={row.original.time} />
      </span>
    ),
  },
  {
    id: "user",
    accessorFn: (e) => e.user,
    header: "User",
    meta: { label: "User", hideable: false },
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
    header: "Application",
    filterFn: facetFilter,
    meta: { label: "Application" },
    cell: ({ row }) => <span className="whitespace-nowrap">{row.original.app}</span>,
  },
  {
    id: "result",
    accessorFn: (e) => e.result,
    header: "Result",
    filterFn: facetFilter,
    meta: { label: "Result" },
    cell: ({ row }) => (
      <span className="inline-flex items-center gap-2 whitespace-nowrap">
        <StatusDot tone={RESULT[row.original.result].tone} />
        {RESULT[row.original.result].label}
      </span>
    ),
  },
  {
    id: "method",
    accessorFn: (e) => e.method,
    header: "Method",
    filterFn: facetFilter,
    meta: { label: "Method" },
    cell: ({ row }) => <span className="whitespace-nowrap">{METHOD[row.original.method].label}</span>,
  },
  {
    id: "risk",
    accessorFn: (e) => e.risk,
    header: "Risk",
    filterFn: facetFilter,
    sortingFn: (a, b) => Object.keys(RISK).indexOf(a.original.risk) - Object.keys(RISK).indexOf(b.original.risk),
    meta: { label: "Risk" },
    cell: ({ row }) =>
      row.original.risk === "none" ? <span className="text-fg-3">—</span> : <Badge tone={RISK[row.original.risk].tone}>{RISK[row.original.risk].label}</Badge>,
  },
  {
    id: "location",
    accessorFn: (e) => e.location,
    header: "Location",
    meta: { label: "Location" },
    cell: ({ row }) => <span className="whitespace-nowrap text-fg-3">{row.original.location}</span>,
  },
  {
    id: "ip",
    accessorFn: (e) => e.ip,
    header: "IP address",
    meta: { label: "IP address" },
    cell: ({ row }) => <span className="font-mono text-code-sm whitespace-nowrap text-fg-3">{row.original.ip}</span>,
  },
  {
    id: "device",
    accessorFn: (e) => e.device,
    header: "Device",
    meta: { label: "Device" },
    cell: ({ row }) => <span className="whitespace-nowrap text-fg-3">{row.original.device}</span>,
  },
];

const views: SavedView[] = [
  { id: "all", label: "All sign-ins", filters: [] },
  { id: "failures", label: "Failures", filters: [{ id: "result", value: ["failure", "interrupted"] }] },
  { id: "risky", label: "High and medium risk", filters: [{ id: "risk", value: ["high", "medium"] }] },
];

function csv(events: Row[]) {
  const head = ["time", "user", "email", "application", "result", "reason", "method", "risk", "location", "ip", "device", "event_id"];
  const lines = events.map((e) =>
    [e.time, e.user, e.email, e.app, e.result, e.reason ?? "", e.method, e.risk, e.location, e.ip, e.device, e.id]
      .map((value) => `"${value.replaceAll('"', '""')}"`)
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
        columns={columns}
        getRowId={(e) => e.id}
        label="Sign-in logs"
        noun={["sign-in", "sign-ins"]}
        storageKey="sign-ins"
        searchText={(e) => `${e.user} ${e.email} ${e.app} ${e.ip} ${e.location} ${e.id}`}
        searchPlaceholder="Search user, application, IP or event ID"
        views={views}
        initialViewId={initialView}
        initialVisibility={{ device: false }}
        facets={[
          { column: "result", label: "Result", options: Object.entries(RESULT).map(([value, r]) => ({ value, label: r.label })) },
          { column: "risk", label: "Risk", options: Object.entries(RISK).map(([value, r]) => ({ value, label: r.label })) },
          { column: "application", label: "Application", options: apps.map((app) => ({ value: app, label: app })) },
          { column: "method", label: "Method", options: methods.map((m) => ({ value: m, label: METHOD[m].label })) },
        ]}
        onRowOpen={(e) => open(e.id)}
        toolbarEnd={(rows) => (
          <Button size="sm" variant="secondary" onClick={() => download(rows)}>
            <Download aria-hidden="true" size={16} strokeWidth={1.75} />
            Export CSV
          </Button>
        )}
      />
      <Dialog
        open={Boolean(selected)}
        onOpenChange={(next) => {
          if (!next) open(undefined);
        }}
        width="wide"
        title={selected ? `Sign-in to ${selected.app}` : "Sign-in"}
        description={selected ? `${selected.user} · ${formatDateTime(selected.time)}` : undefined}
      >
        {selected ? <SignInDetail event={selected} /> : null}
      </Dialog>
    </>
  );
}

function SignInDetail({ event }: { event: Row }) {
  const guidance = event.reason ? GUIDANCE[event.reason] : undefined;
  const failed = event.result === "failure";

  return (
    <div className="flex flex-col gap-6 pb-4">
      {event.result === "success" ? (
        <div className="flex items-center gap-3 rounded-md border border-success/20 bg-success-container p-4">
          <CircleCheck aria-hidden="true" size={20} strokeWidth={1.75} className="shrink-0 text-success" />
          <p className="text-body-sm text-fg">
            <span className="font-semibold">Signed in</span> with {METHOD[event.method].label.toLowerCase()}.
          </p>
        </div>
      ) : (
        <div className={cn("flex gap-3 rounded-md border p-4", failed ? "border-danger/20 bg-danger-container" : "border-warning/20 bg-warning-container")}>
          <TriangleAlert aria-hidden="true" size={20} strokeWidth={1.75} className={cn("mt-px shrink-0", failed ? "text-danger" : "text-warning")} />
          <dl className="flex min-w-0 flex-col gap-3">
            <div className="flex flex-col gap-1">
              <dt className="text-label text-fg-3">{failed ? "Sign-in failed" : "Sign-in interrupted"}</dt>
              <dd className="text-body-sm font-semibold text-fg">{event.reason}</dd>
            </div>
            {guidance ? (
              <>
                <div className="flex flex-col gap-1">
                  <dt className="text-label text-fg-3">Why</dt>
                  <dd className="text-body-sm text-fg-2">{guidance.why}</dd>
                </div>
                <div className="flex flex-col gap-1">
                  <dt className="text-label text-fg-3">What to do</dt>
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
            label: "User",
            value: event.userId ? (
              <Link href={`/admin/users/${event.userId}`} className="font-medium text-link hover:underline">
                {event.user}
              </Link>
            ) : (
              event.email
            ),
          },
          { label: "Application", value: event.app },
          { label: "Method", value: METHOD[event.method].label },
          {
            label: "Risk",
            value: event.risk === "none" ? "None detected" : <Badge tone={RISK[event.risk].tone}>{RISK[event.risk].label}</Badge>,
          },
          {
            label: "IP address",
            value: (
              <span className="-my-2 flex items-center gap-2">
                <span className="font-mono text-code-sm">{event.ip}</span>
                <CopyButton value={event.ip} label="Copy IP" />
              </span>
            ),
          },
          { label: "Location", value: event.location },
          { label: "Device", value: event.device },
          {
            label: "Event ID",
            value: (
              <span className="-my-2 flex items-center gap-2">
                <span className="font-mono text-code-sm break-all">{event.id}</span>
                <CopyButton value={event.id} label="Copy ID" />
              </span>
            ),
          },
        ]}
      />
    </div>
  );
}
