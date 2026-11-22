"use client";

import Link from "next/link";
import { CircleAlert, CircleCheck, ShieldCheck, TriangleAlert } from "lucide-react";
import { useId, useState, type ReactNode } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Button, buttonClasses } from "@/components/ui/button";
import { Card, CardHeader } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Field, Textarea } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import { cn } from "@/lib/cn";
import { formatDate } from "@/lib/format";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { monitoringMessages } from "@/lib/i18n/console/monitoring";
import type { Recommendation } from "@/lib/types";

const STATUS = {
  fail: { icon: TriangleAlert, className: "border-danger/20 bg-danger-container text-danger" },
  attention: { icon: CircleAlert, className: "border-warning/20 bg-warning-container text-warning" },
  pass: { icon: CircleCheck, className: "border-success/20 bg-success-container text-success" },
};

function Row({ check, children }: { check: Recommendation; children?: ReactNode }) {
  const locale = useLocale();
  const t = monitoringMessages[locale].recommendations;
  const status = STATUS[check.status];
  const Icon = status.icon;
  return (
    <li className="flex flex-col gap-4 border-t border-border px-6 py-4 sm:flex-row sm:items-center">
      <span className={cn("grid size-8 shrink-0 place-items-center rounded-md border", status.className)}>
        <Icon aria-hidden="true" size={16} strokeWidth={1.75} />
        <span className="sr-only">{t.status[check.status]}</span>
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <p className="text-body-sm font-semibold text-fg">{check.title}</p>
        <p className="text-body-sm text-fg-3">{check.detail}</p>
        {check.dismissal ? (
          <p className="text-body-sm text-fg-2">
            {t.dismissedBy(check.dismissal.by?.name, formatDate(check.dismissal.at, locale))}
            {check.dismissal.reason}
          </p>
        ) : null}
      </div>
      {children ? <div className="flex shrink-0 flex-wrap gap-2 self-start sm:self-center">{children}</div> : null}
    </li>
  );
}

function DismissDialog({ check, onClose }: { check: Recommendation; onClose: () => void }) {
  const t = useMessages(monitoringMessages).recommendations;
  const { pending, run, errorFor, toast } = useMutation();
  const formId = useId();
  const [reason, setReason] = useState("");

  return (
    <Dialog
      open
      onOpenChange={onClose}
      error={errorFor("dismiss")}
      title={t.dismissTitle(check.title)}
      description={t.dismissDescription}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t.cancel}
          </Button>
          <Button variant="primary" type="submit" form={formId} loading={pending === "dismiss"}>
            {t.dismiss}
          </Button>
        </>
      }
    >
      <form
        id={formId}
        noValidate
        onSubmit={(event) => {
          event.preventDefault();
          void run(
            "dismiss",
            () => api(`/recommendations/${check.id}/dismissal`, { method: "PUT", body: { reason } }),
            () => {
              onClose();
              toast({ title: t.dismissed, description: t.dismissedHint });
            },
            "inline",
          );
        }}
      >
        <Field label={t.reason} hint={t.reasonHint}>
          {(props) => <Textarea {...props} rows={3} maxLength={500} value={reason} onChange={(event) => setReason(event.target.value)} />}
        </Field>
      </form>
    </Dialog>
  );
}

export function Recommendations({ checks, canEdit }: { checks: Recommendation[]; canEdit: boolean }) {
  const t = useMessages(monitoringMessages).recommendations;
  const { pending, run, toast } = useMutation();
  const [dismissing, setDismissing] = useState<Recommendation | null>(null);
  const open = checks.filter((c) => c.status !== "pass" && !c.dismissal).sort((a, b) => Number(b.status === "fail") - Number(a.status === "fail"));
  const passing = checks.filter((c) => c.status === "pass" && !c.dismissal);
  const dismissed = checks.filter((c) => c.dismissal);

  return (
    <>
      <Card>
        <CardHeader
          title={t.needsAction}
          description={open.length ? t.checks(open.length) : undefined}
        />
        {open.length === 0 ? <EmptyState icon={ShieldCheck} title={t.nothing} description={t.nothingHint} /> : null}
        <ul>
          {open.map((check) => (
            <Row key={check.id} check={check}>
              <Link href={check.href} className={buttonClasses("secondary", "sm")}>
                {check.action}
              </Link>
              {canEdit ? (
                <Button size="sm" variant="quiet" onClick={() => setDismissing(check)}>
                  {t.dismiss}
                </Button>
              ) : null}
            </Row>
          ))}
        </ul>
      </Card>

      {dismissed.length ? (
        <Card>
          <CardHeader title={t.dismissedTitle} description={t.dismissedDescription} />
          <ul>
            {dismissed.map((check) => (
              <Row key={check.id} check={check}>
                {canEdit ? (
                  <Button
                    size="sm"
                    variant="secondary"
                    loading={pending === `restore-${check.id}`}
                    onClick={() =>
                      run(
                        `restore-${check.id}`,
                        () => api(`/recommendations/${check.id}/dismissal`, { method: "DELETE" }),
                        () => toast({ title: t.restored, description: t.restoredHint }),
                      )
                    }
                  >
                    {t.restore}
                  </Button>
                ) : null}
              </Row>
            ))}
          </ul>
        </Card>
      ) : null}

      {passing.length ? (
        <Card>
          <CardHeader title={t.passing} />
          <ul>
            {passing.map((check) => (
              <Row key={check.id} check={check} />
            ))}
          </ul>
        </Card>
      ) : null}

      {dismissing ? <DismissDialog check={dismissing} onClose={() => setDismissing(null)} /> : null}
    </>
  );
}
