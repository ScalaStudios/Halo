"use client";

import Link from "next/link";
import type { ColumnDef } from "@tanstack/react-table";
import { Check, Inbox, X } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Avatar } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DataTable, facetFilter } from "@/components/ui/data-table";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Field, Textarea } from "@/components/ui/input";
import { RelativeTime } from "@/components/ui/relative-time";
import { api } from "@/lib/api/client";
import { formatDateTime, pluralize } from "@/lib/format";
import type { AccessRequest } from "@/lib/governance-types";
import { approversLabel, durationLabel, grantLabel, joinNames, REQUEST_STATUS } from "./shared";

export function PendingRequests({ requests, linkPeople = false }: { requests: AccessRequest[]; linkPeople?: boolean }) {
  const [deciding, setDeciding] = useState<{ request: AccessRequest; approve: boolean } | null>(null);

  if (requests.length === 0) {
    return <EmptyState icon={Inbox} title="No pending requests" description="New access requests that need a decision appear here." className="py-8" />;
  }

  return (
    <>
      <ul className="flex flex-col">
        {requests.map((request) => (
          <li key={request.id} className="flex items-start gap-3 border-t border-border py-4 first:border-t-0 first:pt-0 last:pb-0">
            <Avatar name={request.requester.name} size="md" />
            <div className="flex min-w-0 flex-1 flex-col gap-1">
              <p className="text-body-sm text-fg">
                {linkPeople ? (
                  <Link href={`/admin/users/${request.requester.id}`} className="font-semibold hover:underline">
                    {request.requester.name}
                  </Link>
                ) : (
                  <span className="font-semibold">{request.requester.name}</span>
                )}{" "}
                <span className="text-fg-3">requests</span> <span className="font-semibold">{request.package.name}</span>
              </p>
              {request.justification ? <p className="text-body-sm text-fg-3">“{request.justification}”</p> : null}
              <p className="text-caption text-fg-3">
                {durationLabel(request.durationDays)} · Grants {joinNames(request.groups)} · <RelativeTime iso={request.createdAt} />
              </p>
              {request.canDecide ? (
                <div className="mt-2 flex gap-2">
                  <Button size="sm" variant="secondary" onClick={() => setDeciding({ request, approve: true })}>
                    <Check aria-hidden="true" size={16} strokeWidth={1.75} />
                    Approve
                  </Button>
                  <Button size="sm" variant="quiet" onClick={() => setDeciding({ request, approve: false })}>
                    <X aria-hidden="true" size={16} strokeWidth={1.75} />
                    Deny
                  </Button>
                </div>
              ) : (
                <p className="text-caption text-fg-3">Waiting on {approversLabel(request.approvers)}.</p>
              )}
            </div>
          </li>
        ))}
      </ul>
      {deciding ? <DecideDialog {...deciding} onClose={() => setDeciding(null)} /> : null}
    </>
  );
}

function DecideDialog({ request, approve, onClose }: { request: AccessRequest; approve: boolean; onClose: () => void }) {
  const [note, setNote] = useState("");
  const { pending, run, toast } = useMutation();
  const who = request.requester.name;

  function submit() {
    void run(
      "decide",
      () => api<AccessRequest>(`/access-requests/${request.id}/${approve ? "approve" : "deny"}`, { body: { note: note.trim() } }),
      (updated) => {
        onClose();
        toast(
          approve
            ? {
                title: "Request approved",
                description: `${who} has ${request.package.name} ${updated.expiresAt ? `until ${formatDateTime(updated.expiresAt)}` : "until someone revokes it"}. They get an email.`,
              }
            : { title: "Request denied", description: `${who} gets an email${note.trim() ? " with your note" : ""}.` },
        );
      },
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => (open ? undefined : onClose())}
      title={approve ? `Approve ${who}'s request?` : `Deny ${who}'s request?`}
      description={
        approve
          ? `${who} joins ${joinNames(request.groups)} ${request.durationDays === null ? "until someone revokes the access" : `for ${pluralize(request.durationDays, "day")}, then Halo removes the access`}.`
          : `${who} keeps their current access and gets an email with your decision.`
      }
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant={approve ? "primary" : "destructive"} loading={pending === "decide"} onClick={submit}>
            {approve ? "Approve request" : "Deny request"}
          </Button>
        </>
      }
    >
      <Field label="Note" hint={`Optional. ${who} sees it in the email.`}>
        {(props) => <Textarea {...props} autoFocus value={note} onChange={(event) => setNote(event.target.value)} />}
      </Field>
    </Dialog>
  );
}

function RevokeButton({ request }: { request: AccessRequest }) {
  const [open, setOpen] = useState(false);
  const { pending, run, toast } = useMutation();
  return (
    <>
      <Button size="sm" variant="quiet" onClick={() => setOpen(true)}>
        Revoke
      </Button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        title={`Revoke ${request.package.name} for ${request.requester.name}?`}
        description="Halo removes the group memberships this approval added, unless another active approval still needs them. Memberships they had before the approval stay."
        footer={
          <>
            <Button variant="secondary" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              loading={pending === "revoke"}
              onClick={() =>
                run(
                  "revoke",
                  () => api(`/access-requests/${request.id}/revoke`, { method: "POST" }),
                  () => {
                    setOpen(false);
                    toast({
                      title: "Access revoked",
                      description: `${request.requester.name} no longer has ${request.package.name}. Recorded in the audit log.`,
                    });
                  },
                )
              }
            >
              Revoke access
            </Button>
          </>
        }
      />
    </>
  );
}

const decidedColumns: ColumnDef<AccessRequest, any>[] = [
  {
    id: "requester",
    accessorFn: (r) => r.requester.name,
    header: "Requester",
    meta: { label: "Requester", hideable: false },
    cell: ({ row }) => (
      <Link href={`/admin/users/${row.original.requester.id}`} className="flex items-center gap-3 font-medium whitespace-nowrap text-fg hover:underline">
        <Avatar name={row.original.requester.name} size="md" />
        <span className="truncate">{row.original.requester.name}</span>
      </Link>
    ),
  },
  {
    id: "package",
    accessorFn: (r) => r.package.name,
    header: "Package",
    filterFn: facetFilter,
    meta: { label: "Package" },
    cell: ({ row }) => <span className="whitespace-nowrap">{row.original.package.name}</span>,
  },
  {
    id: "status",
    accessorFn: (r) => r.status,
    header: "Decision",
    filterFn: facetFilter,
    meta: { label: "Decision" },
    cell: ({ row }) => <Badge tone={REQUEST_STATUS[row.original.status].tone}>{REQUEST_STATUS[row.original.status].label}</Badge>,
  },
  {
    id: "decided",
    accessorFn: (r) => (r.decidedAt ? new Date(r.decidedAt).getTime() : 0),
    header: "Decided",
    meta: { label: "Decided" },
    cell: ({ row }) => (
      <span className="flex max-w-56 flex-col py-2">
        <span className="whitespace-nowrap">
          {row.original.decidedBy?.name ?? (row.original.status === "cancelled" ? "Cancelled by requester" : "—")}
          {row.original.decidedAt ? (
            <span className="text-fg-3" title={formatDateTime(row.original.decidedAt)}>
              {" "}
              · <RelativeTime iso={row.original.decidedAt} />
            </span>
          ) : null}
        </span>
        {row.original.decisionNote ? (
          <span className="truncate text-caption text-fg-3" title={row.original.decisionNote}>
            “{row.original.decisionNote}”
          </span>
        ) : null}
      </span>
    ),
  },
  {
    id: "access",
    accessorFn: (r) => grantLabel(r),
    header: "Access",
    meta: { label: "Access" },
    cell: ({ row }) => {
      const active = row.original.status === "approved" && !row.original.endedAt;
      return (
        <span className="flex items-center gap-2 whitespace-nowrap">
          <span className={active ? "text-fg" : "text-fg-3"} title={row.original.expiresAt ? formatDateTime(row.original.expiresAt) : undefined}>
            {grantLabel(row.original, false)}
          </span>
          {active ? <RevokeButton request={row.original} /> : null}
        </span>
      );
    },
  },
];

export function DecidedRequests({ requests }: { requests: AccessRequest[] }) {
  const packages = [...new Set(requests.map((r) => r.package.name))].sort();
  return (
    <DataTable
      data={requests}
      columns={decidedColumns}
      getRowId={(r) => r.id}
      label="Decided access requests"
      noun={["request", "requests"]}
      storageKey="access-requests-decided"
      searchText={(r) => `${r.requester.name} ${r.package.name} ${r.decidedBy?.name ?? ""} ${r.justification} ${r.decisionNote}`}
      searchPlaceholder="Search requester, package or note"
      facets={[
        {
          column: "status",
          label: "Decision",
          options: (["approved", "denied", "cancelled"] as const).map((value) => ({ value, label: REQUEST_STATUS[value].label })),
        },
        { column: "package", label: "Package", options: packages.map((name) => ({ value: name, label: name })) },
      ]}
    />
  );
}
