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
import { METHOD } from "@/lib/labels";

export function LinkedAccounts() {
  const toast = useToast();
  const [accounts, setAccounts] = useState<LinkedAccount[] | null>(null);
  const [failed, setFailed] = useState(false);
  const [pending, setPending] = useState<LinkedAccount | null>(null);
  const [removing, setRemoving] = useState(false);
  const Icon = METHOD.federated.icon;

  useEffect(() => {
    api<LinkedAccount[]>("/me/identities").then(setAccounts, () => setFailed(true));
  }, []);

  async function unlink() {
    if (!pending) return;
    setRemoving(true);
    try {
      await api(`/me/identities/${pending.id}`, { method: "DELETE" });
      setAccounts((current) => current?.filter((a) => a.id !== pending.id) ?? null);
      toast({ title: `${pending.providerName} unlinked`, description: `${pending.email} can no longer sign you in to Halo.` });
      setPending(null);
    } catch (error) {
      toast({ title: "That didn't work", description: describeFailure(error), tone: "danger" });
    } finally {
      setRemoving(false);
    }
  }

  return (
    <section className="flex flex-col gap-4">
      <SectionTitle
        title="Linked accounts"
        description="Accounts at identity providers your organization set up, such as Google or GitHub. Halo links one the first time you sign in with it."
      />
      {failed ? (
        <p role="alert" className="rounded-lg border border-border px-6 py-4 text-body-sm text-fg-3">
          Halo could not load your linked accounts. Reload the page to try again.
        </p>
      ) : accounts === null ? (
        <div aria-busy="true" aria-label="Loading linked accounts" className="flex items-center gap-4 rounded-lg border border-border bg-surface p-4 sm:px-6">
          <span className="grid size-10 shrink-0 place-items-center rounded-md border border-border bg-sunken text-fg-3">
            <Icon aria-hidden="true" size={20} strokeWidth={1.75} />
          </span>
          <div className="flex flex-1 flex-col gap-2">
            <Skeleton className="w-32" />
            <Skeleton className="w-48" />
          </div>
        </div>
      ) : accounts.length === 0 ? (
        <p className="rounded-lg border border-border px-6 py-4 text-body-sm text-fg-3">
          No linked accounts. Sign in with an identity provider on the sign-in page to link one.
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
                <p className="truncate text-body-sm text-fg-3">{account.email || "No email shared"}</p>
                <p className="text-caption text-fg-3">
                  Linked {formatDate(account.createdAt)} · {account.lastUsedAt ? <>Last used <RelativeTime iso={account.lastUsedAt} lowercase /></> : "Never used"}
                </p>
              </div>
              <Button size="sm" variant="quiet" onClick={() => setPending(account)} className="self-start sm:self-center" aria-label={`Unlink ${account.providerName}`}>
                <Unlink aria-hidden="true" size={16} strokeWidth={1.75} />
                Unlink
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
        title={`Unlink ${pending?.providerName ?? "account"}?`}
        description={`${pending?.email || "This account"} stops signing you in to Halo. Signing in with ${pending?.providerName ?? "the provider"} again links it again. Halo refuses if it's your last way to sign in.`}
        footer={
          <>
            <Button variant="secondary" onClick={() => setPending(null)}>
              Cancel
            </Button>
            <Button variant="destructive" loading={removing} onClick={unlink}>
              Unlink
            </Button>
          </>
        }
      />
    </section>
  );
}
