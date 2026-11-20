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
import { scopeLabel } from "@/lib/labels";
import type { Consent } from "@/lib/types";

export function AppAccess({ consents }: { consents: Consent[] }) {
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
      toast({ title: `${pending.name} no longer has access`, description: "Its tokens stopped working. It asks for your permission again the next time you use it." });
      setPending(null);
    } catch (error) {
      toast({ title: "That didn't work", description: describeFailure(error), tone: "danger" });
    } finally {
      setRemoving(false);
    }
  }

  if (items.length === 0) return null;

  return (
    <section className="flex flex-col gap-4">
      <SectionTitle title="Access you allowed" description="Applications that asked for your permission to see your Halo account information." />
      <ul className="overflow-hidden rounded-lg border border-border bg-surface">
        {items.map((consent) => (
          <li key={consent.appId} className="flex flex-col gap-4 border-t border-border p-4 first:border-t-0 sm:flex-row sm:items-center sm:px-6">
            <Avatar name={consent.name} src={consent.logoUrl} size="lg" square />
            <div className="flex min-w-0 flex-1 flex-col gap-1">
              <p className="text-body-sm font-semibold text-fg">{consent.name}</p>
              <p className="text-body-sm text-fg-3">{consent.scopes.map(scopeLabel).join(" · ")}</p>
              <p className="text-caption text-fg-3">Allowed {formatDate(consent.grantedAt)}</p>
            </div>
            <Button size="sm" variant="secondary" onClick={() => setPending(consent)} className="self-start sm:self-center" aria-label={`Remove access for ${consent.name}`}>
              Remove access
            </Button>
          </li>
        ))}
      </ul>
      <Dialog
        open={pending !== null}
        onOpenChange={(open) => {
          if (!open) setPending(null);
        }}
        title={`Remove access for ${pending?.name ?? "this application"}?`}
        description={`${pending?.name ?? "The application"} is signed out and its tokens stop working. It asks for your permission again the next time you use it.`}
        footer={
          <>
            <Button variant="secondary" onClick={() => setPending(null)}>
              Cancel
            </Button>
            <Button variant="destructive" loading={removing} onClick={remove}>
              Remove access
            </Button>
          </>
        }
      />
    </section>
  );
}
