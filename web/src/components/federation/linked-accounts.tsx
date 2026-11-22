"use client";

import { Unlink } from "lucide-react";
import { useEffect, useState } from "react";
import { describeFailure } from "@/components/auth/passkey";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { RelativeTime } from "@/components/ui/relative-time";
import { SectionTitle } from "@/components/ui/section-title";
import { Skeleton } from "@/components/ui/skeleton";
import { useToast } from "@/components/ui/toast";
import { api } from "@/lib/api/client";
import type { LinkedAccount } from "@/lib/federation-types";
import { formatDate } from "@/lib/format";
import { useLocale } from "@/lib/i18n/client";
import { settingsMessages } from "@/lib/i18n/console/settings";
import { METHOD } from "@/lib/labels";

export function LinkedAccounts() {
  const toast = useToast();
  const [accounts, setAccounts] = useState<LinkedAccount[] | null>(null);
  const [failed, setFailed] = useState(false);
  const [pending, setPending] = useState<LinkedAccount | null>(null);
  const [removing, setRemoving] = useState(false);
  const Icon = METHOD.federated.icon;
  const locale = useLocale();
  const { linkedAccounts: t, common } = settingsMessages[locale];

  useEffect(() => {
    api<LinkedAccount[]>("/me/identities").then(setAccounts, () => setFailed(true));
  }, []);

  async function unlink() {
    if (!pending) return;
    setRemoving(true);
    try {
      await api(`/me/identities/${pending.id}`, { method: "DELETE" });
      setAccounts((current) => current?.filter((a) => a.id !== pending.id) ?? null);
      toast({ title: t.unlinked(pending.providerName), description: t.unlinkedHint(pending.email) });
      setPending(null);
    } catch (error) {
      toast({ title: common.failed, description: describeFailure(error), tone: "danger" });
    } finally {
      setRemoving(false);
    }
  }

  return (
    <section className="flex flex-col gap-4">
      <SectionTitle
        title={t.title}
        description={t.description}
      />
      {failed ? (
        <p role="alert" className="rounded-lg border border-border px-6 py-4 text-body-sm text-fg-3">
          {t.loadFailed}
        </p>
      ) : accounts === null ? (
        <div aria-busy="true" aria-label={t.loading} className="flex items-center gap-4 rounded-lg border border-border bg-surface p-4 sm:px-6">
          <span className="grid size-10 shrink-0 place-items-center rounded-md border border-border bg-sunken text-fg-3">
            <Icon aria-hidden="true" size={20} strokeWidth={1.75} />
          </span>
          <div className="flex flex-1 flex-col gap-2">
            <Skeleton className="h-3 w-32 rounded-sm" />
            <Skeleton className="h-3 w-48 rounded-sm" />
          </div>
        </div>
      ) : accounts.length === 0 ? (
        <p className="rounded-lg border border-border px-6 py-4 text-body-sm text-fg-3">
          {t.empty}
        </p>
      ) : (
        <ul className="animate-enter overflow-hidden rounded-lg border border-border bg-surface">
          {accounts.map((account) => (
            <li key={account.id} className="flex flex-col gap-4 border-t border-border p-4 first:border-t-0 sm:flex-row sm:items-center sm:px-6">
              <span className="grid size-10 shrink-0 place-items-center rounded-md border border-border bg-sunken text-fg-2">
                <Icon aria-hidden="true" size={20} strokeWidth={1.75} />
              </span>
              <div className="flex min-w-0 flex-1 flex-col gap-1">
                <p className="text-body-sm font-semibold text-fg">{account.providerName}</p>
                <p className="truncate text-body-sm text-fg-3">{account.email || t.noEmail}</p>
                <p className="text-caption text-fg-3">
                  {t.linked(formatDate(account.createdAt, locale))} · {account.lastUsedAt ? <>{t.lastUsed} <RelativeTime iso={account.lastUsedAt} lowercase /></> : t.neverUsed}
                </p>
              </div>
              <Button size="sm" variant="quiet" onClick={() => setPending(account)} className="self-start sm:self-center" aria-label={t.unlinkLabel(account.providerName)}>
                <Unlink aria-hidden="true" size={16} strokeWidth={1.75} />
                {t.unlink}
              </Button>
            </li>
          ))}
        </ul>
      )}
      <Dialog
        open={pending !== null}
        onOpenChange={(open) => {
          if (!open) setPending(null);
        }}
        title={t.confirmTitle(pending?.providerName ?? t.accountFallback)}
        description={t.confirmDescription(pending?.email || t.thisAccount, pending?.providerName ?? t.providerFallback)}
        footer={
          <>
            <Button variant="secondary" onClick={() => setPending(null)}>
              {common.cancel}
            </Button>
            <Button variant="destructive" loading={removing} onClick={unlink}>
              {t.unlink}
            </Button>
          </>
        }
      />
    </section>
  );
}
