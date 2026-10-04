"use client";

import { CircleAlert, CircleCheck, CircleUserRound, Clock, Fingerprint, type LucideIcon } from "lucide-react";
import { useEffect, useState, type FormEvent } from "react";
import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/input";
import { api, ApiError } from "@/lib/api/client";
import { createCredential } from "@/lib/webauthn";
import { cancelled, describeFailure, deviceLabel, usePasskeySupport, type Ceremony } from "./passkey";
import type { Problem } from "@/lib/api/server";

export type Enrollee = { user: { name: string; email: string }; purpose: string };
type Stop = { icon: LucideIcon; title: string; body: string };

function stopFor(problem: Problem): Stop {
  if (problem.code === "ERR_LINK_EXPIRED") {
    return { icon: Clock, title: "This setup link has expired", body: "It may have been used already. Ask an administrator to send you a new link." };
  }
  if (problem.code === "ERR_ACCOUNT_SUSPENDED") return { icon: CircleAlert, title: "This account is suspended", body: problem.message };
  return { icon: CircleAlert, title: "Halo couldn't open this link", body: problem.message };
}

export function EnrollFlow({ token, enrollee, problem }: { token: string; enrollee: Enrollee | null; problem: Problem | null }) {
  const supported = usePasskeySupport();
  const [stop, setStop] = useState(() => (problem ? stopFor(problem) : null));
  const [label, setLabel] = useState("");
  const [labelError, setLabelError] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);

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
      const { ceremony, options } = await api<Ceremony>("/auth/enroll/begin", { body: { token } });
      const credential = await createCredential(options);
      const { redirect } = await api<{ redirect: string }>("/auth/enroll/finish", { body: { token, ceremony, credential, label: label.trim() } });
      setDone(true);
      window.location.assign(redirect);
    } catch (error) {
      if (error instanceof ApiError && (error.code === "ERR_LINK_EXPIRED" || error.code === "ERR_ACCOUNT_SUSPENDED")) setStop(stopFor(error));
      else if (!cancelled(error)) setError(describeFailure(error));
    } finally {
      setBusy(false);
    }
  }

  if (stop || !enrollee) {
    const { icon: Icon, title, body } = stop ?? stopFor({ code: "", message: "Reload the page to try again." });
    return (
      <div className="flex animate-enter flex-col items-center gap-3 text-center">
        <Icon aria-hidden="true" size={24} strokeWidth={1.75} className="text-fg-3" />
        <h1 className="text-h3 text-fg">{title}</h1>
        <p className="text-body text-fg-3">{body}</p>
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
