"use client";

import { KeyRound, Plus, RadioTower, Trash2 } from "lucide-react";
import { useState } from "react";
import { SecretWarning } from "@/components/applications/shared";
import { useMutation } from "@/components/console/use-mutation";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Field, Input } from "@/components/ui/input";
import { SectionTitle } from "@/components/ui/section-title";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { formatDateTime } from "@/lib/format";
import { useLocale } from "@/lib/i18n/client";
import { infrastructureMessages } from "@/lib/i18n/console/infrastructure";
import { remoteMessages } from "@/lib/i18n/console/infrastructure/remote";
import type { Outpost } from "@/lib/infra-types";

type Enrollment = { outpost: Outpost; token: string };

export function enrollCommand(issuer: string, token: string) {
  return `halo-outpost --server ${issuer} --token ${token}`;
}

export function OutpostsSection({ outposts, issuer, canEdit }: { outposts: Outpost[]; issuer: string; canEdit: boolean }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [enrolling, setEnrolling] = useState<Outpost | "new" | null>(null);
  const [deleting, setDeleting] = useState<Outpost | null>(null);
  const locale = useLocale();
  const common = infrastructureMessages[locale];
  const t = remoteMessages[locale].outposts;

  function remove() {
    const outpost = deleting;
    if (!outpost) return;
    void run(
      "delete",
      () => api(`/outposts/${outpost.id}`, { method: "DELETE" }),
      () => {
        setDeleting(null);
        toast({ title: common.deleted(outpost.name), description: t.deletedHint });
      },
      "inline",
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <SectionTitle
        title={t.title}
        description={t.hint}
        action={
          canEdit ? (
            <Button size="sm" variant="primary" onClick={() => setEnrolling("new")}>
              <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
              {t.register}
            </Button>
          ) : undefined
        }
      />
      {outposts.length ? (
        <SimpleTable
          caption={t.title}
          head={t.head}
          rows={outposts.map((o) => [
            <span key="name" className="font-medium text-fg">
              {o.name}
            </span>,
            <span key="status" className="flex flex-col items-start gap-1 whitespace-nowrap">
              {o.connected ? (
                <Badge tone="success" dot>
                  {t.connected}
                </Badge>
              ) : o.enrolledAt ? (
                <Badge tone="warning" dot>
                  {t.notConnected}
                </Badge>
              ) : (
                <Badge dot>{common.enrollment.waiting}</Badge>
              )}
              {!o.enrolledAt && o.enrollmentExpiresAt ? <span className="text-caption text-fg-3">{common.enrollment.tokenUntil(formatDateTime(o.enrollmentExpiresAt, locale))}</span> : null}
            </span>,
            <span key="targets" className="tnum">
              {t.applications(o.targets)}
            </span>,
            o.lastSeenAt ? (
              <span key="seen" className="flex flex-col whitespace-nowrap">
                <span>{formatDateTime(o.lastSeenAt, locale)}</span>
                <span className="font-mono text-code-sm text-fg-3">
                  {o.lastIp}
                  {o.version ? ` · ${o.version}` : null}
                </span>
              </span>
            ) : (
              <span key="seen" className="text-fg-3">
                {t.never}
              </span>
            ),
            canEdit ? (
              <span key="actions" className="flex gap-1">
                <Button size="icon-sm" variant="quiet" aria-label={common.enrollment.newToken(o.name)} onClick={() => setEnrolling(o)}>
                  <KeyRound aria-hidden="true" size={16} strokeWidth={1.75} />
                </Button>
                <Button size="icon-sm" variant="quiet" aria-label={common.delete(o.name)} onClick={() => setDeleting(o)}>
                  <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
                </Button>
              </span>
            ) : (
              <span key="actions" />
            ),
          ])}
        />
      ) : (
        <Card>
          <EmptyState
            icon={RadioTower}
            title={t.empty}
            description={t.emptyHint}
          />
        </Card>
      )}
      {enrolling ? <EnrollmentDialog key={enrolling === "new" ? "new" : enrolling.id} outpost={enrolling === "new" ? null : enrolling} issuer={issuer} onClose={() => setEnrolling(null)} /> : null}
      <Dialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (open) return;
          setDeleting(null);
          clearError();
        }}
        error={errorFor("delete")}
        title={t.deleteTitle(deleting?.name ?? null)}
        description={t.deleteHint}
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(null)}>
              {common.cancel}
            </Button>
            <Button variant="destructive" loading={pending === "delete"} onClick={remove}>
              {t.deleteConfirm}
            </Button>
          </>
        }
      />
    </div>
  );
}

function EnrollmentDialog({ outpost, issuer, onClose }: { outpost: Outpost | null; issuer: string; onClose: () => void }) {
  const { pending, run, errorFor } = useMutation();
  const [name, setName] = useState("");
  const [error, setError] = useState<string>();
  const [issued, setIssued] = useState<Enrollment | null>(null);
  const locale = useLocale();
  const common = infrastructureMessages[locale];
  const t = remoteMessages[locale].outposts;

  function submit() {
    if (!outpost && !name.trim()) {
      setError(t.nameError);
      return;
    }
    setError(undefined);
    void run(
      "enroll",
      () => (outpost ? api<Enrollment>(`/outposts/${outpost.id}/enrollment-token`, { method: "POST" }) : api<Enrollment>("/outposts", { body: { name: name.trim() } })),
      setIssued,
      "inline",
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      error={errorFor("enroll")}
      title={issued ? common.enrollment.enroll(issued.outpost.name) : outpost ? common.enrollment.newToken(outpost.name) : t.register}
      description={
        issued
          ? t.enrollHint
          : outpost
            ? t.tokenHint
            : t.registerHint
      }
      footer={
        issued ? (
          <Button variant="primary" onClick={onClose}>
            {common.done}
          </Button>
        ) : (
          <>
            <Button variant="secondary" onClick={onClose}>
              {common.cancel}
            </Button>
            <Button variant="primary" loading={pending === "enroll"} onClick={submit}>
              {outpost ? common.enrollment.createToken : t.register}
            </Button>
          </>
        )
      }
    >
      {issued ? (
        <div className="flex animate-enter flex-col gap-4">
          <CopyField label={common.enrollment.command} value={enrollCommand(issuer, issued.token)} secret />
          <SecretWarning>
            {common.enrollment.onlyOnce(issued.outpost.enrollmentExpiresAt ? formatDateTime(issued.outpost.enrollmentExpiresAt, locale) : null)}
          </SecretWarning>
        </div>
      ) : outpost ? null : (
        <form
          className="flex flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <Field label={common.name} error={error}>
            {(props) => <Input {...props} autoFocus autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder={t.namePlaceholder} />}
          </Field>
          <button type="submit" hidden />
        </form>
      )}
    </Dialog>
  );
}
