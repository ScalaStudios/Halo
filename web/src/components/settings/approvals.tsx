"use client";

import Link from "next/link";
import type { ColumnDef } from "@tanstack/react-table";
import { Check, Inbox, X } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { PeoplePicker } from "@/components/governance/people-picker";
import type { Person } from "@/components/governance/shared";
import { ActionError } from "@/components/ui/action-error";
import { Avatar } from "@/components/ui/avatar";
import { Badge, type Tone } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { DataTable, facetFilter } from "@/components/ui/data-table";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Checkbox, Field } from "@/components/ui/input";
import { RelativeTime } from "@/components/ui/relative-time";
import { api } from "@/lib/api/client";
import { formatDateTime } from "@/lib/format";
import type { ApprovalAction, ApprovalRequest, ApprovalStatus, Settings } from "@/lib/settings-types";
import { ReadOnlyNote, useSaveSettings } from "./shared";

const STATUS: Record<ApprovalStatus, { label: string; tone: Tone }> = {
  pending: { label: "Pending", tone: "warning" },
  approved: { label: "Approved", tone: "success" },
  rejected: { label: "Rejected", tone: "danger" },
  failed: { label: "Not applied", tone: "danger" },
  expired: { label: "Expired", tone: "neutral" },
};

const ACTIONS: { value: ApprovalAction; label: string; description: string }[] = [
  {
    value: "global-admin",
    label: "Granting the global administrator role",
    description: "To a person or a service account, or making a person eligible to activate it. Inviting someone or creating a service account with the role is refused; grant it afterwards.",
  },
  { value: "policy-disable", label: "Stopping an enforced access policy", description: "Turning it off, switching it to report-only or deleting it." },
  { value: "password-sign-in", label: "Turning on password sign-in", description: "For the whole organization, under Methods." },
  { value: "identity-providers", label: "Changing identity providers", description: "Adding, editing, turning on or off, or deleting one." },
];

const TARGET_HREF: Record<string, (id: string) => string> = {
  user: (id) => `/admin/users/${id}`,
  service_account: (id) => `/admin/service-accounts/${id}`,
  policy: (id) => `/admin/policies/${id}`,
  identity_provider: () => "/admin/identity-providers",
  method: () => "/admin/methods",
};

function Target({ request }: { request: ApprovalRequest }) {
  const href = request.targetId ? TARGET_HREF[request.targetType]?.(request.targetId) : undefined;
  if (!href) return <span className="text-fg-2">{request.targetLabel}</span>;
  return (
    <Link href={href} className="text-fg-2 hover:text-fg hover:underline">
      {request.targetLabel}
    </Link>
  );
}

export function PendingApprovals({ requests }: { requests: ApprovalRequest[] }) {
  const [deciding, setDeciding] = useState<{ request: ApprovalRequest; approve: boolean } | null>(null);
  const dialog = deciding ? <DecideDialog {...deciding} onClose={() => setDeciding(null)} /> : null;

  if (requests.length === 0) {
    return (
      <>
        <EmptyState icon={Inbox} title="Nothing waiting" description="Changes that need a second global administrator appear here." className="py-8" />
        {dialog}
      </>
    );
  }

  return (
    <>
      <ul className="flex flex-col">
        {requests.map((request) => (
          <li key={request.id} className="flex items-start gap-3 border-t border-border py-4 first:border-t-0 first:pt-0 last:pb-0">
            <Avatar name={request.requester.name} size="md" />
            <div className="flex min-w-0 flex-1 flex-col gap-1">
              <p className="text-body-sm text-fg">
                <span className="font-semibold">{request.requester.name}</span> <span className="text-fg-3">asks to</span>{" "}
                <span className="font-semibold">{request.summary.charAt(0).toLowerCase() + request.summary.slice(1)}</span>
              </p>
              <p className="text-caption text-fg-3">
                <Target request={request} /> · <RelativeTime iso={request.createdAt} /> · Expires {formatDateTime(request.expiresAt)}
              </p>
              {request.canDecide ? (
                <div className="mt-2 flex gap-2">
                  <Button size="sm" variant="secondary" onClick={() => setDeciding({ request, approve: true })}>
                    <Check aria-hidden="true" size={16} strokeWidth={1.75} />
                    Approve
                  </Button>
                  <Button size="sm" variant="quiet" onClick={() => setDeciding({ request, approve: false })}>
                    <X aria-hidden="true" size={16} strokeWidth={1.75} />
                    Reject
                  </Button>
                </div>
              ) : (
                <p className="text-caption text-fg-3">Waiting for a global administrator other than {request.requester.name}.</p>
              )}
            </div>
          </li>
        ))}
      </ul>
      {dialog}
    </>
  );
}

function DecideDialog({ request, approve, onClose }: { request: ApprovalRequest; approve: boolean; onClose: () => void }) {
  const { pending, run, errorFor, toast } = useMutation();
  const who = request.requester.name;

  function submit() {
    void run(
      "decide",
      () => api<ApprovalRequest>(`/approvals/${request.id}/${approve ? "approve" : "reject"}`, { method: "POST" }),
      () => {
        onClose();
        toast(
          approve
            ? { title: "Change approved", description: `“${request.summary}” is applied and recorded in the audit log.` }
            : { title: "Change rejected", description: `Nothing changed. ${who} can ask again.` },
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
      title={approve ? "Approve this change?" : "Reject this change?"}
      description={
        approve
          ? `Halo applies “${request.summary}” to ${request.targetLabel} now, as ${who}'s change. If anything about ${request.targetLabel} changed since the request, nothing is applied.`
          : `“${request.summary}” is not applied. ${who} can ask again.`
      }
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant={approve ? "primary" : "destructive"} loading={pending === "decide"} onClick={submit}>
            {approve ? "Approve change" : "Reject change"}
          </Button>
        </>
      }
    />
  );
}

const decidedColumns: ColumnDef<ApprovalRequest, any>[] = [
  {
    id: "change",
    accessorFn: (r) => r.summary,
    header: "Change",
    meta: { label: "Change", hideable: false },
    cell: ({ row }) => (
      <span className="flex max-w-80 flex-col py-2">
        <span className="truncate text-fg">{row.original.summary}</span>
        <span className="truncate text-caption">
          <Target request={row.original} />
        </span>
      </span>
    ),
  },
  {
    id: "requester",
    accessorFn: (r) => r.requester.name,
    header: "Requested by",
    meta: { label: "Requested by" },
    cell: ({ row }) => (
      <span className="flex items-center gap-3 whitespace-nowrap">
        <Avatar name={row.original.requester.name} size="md" />
        <span className="flex flex-col">
          <span className="text-fg">{row.original.requester.name}</span>
          <span className="text-caption text-fg-3" title={formatDateTime(row.original.createdAt)}>
            <RelativeTime iso={row.original.createdAt} />
          </span>
        </span>
      </span>
    ),
  },
  {
    id: "status",
    accessorFn: (r) => r.status,
    header: "Outcome",
    filterFn: facetFilter,
    meta: { label: "Outcome" },
    cell: ({ row }) => <Badge tone={STATUS[row.original.status].tone}>{STATUS[row.original.status].label}</Badge>,
  },
  {
    id: "decided",
    accessorFn: (r) => (r.decidedAt ? new Date(r.decidedAt).getTime() : 0),
    header: "Decided",
    meta: { label: "Decided" },
    cell: ({ row }) => (
      <span className="flex max-w-72 flex-col py-2">
        <span className="whitespace-nowrap">
          {row.original.decidedBy?.name ?? "—"}
          {row.original.decidedAt ? (
            <span className="text-fg-3" title={formatDateTime(row.original.decidedAt)}>
              {" "}
              · <RelativeTime iso={row.original.decidedAt} />
            </span>
          ) : null}
        </span>
        {row.original.outcome ? (
          <span className="truncate text-caption text-fg-3" title={row.original.outcome}>
            {row.original.outcome}
          </span>
        ) : null}
      </span>
    ),
  },
];

export function DecidedApprovals({ requests }: { requests: ApprovalRequest[] }) {
  return (
    <DataTable
      data={requests}
      columns={decidedColumns}
      getRowId={(r) => r.id}
      label="Decided approval requests"
      noun={["request", "requests"]}
      storageKey="approvals-decided"
      searchText={(r) => `${r.summary} ${r.targetLabel} ${r.requester.name} ${r.decidedBy?.name ?? ""} ${r.outcome}`}
      searchPlaceholder="Search changes or people"
      facets={[
        {
          column: "status",
          label: "Outcome",
          options: (["approved", "rejected", "failed", "expired"] as const).map((value) => ({ value, label: STATUS[value].label })),
        },
      ]}
    />
  );
}

export function ApprovalSettingsForm({ settings, people, canEdit }: { settings: Settings; people: Person[]; canEdit: boolean }) {
  const { saving, failure, save } = useSaveSettings();
  const [actions, setActions] = useState(settings.approvalActions);
  const [emergency, setEmergency] = useState(settings.emergencyAccessIds);
  const body = { approvalActions: ACTIONS.map((a) => a.value).filter((value) => actions.includes(value)), emergencyAccessIds: emergency };
  const dirty = JSON.stringify(body) !== JSON.stringify({ approvalActions: settings.approvalActions, emergencyAccessIds: settings.emergencyAccessIds });
  const on = settings.approvalActions.length > 0;

  return (
    <Card>
      <CardHeader
        title="Changes that need approval"
        description="Each one waits for a second global administrator, who approves or rejects it here within 72 hours. The person who asked can't approve their own change."
      />
      <CardBody>
        <form
          noValidate
          className="flex max-w-xl flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            void save(body, "Approval settings saved", body.approvalActions.length ? "The chosen changes now wait for a second global administrator." : "No changes need approval.");
          }}
        >
          <fieldset className="flex flex-col gap-4">
            <legend className="sr-only">Changes that need approval</legend>
            {ACTIONS.map((action) => (
              <label key={action.value} className="flex cursor-pointer items-start gap-3">
                <Checkbox
                  className="mt-0.5"
                  disabled={!canEdit}
                  checked={actions.includes(action.value)}
                  onChange={(event) => setActions((current) => (event.target.checked ? [...current, action.value] : current.filter((value) => value !== action.value)))}
                />
                <span className="flex flex-col">
                  <span className="text-body-sm text-fg">{action.label}</span>
                  <span className="text-caption text-fg-3">{action.description}</span>
                </span>
              </label>
            ))}
          </fieldset>
          {canEdit ? (
            <Field label="Emergency access accounts" hint="Up to five people whose changes skip approval, such as a break-glass account. Each use is recorded in the audit log and emailed to the other global administrators.">
              {(props) => <PeoplePicker {...props} people={people} value={emergency} onChange={setEmergency} />}
            </Field>
          ) : (
            <div className="flex flex-col gap-2">
              <p className="text-label text-fg">Emergency access accounts</p>
              <p className="text-body-sm text-fg-2">{emergency.map((id) => people.find((p) => p.id === id)?.name ?? "Someone no longer in the directory").join(", ") || "None"}</p>
            </div>
          )}
          {canEdit ? (
            <div className="flex flex-col gap-3">
              {on ? <ReadOnlyNote>While any change needs approval, saving these settings needs approval too.</ReadOnlyNote> : null}
              <div className="flex flex-wrap items-center gap-4">
                <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
                  Save approval settings
                </Button>
                <ActionError message={failure} />
              </div>
            </div>
          ) : (
            <ReadOnlyNote>Only global administrators can change which changes need approval.</ReadOnlyNote>
          )}
        </form>
      </CardBody>
    </Card>
  );
}
