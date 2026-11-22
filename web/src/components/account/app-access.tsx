"use client";

import { useState } from "react";
import { describeFailure } from "@/components/auth/passkey";
import { Avatar } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { SectionTitle } from "@/components/ui/section-title";
import { useToast } from "@/components/ui/toast";
import { api } from "@/lib/api/client";
import { formatDate } from "@/lib/format";
import { accountMessages } from "@/lib/i18n/account";
import { useLocale } from "@/lib/i18n/client";
import { scopeLabel } from "@/lib/labels";
import type { Consent } from "@/lib/types";

export function AppAccess({ consents }: { consents: Consent[] }) {
  const locale = useLocale();
  const copy = accountMessages(locale);
  const t = copy.applications;
  const toast = useToast();
  const [items, setItems] = useState(consents);
  const [pending, setPending] = useState<Consent | null>(null);
  const [removing, setRemoving] = useState(false);

  async function remove() {
    if (!pending) return;
    setRemoving(true);
    try {
      await api(`/me/consents/${pending.appId}`, { method: "DELETE" });
      setItems((current) => current.filter((c) => c.appId !== pending.appId));
      toast({ title: t.removed(pending.name), description: t.removedHint });
      setPending(null);
    } catch (error) {
      toast({ title: copy.sessions.failed, description: describeFailure(error), tone: "danger" });
    } finally {
      setRemoving(false);
    }
  }

  if (items.length === 0) return null;

  return (
    <section className="flex flex-col gap-4">
      <SectionTitle title={t.allowed} description={t.allowedHint} />
      <ul className="overflow-hidden rounded-lg border border-border bg-surface">
        {items.map((consent) => (
          <li key={consent.appId} className="flex flex-col gap-4 border-t border-border p-4 first:border-t-0 sm:flex-row sm:items-center sm:px-6">
            <Avatar name={consent.name} src={consent.logoUrl} size="lg" square />
            <div className="flex min-w-0 flex-1 flex-col gap-1">
              <p className="text-body-sm font-semibold text-fg">{consent.name}</p>
              <p className="text-body-sm text-fg-3">{consent.scopes.map((scope) => scopeLabel(scope, locale)).join(" · ")}</p>
              <p className="text-caption text-fg-3">{t.allowedOn(formatDate(consent.grantedAt, locale))}</p>
            </div>
            <Button size="sm" variant="secondary" onClick={() => setPending(consent)} className="self-start sm:self-center" aria-label={t.removeAccessFor(consent.name)}>
              {t.removeAccess}
            </Button>
          </li>
        ))}
      </ul>
      <Dialog
        open={pending !== null}
        onOpenChange={(open) => {
          if (!open) setPending(null);
        }}
        title={t.removeTitle(pending?.name)}
        description={t.removeDescription(pending?.name)}
        footer={
          <>
            <Button variant="secondary" onClick={() => setPending(null)}>
              {copy.cancel}
            </Button>
            <Button variant="destructive" loading={removing} onClick={remove}>
              {t.removeAccess}
            </Button>
          </>
        }
      />
    </section>
  );
}
