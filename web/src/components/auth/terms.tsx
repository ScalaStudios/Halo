"use client";

import { CircleAlert, ExternalLink } from "lucide-react";
import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { api, ApiError } from "@/lib/api/client";
import { authMessages } from "@/lib/i18n/auth";
import { useLocale } from "@/lib/i18n/client";
import type { Terms } from "@/lib/policy-types";
import { describeFailure } from "./passkey";

export type SignedIn = { redirect: string; terms?: string };

export function TermsDocuments({ terms }: { terms: Terms[] }) {
  const messages = authMessages(useLocale());
  const copy = messages.terms;
  return (
    <ul className="flex flex-col gap-4">
      {terms.map((t) => (
        <li key={t.id} className="flex flex-col gap-3 rounded-md border border-border bg-surface p-4">
          <div className="flex items-baseline justify-between gap-3">
            <h2 className="text-body font-semibold text-fg">{t.name}</h2>
            <span className="shrink-0 text-caption text-fg-3">{copy.version(t.version)}</span>
          </div>
          {t.text ? (
            <div
              role="region"
              tabIndex={0}
              aria-label={copy.document(t.name, t.version)}
              className="max-h-64 overflow-y-auto rounded-sm bg-sunken p-3 text-body-sm whitespace-pre-wrap text-fg-2"
            >
              {t.text}
            </div>
          ) : null}
          {t.url ? (
            <a
              href={t.url}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-2 self-start rounded-sm text-body-sm text-link underline underline-offset-3 transition-colors duration-fast ease-brand hover:text-ember-hover"
            >
              {copy.readFull}
              <ExternalLink aria-hidden="true" size={16} strokeWidth={1.75} />
              <span className="sr-only">{messages.newTab}</span>
            </a>
          ) : null}
        </li>
      ))}
    </ul>
  );
}

export function TermsStep({ ceremony, app, onAccepted, onLeave }: { ceremony: string; app: string | null; onAccepted: (result: SignedIn) => void; onLeave: () => void }) {
  const [terms, setTerms] = useState<Terms[] | null>(null);
  const [problem, setProblem] = useState<string | null>(null);
  const [expired, setExpired] = useState(false);
  const [busy, setBusy] = useState(false);
  const copy = authMessages(useLocale()).terms;

  function fail(error: unknown) {
    setExpired(error instanceof ApiError && error.code === "ERR_TERMS_EXPIRED");
    setProblem(describeFailure(error));
  }

  useEffect(() => {
    let live = true;
    api<{ terms: Terms[] }>(`/auth/terms/${encodeURIComponent(ceremony)}`)
      .then((result) => live && setTerms(result.terms))
      .catch((error) => live && fail(error));
    return () => {
      live = false;
    };
  }, [ceremony]);

  async function accept() {
    setBusy(true);
    setProblem(null);
    try {
      onAccepted(await api<SignedIn>("/auth/terms", { body: { ceremony } }));
    } catch (error) {
      fail(error);
      setBusy(false);
    }
  }

  return (
    <>
      <div className="flex flex-col gap-3 text-center">
        <h1 className="text-h3 text-fg">{copy.heading(app)}</h1>
        <p className="text-body text-fg-3">{copy.intro}</p>
      </div>
      {terms ? <TermsDocuments terms={terms} /> : null}
      {!terms && !problem ? (
        <div role="status" className="flex items-center justify-center gap-3 text-body-sm text-fg-3">
          <Spinner size={20} />
          {copy.loading}
        </div>
      ) : null}
      {problem ? (
        <div role="alert" className="flex items-start gap-3 rounded-md border border-danger/20 bg-danger-container p-4">
          <CircleAlert aria-hidden="true" size={20} strokeWidth={1.75} className="mt-px shrink-0 text-danger" />
          <p className="text-body-sm text-fg-2">{problem}</p>
        </div>
      ) : null}
      {expired ? (
        <Button variant="primary" size="lg" className="w-full" onClick={onLeave}>
          {copy.signInAgain}
        </Button>
      ) : (
        <div className="flex flex-col gap-2">
          <Button variant="primary" size="lg" className="w-full" disabled={!terms} loading={busy} onClick={accept}>
            {copy.accept}
          </Button>
          <Button size="lg" className="w-full" disabled={busy} onClick={onLeave}>
            {copy.decline}
          </Button>
        </div>
      )}
    </>
  );
}
