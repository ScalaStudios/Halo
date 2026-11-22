"use client";

import { Laptop, LogOut, MonitorSmartphone, Smartphone, Terminal } from "lucide-react";
import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/ui/empty-state";
import { HoldToConfirm } from "@/components/ui/hold-to-confirm";
import { RelativeTime } from "@/components/ui/relative-time";
import { useToast } from "@/components/ui/toast";
import { api, ApiError } from "@/lib/api/client";
import { accountMessages } from "@/lib/i18n/account";
import type { Locale } from "@/lib/i18n/locale";
import { monitoringMessages } from "@/lib/i18n/console/monitoring";
import type { Session } from "@/lib/types";

function deviceIcon(session: Session) {
  if (/kubelogin|cli/i.test(session.browser)) return Terminal;
  if (/iOS|Android|iPhone|Pixel|phone/i.test(`${session.os} ${session.device}`)) return Smartphone;
  return Laptop;
}

export function SessionList({ sessions, userId, locale }: { sessions: Session[]; userId?: string; locale: Locale }) {
  const account = accountMessages(locale);
  const copy = account.sessions;
  const self = userId === undefined;
  const [items, setItems] = useState(sessions);
  const [revoking, setRevoking] = useState<string | null>(null);
  const toast = useToast();
  const others = items.filter((s) => !s.current);

  function fail(error: unknown) {
    toast({ title: copy.failed, description: error instanceof ApiError ? error.message : copy.connectionFailed, tone: "danger" });
  }

  async function revoke(session: Session) {
    setRevoking(session.id);
    try {
      await api(self ? `/me/sessions/${session.id}` : `/sessions/${session.id}`, { method: "DELETE" });
      setItems((current) => current.filter((s) => s.id !== session.id));
      toast({
        title: copy.ended,
        description: session.client ? copy.cliEnded(session.device) : copy.browserEnded(session.device, session.browser),
      });
    } catch (error) {
      fail(error);
    } finally {
      setRevoking(null);
    }
  }

  async function revokeAll() {
    try {
      const { revoked } = await api<{ revoked: number }>(self ? "/me/sessions/revoke-others" : `/users/${userId}/revoke-sessions`, { method: "POST", body: {} });
      setItems((current) => current.filter((s) => s.current));
      toast({
        title: self ? copy.othersEnded : copy.allRevoked,
        description: self ? copy.othersEndedDetail(revoked) : copy.allRevokedHint,
      });
      return true;
    } catch (error) {
      fail(error);
      return false;
    }
  }

  if (items.length === 0) {
    return <EmptyState icon={MonitorSmartphone} title={copy.empty} description={copy.emptyHint} />;
  }

  return (
    <div className="flex flex-col gap-4">
      <ul className="overflow-hidden rounded-lg border border-border bg-surface">
        {items.map((session) => {
          const Icon = deviceIcon(session);
          const method = monitoringMessages[locale].methods[session.method].toLowerCase();
          return (
            <li key={session.id} className="flex flex-col gap-4 border-t border-border p-4 first:border-t-0 sm:flex-row sm:items-center sm:px-6">
              <span className="grid size-10 shrink-0 place-items-center rounded-md border border-border bg-sunken text-fg-3">
                <Icon aria-hidden="true" size={20} strokeWidth={1.75} />
              </span>
              {session.client ? (
                <div className="flex min-w-0 flex-1 flex-col gap-1">
                  <p className="text-body-sm font-semibold text-fg">Halo CLI</p>
                  <p className="text-body-sm text-fg-3">
                    {session.device} · <span className="font-mono text-code-sm">{session.ip}</span>
                  </p>
                  <p className="text-caption text-fg-3">
                    {copy.approved} <RelativeTime locale={locale} iso={session.createdAt} lowercase /> {copy.withMethod(method)} · {copy.lastRefresh}{" "}
                    <RelativeTime locale={locale} iso={session.lastActiveAt} lowercase />
                  </p>
                </div>
              ) : (
                <div className="flex min-w-0 flex-1 flex-col gap-1">
                  <p className="flex flex-wrap items-center gap-2 text-body-sm font-semibold text-fg">
                    {session.device} · {session.browser}
                    {session.current ? <Badge tone="success">{copy.current}</Badge> : null}
                    {session.impersonationId ? <Badge tone="warning">{account.impersonation}</Badge> : null}
                  </p>
                  <p className="text-body-sm text-fg-3">
                    {session.location} · <span className="font-mono text-code-sm">{session.ip}</span>
                  </p>
                  <p className="text-caption text-fg-3">
                    {copy.signedIn} <RelativeTime locale={locale} iso={session.createdAt} lowercase /> {copy.withMethod(method)} · {copy.active}{" "}
                    <RelativeTime locale={locale} iso={session.lastActiveAt} lowercase />
                  </p>
                </div>
              )}
              {session.current ? null : (
                <Button size="sm" variant="secondary" loading={revoking === session.id} onClick={() => revoke(session)} className="self-start sm:self-center">
                  <LogOut aria-hidden="true" size={16} strokeWidth={1.75} />
                  {account.signOut}
                </Button>
              )}
            </li>
          );
        })}
      </ul>
      {others.length > 0 ? (
        <div className="flex flex-col gap-3 rounded-lg border border-border p-4 sm:flex-row sm:items-center sm:justify-between sm:px-6">
          <div className="flex flex-col gap-1">
            <p className="text-body-sm font-semibold text-fg">{self ? copy.signOutElsewhere : copy.revokeAll}</p>
            <p className="text-body-sm text-fg-3">
              {self ? copy.signOutElsewhereHint(others.length) : copy.revokeAllHint(others.length)}
            </p>
          </div>
          <HoldToConfirm size="sm" doneLabel={copy.done} onConfirm={revokeAll}>
            {copy.hold(others.length)}
          </HoldToConfirm>
        </div>
      ) : null}
    </div>
  );
}
