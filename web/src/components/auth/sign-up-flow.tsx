"use client";

import { CircleAlert, CircleUserRound, MailCheck } from "lucide-react";
import Link from "next/link";
import { useRef, useState, type FormEvent } from "react";
import { Button, buttonClasses } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/input";
import { api, ApiError } from "@/lib/api/client";
import { authMessages } from "@/lib/i18n/auth";
import type { Locale } from "@/lib/i18n/locale";
import type { Challenge } from "@/lib/settings-types";
import { ChallengeCheck } from "./challenge";
import { describeFailure } from "./passkey";
import { emailProblem, linkButton } from "./sign-in-flow";

export function SignUpFlow({ open, challenge, locale }: { open: boolean; challenge: Challenge | null; locale: Locale }) {
  const messages = authMessages(locale);
  const copy = messages.registration;
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
      <div lang={locale} className="flex animate-enter flex-col items-center gap-6 text-center">
        <div className="flex flex-col items-center gap-3">
          <CircleAlert aria-hidden="true" size={24} strokeWidth={1.75} className="text-fg-3" />
          <h1 className="text-h3 text-fg">{copy.closed}</h1>
          <p className="text-body text-fg-3">{copy.invitation}</p>
        </div>
        <Link href="/sign-in" className={buttonClasses("secondary", "lg", "w-full")}>
          {messages.backToSignIn}
        </Link>
      </div>
    );
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    const missingName = name.trim() ? null : copy.nameRequired;
    const badEmail = emailProblem(email, messages);
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
        setEmailError(copy.domain);
        emailRef.current?.focus();
      } else setNotice(describeFailure(error));
    } finally {
      setBusy(false);
    }
  }

  if (sent) {
    return (
      <div lang={locale} key="sent" className="flex animate-enter flex-col gap-8">
        <div className="flex flex-col gap-4">
          <div className="flex flex-col items-center gap-3 text-center">
            <MailCheck aria-hidden="true" size={24} strokeWidth={1.75} className="text-fg-3" />
            <h1 className="text-h3 text-fg">{messages.inbox}</h1>
          </div>
          <div className="flex items-center gap-3 rounded-md border border-border bg-surface px-4 py-3">
            <CircleUserRound aria-hidden="true" size={20} strokeWidth={1.75} className="shrink-0 text-fg-3" />
            <span className="truncate text-body-sm text-fg">{email}</span>
          </div>
        </div>
        <p role="status" className="text-center text-body text-fg-3">
          {copy.sent}
        </p>
        <div className="flex flex-col gap-2">
          <Link href="/sign-in" className={buttonClasses("secondary", "lg", "w-full")}>
            {messages.backToSignIn}
          </Link>
          <button type="button" onClick={() => setSent(false)} className={linkButton}>
            {copy.differentEmail}
          </button>
        </div>
      </div>
    );
  }

  return (
    <div lang={locale} key="form" className="flex animate-enter flex-col gap-8">
      <div className="flex flex-col gap-3 text-center">
        <h1 className="text-h3 text-fg">{copy.create}</h1>
        <p className="text-body text-fg-3">{copy.intro}</p>
      </div>
      {notice ? (
        <div role="alert" className="flex items-start gap-3 rounded-md border border-danger/20 bg-danger-container p-4">
          <CircleAlert aria-hidden="true" size={20} strokeWidth={1.75} className="mt-px shrink-0 text-danger" />
          <p lang="en" className="text-body-sm text-fg-2">{notice}</p>
        </div>
      ) : null}
      <form noValidate className="flex flex-col gap-4" onSubmit={submit}>
        <Field label={copy.name} error={nameError}>
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
        <Field label={messages.email} error={emailError}>
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
          {messages.continue}
        </Button>
      </form>
      {checking && challenge ? <ChallengeCheck locale={locale} challenge={challenge} onToken={(token) => void send(token)} onCancel={() => setChecking(false)} /> : null}
      <Link href="/sign-in" className={linkButton}>
        {copy.existing}
      </Link>
    </div>
  );
}
