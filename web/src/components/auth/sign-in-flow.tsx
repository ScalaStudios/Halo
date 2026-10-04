"use client";

import { ChevronDown, CircleAlert, CircleCheck, CircleUserRound, Clock, Fingerprint } from "lucide-react";
import { useSearchParams } from "next/navigation";
import { useEffect, useId, useRef, useState, type FormEvent } from "react";
import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { api, ApiError } from "@/lib/api/client";
import { cn } from "@/lib/cn";
import { METHOD } from "@/lib/labels";
import { getAssertion } from "@/lib/webauthn";
import { cancelled, describeFailure, usePasskeySupport, type Ceremony } from "./passkey";
import { federationProblem, ProviderButtons, useSignInProviders } from "./providers";
import { VerifyCode } from "./verify-code";

type Step = "loading" | "start" | "choose" | "passkey" | "security-key" | "code" | "recovery" | "inbox" | "done" | "expired";
type Choice = "passkey" | "security-key" | "totp" | "recovery-codes" | "magic-link";

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

const linkButton =
  "inline-flex min-h-12 items-center self-center rounded-sm text-body-sm text-link underline underline-offset-3 transition-colors duration-fast ease-brand hover:text-ember-hover";

function emailProblem(value: string) {
  if (!value.trim()) return "Enter your work email address, for example name@example.com.";
  if (!EMAIL.test(value.trim())) return "That doesn't look like an email address. Check it has an @ and a domain, like name@example.com.";
  return null;
}

function choiceLabel(kind: Choice) {
  if (kind === "magic-link") return "Email me a sign-in link";
  return kind === "recovery-codes" ? "Recovery code" : METHOD[kind].label;
}

function expired(error: unknown) {
  return error instanceof ApiError && error.code === "ERR_AUTH_REQUEST_EXPIRED";
}

function sameOrigin(path: string | undefined) {
  if (!path) return null;
  try {
    const url = new URL(path, window.location.origin);
    return url.origin === window.location.origin ? url.pathname + url.search + url.hash : null;
  } catch {
    return null;
  }
}

export function SignInFlow({
  authRequest,
  app,
  expired: requestExpired,
  next,
  signedIn,
  allowed,
}: {
  authRequest?: string;
  app: string | null;
  expired: boolean;
  next?: string;
  signedIn: boolean;
  allowed: string[];
}) {
  const optionsId = useId();
  const emailRef = useRef<HTMLInputElement>(null);
  const attempt = useRef<AbortController | null>(null);
  const checked = useRef(false);
  const supported = usePasskeySupport();
  const providers = useSignInProviders();
  const failure = useSearchParams().get("error");
  const [step, setStep] = useState<Step>(
    requestExpired || failure === "ERR_AUTH_REQUEST_EXPIRED" ? "expired" : authRequest && signedIn && !failure ? "loading" : "start",
  );
  const [email, setEmail] = useState("");
  const [methods, setMethods] = useState<Choice[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(() => federationProblem(failure));
  const [options, setOptions] = useState(false);
  const [focus, setFocus] = useState<"passkey" | "email" | null>(null);
  const [busy, setBusy] = useState(false);
  const [recovery, setRecovery] = useState("");
  const [recoveryError, setRecoveryError] = useState<string | null>(null);
  const [sending, setSending] = useState(false);
  const [cooldown, setCooldown] = useState(0);
  const [resent, setResent] = useState(false);
  const home = methods.length > 0 ? "choose" : "start";

  useEffect(() => {
    if (cooldown <= 0) return;
    const timer = setTimeout(() => setCooldown((seconds) => seconds - 1), 1000);
    return () => clearTimeout(timer);
  }, [cooldown]);

  useEffect(() => {
    if (step !== "loading" || checked.current) return;
    checked.current = true;
    api<{ redirect: string }>("/auth/continue", { body: { authRequest } })
      .then(({ redirect }) => {
        setStep("done");
        window.location.assign(redirect);
      })
      .catch((error) => {
        if (expired(error)) return setStep("expired");
        if (!(error instanceof ApiError && error.status === 401)) setNotice(describeFailure(error));
        setStep("start");
      });
  }, [step, authRequest]);

  function finish(redirect: string) {
    setStep("done");
    window.location.assign(authRequest ? redirect : (sameOrigin(next) ?? redirect));
  }

  function back(to: Step = home) {
    setError(null);
    setFocus("passkey");
    setStep(to);
  }

  function forget() {
    setEmail("");
    setMethods([]);
    setCooldown(0);
    setError(null);
    setNotice(null);
    setFocus("email");
    setStep("start");
  }

  function differentMethod() {
    if (home === "start") setOptions(true);
    back();
  }

  async function passkey(kind: "passkey" | "security-key") {
    const controller = new AbortController();
    attempt.current?.abort();
    attempt.current = controller;
    setNotice(null);
    setStep(kind);
    try {
      const { ceremony, options } = await api<Ceremony>("/auth/passkey/begin", { method: "POST" });
      const credential = await getAssertion(options, controller.signal);
      const { redirect } = await api<{ redirect: string }>("/auth/passkey/finish", { body: { ceremony, credential, authRequest } });
      finish(redirect);
    } catch (error) {
      if (attempt.current !== controller) return;
      attempt.current = null;
      if (expired(error)) return setStep("expired");
      if (!cancelled(error)) setNotice(describeFailure(error));
      back();
    }
  }

  function cancelPasskey() {
    attempt.current?.abort();
    attempt.current = null;
    back();
  }

  async function sendLink() {
    const again = step === "inbox";
    setSending(true);
    setNotice(null);
    try {
      await api("/auth/magic-link", { body: { email: email.trim(), authRequest } });
      setResent(again);
      setCooldown(30);
      setStep("inbox");
    } catch (error) {
      if (expired(error)) return setStep("expired");
      setNotice(describeFailure(error));
    } finally {
      setSending(false);
    }
  }

  function pick(kind: Choice) {
    if (kind === "passkey" || kind === "security-key") return passkey(kind);
    if (kind === "magic-link") return sendLink();
    setRecovery("");
    setRecoveryError(null);
    setStep(kind === "totp" ? "code" : "recovery");
  }

  function validEmail() {
    const problem = emailProblem(email);
    setError(problem);
    if (problem) emailRef.current?.focus();
    else setEmail(email.trim());
    return !problem;
  }

  function other(kind: Choice) {
    if (kind === "security-key" || validEmail()) pick(kind);
  }

  async function identify(event: FormEvent) {
    event.preventDefault();
    if (!validEmail()) return;
    setBusy(true);
    try {
      const result = await api<{ methods: Choice[] }>("/auth/identify", { body: { email: email.trim() } });
      setMethods(result.methods.filter((kind) => supported || (kind !== "passkey" && kind !== "security-key")));
      setNotice(null);
      setStep("choose");
    } catch (error) {
      setError(describeFailure(error));
    } finally {
      setBusy(false);
    }
  }

  async function signInWithCode(path: "/auth/totp" | "/auth/recovery", code: string) {
    try {
      const { redirect } = await api<{ redirect: string }>(path, { body: { email, code, authRequest } });
      finish(redirect);
    } catch (error) {
      if (expired(error)) setStep("expired");
      throw error;
    }
  }

  async function submitRecovery(event: FormEvent) {
    event.preventDefault();
    if (!recovery.trim()) {
      setRecoveryError("Enter one of your recovery codes, like abcd-efgh-ijkl-mnop.");
      return;
    }
    setBusy(true);
    try {
      await signInWithCode("/auth/recovery", recovery.trim());
    } catch (error) {
      setRecoveryError(describeFailure(error));
    } finally {
      setBusy(false);
    }
  }

  const account = (
    <div className="flex items-center justify-between gap-3 rounded-md border border-border bg-surface py-1 pr-3 pl-4">
      <span className="flex min-w-0 items-center gap-3">
        <CircleUserRound aria-hidden="true" size={20} strokeWidth={1.75} className="shrink-0 text-fg-3" />
        <span className="truncate text-body-sm text-fg">{email}</span>
      </span>
      <button type="button" onClick={forget} className={cn(linkButton, "shrink-0 px-1")}>
        Not you?
      </button>
    </div>
  );

  const alert = notice ? (
    <div role="alert" className="flex items-start gap-3 rounded-md border border-danger/20 bg-danger-container p-4">
      <CircleAlert aria-hidden="true" size={20} strokeWidth={1.75} className="mt-px shrink-0 text-danger" />
      <p className="text-body-sm text-fg-2">{notice}</p>
    </div>
  ) : null;

  const passkeys = supported && allowed.includes("passkey");
  const permitted = (kind: Choice) => allowed.includes(kind);
  const otherKinds: Choice[] = (supported ? (["security-key", "totp", "recovery-codes", "magic-link"] as Choice[]) : (["totp", "recovery-codes", "magic-link"] as Choice[])).filter(permitted);
  const choices: Choice[] = ([...methods, "magic-link"] as Choice[]).filter(permitted);

  return (
    <div key={step} className="flex animate-enter flex-col gap-8">
      {step === "loading" ? (
        <div role="status" className="flex items-center justify-center gap-3 text-body-sm text-fg-3">
          <Spinner size={20} />
          {app ? `Continuing to ${app}` : "Checking your session"}
        </div>
      ) : null}

      {step === "start" ? (
        <>
          <div className="flex flex-col gap-3 text-center">
            <h1 className="text-h3 text-fg">{app ? `Sign in to continue to ${app}` : "Sign in to continue"}</h1>
            {supported ? null : <p className="text-body-sm text-fg-3">This browser can&apos;t use passkeys, so sign in with your email instead.</p>}
          </div>
          {alert}
          <div className="flex flex-col gap-6">
            {passkeys || providers.length > 0 ? (
              <>
                <div className="flex flex-col gap-2">
                  {passkeys ? (
                    <Button variant="primary" size="lg" className="w-full" autoFocus={focus === "passkey"} onClick={() => passkey("passkey")}>
                      <Fingerprint aria-hidden="true" size={20} strokeWidth={1.75} />
                      Continue with passkey
                    </Button>
                  ) : null}
                  <ProviderButtons providers={providers} authRequest={authRequest} next={next} />
                </div>
                <div className="flex items-center gap-3 text-caption text-fg-3">
                  <span aria-hidden="true" className="h-px flex-1 bg-border" />
                  or
                  <span aria-hidden="true" className="h-px flex-1 bg-border" />
                </div>
              </>
            ) : null}
            <form noValidate className="flex flex-col gap-4" onSubmit={identify}>
              <Field label="Email" error={error}>
                {(field) => (
                  <Input
                    {...field}
                    ref={emailRef}
                    type="email"
                    name="email"
                    inputMode="email"
                    autoComplete="username webauthn"
                    autoCapitalize="none"
                    spellCheck={false}
                    placeholder="name@example.com"
                    autoFocus={focus === "email" || !supported}
                    value={email}
                    onChange={(event) => {
                      setEmail(event.target.value);
                      if (error) setError(null);
                    }}
                    className="h-12"
                  />
                )}
              </Field>
              <Button type="submit" variant={passkeys ? "secondary" : "primary"} size="lg" className="w-full" loading={busy}>
                Continue
              </Button>
            </form>
            <div className="flex flex-col gap-2">
              <Button
                variant="quiet"
                size="lg"
                className="self-center"
                aria-expanded={options}
                aria-controls={options ? optionsId : undefined}
                onClick={() => setOptions((open) => !open)}
              >
                Other options
                <ChevronDown aria-hidden="true" size={16} strokeWidth={1.75} className={cn("transition-transform duration-fast ease-brand", options && "rotate-180")} />
              </Button>
              {options ? (
                <ul id={optionsId} className="flex animate-enter flex-col gap-2">
                  {otherKinds.map((kind) => {
                    const Icon = METHOD[kind].icon;
                    return (
                      <li key={kind}>
                        <Button size="lg" className="w-full" loading={kind === "magic-link" && sending} onClick={() => other(kind)}>
                          <Icon aria-hidden="true" size={20} strokeWidth={1.75} className="text-fg-3" />
                          {choiceLabel(kind)}
                        </Button>
                      </li>
                    );
                  })}
                </ul>
              ) : null}
            </div>
          </div>
        </>
      ) : null}

      {step === "choose" ? (
        <>
          <div className="flex flex-col gap-4">
            <h1 className="text-center text-h3 text-fg">Choose how to sign in</h1>
            {account}
          </div>
          {alert}
          <ul className="flex flex-col gap-2">
            {choices.map((kind, index) => {
              const Icon = METHOD[kind].icon;
              return (
                <li key={kind}>
                  <Button
                    variant={index === 0 ? "primary" : "secondary"}
                    size="lg"
                    className="w-full"
                    autoFocus={index === 0}
                    loading={kind === "magic-link" && sending}
                    onClick={() => pick(kind)}
                  >
                    <Icon aria-hidden="true" size={20} strokeWidth={1.75} className={index === 0 ? undefined : "text-fg-3"} />
                    {choiceLabel(kind)}
                  </Button>
                </li>
              );
            })}
          </ul>
        </>
      ) : null}

      {step === "inbox" ? (
        <>
          <div className="flex flex-col gap-4">
            <h1 className="text-center text-h3 text-fg">Check your inbox</h1>
            {account}
          </div>
          {alert}
          <p className="text-center text-body text-fg-3">
            If this address has a Halo account, a sign-in link is on its way. It works once and expires in 10 minutes.
            {app ? ` Open it in this browser to continue to ${app}.` : null}
          </p>
          <div className="flex flex-col gap-2">
            <Button size="lg" className="w-full" loading={sending} disabled={cooldown > 0} onClick={sendLink}>
              {cooldown > 0 ? `Resend link in ${cooldown}s` : "Resend link"}
            </Button>
            <p aria-live="polite" className="text-center text-caption text-fg-3">
              {resent ? "New link requested." : null}
            </p>
          </div>
          <button type="button" onClick={differentMethod} className={linkButton}>
            Use a different method
          </button>
        </>
      ) : null}

      {step === "passkey" || step === "security-key" ? (
        <>
          <div className="flex flex-col gap-3 text-center">
            <h1 className="text-h3 text-fg">{step === "passkey" ? "Waiting for your passkey" : "Waiting for your security key"}</h1>
            <p className="text-body text-fg-3">
              {step === "passkey"
                ? "Your browser will ask you to confirm with your fingerprint, face, screen lock or phone. Your biometrics never leave your device."
                : "Insert your security key and touch it when it flashes. For an NFC key, hold it against the back of your phone."}
            </p>
          </div>
          <div role="status" className="flex items-center justify-center gap-3 rounded-lg border border-border bg-surface p-4 text-body-sm text-fg-2">
            <Spinner size={20} />
            Waiting for your browser
          </div>
          <Button size="lg" className="w-full" autoFocus onClick={cancelPasskey}>
            Cancel
          </Button>
        </>
      ) : null}

      {step === "code" ? (
        <>
          <div className="flex flex-col gap-4">
            <h1 className="text-center text-h3 text-fg">Enter the code from your authenticator app</h1>
            {account}
          </div>
          <VerifyCode
            verify={(code) => signInWithCode("/auth/totp", code)}
            hint="Open your authenticator app and enter the 6-digit code shown for Halo."
          />
          <button type="button" onClick={differentMethod} className={linkButton}>
            Use a different method
          </button>
        </>
      ) : null}

      {step === "recovery" ? (
        <>
          <div className="flex flex-col gap-4">
            <h1 className="text-center text-h3 text-fg">Enter a recovery code</h1>
            {account}
          </div>
          <form noValidate className="flex flex-col gap-4" onSubmit={submitRecovery}>
            <Field label="Recovery code" hint="One of the codes you saved when you set up Halo. Each code works once." error={recoveryError}>
              {(field) => (
                <Input
                  {...field}
                  mono
                  autoFocus
                  autoComplete="one-time-code"
                  autoCapitalize="none"
                  spellCheck={false}
                  placeholder="abcd-efgh-ijkl-mnop"
                  value={recovery}
                  onChange={(event) => {
                    setRecovery(event.target.value);
                    if (recoveryError) setRecoveryError(null);
                  }}
                  className="h-12"
                />
              )}
            </Field>
            <Button type="submit" variant="primary" size="lg" className="w-full" loading={busy}>
              Sign in
            </Button>
          </form>
          <button type="button" onClick={differentMethod} className={linkButton}>
            Use a different method
          </button>
        </>
      ) : null}

      {step === "done" ? (
        <div role="status" className="flex flex-col items-center gap-3 text-center">
          <CircleCheck aria-hidden="true" size={24} strokeWidth={1.75} className="text-success" />
          <h1 className="text-h3 text-fg">Signed in</h1>
          <p className="text-body text-fg-3">{app ? `Returning you to ${app}…` : "Opening your Halo account…"}</p>
        </div>
      ) : null}

      {step === "expired" ? (
        <div className="flex flex-col items-center gap-3 text-center">
          <Clock aria-hidden="true" size={24} strokeWidth={1.75} className="text-fg-3" />
          <h1 className="text-h3 text-fg">This sign-in request expired</h1>
          <p className="text-body text-fg-3">Go back to the application and start again.</p>
        </div>
      ) : null}
    </div>
  );
}
