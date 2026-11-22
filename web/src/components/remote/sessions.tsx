"use client";

import { Download, SquareTerminal, SquareX } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Badge } from "@/components/ui/badge";
import { Button, buttonClasses } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { formatDateTime, formatDuration, pluralize } from "@/lib/format";
import type { RemoteSession } from "@/lib/infra-types";

function elapsed(from: string, to: string) {
  const seconds = Math.max(0, Math.round((new Date(to).getTime() - new Date(from).getTime()) / 1000));
  return seconds < 60 ? pluralize(seconds, "second") : formatDuration(Math.round(seconds / 60) * 60);
}

function size(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  const [value, unit] = bytes < 1024 * 1024 ? [bytes / 1024, "KB"] : [bytes / 1024 / 1024, "MB"];
  return `${value.toLocaleString("en-US", { maximumFractionDigits: 1 })} ${unit}`;
}

export function RemoteSessions({ sessions, showApplication = true, canEnd, canDownload, empty }: { sessions: RemoteSession[]; showApplication?: boolean; canEnd: boolean; canDownload: boolean; empty: string }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [ending, setEnding] = useState<RemoteSession | null>(null);

  function end() {
    const session = ending;
    if (!session) return;
    void run(
      "end",
      () => api(`/remote-sessions/${session.id}/terminate`, { method: "POST" }),
      () => {
        setEnding(null);
        toast({ title: "Session ended", description: `${session.email} was disconnected from ${session.address}. Recorded in the audit log.` });
      },
      "inline",
    );
  }

  if (!sessions.length) {
    return (
      <Card>
        <EmptyState icon={SquareTerminal} title="No remote sessions yet" description={empty} />
      </Card>
    );
  }

  return (
    <>
      <SimpleTable
        caption="Remote sessions"
        head={["Person", showApplication ? "Application" : "Host", "Started", "Status", "Transferred", "Actions"]}
        rows={sessions.map((s) => [
          <span key="person" className="flex flex-col">
            <span className="text-fg">{s.email}</span>
            <span className="font-mono text-code-sm text-fg-3">{s.ip}</span>
          </span>,
          <span key="target" className="flex flex-col">
            {showApplication ? <span className="text-fg">{s.applicationName}</span> : null}
            <span className="font-mono text-code-sm break-all text-fg-2">
              {s.username}@{s.address}
            </span>
          </span>,
          <span key="started" className="whitespace-nowrap">
            {formatDateTime(s.startedAt ?? s.createdAt)}
          </span>,
          <span key="status" className="flex max-w-80 flex-col items-start gap-1">
            {s.status === "active" ? (
              <Badge tone="success" dot>
                Live
              </Badge>
            ) : s.status === "pending" ? (
              <Badge tone="warning" dot>
                Connecting
              </Badge>
            ) : (
              <Badge dot>{s.startedAt && s.endedAt ? `Ended after ${elapsed(s.startedAt, s.endedAt)}` : "Ended"}</Badge>
            )}
            {s.endReason ? <span className="text-caption text-fg-3">{s.endReason}</span> : null}
          </span>,
          <span key="bytes" className="tnum flex flex-col whitespace-nowrap">
            <span>{size(s.bytesOut)} out</span>
            <span className="text-caption text-fg-3">{size(s.bytesIn)} typed</span>
          </span>,
          <span key="actions" className="flex gap-1">
            {canDownload && s.recorded ? (
              <a href={`/api/v1/remote-sessions/${s.id}/recording`} download aria-label={`Download the recording of ${s.email}'s session`} className={buttonClasses("quiet", "icon-sm")}>
                <Download aria-hidden="true" size={16} strokeWidth={1.75} />
              </a>
            ) : null}
            {canEnd && s.status !== "ended" ? (
              <Button size="icon-sm" variant="quiet" aria-label={`End ${s.email}'s session`} onClick={() => setEnding(s)}>
                <SquareX aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
            ) : null}
          </span>,
        ])}
      />
      <Dialog
        open={ending !== null}
        onOpenChange={(open) => {
          if (open) return;
          setEnding(null);
          clearError();
        }}
        error={errorFor("end")}
        title={`End ${ending ? `${ending.email}'s` : "this"} session?`}
        description={`Halo closes the remote session to ${ending?.address ?? "the host"} and tells the browser it was ended from the console. They can start a new one if they still have access.`}
        footer={
          <>
            <Button variant="secondary" onClick={() => setEnding(null)}>
              Cancel
            </Button>
            <Button variant="destructive" loading={pending === "end"} onClick={end}>
              End session
            </Button>
          </>
        }
      />
    </>
  );
}
