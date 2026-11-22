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
import { formatDateTime, pluralize } from "@/lib/format";
import type { Outpost } from "@/lib/infra-types";

type Enrollment = { outpost: Outpost; token: string };

export function enrollCommand(issuer: string, token: string) {
  return `halo-outpost --server ${issuer} --token ${token}`;
}

export function OutpostsSection({ outposts, issuer, canEdit }: { outposts: Outpost[]; issuer: string; canEdit: boolean }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [enrolling, setEnrolling] = useState<Outpost | "new" | null>(null);
  const [deleting, setDeleting] = useState<Outpost | null>(null);

  function remove() {
    const outpost = deleting;
    if (!outpost) return;
    void run(
      "delete",
      () => api(`/outposts/${outpost.id}`, { method: "DELETE" }),
      () => {
        setDeleting(null);
        toast({ title: `${outpost.name} deleted`, description: "Its key stopped working and it was disconnected. Recorded in the audit log." });
      },
      "inline",
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <SectionTitle
        title="Outposts"
        description="Agents inside your networks that connect out to Halo and reach hosts for it. Each remote access application names the outpost that reaches its host."
        action={
          canEdit ? (
            <Button size="sm" variant="primary" onClick={() => setEnrolling("new")}>
              <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
              Register outpost
            </Button>
          ) : undefined
        }
      />
      {outposts.length ? (
        <SimpleTable
          caption="Outposts"
          head={["Name", "Status", "Applications", "Last seen", "Actions"]}
          rows={outposts.map((o) => [
            <span key="name" className="font-medium text-fg">
              {o.name}
            </span>,
            <span key="status" className="flex flex-col items-start gap-1 whitespace-nowrap">
              {o.connected ? (
                <Badge tone="success" dot>
                  Connected
                </Badge>
              ) : o.enrolledAt ? (
                <Badge tone="warning" dot>
                  Not connected
                </Badge>
              ) : (
                <Badge dot>Waiting for enrollment</Badge>
              )}
              {!o.enrolledAt && o.enrollmentExpiresAt ? <span className="text-caption text-fg-3">Token works until {formatDateTime(o.enrollmentExpiresAt)}</span> : null}
            </span>,
            <span key="targets" className="tnum">
              {pluralize(o.targets, "application")}
            </span>,
            o.lastSeenAt ? (
              <span key="seen" className="flex flex-col whitespace-nowrap">
                <span>{formatDateTime(o.lastSeenAt)}</span>
                <span className="font-mono text-code-sm text-fg-3">
                  {o.lastIp}
                  {o.version ? ` · ${o.version}` : null}
                </span>
              </span>
            ) : (
              <span key="seen" className="text-fg-3">
                Never
              </span>
            ),
            canEdit ? (
              <span key="actions" className="flex gap-1">
                <Button size="icon-sm" variant="quiet" aria-label={`New enrollment token for ${o.name}`} onClick={() => setEnrolling(o)}>
                  <KeyRound aria-hidden="true" size={16} strokeWidth={1.75} />
                </Button>
                <Button size="icon-sm" variant="quiet" aria-label={`Delete ${o.name}`} onClick={() => setDeleting(o)}>
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
            title="No outposts yet"
            description="Register an outpost, then run halo-outpost on a machine that can reach your hosts. It only connects out to Halo, so it needs no inbound firewall rule."
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
        title={`Delete ${deleting?.name ?? "this outpost"}?`}
        description="Its key stops working at once, which disconnects it and ends its live sessions. Move or delete the applications that use it first."
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(null)}>
              Cancel
            </Button>
            <Button variant="destructive" loading={pending === "delete"} onClick={remove}>
              Delete outpost
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

  function submit() {
    if (!outpost && !name.trim()) {
      setError("Name the outpost after the network it reaches, like Office LAN.");
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
      title={issued ? `Enroll ${issued.outpost.name}` : outpost ? `New enrollment token for ${outpost.name}` : "Register outpost"}
      description={
        issued
          ? "Run this on a machine inside the network. halo-outpost enrolls once, saves its key in halo-outpost.key and then connects with the key."
          : outpost
            ? "Enrolling with the new token gives the outpost a new key. The current key stops working then, so delete halo-outpost.key on the machine before you run it."
            : "An outpost reaches the hosts of one network for Halo. Register one per network."
      }
      footer={
        issued ? (
          <Button variant="primary" onClick={onClose}>
            Done
          </Button>
        ) : (
          <>
            <Button variant="secondary" onClick={onClose}>
              Cancel
            </Button>
            <Button variant="primary" loading={pending === "enroll"} onClick={submit}>
              {outpost ? "Create token" : "Register outpost"}
            </Button>
          </>
        )
      }
    >
      {issued ? (
        <div className="flex animate-enter flex-col gap-4">
          <CopyField label="Enrollment command" value={enrollCommand(issuer, issued.token)} secret />
          <SecretWarning>
            This is the only time Halo shows this token. It works once, until {issued.outpost.enrollmentExpiresAt ? formatDateTime(issued.outpost.enrollmentExpiresAt) : "it expires"}.
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
          <Field label="Name" error={error}>
            {(props) => <Input {...props} autoFocus autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder="Office LAN" />}
          </Field>
          <button type="submit" hidden />
        </form>
      )}
    </Dialog>
  );
}
