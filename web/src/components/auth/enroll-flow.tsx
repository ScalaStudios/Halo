"use client";

import { CircleAlert, CircleCheck, CircleUserRound, Clock, Fingerprint, type LucideIcon } from "lucide-react";
import Link from "next/link";
import { useEffect, useState, type FormEvent } from "react";
import { Button, buttonClasses } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/input";
import { api, ApiError } from "@/lib/api/client";
import { createCredential } from "@/lib/webauthn";
import { cancelled, describeFailure, deviceLabel, usePasskeySupport, type Ceremony } from "./passkey";
import type { Problem } from "@/lib/api/server";

export type Enrollee = { user: { name: string; email: string }; purpose: string };
type Stop = { icon: LucideIcon; title: string; body: string };

const TERMINAL = ["ERR_LINK_EXPIRED", "ERR_ACCOUNT_SUSPENDED", "ERR_ACCOUNT_EXISTS", "ERR_SIGN_UP_OFF", "ERR_SIGN_UP_DOMAIN"];

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
  const [done, setDone] = useState<"signed-in" | "pending" | null>(null);

  useEffect(() => setLabel(deviceLabel()), []);

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
      const result = await api<{ redirect?: string; pending?: boolean }>(`${base}/finish`, { body: { token, ceremony, credential, label: label.trim() } });
      setDone(result.pending ? "pending" : "signed-in");
      if (result.redirect) window.location.assign(result.redirect);
    } catch (error) {
      if (error instanceof ApiError && TERMINAL.includes(error.code)) setStop(stopFor(error, signUp));
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

  if (done === "pending") {
    return (
      <div role="status" className="flex animate-enter flex-col items-center gap-6 text-center">
        <div className="flex flex-col items-center gap-3">
          <Clock aria-hidden="true" size={24} strokeWidth={1.75} className="text-fg-3" />
          <h1 className="text-h3 text-fg">Waiting for approval</h1>
          <p className="text-body text-fg-3">
            Your email address is confirmed and your passkey is ready. An administrator approves each new account. Once they approve yours, sign in with your passkey.
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
                : "You'll use it to sign in to Halo with your fingerprint, face or screen lock. Your biometrics never leave your device."}
          </p>
        </div>
        <div className="flex items-center gap-3 rounded-md border border-border bg-surface px-4 py-3">
          <CircleUserRound aria-hidden="true" size={20} strokeWidth={1.75} className="shrink-0 text-fg-3" />
          <span className="truncate text-body-sm text-fg">{enrollee.user.email}</span>
        </div>
      </div>
      {error ? (
        <div role="alert" className="flex items-start gap-3 rounded-md border border-danger/20 bg-danger-container p-4">
          <CircleAlert aria-hidden="true" size={20} strokeWidth={1.75} className="mt-px shrink-0 text-danger" />
          <p className="text-body-sm text-fg-2">{error}</p>
        </div>
      ) : null}
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
