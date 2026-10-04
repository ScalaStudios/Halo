"use client";

import { CircleCheck, CircleX, Terminal } from "lucide-react";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { Tag } from "@/components/ui/badge";
import { Button, buttonClasses } from "@/components/ui/button";
import { DescriptionList } from "@/components/ui/description-list";
import { Field, Input } from "@/components/ui/input";
import { RelativeTime } from "@/components/ui/relative-time";
import { api, ApiError } from "@/lib/api/client";
import type { DeviceRequest } from "@/lib/infra-types";

type Step = "enter" | "confirm" | "approved" | "denied";

export function DeviceApproval({ initialCode, signIn }: { initialCode: string; signIn: string }) {
  const started = useRef(false);
  const [code, setCode] = useState(initialCode.toUpperCase());
  const [step, setStep] = useState<Step>("enter");
  const [request, setRequest] = useState<DeviceRequest | null>(null);
  const [busy, setBusy] = useState<"lookup" | "approve" | "deny" | null>(initialCode ? "lookup" : null);
  const [error, setError] = useState<string | null>(null);
  const [signedOut, setSignedOut] = useState(false);

  function fail(cause: unknown) {
    setSignedOut(cause instanceof ApiError && cause.status === 401);
    setError(cause instanceof ApiError ? cause.message : "Halo could not reach the server. Try again.");
  }

  async function lookup(value: string) {
    if (!value.trim()) {
      setError("Enter the code your terminal shows, such as BCDF-GHJK.");
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
          Sign in again
        </Link>
      ) : null}
    </>
  ) : null;

  if (step === "approved" && request) {
    return (
      <div role="status" className="flex animate-enter flex-col items-center gap-3 text-center">
        <CircleCheck aria-hidden="true" size={24} strokeWidth={1.75} className="text-success" />
        <h1 className="text-h3 text-fg">{request.application} is signed in</h1>
        <p className="text-body text-fg-3">Go back to your terminal to continue. You can close this tab.</p>
      </div>
    );
  }

  if (step === "denied" && request) {
    return (
      <div className="flex animate-enter flex-col items-center gap-6 text-center">
        <div role="status" className="flex flex-col items-center gap-3">
          <CircleX aria-hidden="true" size={24} strokeWidth={1.75} className="text-fg-3" />
          <h1 className="text-h3 text-fg">Sign-in denied</h1>
          <p className="text-body text-fg-3">
            {request.application} was not signed in. If you didn&apos;t start this sign-in, tell your administrator: someone may be trying to get you to approve their device.
          </p>
        </div>
        <Link href="/account" className={buttonClasses("secondary", "lg", "w-full")}>
          Go to your account
        </Link>
      </div>
    );
  }

  if (step === "confirm" && request) {
    return (
      <div className="flex animate-enter flex-col gap-8">
        <div className="flex flex-col items-center gap-3 text-center">
          <span className="grid size-12 place-items-center rounded-lg border border-border bg-sunken text-fg-3">
            <Terminal aria-hidden="true" size={24} strokeWidth={1.75} />
          </span>
          <h1 className="text-h3 text-fg">Allow {request.application} to sign in as you?</h1>
          <p className="text-body text-fg-3">Only approve a sign-in you started yourself, when the code matches the one in your terminal.</p>
        </div>
        <DescriptionList
          className="rounded-lg border border-border bg-surface p-6"
          items={[
            { label: "Code", value: request.userCode, mono: true },
            { label: "Requested from", value: request.ip || "Unknown address", mono: true },
            { label: "Device", value: request.userAgent || "Not reported" },
            { label: "Requested", value: <><RelativeTime iso={request.requestedAt} /> · code expires <RelativeTime iso={request.expiresAt} lowercase /></> },
            {
              label: "Access",
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
            Approve sign-in
          </Button>
          <Button variant="secondary" size="lg" className="w-full" loading={busy === "deny"} disabled={busy !== null} onClick={() => void decide(false)}>
            Deny
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="flex animate-enter flex-col gap-8">
      <div className="flex flex-col gap-2 text-center">
        <h1 className="text-h3 text-fg">Sign in a device</h1>
        <p className="text-body text-fg-3">
          Enter the code that <code className="font-mono text-code-sm text-fg-2">halo login</code> shows in your terminal.
        </p>
      </div>
      <form
        className="flex flex-col gap-6"
        onSubmit={(event) => {
          event.preventDefault();
          void lookup(code);
        }}
      >
        <Field label="Code" error={problem}>
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
          Continue
        </Button>
      </form>
    </div>
  );
}
