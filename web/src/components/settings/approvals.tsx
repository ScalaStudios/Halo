"use client";

import Link from "next/link";
import type { ColumnDef } from "@tanstack/react-table";
import { Check, Inbox, X } from "lucide-react";
import { useMemo, useState } from "react";
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
import { useLocale, useMessages } from "@/lib/i18n/client";
import { settingsMessages } from "@/lib/i18n/console/settings";
import type { Locale } from "@/lib/i18n/locale";
import type { ApprovalAction, ApprovalRequest, ApprovalStatus, Settings } from "@/lib/settings-types";
import { ReadOnlyNote, useSaveSettings } from "./shared";

const TONES: Record<ApprovalStatus, Tone> = { pending: "warning", approved: "success", rejected: "danger", failed: "danger", expired: "neutral" };

const ACTIONS: ApprovalAction[] = ["global-admin", "policy-disable", "password-sign-in", "identity-providers", "impersonation", "user-deletion"];

type Messages = (typeof settingsMessages)["en"]["approvals"];

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
  const locale = useLocale();
  const t = settingsMessages[locale].approvals;

  if (requests.length === 0) {
    return (
      <>
        <EmptyState icon={Inbox} title={t.nothingTitle} description={t.nothingDescription} className="py-8" />
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
                <span className="font-semibold">{request.requester.name}</span> <span className="text-fg-3">{t.asksTo}</span>{" "}
                <span className="font-semibold">{request.summary.charAt(0).toLowerCase() + request.summary.slice(1)}</span>
              </p>
              <p className="text-caption text-fg-3">
                <Target request={request} /> · <RelativeTime iso={request.createdAt} /> · {t.expires(formatDateTime(request.expiresAt, locale))}
              </p>
              {request.canDecide ? (
                <div className="mt-2 flex gap-2">
                  <Button size="sm" variant="secondary" onClick={() => setDeciding({ request, approve: true })}>
                    <Check aria-hidden="true" size={16} strokeWidth={1.75} />
                    {t.approve}
                  </Button>
                  <Button size="sm" variant="quiet" onClick={() => setDeciding({ request, approve: false })}>
                    <X aria-hidden="true" size={16} strokeWidth={1.75} />
                    {t.reject}
                  </Button>
                </div>
              ) : (
                <p className="text-caption text-fg-3">{t.waiting(request.requester.name)}</p>
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
  const { approvals: t, common } = useMessages(settingsMessages);

  function submit() {
    void run(
      "decide",
      () => api<ApprovalRequest>(`/approvals/${request.id}/${approve ? "approve" : "reject"}`, { method: "POST" }),
      () => {
        onClose();
        toast(
          approve
            ? { title: t.approved, description: t.approvedHint(request.summary) }
            : { title: t.rejected, description: t.rejectedHint(who) },
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
      title={approve ? t.approveTitle : t.rejectTitle}
      description={approve ? t.approveDescription(request.summary, request.targetLabel, who) : t.rejectDescription(request.summary, who)}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {common.cancel}
          </Button>
          <Button variant={approve ? "primary" : "destructive"} loading={pending === "decide"} onClick={submit}>
            {approve ? t.approveChange : t.rejectChange}
          </Button>
        </>
      }
    />
  );
}

const decidedColumns = (t: Messages, locale: Locale): ColumnDef<ApprovalRequest, any>[] => [
  {
    id: "change",
    accessorFn: (r) => r.summary,
    header: t.change,
    meta: { label: t.change, hideable: false },
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
    header: t.requester,
    meta: { label: t.requester },
    cell: ({ row }) => (
      <span className="flex items-center gap-3 whitespace-nowrap">
        <Avatar name={row.original.requester.name} size="md" />
        <span className="flex flex-col">
          <span className="text-fg">{row.original.requester.name}</span>
          <span className="text-caption text-fg-3" title={formatDateTime(row.original.createdAt, locale)}>
            <RelativeTime iso={row.original.createdAt} />
          </span>
        </span>
      </span>
    ),
  },
  {
    id: "status",
    accessorFn: (r) => r.status,
    header: t.outcome,
    filterFn: facetFilter,
    meta: { label: t.outcome },
    cell: ({ row }) => <Badge tone={TONES[row.original.status]}>{t.status[row.original.status]}</Badge>,
  },
  {
    id: "decided",
    accessorFn: (r) => (r.decidedAt ? new Date(r.decidedAt).getTime() : 0),
    header: t.decided,
    meta: { label: t.decided },
    cell: ({ row }) => (
      <span className="flex max-w-72 flex-col py-2">
        <span className="whitespace-nowrap">
          {row.original.decidedBy?.name ?? "—"}
          {row.original.decidedAt ? (
            <span className="text-fg-3" title={formatDateTime(row.original.decidedAt, locale)}>
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
  const locale = useLocale();
  const t = settingsMessages[locale].approvals;
  const columns = useMemo(() => decidedColumns(t, locale), [t, locale]);
  return (
    <DataTable
      data={requests}
      columns={columns}
      getRowId={(r) => r.id}
      label={t.label}
      noun={t.noun}
      storageKey="approvals-decided"
      searchText={(r) => `${r.summary} ${r.targetLabel} ${r.requester.name} ${r.decidedBy?.name ?? ""} ${r.outcome}`}
      searchPlaceholder={t.search}
      facets={[
        {
          column: "status",
          label: t.outcome,
          options: (["approved", "rejected", "failed", "expired"] as const).map((value) => ({ value, label: t.status[value] })),
        },
      ]}
    />
  );
}

export function ApprovalSettingsForm({ settings, people, canEdit }: { settings: Settings; people: Person[]; canEdit: boolean }) {
  const { saving, failure, save } = useSaveSettings();
  const [actions, setActions] = useState(settings.approvalActions);
  const [emergency, setEmergency] = useState(settings.emergencyAccessIds);
  const t = useMessages(settingsMessages).approvals;
  const body = { approvalActions: ACTIONS.filter((value) => actions.includes(value)), emergencyAccessIds: emergency };
  const dirty = JSON.stringify(body) !== JSON.stringify({ approvalActions: settings.approvalActions, emergencyAccessIds: settings.emergencyAccessIds });
  const on = settings.approvalActions.length > 0;

  return (
    <Card>
      <CardHeader
        title={t.settingsTitle}
        description={t.settingsDescription}
      />
      <CardBody>
        <form
          noValidate
          className="flex max-w-xl flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            void save(body, t.saved, body.approvalActions.length ? t.savedOn : t.savedOff);
          }}
        >
          <fieldset className="flex flex-col gap-4">
            <legend className="sr-only">{t.settingsTitle}</legend>
            {ACTIONS.map((action) => (
              <label key={action} className="flex cursor-pointer items-start gap-3">
                <Checkbox
                  className="mt-0.5"
                  disabled={!canEdit}
                  checked={actions.includes(action)}
                  onChange={(event) => setActions((current) => (event.target.checked ? [...current, action] : current.filter((value) => value !== action)))}
                />
                <span className="flex flex-col">
                  <span className="text-body-sm text-fg">{t.actions[action].label}</span>
                  <span className="text-caption text-fg-3">{t.actions[action].description}</span>
                </span>
              </label>
            ))}
          </fieldset>
          {canEdit ? (
            <Field label={t.emergency} hint={t.emergencyHint}>
              {(props) => <PeoplePicker {...props} people={people} value={emergency} onChange={setEmergency} />}
            </Field>
          ) : (
            <div className="flex flex-col gap-2">
              <p className="text-label text-fg">{t.emergency}</p>
              <p className="text-body-sm text-fg-2">{emergency.map((id) => people.find((p) => p.id === id)?.name ?? t.gone).join(", ") || t.none}</p>
            </div>
          )}
          {canEdit ? (
            <div className="flex flex-col gap-3">
              {on ? <ReadOnlyNote>{t.selfApproval}</ReadOnlyNote> : null}
              <div className="flex flex-wrap items-center gap-4">
                <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
                  {t.save}
                </Button>
                <ActionError message={failure} />
              </div>
            </div>
          ) : (
            <ReadOnlyNote>{t.readOnly}</ReadOnlyNote>
          )}
        </form>
      </CardBody>
    </Card>
  );
}
