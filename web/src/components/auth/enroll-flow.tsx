"use client";

import { CircleAlert, CircleCheck, CircleUserRound, Clock, Fingerprint, IdCard, ScanFace, type LucideIcon } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState, type FormEvent } from "react";
import { Button, buttonClasses } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { api, ApiError } from "@/lib/api/client";
import { authMessages, type SignInMessages } from "@/lib/i18n/auth";
import type { Locale } from "@/lib/i18n/locale";
import { createCredential } from "@/lib/webauthn";
import { cancelled, describeFailure, deviceLabel, usePasskeySupport, type Ceremony } from "./passkey";
import type { Problem } from "@/lib/api/server";

export type IdentityProofState = { required: boolean; status: "" | "pending" | "verified" | "failed"; provider: string };
export type Enrollee = { user: { name: string; email: string }; purpose: string; identityProof?: IdentityProofState };
type Stop = { icon: LucideIcon; title: string; body: string; bodyLocale?: Locale };

const TERMINAL = ["ERR_LINK_EXPIRED", "ERR_ACCOUNT_SUSPENDED", "ERR_ACCOUNT_EXISTS", "ERR_SIGN_UP_OFF", "ERR_SIGN_UP_DOMAIN"];
const RESUME_KEY = "halo.identity-check.return";

function stopFor(problem: Problem, signUp: boolean, messages: SignInMessages["enrollment"]): Stop {
  if (problem.code === "ERR_LINK_EXPIRED" && signUp) {
    return { icon: Clock, title: messages.expiredSignUp, body: messages.expiredSignUpHint };
  }
  if (problem.code === "ERR_LINK_EXPIRED") {
    return { icon: Clock, title: messages.expired, body: messages.expiredHint };
  }
  if (problem.code === "ERR_ACCOUNT_SUSPENDED") return { icon: CircleAlert, title: messages.suspended, body: problem.message, bodyLocale: "en" };
  if (problem.code === "ERR_ACCOUNT_EXISTS") return { icon: CircleUserRound, title: messages.existing, body: problem.message, bodyLocale: "en" };
  return { icon: CircleAlert, title: messages.openFailed, body: problem.message, bodyLocale: "en" };
}

export function EnrollFlow({ token, enrollee, problem, signUp = false, locale = "en" }: { token: string; enrollee: Enrollee | null; problem: Problem | null; signUp?: boolean; locale?: Locale }) {
  const copy = authMessages(locale);
  const messages = copy.enrollment;
  const supported = usePasskeySupport();
  const base = signUp ? "/auth/sign-up" : "/auth/enroll";
  const [stop, setStop] = useState(() => (problem ? stopFor(problem, signUp, messages) : null));
  const [label, setLabel] = useState("");
  const [labelError, setLabelError] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState<"signed-in" | "pending" | "identity" | null>(null);
  const [inPerson, setInPerson] = useState(false);
  const router = useRouter();
  const proof = enrollee?.identityProof;
  const checking = Boolean(proof?.required && proof.status !== "verified" && !(signUp && inPerson));

  useEffect(() => setLabel(deviceLabel()), []);

  useEffect(() => {
    if (proof?.status !== "pending") return;
    const timer = window.setInterval(() => router.refresh(), 5000);
    return () => window.clearInterval(timer);
  }, [proof?.status, router]);

  async function verify() {
    setBusy(true);
    setError(null);
    try {
      const { url } = await api<{ url: string }>(`${base}/identity-proof`, { body: { token } });
      try {
        window.sessionStorage.setItem(RESUME_KEY, window.location.href);
      } catch {}
      window.location.assign(url);
    } catch (error) {
      if (error instanceof ApiError && TERMINAL.includes(error.code)) setStop(stopFor(error, signUp, messages));
      else setError(describeFailure(error));
      setBusy(false);
    }
  }

  async function create(event: FormEvent) {
    event.preventDefault();
    if (!label.trim()) {
      setLabelError(true);
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const { ceremony, options } = await api<Ceremony>(`${base}/begin`, { body: { token } });
      const credential = await createCredential(options);
      const result = await api<{ redirect?: string; pending?: boolean; identityCheck?: boolean }>(`${base}/finish`, { body: { token, ceremony, credential, label: label.trim() } });
      setDone(result.identityCheck ? "identity" : result.pending ? "pending" : "signed-in");
      if (result.redirect) window.location.assign(result.redirect);
    } catch (error) {
      if (error instanceof ApiError && TERMINAL.includes(error.code)) setStop(stopFor(error, signUp, messages));
      else if (error instanceof ApiError && error.code === "ERR_IDENTITY_CHECK_REQUIRED") router.refresh();
      else if (!cancelled(error)) setError(describeFailure(error));
    } finally {
      setBusy(false);
    }
  }

  if (stop || !enrollee) {
    const { icon: Icon, title, body, bodyLocale } = stop ?? { icon: CircleAlert, title: messages.openFailed, body: messages.reload, bodyLocale: locale };
    return (
      <div lang={locale} className="flex animate-enter flex-col items-center gap-6 text-center">
        <div className="flex flex-col items-center gap-3">
          <Icon aria-hidden="true" size={24} strokeWidth={1.75} className="text-fg-3" />
          <h1 className="text-h3 text-fg">{title}</h1>
          <p lang={bodyLocale} className="text-body text-fg-3">{body}</p>
        </div>
        {signUp ? (
          <Link href="/sign-in" className={buttonClasses("secondary", "lg", "w-full")}>
            {copy.backToSignIn}
          </Link>
        ) : null}
      </div>
    );
  }

  if (done === "pending" || done === "identity") {
    return (
      <div lang={locale} role="status" className="flex animate-enter flex-col items-center gap-6 text-center">
        <div className="flex flex-col items-center gap-3">
          <Clock aria-hidden="true" size={24} strokeWidth={1.75} className="text-fg-3" />
          <h1 className="text-h3 text-fg">{done === "identity" ? messages.identityWaiting : messages.approvalWaiting}</h1>
          <p className="text-body text-fg-3">
            {done === "identity"
              ? messages.identityWaitingHint
              : messages.approvalWaitingHint}
          </p>
        </div>
        <Link href="/sign-in" className={buttonClasses("secondary", "lg", "w-full")}>
          {copy.backToSignIn}
        </Link>
      </div>
    );
  }

  if (done) {
    return (
      <div lang={locale} role="status" className="flex animate-enter flex-col items-center gap-3 text-center">
        <CircleCheck aria-hidden="true" size={24} strokeWidth={1.75} className="text-success" />
        <h1 className="text-h3 text-fg">{messages.created}</h1>
        <p className="text-body text-fg-3">{copy.openingAccount}</p>
      </div>
    );
  }

  const email = (
    <div className="flex items-center gap-3 rounded-md border border-border bg-surface px-4 py-3">
      <CircleUserRound aria-hidden="true" size={20} strokeWidth={1.75} className="shrink-0 text-fg-3" />
      <span className="truncate text-body-sm text-fg">{enrollee.user.email}</span>
    </div>
  );
  const alert = error ? (
    <div role="alert" lang="en" className="flex items-start gap-3 rounded-md border border-danger/20 bg-danger-container p-4">
      <CircleAlert aria-hidden="true" size={20} strokeWidth={1.75} className="mt-px shrink-0 text-danger" />
      <p className="text-body-sm text-fg-2">{error}</p>
    </div>
  ) : null;

  if (checking && proof) {
    return (
      <div lang={locale} className="flex animate-enter flex-col gap-8">
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-3 text-center">
            <h1 className="text-h3 text-fg">{messages.confirmIdentity(enrollee.user.name)}</h1>
            <p className="text-body text-fg-3">
              {proof.provider
                ? messages.providerProof(proof.provider)
                : messages.proofInPerson}
            </p>
          </div>
          {email}
        </div>
        {proof.status === "pending" ? (
          <div role="status" className="flex items-start gap-3 rounded-md border border-border bg-surface p-4">
            <Spinner className="mt-0.5 shrink-0 text-fg-3" />
            <p className="text-body-sm text-fg-2">{messages.providerWaiting(proof.provider)}</p>
          </div>
        ) : null}
        {proof.status === "failed" ? (
          <div role="alert" className="flex items-start gap-3 rounded-md border border-danger/20 bg-danger-container p-4">
            <CircleAlert aria-hidden="true" size={20} strokeWidth={1.75} className="mt-px shrink-0 text-danger" />
            <p className="text-body-sm text-fg-2">
              {messages.providerFailed(proof.provider)}
            </p>
          </div>
        ) : null}
        {alert}
        <div className="flex flex-col gap-3">
          {proof.provider ? (
            <Button variant="primary" size="lg" className="w-full" loading={busy} onClick={() => void verify()}>
              <ScanFace aria-hidden="true" size={20} strokeWidth={1.75} />
              {proof.status === "failed" ? messages.tryAgain : messages.verifyWith(proof.provider)}
            </Button>
          ) : null}
          {signUp ? (
            <Button variant={proof.provider ? "quiet" : "primary"} size="lg" className="w-full" onClick={() => setInPerson(true)}>
              <IdCard aria-hidden="true" size={20} strokeWidth={1.75} />
              {proof.provider ? messages.verifyInPerson : messages.waitForCheck}
            </Button>
          ) : (
            <Button variant={proof.provider ? "quiet" : "secondary"} size="lg" className="w-full" onClick={() => router.refresh()}>
              {proof.provider ? messages.checkedInPerson : messages.checked}
            </Button>
          )}
        </div>
      </div>
    );
  }

  return (
    <div lang={locale} className="flex animate-enter flex-col gap-8">
      <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-3 text-center">
          <h1 className="text-h3 text-fg">{messages.setUpFor(enrollee.user.name)}</h1>
          <p className="text-body text-fg-3">
            {enrollee.purpose === "reset"
              ? messages.reset
              : enrollee.purpose === "sign-up"
                ? messages.signUp
                : enrollee.purpose === "access-pass"
                  ? messages.accessPass
                  : messages.invite}
            {proof?.required && proof.status !== "verified" ? messages.identityRequired : ""}
          </p>
        </div>
        {email}
      </div>
      {alert}
      {supported ? (
        <form noValidate className="flex flex-col gap-6" onSubmit={create}>
          <Field
            label={messages.name}
            hint={messages.nameHint}
            error={labelError ? messages.nameRequired : undefined}
          >
            {(field) => (
              <Input
                {...field}
                value={label}
                onChange={(event) => {
                  setLabel(event.target.value);
                  setLabelError(false);
                }}
                className="h-12"
              />
            )}
          </Field>
          <Button type="submit" variant="primary" size="lg" className="w-full" loading={busy}>
            <Fingerprint aria-hidden="true" size={20} strokeWidth={1.75} />
            {messages.create}
          </Button>
        </form>
      ) : (
        <p className="text-center text-body text-fg-3">{messages.unavailable}</p>
      )}
    </div>
  );
}

export function ResumeAfterCheck({ signUp = false, locale = "en" }: { signUp?: boolean; locale?: Locale }) {
  const messages = authMessages(locale).enrollment;
  const [lost, setLost] = useState(false);

  useEffect(() => {
    let target: string | null = null;
    try {
      target = window.sessionStorage.getItem(RESUME_KEY);
      window.sessionStorage.removeItem(RESUME_KEY);
    } catch {}
    const prefix = `${window.location.origin}${signUp ? "/sign-up?" : "/enroll?"}`;
    if (target?.startsWith(prefix)) window.location.replace(target);
    else setLost(true);
  }, [signUp]);

  if (!lost) {
    return (
      <div lang={locale} role="status" className="flex animate-enter flex-col items-center gap-3 text-center">
        <Spinner className="text-fg-3" size={24} />
        <p className="text-body text-fg-3">{messages.resume}</p>
      </div>
    );
  }
  return (
    <div lang={locale} className="flex animate-enter flex-col items-center gap-6 text-center">
      <div className="flex flex-col items-center gap-3">
        <CircleCheck aria-hidden="true" size={24} strokeWidth={1.75} className="text-fg-3" />
        <h1 className="text-h3 text-fg">{messages.identitySent}</h1>
        <p className="text-body text-fg-3">
          {signUp ? messages.signUpResumeHint : messages.resumeHint}
        </p>
      </div>
    </div>
  );
}
