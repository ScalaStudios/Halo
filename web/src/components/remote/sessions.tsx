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
import { formatDateTime } from "@/lib/format";
import { useLocale } from "@/lib/i18n/client";
import { infrastructureMessages } from "@/lib/i18n/console/infrastructure";
import { remoteMessages } from "@/lib/i18n/console/infrastructure/remote";
import type { Locale } from "@/lib/i18n/locale";
import type { RemoteSession } from "@/lib/infra-types";

function elapsed(from: string, to: string, locale: Locale) {
  const seconds = Math.max(0, Math.round((new Date(to).getTime() - new Date(from).getTime()) / 1000));
  return seconds < 60 ? remoteMessages[locale].sessions.seconds(seconds) : infrastructureMessages[locale].duration(Math.round(seconds / 60) * 60);
}

function size(bytes: number, locale: Locale) {
  if (bytes < 1024) return `${bytes} B`;
  const [value, unit] = bytes < 1024 * 1024 ? [bytes / 1024, "KB"] : [bytes / 1024 / 1024, "MB"];
  return `${value.toLocaleString(locale, { maximumFractionDigits: 1 })} ${unit}`;
}

export function RemoteSessions({ sessions, showApplication = true, canEnd, canDownload, empty }: { sessions: RemoteSession[]; showApplication?: boolean; canEnd: boolean; canDownload: boolean; empty: string }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [ending, setEnding] = useState<RemoteSession | null>(null);
  const locale = useLocale();
  const t = remoteMessages[locale].sessions;
  const common = infrastructureMessages[locale];

  function end() {
    const session = ending;
    if (!session) return;
    void run(
      "end",
      () => api(`/remote-sessions/${session.id}/terminate`, { method: "POST" }),
      () => {
        setEnding(null);
        toast({ title: t.ended, description: t.endedHint(session.email, session.address) });
      },
      "inline",
    );
  }

  if (!sessions.length) {
    return (
      <Card>
        <EmptyState icon={SquareTerminal} title={t.empty} description={empty} />
      </Card>
    );
  }

  return (
    <>
      <SimpleTable
        caption={t.caption}
        head={[t.person, showApplication ? t.application : t.host, t.started, t.status, t.transferred, common.actions]}
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
            {formatDateTime(s.startedAt ?? s.createdAt, locale)}
          </span>,
          <span key="status" className="flex max-w-80 flex-col items-start gap-1">
            {s.status === "active" ? (
              <Badge tone="success" dot>
                {t.live}
              </Badge>
            ) : s.status === "pending" ? (
              <Badge tone="warning" dot>
                {t.connecting}
              </Badge>
            ) : (
              <Badge dot>{s.startedAt && s.endedAt ? t.endedAfter(elapsed(s.startedAt, s.endedAt, locale)) : t.endedBadge}</Badge>
            )}
            {s.endReason ? <span lang="en" className="text-caption text-fg-3">{s.endReason}</span> : null}
          </span>,
          <span key="bytes" className="tnum flex flex-col whitespace-nowrap">
            <span>{t.out(size(s.bytesOut, locale))}</span>
            <span className="text-caption text-fg-3">{t.typed(size(s.bytesIn, locale))}</span>
          </span>,
          <span key="actions" className="flex gap-1">
            {canDownload && s.recorded ? (
              <a href={`/api/v1/remote-sessions/${s.id}/recording`} download aria-label={t.download(s.email)} className={buttonClasses("quiet", "icon-sm")}>
                <Download aria-hidden="true" size={16} strokeWidth={1.75} />
              </a>
            ) : null}
            {canEnd && s.status !== "ended" ? (
              <Button size="icon-sm" variant="quiet" aria-label={t.end(s.email)} onClick={() => setEnding(s)}>
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
        title={t.endTitle(ending?.email ?? null)}
        description={t.endHint(ending?.address ?? null)}
        footer={
          <>
            <Button variant="secondary" onClick={() => setEnding(null)}>
              {common.cancel}
            </Button>
            <Button variant="destructive" loading={pending === "end"} onClick={end}>
              {t.endConfirm}
            </Button>
          </>
        }
      />
    </>
  );
}
