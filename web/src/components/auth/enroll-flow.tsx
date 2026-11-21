"use client";

import { CircleAlert, CircleCheck, CircleUserRound, Clock, Fingerprint, IdCard, ScanFace, type LucideIcon } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState, type FormEvent } from "react";
import { Button, buttonClasses } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { api, ApiError } from "@/lib/api/client";
import { createCredential } from "@/lib/webauthn";
import { cancelled, describeFailure, deviceLabel, usePasskeySupport, type Ceremony } from "./passkey";
import type { Problem } from "@/lib/api/server";

export type IdentityProofState = { required: boolean; status: "" | "pending" | "verified" | "failed"; provider: string };
export type Enrollee = { user: { name: string; email: string }; purpose: string; identityProof?: IdentityProofState };
type Stop = { icon: LucideIcon; title: string; body: string };

const TERMINAL = ["ERR_LINK_EXPIRED", "ERR_ACCOUNT_SUSPENDED", "ERR_ACCOUNT_EXISTS", "ERR_SIGN_UP_OFF", "ERR_SIGN_UP_DOMAIN"];
const RESUME_KEY = "halo.identity-check.return";

function stopFor(problem: Problem, signUp: boolean): Stop {
  if (problem.code === "ERR_LINK_EXPIRED" && signUp) {
    return { icon: Clock, title: "This sign-up link has expired", body: "It may have been used already, or a newer link replaced it. Sign up again to get a new one." };
  }
  if (problem.code === "ERR_LINK_EXPIRED") {
    return { icon: Clock, title: "This setup link has expired", body: "It may have been used already. Ask an administrator to send you a new link." };
  }
  if (problem.code === "ERR_ACCOUNT_SUSPENDED") return { icon: CircleAlert, title: "This account is suspended", body: problem.message };
  if (problem.code === "ERR_ACCOUNT_EXISTS") return { icon: CircleUserRound, title: "You already have an account", body: problem.message };
  return { icon: CircleAlert, title: "Halo couldn't open this link", body: problem.message };
}

export function EnrollFlow({ token, enrollee, problem, signUp = false }: { token: string; enrollee: Enrollee | null; problem: Problem | null; signUp?: boolean }) {
  const supported = usePasskeySupport();
  const base = signUp ? "/auth/sign-up" : "/auth/enroll";
  const [stop, setStop] = useState(() => (problem ? stopFor(problem, signUp) : null));
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
      if (error instanceof ApiError && TERMINAL.includes(error.code)) setStop(stopFor(error, signUp));
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
      if (error instanceof ApiError && TERMINAL.includes(error.code)) setStop(stopFor(error, signUp));
      else if (error instanceof ApiError && error.code === "ERR_IDENTITY_CHECK_REQUIRED") router.refresh();
      else if (!cancelled(error)) setError(describeFailure(error));
    } finally {
      setBusy(false);
    }
  }

  if (stop || !enrollee) {
    const { icon: Icon, title, body } = stop ?? stopFor({ code: "", message: "Reload the page to try again." }, signUp);
    return (
      <div className="flex animate-enter flex-col items-center gap-6 text-center">
        <div className="flex flex-col items-center gap-3">
          <Icon aria-hidden="true" size={24} strokeWidth={1.75} className="text-fg-3" />
          <h1 className="text-h3 text-fg">{title}</h1>
          <p className="text-body text-fg-3">{body}</p>
        </div>
        {signUp ? (
          <Link href="/sign-in" className={buttonClasses("secondary", "lg", "w-full")}>
            Back to sign in
          </Link>
        ) : null}
      </div>
    );
  }

  if (done === "pending" || done === "identity") {
    return (
      <div role="status" className="flex animate-enter flex-col items-center gap-6 text-center">
        <div className="flex flex-col items-center gap-3">
          <Clock aria-hidden="true" size={24} strokeWidth={1.75} className="text-fg-3" />
          <h1 className="text-h3 text-fg">{done === "identity" ? "Waiting for an identity check" : "Waiting for approval"}</h1>
          <p className="text-body text-fg-3">
            {done === "identity"
              ? "Your email address is confirmed and your passkey is ready. An administrator checks your identity before your account is activated. Once they have, sign in with your passkey."
              : "Your email address is confirmed and your passkey is ready. An administrator approves each new account. Once they approve yours, sign in with your passkey."}
          </p>
        </div>
        <Link href="/sign-in" className={buttonClasses("secondary", "lg", "w-full")}>
          Back to sign in
        </Link>
      </div>
    );
  }

  if (done) {
    return (
      <div role="status" className="flex animate-enter flex-col items-center gap-3 text-center">
        <CircleCheck aria-hidden="true" size={24} strokeWidth={1.75} className="text-success" />
        <h1 className="text-h3 text-fg">Passkey created</h1>
        <p className="text-body text-fg-3">Opening your Halo account…</p>
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
    <div role="alert" className="flex items-start gap-3 rounded-md border border-danger/20 bg-danger-container p-4">
      <CircleAlert aria-hidden="true" size={20} strokeWidth={1.75} className="mt-px shrink-0 text-danger" />
      <p className="text-body-sm text-fg-2">{error}</p>
    </div>
  ) : null;

  if (checking && proof) {
    return (
      <div className="flex animate-enter flex-col gap-8">
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-3 text-center">
            <h1 className="text-h3 text-fg">Confirm who you are, {enrollee.user.name}</h1>
            <p className="text-body text-fg-3">
              {proof.provider
                ? `Your organization checks each new person's identity before their account is activated. ${proof.provider} asks for a photo of an identity document and of your face. Halo keeps only the result.`
                : "Your organization checks each new person's identity before their account is activated. Ask your administrator to check yours in person or on a video call, with an identity document that has your photo."}
            </p>
          </div>
          {email}
        </div>
        {proof.status === "pending" ? (
          <div role="status" className="flex items-start gap-3 rounded-md border border-border bg-surface p-4">
            <Spinner className="mt-0.5 shrink-0 text-fg-3" />
            <p className="text-body-sm text-fg-2">Waiting for the result from {proof.provider || "the provider"}. This page updates by itself.</p>
          </div>
        ) : null}
        {proof.status === "failed" ? (
          <div role="alert" className="flex items-start gap-3 rounded-md border border-danger/20 bg-danger-container p-4">
            <CircleAlert aria-hidden="true" size={20} strokeWidth={1.75} className="mt-px shrink-0 text-danger" />
            <p className="text-body-sm text-fg-2">
              {proof.provider || "The provider"} couldn&apos;t confirm your identity. Try again with a valid document in good light, or ask your administrator to check you in person.
            </p>
          </div>
        ) : null}
        {alert}
        <div className="flex flex-col gap-3">
          {proof.provider ? (
            <Button variant="primary" size="lg" className="w-full" loading={busy} onClick={() => void verify()}>
              <ScanFace aria-hidden="true" size={20} strokeWidth={1.75} />
              {proof.status === "failed" ? "Try again" : `Verify with ${proof.provider}`}
            </Button>
          ) : null}
          {signUp ? (
            <Button variant={proof.provider ? "quiet" : "primary"} size="lg" className="w-full" onClick={() => setInPerson(true)}>
              <IdCard aria-hidden="true" size={20} strokeWidth={1.75} />
              {proof.provider ? "Have my identity checked in person instead" : "Continue and wait for the check"}
            </Button>
          ) : (
            <Button variant={proof.provider ? "quiet" : "secondary"} size="lg" className="w-full" onClick={() => router.refresh()}>
              {proof.provider ? "My administrator checked me in person" : "My administrator has checked me"}
            </Button>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className="flex animate-enter flex-col gap-8">
      <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-3 text-center">
          <h1 className="text-h3 text-fg">Set up your passkey, {enrollee.user.name}</h1>
          <p className="text-body text-fg-3">
            {enrollee.purpose === "reset"
              ? "An administrator reset how you sign in, so your old sign-in methods were removed. Create a new passkey to get back into your account."
              : enrollee.purpose === "sign-up"
                ? "Last step: create the passkey you'll use to sign in to Halo with your fingerprint, face or screen lock. Your biometrics never leave your device."
                : enrollee.purpose === "access-pass"
                  ? "Your temporary access pass worked. Create a passkey in the next 15 minutes to finish signing in."
                  : "You'll use it to sign in to Halo with your fingerprint, face or screen lock. Your biometrics never leave your device."}
            {proof?.required && proof.status !== "verified" ? " An administrator checks your identity before your account is activated." : ""}
          </p>
        </div>
        {email}
      </div>
      {alert}
      {supported ? (
        <form noValidate className="flex flex-col gap-6" onSubmit={create}>
          <Field
            label="Passkey name"
            hint="Helps you tell your passkeys apart later, for example the browser and device."
            error={labelError ? "Give it a name you'll recognise, like “Firefox on Linux”." : undefined}
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
            Create passkey
          </Button>
        </form>
      ) : (
        <p className="text-center text-body text-fg-3">This browser can&apos;t create passkeys. Open this link in an up-to-date browser to continue.</p>
      )}
    </div>
  );
}

export function ResumeAfterCheck({ signUp = false }: { signUp?: boolean }) {
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
      <div role="status" className="flex animate-enter flex-col items-center gap-3 text-center">
        <Spinner className="text-fg-3" size={24} />
        <p className="text-body text-fg-3">Returning to your setup…</p>
      </div>
    );
  }
  return (
    <div className="flex animate-enter flex-col items-center gap-6 text-center">
      <div className="flex flex-col items-center gap-3">
        <CircleCheck aria-hidden="true" size={24} strokeWidth={1.75} className="text-fg-3" />
        <h1 className="text-h3 text-fg">Identity check sent</h1>
        <p className="text-body text-fg-3">
          {signUp ? "Open the sign-up link from your email again to see the result and create your passkey." : "Open your setup link again to see the result and create your passkey."}
        </p>
      </div>
    </div>
  );
}
