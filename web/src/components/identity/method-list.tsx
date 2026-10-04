"use client";

import { useRouter } from "next/navigation";
import { ShieldCheck, Trash2 } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { RelativeTime } from "@/components/ui/relative-time";
import { useToast } from "@/components/ui/toast";
import { api, ApiError } from "@/lib/api/client";
import { formatDate } from "@/lib/format";
import { METHOD } from "@/lib/labels";
import type { AuthMethod } from "@/lib/types";

export function MethodList({
  methods,
  userId,
  empty,
  strongElsewhere = 0,
}: {
  methods: AuthMethod[];
  userId?: string;
  empty?: string;
  strongElsewhere?: number;
}) {
  const self = userId === undefined;
  const [items, setItems] = useState(methods);
  const [pending, setPending] = useState<AuthMethod | null>(null);
  const [removing, setRemoving] = useState(false);
  const toast = useToast();
  const router = useRouter();
  const strongCount = items.filter((m) => METHOD[m.kind].phishingResistant).length + strongElsewhere;

  async function remove() {
    if (!pending) return;
    setRemoving(true);
    try {
      await api(self ? `/me/methods/${pending.id}` : `/users/${userId}/methods/${pending.id}`, { method: "DELETE" });
      setItems((current) => current.filter((m) => m.id !== pending.id));
      toast({ title: `${pending.label} removed`, description: self ? "It can no longer be used to sign in to Halo." : "The user can no longer sign in with it." });
      setPending(null);
      router.refresh();
    } catch (error) {
      toast({ title: "That didn't work", description: error instanceof ApiError ? error.message : "Halo could not reach the server. Try again.", tone: "danger" });
    } finally {
      setRemoving(false);
    }
  }

  if (items.length === 0) {
    return <EmptyState icon={ShieldCheck} title="No sign-in methods" description={empty ?? "This user has not enrolled a method yet."} />;
  }

  const lastStrong = pending && METHOD[pending.kind].phishingResistant && strongCount === 1;

  return (
    <>
      <ul className="overflow-hidden rounded-lg border border-border bg-surface">
        {items.map((method) => {
          const meta = METHOD[method.kind];
          const Icon = meta.icon;
          return (
            <li key={method.id} className="flex flex-col gap-4 border-t border-border p-4 first:border-t-0 sm:flex-row sm:items-center sm:px-6">
              <span className="grid size-10 shrink-0 place-items-center rounded-md border border-border bg-sunken text-fg-2">
                <Icon aria-hidden="true" size={20} strokeWidth={1.75} />
              </span>
              <div className="flex min-w-0 flex-1 flex-col gap-1">
                <p className="flex flex-wrap items-center gap-2 text-body-sm font-semibold text-fg">
                  {method.label}
                  {meta.phishingResistant ? (
                    <span className="inline-flex items-center gap-1 text-caption font-normal text-success">
                      <ShieldCheck aria-hidden="true" size={16} strokeWidth={1.75} />
                      Phishing-resistant
                    </span>
                  ) : null}
                </p>
                <p className="text-body-sm text-fg-3">
                  {meta.label}
                  {method.detail ? ` · ${method.detail}` : ""}
                </p>
                <p className="text-caption text-fg-3">
                  Added {formatDate(method.addedAt)} · {method.lastUsedAt ? <>Last used <RelativeTime iso={method.lastUsedAt} lowercase /></> : "Never used"}
                </p>
              </div>
              <Button size="sm" variant="quiet" onClick={() => setPending(method)} className="self-start sm:self-center" aria-label={`Remove ${method.label}`}>
                <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
                Remove
              </Button>
            </li>
          );
        })}
      </ul>
      <Dialog
        open={pending !== null}
        onOpenChange={(open) => {
          if (!open) setPending(null);
        }}
        title={`Remove ${pending?.label ?? "method"}?`}
        description={
          lastStrong
            ? "This is the last phishing-resistant method. Sign-in will fall back to weaker methods until a new passkey or security key is added."
            : self
              ? "You will not be able to sign in with it again unless you add it back."
              : "The user will not be able to sign in with it. Their active sessions are not affected."
        }
        footer={
          <>
            <Button variant="secondary" onClick={() => setPending(null)}>
              Cancel
            </Button>
            <Button variant="destructive" loading={removing} onClick={remove}>
              Remove
            </Button>
          </>
        }
      />
    </>
  );
}
