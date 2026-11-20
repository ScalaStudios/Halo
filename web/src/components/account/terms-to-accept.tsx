"use client";

import { useMutation } from "@/components/console/use-mutation";
import { sameOrigin } from "@/components/auth/sign-in-flow";
import { TermsDocuments } from "@/components/auth/terms";
import { ActionError } from "@/components/ui/action-error";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api/client";
import type { Terms } from "@/lib/policy-types";

export function TermsToAccept({ terms, next }: { terms: Terms[]; next?: string }) {
  const { pending, run, errorFor, toast } = useMutation();

  function proceed() {
    const target = sameOrigin(next);
    if (!target) return false;
    window.location.assign(target);
    return true;
  }

  function accept() {
    void run(
      "accept",
      async () => {
        for (const t of terms) await api(`/me/terms/${t.id}/accept`, { body: { version: t.version } });
      },
      () => {
        if (proceed()) return;
        toast({ title: terms.length === 1 ? `${terms[0]!.name} accepted` : "Terms accepted", description: "Halo recorded the version and the time you accepted." });
      },
      "inline",
    );
  }

  if (!terms.length) {
    return next ? (
      <Button variant="primary" className="self-start" onClick={proceed}>
        Continue
      </Button>
    ) : null;
  }
  return (
    <div className="flex flex-col gap-4">
      <TermsDocuments terms={terms} />
      <ActionError message={errorFor("accept")} />
      <Button variant="primary" className="self-start" loading={pending === "accept"} onClick={accept}>
        {next ? "Accept and continue" : terms.length === 1 ? "Accept" : "Accept all"}
      </Button>
    </div>
  );
}
