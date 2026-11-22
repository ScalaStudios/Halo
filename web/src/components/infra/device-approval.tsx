"use client";

import { CircleCheck, CircleX } from "lucide-react";
import Link from "next/link";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { Avatar } from "@/components/ui/avatar";
import { Tag } from "@/components/ui/badge";
import { Button, buttonClasses } from "@/components/ui/button";
import { DescriptionList } from "@/components/ui/description-list";
import { Field, Input } from "@/components/ui/input";
import { RelativeTime } from "@/components/ui/relative-time";
import { api, ApiError } from "@/lib/api/client";
import { useMessages } from "@/lib/i18n/client";
import { infrastructureMessages } from "@/lib/i18n/console/infrastructure";
import type { DeviceRequest } from "@/lib/infra-types";

type Step = "enter" | "confirm" | "approved" | "denied";

export function DeviceApproval({ initialCode, signIn }: { initialCode: string; signIn: string }) {
  const t = useMessages(infrastructureMessages).device;
  const started = useRef(false);
  const [code, setCode] = useState(initialCode.toUpperCase());
  const [step, setStep] = useState<Step>("enter");
  const [request, setRequest] = useState<DeviceRequest | null>(null);
  const [busy, setBusy] = useState<"lookup" | "approve" | "deny" | null>(initialCode ? "lookup" : null);
  const [error, setError] = useState<ReactNode>(null);
  const [signedOut, setSignedOut] = useState(false);
  const [terms, setTerms] = useState(false);

  function fail(cause: unknown) {
    setSignedOut(cause instanceof ApiError && cause.status === 401);
    setTerms(cause instanceof ApiError && cause.code === "ERR_TERMS_REQUIRED");
    setError(cause instanceof ApiError ? <span lang="en">{cause.message}</span> : t.unreachable);
  }

  async function lookup(value: string) {
    if (!value.trim()) {
      setError(t.enterCode);
      return;
    }
    setBusy("lookup");
    setError(null);
    try {
      setRequest(await api<DeviceRequest>(`/device/${encodeURIComponent(value.trim())}`));
      setStep("confirm");
    } catch (cause) {
      fail(cause);
    } finally {
      setBusy(null);
    }
  }

  async function decide(approve: boolean) {
    if (!request) return;
    setBusy(approve ? "approve" : "deny");
    setError(null);
    try {
      await api(`/device/${encodeURIComponent(request.userCode)}/${approve ? "approve" : "deny"}`, { method: "POST" });
      setStep(approve ? "approved" : "denied");
    } catch (cause) {
      fail(cause);
    } finally {
      setBusy(null);
    }
  }

  useEffect(() => {
    if (!initialCode || started.current) return;
    started.current = true;
    void lookup(initialCode);
  }, [initialCode]);

  const problem = error ? (
    <>
      {error}{" "}
      {signedOut ? (
        <Link href={signIn} className="text-link underline underline-offset-3 hover:text-ember">
          {t.signInAgain}
        </Link>
      ) : null}
      {terms && request ? (
        <Link
          href={`/account/terms?next=${encodeURIComponent(`/device?user_code=${encodeURIComponent(request.userCode)}`)}`}
          className="text-link underline underline-offset-3 hover:text-ember"
        >
          {t.acceptTerms}
        </Link>
      ) : null}
    </>
  ) : null;

  if (step === "approved" && request) {
    return (
      <div role="status" className="flex animate-enter flex-col items-center gap-3 text-center">
        <CircleCheck aria-hidden="true" size={24} strokeWidth={1.75} className="text-success" />
        <h1 className="text-h3 text-fg">{t.signedIn(request.application)}</h1>
        <p className="text-body text-fg-3">{t.signedInHint}</p>
      </div>
    );
  }

  if (step === "denied" && request) {
    return (
      <div className="flex animate-enter flex-col items-center gap-6 text-center">
        <div role="status" className="flex flex-col items-center gap-3">
          <CircleX aria-hidden="true" size={24} strokeWidth={1.75} className="text-fg-3" />
          <h1 className="text-h3 text-fg">{t.denied}</h1>
          <p className="text-body text-fg-3">{t.deniedHint(request.application)}</p>
        </div>
        <Link href="/account" className={buttonClasses("secondary", "lg", "w-full")}>
          {t.goToAccount}
        </Link>
      </div>
    );
  }

  if (step === "confirm" && request) {
    return (
      <div className="flex animate-enter flex-col gap-8">
        <div className="flex flex-col items-center gap-3 text-center">
          <Avatar name={request.application} src={request.logoUrl} size="lg" square />
          <h1 className="text-h3 text-fg">{t.allow(request.application)}</h1>
          <p className="text-body text-fg-3">{t.allowHint}</p>
        </div>
        <DescriptionList
          className="rounded-lg border border-border bg-surface p-6"
          items={[
            { label: t.code, value: request.userCode, mono: true },
            { label: t.requestedFrom, value: request.ip || t.unknownAddress, mono: true },
            { label: t.device, value: request.userAgent || t.notReported },
            { label: t.requested, value: <><RelativeTime iso={request.requestedAt} /> · {t.codeExpires} <RelativeTime iso={request.expiresAt} lowercase /></> },
            {
              label: t.access,
              value: (
                <span className="flex flex-wrap gap-1">
                  {request.scopes.map((scope) => (
                    <Tag key={scope}>{scope}</Tag>
                  ))}
                </span>
              ),
            },
          ]}
        />
        {problem ? (
          <p role="alert" className="text-body-sm text-danger">
            {problem}
          </p>
        ) : null}
        <div className="flex flex-col gap-3">
          <Button variant="primary" size="lg" className="w-full" loading={busy === "approve"} disabled={busy !== null} onClick={() => void decide(true)}>
            {t.approve}
          </Button>
          <Button variant="secondary" size="lg" className="w-full" loading={busy === "deny"} disabled={busy !== null} onClick={() => void decide(false)}>
            {t.deny}
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="flex animate-enter flex-col gap-8">
      <div className="flex flex-col gap-2 text-center">
        <h1 className="text-h3 text-fg">{t.title}</h1>
        <p className="text-body text-fg-3">{t.intro((text) => <code key={text} className="font-mono text-code-sm text-fg-2">{text}</code>)}</p>
      </div>
      <form
        className="flex flex-col gap-6"
        onSubmit={(event) => {
          event.preventDefault();
          void lookup(code);
        }}
      >
        <Field label={t.code} error={problem}>
          {(props) => (
            <Input
              {...props}
              mono
              autoFocus
              autoComplete="one-time-code"
              autoCapitalize="characters"
              spellCheck={false}
              maxLength={12}
              placeholder="BCDF-GHJK"
              value={code}
              onChange={(event) => setCode(event.target.value.toUpperCase())}
            />
          )}
        </Field>
        <Button type="submit" variant="primary" size="lg" className="w-full" loading={busy === "lookup"}>
          {t.continue}
        </Button>
      </form>
    </div>
  );
}
