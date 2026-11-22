"use client";

import Link from "next/link";
import type { ColumnDef } from "@tanstack/react-table";
import { Check, Inbox, X } from "lucide-react";
import { useMemo, useState } from "react";
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
import { formatDateTime } from "@/lib/format";
import type { AccessRequest } from "@/lib/governance-types";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { governanceMessages } from "@/lib/i18n/console/governance";
import type { Locale } from "@/lib/i18n/locale";
import { approversLabel, grantLabel, joinNames, REQUEST_STATUS, requestDurationLabel } from "./shared";

export function PendingRequests({ requests, linkPeople = false }: { requests: AccessRequest[]; linkPeople?: boolean }) {
  const [deciding, setDeciding] = useState<{ request: AccessRequest; approve: boolean } | null>(null);
  const locale = useLocale();
  const t = governanceMessages[locale];

  if (requests.length === 0) {
    return <EmptyState icon={Inbox} title={t.requests.emptyTitle} description={t.requests.emptyDescription} className="py-8" />;
  }

  return (
    <>
      <ul className="flex flex-col">
        {requests.map((request) => {
          const requester = linkPeople ? (
            <Link href={`/admin/users/${request.requester.id}`} className="font-semibold hover:underline">
              {request.requester.name}
            </Link>
          ) : (
            <span className="font-semibold">{request.requester.name}</span>
          );
          const verb = <span className="text-fg-3">{request.extends ? t.requests.verbExtend : t.requests.verbRequest}</span>;
          return (
            <li key={request.id} className="flex items-start gap-3 border-t border-border py-4 first:border-t-0 first:pt-0 last:pb-0">
              <Avatar name={request.requester.name} size="md" />
              <div className="flex min-w-0 flex-1 flex-col gap-1">
                <p className="text-body-sm text-fg">
                  {request.requestedBy ? (
                    <>
                      <span className="font-semibold">{request.requestedBy.name}</span> {verb} <span className="font-semibold">{request.package.name}</span>{" "}
                      <span className="text-fg-3">{t.requests.for}</span> {requester}
                    </>
                  ) : (
                    <>
                      {requester} {verb} <span className="font-semibold">{request.package.name}</span>
                    </>
                  )}
                </p>
                {request.justification ? <p className="text-body-sm text-fg-3">“{request.justification}”</p> : null}
                {request.answers.length ? (
                  <dl className="flex flex-col gap-1 text-body-sm">
                    {request.answers.map((answer) => (
                      <div key={answer.questionId} className="flex flex-wrap gap-x-2">
                        <dt className="text-fg-3">{answer.question}</dt>
                        <dd className="text-fg">{answer.answer}</dd>
                      </div>
                    ))}
                  </dl>
                ) : null}
                <p className="text-caption text-fg-3">
                  {requestDurationLabel(request, locale)} · {t.requests.grants(joinNames(request.groups, locale))} · <RelativeTime iso={request.createdAt} />
                  {request.stages > 1 ? ` · ${t.requests.stageOf(request.stage, request.stages)}` : null}
                </p>
                {request.stageApprovals.length ? (
                  <p className="text-caption text-fg-3">
                    {request.stageApprovals.map((a) => t.requests.approvedStage(a.by?.name ?? t.requests.someone, a.stage)).join(" · ")}
                  </p>
                ) : null}
                {request.canDecide ? (
                  <div className="mt-2 flex gap-2">
                    <Button size="sm" variant="secondary" onClick={() => setDeciding({ request, approve: true })}>
                      <Check aria-hidden="true" size={16} strokeWidth={1.75} />
                      {t.requests.approve}
                    </Button>
                    <Button size="sm" variant="quiet" onClick={() => setDeciding({ request, approve: false })}>
                      <X aria-hidden="true" size={16} strokeWidth={1.75} />
                      {t.requests.deny}
                    </Button>
                  </div>
                ) : (
                  <p className="text-caption text-fg-3">
                    {t.requests.waitingOn(approversLabel(request.approvers, locale), request.stageDueAt ? formatDateTime(request.stageDueAt, locale) : undefined)}
                  </p>
                )}
              </div>
            </li>
          );
        })}
      </ul>
      {deciding ? <DecideDialog {...deciding} onClose={() => setDeciding(null)} /> : null}
    </>
  );
}

function DecideDialog({ request, approve, onClose }: { request: AccessRequest; approve: boolean; onClose: () => void }) {
  const [note, setNote] = useState("");
  const { pending, run, errorFor, toast } = useMutation();
  const locale = useLocale();
  const t = governanceMessages[locale];
  const copy = t.requests;
  const who = request.requester.name;
  const groups = joinNames(request.groups, locale);

  function submit() {
    void run(
      "decide",
      () => api<AccessRequest>(`/access-requests/${request.id}/${approve ? "approve" : "deny"}`, { body: { note: note.trim() } }),
      (updated) => {
        onClose();
        toast(
          updated.status === "pending"
            ? { title: copy.stageApproved, description: copy.stageApprovedHint(who, updated.stage, updated.stages) }
            : approve
              ? {
                  title: copy.approved,
                  description: copy.approvedHint(who, request.package.name, updated.expiresAt ? formatDateTime(updated.expiresAt, locale) : undefined),
                }
              : { title: copy.denied, description: copy.deniedHint(who, Boolean(note.trim())) },
        );
      },
      "inline",
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => (open ? undefined : onClose())}
      error={errorFor("decide")}
      title={approve ? copy.approveTitle(who) : copy.denyTitle(who)}
      description={
        !approve
          ? copy.denyDescription(who)
          : request.stage < request.stages
            ? copy.nextStageDescription(request.stage + 1, request.stages, who)
            : request.extends && request.durationDays !== null
              ? copy.extendDescription(who, groups, request.durationDays)
              : copy.joinDescription(who, groups, request.durationDays)
      }
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t.cancel}
          </Button>
          <Button variant={approve ? "primary" : "destructive"} loading={pending === "decide"} onClick={submit}>
            {approve ? copy.approveRequest : copy.denyRequest}
          </Button>
        </>
      }
    >
      <Field label={t.note} hint={copy.noteHint(who)}>
        {(props) => <Textarea {...props} autoFocus value={note} onChange={(event) => setNote(event.target.value)} />}
      </Field>
    </Dialog>
  );
}

function RevokeButton({ request }: { request: AccessRequest }) {
  const [open, setOpen] = useState(false);
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const t = useMessages(governanceMessages);
  return (
    <>
      <Button size="sm" variant="quiet" onClick={() => setOpen(true)}>
        {t.requests.revoke}
      </Button>
      <Dialog
        open={open}
        onOpenChange={(next) => {
          clearError();
          setOpen(next);
        }}
        error={errorFor("revoke")}
        title={t.requests.revokeTitle(request.package.name, request.requester.name)}
        description={t.requests.revokeDescription}
        footer={
          <>
            <Button variant="secondary" onClick={() => setOpen(false)}>
              {t.cancel}
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
                      title: t.requests.revoked,
                      description: t.requests.revokedHint(request.requester.name, request.package.name),
                    });
                  },
                  "inline",
                )
              }
            >
              {t.requests.revokeAccess}
            </Button>
          </>
        }
      />
    </>
  );
}

const decidedColumns = (t: typeof governanceMessages.en, locale: Locale): ColumnDef<AccessRequest, any>[] => [
  {
    id: "requester",
    accessorFn: (r) => r.requester.name,
    header: t.requests.requester,
    meta: { label: t.requests.requester, hideable: false },
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
    header: t.package,
    filterFn: facetFilter,
    meta: { label: t.package },
    cell: ({ row }) => <span className="whitespace-nowrap">{row.original.package.name}</span>,
  },
  {
    id: "status",
    accessorFn: (r) => r.status,
    header: t.decision,
    filterFn: facetFilter,
    meta: { label: t.decision },
    cell: ({ row }) => <Badge tone={REQUEST_STATUS[row.original.status].tone}>{t.requestStatus[row.original.status]}</Badge>,
  },
  {
    id: "decided",
    accessorFn: (r) => (r.decidedAt ? new Date(r.decidedAt).getTime() : 0),
    header: t.requests.decidedColumn,
    meta: { label: t.requests.decidedColumn },
    cell: ({ row }) => (
      <span className="flex max-w-56 flex-col py-2">
        <span className="whitespace-nowrap">
          {row.original.decidedBy?.name ?? t.requests.undecided[row.original.status] ?? "—"}
          {row.original.decidedAt ? (
            <span className="text-fg-3" title={formatDateTime(row.original.decidedAt, locale)}>
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
    accessorFn: (r) => grantLabel(r, true, locale),
    header: t.requests.access,
    meta: { label: t.requests.access },
    cell: ({ row }) => {
      const active = row.original.status === "approved" && !row.original.endedAt;
      return (
        <span className="flex items-center gap-2 whitespace-nowrap">
          <span className={active ? "text-fg" : "text-fg-3"} title={row.original.expiresAt ? formatDateTime(row.original.expiresAt, locale) : undefined}>
            {grantLabel(row.original, false, locale)}
          </span>
          {active ? <RevokeButton request={row.original} /> : null}
        </span>
      );
    },
  },
];

export function DecidedRequests({ requests }: { requests: AccessRequest[] }) {
  const locale = useLocale();
  const t = governanceMessages[locale];
  const columns = useMemo(() => decidedColumns(t, locale), [t, locale]);
  const packages = [...new Set(requests.map((r) => r.package.name))].sort();
  return (
    <DataTable
      data={requests}
      columns={columns}
      getRowId={(r) => r.id}
      label={t.requests.label}
      noun={t.requests.noun}
      storageKey="access-requests-decided"
      searchText={(r) => `${r.requester.name} ${r.package.name} ${r.decidedBy?.name ?? ""} ${r.justification} ${r.decisionNote}`}
      searchPlaceholder={t.requests.search}
      facets={[
        {
          column: "status",
          label: t.decision,
          options: (["approved", "denied", "cancelled", "expired"] as const).map((value) => ({ value, label: t.requestStatus[value] })),
        },
        { column: "package", label: t.package, options: packages.map((name) => ({ value: name, label: name })) },
      ]}
    />
  );
}
