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
import { formatDate, pluralize } from "@/lib/format";
import type { Recommendation } from "@/lib/types";

const STATUS = {
  fail: { label: "Fail", icon: TriangleAlert, className: "border-danger/20 bg-danger-container text-danger" },
  attention: { label: "Attention", icon: CircleAlert, className: "border-warning/20 bg-warning-container text-warning" },
  pass: { label: "Pass", icon: CircleCheck, className: "border-success/20 bg-success-container text-success" },
};

function Row({ check, children }: { check: Recommendation; children?: ReactNode }) {
  const status = STATUS[check.status];
  const Icon = status.icon;
  return (
    <li className="flex flex-col gap-4 border-t border-border px-6 py-4 sm:flex-row sm:items-center">
      <span className={cn("grid size-8 shrink-0 place-items-center rounded-md border", status.className)}>
        <Icon aria-hidden="true" size={16} strokeWidth={1.75} />
        <span className="sr-only">{status.label}</span>
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <p className="text-body-sm font-semibold text-fg">{check.title}</p>
        <p className="text-body-sm text-fg-3">{check.detail}</p>
        {check.dismissal ? (
          <p className="text-body-sm text-fg-2">
            Dismissed by {check.dismissal.by?.name ?? "a former administrator"} on {formatDate(check.dismissal.at)}: {check.dismissal.reason}
          </p>
        ) : null}
      </div>
      {children ? <div className="flex shrink-0 flex-wrap gap-2 self-start sm:self-center">{children}</div> : null}
    </li>
  );
}

function DismissDialog({ check, onClose }: { check: Recommendation; onClose: () => void }) {
  const { pending, run, errorFor, toast } = useMutation();
  const formId = useId();
  const [reason, setReason] = useState("");

  return (
    <Dialog
      open
      onOpenChange={onClose}
      error={errorFor("dismiss")}
      title={`Dismiss “${check.title}”?`}
      description="It moves to Dismissed and stays out of the weekly digest until someone restores it. Your reason is shown here and recorded in the audit log."
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" type="submit" form={formId} loading={pending === "dismiss"}>
            Dismiss
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
              toast({ title: "Recommendation dismissed", description: "Recorded in the audit log with your reason." });
            },
            "inline",
          );
        }}
      >
        <Field label="Reason" hint="For example the compensating control, or the date you plan to fix it.">
          {(props) => <Textarea {...props} rows={3} maxLength={500} value={reason} onChange={(event) => setReason(event.target.value)} />}
        </Field>
      </form>
    </Dialog>
  );
}

export function Recommendations({ checks, canEdit }: { checks: Recommendation[]; canEdit: boolean }) {
  const { pending, run, toast } = useMutation();
  const [dismissing, setDismissing] = useState<Recommendation | null>(null);
  const open = checks.filter((c) => c.status !== "pass" && !c.dismissal).sort((a, b) => Number(b.status === "fail") - Number(a.status === "fail"));
  const passing = checks.filter((c) => c.status === "pass" && !c.dismissal);
  const dismissed = checks.filter((c) => c.dismissal);

  return (
    <>
      <Card>
        <CardHeader
          title="Needs action"
          description={open.length ? `${pluralize(open.length, "check")}, most urgent first.` : undefined}
        />
        {open.length === 0 ? <EmptyState icon={ShieldCheck} title="Nothing needs action" description="Every check passes or has been dismissed." /> : null}
        <ul>
          {open.map((check) => (
            <Row key={check.id} check={check}>
              <Link href={check.href} className={buttonClasses("secondary", "sm")}>
                {check.action}
              </Link>
              {canEdit ? (
                <Button size="sm" variant="quiet" onClick={() => setDismissing(check)}>
                  Dismiss
                </Button>
              ) : null}
            </Row>
          ))}
        </ul>
      </Card>

      {dismissed.length ? (
        <Card>
          <CardHeader title="Dismissed" description="Left out of the counts above and the weekly digest, whatever their status." />
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
                        () => toast({ title: "Recommendation restored", description: "Recorded in the audit log." }),
                      )
                    }
                  >
                    Restore
                  </Button>
                ) : null}
              </Row>
            ))}
          </ul>
        </Card>
      ) : null}

      {passing.length ? (
        <Card>
          <CardHeader title="Passing" />
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
