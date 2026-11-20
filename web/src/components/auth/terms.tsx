"use client";

import { CircleAlert, ExternalLink } from "lucide-react";
import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { api, ApiError } from "@/lib/api/client";
import type { Terms } from "@/lib/policy-types";
import { describeFailure } from "./passkey";

export type SignedIn = { redirect: string; terms?: string };

export function TermsDocuments({ terms }: { terms: Terms[] }) {
  return (
    <ul className="flex flex-col gap-4">
      {terms.map((t) => (
        <li key={t.id} className="flex flex-col gap-3 rounded-md border border-border bg-surface p-4">
          <div className="flex items-baseline justify-between gap-3">
            <h2 className="text-body font-semibold text-fg">{t.name}</h2>
            <span className="shrink-0 text-caption text-fg-3">Version {t.version}</span>
          </div>
          {t.text ? (
            <div
              role="region"
              tabIndex={0}
              aria-label={`${t.name}, version ${t.version}`}
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
              Read the full document
              <ExternalLink aria-hidden="true" size={16} strokeWidth={1.75} />
              <span className="sr-only">(opens in a new tab)</span>
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
        <h1 className="text-h3 text-fg">{app ? `Accept the terms to continue to ${app}` : "Accept the terms to continue"}</h1>
        <p className="text-body text-fg-3">Your organization asks you to read and accept these first. Halo keeps a record of the version you accept and when.</p>
      </div>
      {terms ? <TermsDocuments terms={terms} /> : null}
      {!terms && !problem ? (
        <div role="status" className="flex items-center justify-center gap-3 text-body-sm text-fg-3">
          <Spinner size={20} />
          Loading the terms
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
          Sign in again
        </Button>
      ) : (
        <div className="flex flex-col gap-2">
          <Button variant="primary" size="lg" className="w-full" disabled={!terms} loading={busy} onClick={accept}>
            Accept and continue
          </Button>
          <Button size="lg" className="w-full" disabled={busy} onClick={onLeave}>
            Decline
          </Button>
        </div>
      )}
    </>
  );
}
