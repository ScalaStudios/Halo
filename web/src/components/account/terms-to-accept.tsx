"use client";

import { useMutation } from "@/components/console/use-mutation";
import { sameOrigin } from "@/components/auth/sign-in-flow";
import { TermsDocuments } from "@/components/auth/terms";
import { ActionError } from "@/components/ui/action-error";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api/client";
import { accountMessages } from "@/lib/i18n/account";
import { useLocale } from "@/lib/i18n/client";
import type { Terms } from "@/lib/policy-types";

export function TermsToAccept({ terms, next }: { terms: Terms[]; next?: string }) {
  const { pending, run, errorFor, toast } = useMutation();
  const copy = accountMessages(useLocale());
  const t = copy.terms;

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
        toast({ title: terms.length === 1 ? t.acceptedOne(terms[0]!.name) : t.acceptedMany, description: t.acceptedToastHint });
      },
      "inline",
    );
  }

  if (!terms.length) {
    return next ? (
      <Button variant="primary" className="self-start" onClick={proceed}>
        {copy.continue}
      </Button>
    ) : null;
  }
  return (
    <div className="flex flex-col gap-4">
      <TermsDocuments terms={terms} />
      <ActionError message={errorFor("accept")} />
      <Button variant="primary" className="self-start" loading={pending === "accept"} onClick={accept}>
        {next ? t.acceptContinue : terms.length === 1 ? t.accept : t.acceptAll}
      </Button>
    </div>
  );
}
