"use client";

import { Check, CircleAlert, ExternalLink } from "lucide-react";
import { useEffect, useId, useState } from "react";
import { Avatar } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { api, ApiError } from "@/lib/api/client";
import { authMessages } from "@/lib/i18n/auth";
import { useLocale } from "@/lib/i18n/client";
import { scopeLabel } from "@/lib/labels";
import type { Scope } from "@/lib/types";
import { describeFailure } from "./passkey";

type Pending = { application: { name: string; homepage: string; logoUrl: string | null }; email: string; scopes: Scope[] };

export function ConsentStep({ authRequest }: { authRequest: string }) {
  const locale = useLocale();
  const messages = authMessages(locale);
  const t = messages.consent;
  const listId = useId();
  const [pending, setPending] = useState<Pending | null>(null);
  const [problem, setProblem] = useState<string | null>(null);
  const [expired, setExpired] = useState(false);
  const [busy, setBusy] = useState<"allow" | "deny" | null>(null);

  function fail(error: unknown) {
    setExpired(error instanceof ApiError && (error.code === "ERR_AUTH_REQUEST_EXPIRED" || error.status === 401));
    setProblem(describeFailure(error));
  }

  useEffect(() => {
    let live = true;
    api<Pending>(`/auth/consent/${encodeURIComponent(authRequest)}`)
      .then((result) => live && setPending(result))
      .catch((error) => live && fail(error));
    return () => {
      live = false;
    };
  }, [authRequest]);

  async function decide(allow: boolean) {
    setBusy(allow ? "allow" : "deny");
    setProblem(null);
    try {
      const { redirect } = await api<{ redirect: string }>("/auth/consent", { body: { authRequest, allow } });
      window.location.assign(redirect);
    } catch (error) {
      fail(error);
      setBusy(null);
    }
  }

  const app = pending?.application;
  const host = app && URL.canParse(app.homepage) ? new URL(app.homepage).host : "";

  return (
    <div className="flex animate-enter flex-col gap-8">
      <div className="flex flex-col gap-3 text-center">
        {app ? <Avatar name={app.name} src={app.logoUrl} size="lg" square className="self-center" /> : null}
        <h1 className="text-h3 text-fg">{t.heading(app?.name)}</h1>
        {pending ? <p className="text-body text-fg-3">{t.signedInAs(pending.email)}</p> : null}
      </div>
      {app && pending ? (
        <div className="flex flex-col gap-4">
          <section aria-labelledby={listId} className="flex flex-col gap-3 rounded-md border border-border bg-surface p-4">
            <h2 id={listId} className="text-body-sm font-semibold text-fg">
              {t.willBeAbleTo(app.name)}
            </h2>
            <ul className="flex flex-col gap-3">
              {pending.scopes.map((scope) => (
                <li key={scope.name} className="flex items-start gap-3 text-body-sm text-fg-2">
                  <Check aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0 text-fg-3" />
                  {scopeLabel(scope, locale)}
                </li>
              ))}
            </ul>
          </section>
          <p className="text-body-sm text-fg-3">
            {host ? (
              <>
                {t.websiteBefore(app.name)}
                <a
                  href={app.homepage}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-1 rounded-sm text-link underline underline-offset-3 transition-colors duration-fast ease-brand hover:text-ember-hover"
                >
                  {host}
                  <ExternalLink aria-hidden="true" size={16} strokeWidth={1.75} />
                  <span className="sr-only">{messages.newTab}</span>
                </a>
                {t.websiteAfter}
              </>
            ) : null}
            {t.trust}
          </p>
        </div>
      ) : null}
      {!pending && !problem ? (
        <div role="status" className="flex items-center justify-center gap-3 text-body-sm text-fg-3">
          <Spinner size={20} />
          {t.loading}
        </div>
      ) : null}
      {problem ? (
        <div role="alert" className="flex items-start gap-3 rounded-md border border-danger/20 bg-danger-container p-4">
          <CircleAlert aria-hidden="true" size={20} strokeWidth={1.75} className="mt-px shrink-0 text-danger" />
          <p className="text-body-sm text-fg-2">{problem}</p>
        </div>
      ) : null}
      {expired ? null : (
        <div className="flex flex-col gap-2">
          <Button variant="primary" size="lg" className="w-full" disabled={!pending || busy !== null} loading={busy === "allow"} onClick={() => decide(true)}>
            {t.allow}
          </Button>
          <Button size="lg" className="w-full" disabled={!pending || busy !== null} loading={busy === "deny"} onClick={() => decide(false)}>
            {t.deny}
          </Button>
        </div>
      )}
    </div>
  );
}
