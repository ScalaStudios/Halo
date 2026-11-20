"use client";

import { CircleAlert, CircleUserRound, MailCheck } from "lucide-react";
import Link from "next/link";
import { useRef, useState, type FormEvent } from "react";
import { Button, buttonClasses } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/input";
import { api, ApiError } from "@/lib/api/client";
import type { Challenge } from "@/lib/settings-types";
import { ChallengeCheck } from "./challenge";
import { describeFailure } from "./passkey";
import { emailProblem, linkButton } from "./sign-in-flow";

export function SignUpFlow({ open, challenge }: { open: boolean; challenge: Challenge | null }) {
  const nameRef = useRef<HTMLInputElement>(null);
  const emailRef = useRef<HTMLInputElement>(null);
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [nameError, setNameError] = useState<string | null>(null);
  const [emailError, setEmailError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [checking, setChecking] = useState(false);
  const [busy, setBusy] = useState(false);
  const [sent, setSent] = useState(false);

  if (!open) {
    return (
      <div className="flex animate-enter flex-col items-center gap-6 text-center">
        <div className="flex flex-col items-center gap-3">
          <CircleAlert aria-hidden="true" size={24} strokeWidth={1.75} className="text-fg-3" />
          <h1 className="text-h3 text-fg">Sign-up is turned off</h1>
          <p className="text-body text-fg-3">Ask your administrator to invite you.</p>
        </div>
        <Link href="/sign-in" className={buttonClasses("secondary", "lg", "w-full")}>
          Back to sign in
        </Link>
      </div>
    );
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    const missingName = name.trim() ? null : "Enter your name, as you'd like others in your organization to see it.";
    const badEmail = emailProblem(email);
    setNameError(missingName);
    setEmailError(badEmail);
    setNotice(null);
    if (missingName) return nameRef.current?.focus();
    if (badEmail) return emailRef.current?.focus();
    if (challenge) setChecking(true);
    else void send();
  }

  async function send(token?: string) {
    setChecking(false);
    setBusy(true);
    try {
      await api("/auth/sign-up", { body: { name: name.trim(), email: email.trim(), challenge: token } });
      setEmail(email.trim());
      setSent(true);
    } catch (error) {
      if (error instanceof ApiError && error.code === "ERR_SIGN_UP_DOMAIN") {
        setEmailError(error.message);
        emailRef.current?.focus();
      } else setNotice(describeFailure(error));
    } finally {
      setBusy(false);
    }
  }

  if (sent) {
    return (
      <div key="sent" className="flex animate-enter flex-col gap-8">
        <div className="flex flex-col gap-4">
          <div className="flex flex-col items-center gap-3 text-center">
            <MailCheck aria-hidden="true" size={24} strokeWidth={1.75} className="text-fg-3" />
            <h1 className="text-h3 text-fg">Check your inbox</h1>
          </div>
          <div className="flex items-center gap-3 rounded-md border border-border bg-surface px-4 py-3">
            <CircleUserRound aria-hidden="true" size={20} strokeWidth={1.75} className="shrink-0 text-fg-3" />
            <span className="truncate text-body-sm text-fg">{email}</span>
          </div>
        </div>
        <p role="status" className="text-center text-body text-fg-3">
          If this address doesn&apos;t have a Halo account yet, a link to finish signing up is on its way. Open it to confirm the address and create your passkey. It works once and expires in 24 hours.
        </p>
        <div className="flex flex-col gap-2">
          <Link href="/sign-in" className={buttonClasses("secondary", "lg", "w-full")}>
            Back to sign in
          </Link>
          <button type="button" onClick={() => setSent(false)} className={linkButton}>
            Use a different email address
          </button>
        </div>
      </div>
    );
  }

  return (
    <div key="form" className="flex animate-enter flex-col gap-8">
      <div className="flex flex-col gap-3 text-center">
        <h1 className="text-h3 text-fg">Create your account</h1>
        <p className="text-body text-fg-3">We&apos;ll email you a link to confirm your address. Then you create the passkey you sign in with.</p>
      </div>
      {notice ? (
        <div role="alert" className="flex items-start gap-3 rounded-md border border-danger/20 bg-danger-container p-4">
          <CircleAlert aria-hidden="true" size={20} strokeWidth={1.75} className="mt-px shrink-0 text-danger" />
          <p className="text-body-sm text-fg-2">{notice}</p>
        </div>
      ) : null}
      <form noValidate className="flex flex-col gap-4" onSubmit={submit}>
        <Field label="Name" error={nameError}>
          {(field) => (
            <Input
              {...field}
              ref={nameRef}
              name="name"
              autoComplete="name"
              autoFocus
              maxLength={200}
              value={name}
              onChange={(event) => {
                setName(event.target.value);
                setNameError(null);
              }}
              className="h-12"
            />
          )}
        </Field>
        <Field label="Email" error={emailError}>
          {(field) => (
            <Input
              {...field}
              ref={emailRef}
              type="email"
              name="email"
              inputMode="email"
              autoComplete="email"
              autoCapitalize="none"
              spellCheck={false}
              placeholder="name@example.com"
              value={email}
              onChange={(event) => {
                setEmail(event.target.value);
                setEmailError(null);
              }}
              className="h-12"
            />
          )}
        </Field>
        <Button type="submit" variant="primary" size="lg" className="mt-2 w-full" loading={busy} disabled={checking}>
          Continue
        </Button>
      </form>
      {checking && challenge ? <ChallengeCheck challenge={challenge} onToken={(token) => void send(token)} onCancel={() => setChecking(false)} /> : null}
      <Link href="/sign-in" className={linkButton}>
        Already have an account? Sign in
      </Link>
    </div>
  );
}
