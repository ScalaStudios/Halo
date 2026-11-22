"use client";

import { ChevronDown, CircleAlert, CircleCheck, CircleUserRound, Clock, Fingerprint, Monitor } from "lucide-react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useEffect, useId, useRef, useState, type FormEvent, type ReactNode } from "react";
import { Avatar } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/input";
import { PasswordInput } from "@/components/ui/password-input";
import { Spinner } from "@/components/ui/spinner";
import { api, ApiError } from "@/lib/api/client";
import { cn } from "@/lib/cn";
import type { SignInProvider } from "@/lib/federation-types";
import type { Locale } from "@/lib/i18n/locale";
import { authMessages, type SignInMessages } from "@/lib/i18n/auth";
import { METHOD } from "@/lib/labels";
import type { Challenge, Settings } from "@/lib/settings-types";
import { getAssertion } from "@/lib/webauthn";
import { ChallengeCheck } from "./challenge";
import { cancelled, describeFailure, usePasskeySupport, type Ceremony } from "./passkey";
import { federationProblem, ProviderButtons, providerStartUrl, useSignInProviders } from "./providers";
import { TermsStep, type SignedIn } from "./terms";
import { VerifyCode } from "./verify-code";

type Step = "loading" | "redirect" | "start" | "choose" | "passkey" | "security-key" | "code" | "recovery" | "access-pass" | "enroll" | "password" | "new-password" | "second-factor" | "inbox" | "terms" | "done" | "expired";
type Choice = "passkey" | "security-key" | "totp" | "password" | "recovery-codes" | "magic-link" | "access-pass" | "certificate";

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export const linkButton =
  "inline-flex min-h-12 items-center self-center rounded-sm text-body-sm text-link underline underline-offset-3 transition-colors duration-fast ease-brand hover:text-ember-hover";

export function emailProblem(value: string, messages = authMessages("en")) {
  if (!value.trim()) return messages.emailRequired;
  if (!EMAIL.test(value.trim())) return messages.emailInvalid;
  return null;
}

function choiceLabel(kind: Choice, emailed: "link" | "code", messages: SignInMessages) {
  if (kind === "magic-link") return messages.emailMe(emailed);
  return {
    passkey: messages.passkey,
    "security-key": messages.securityKey,
    totp: messages.authenticator,
    password: messages.password,
    "recovery-codes": messages.recoveryCode,
    "access-pass": messages.accessPass,
    certificate: messages.certificate,
  }[kind];
}

function expired(error: unknown) {
  return error instanceof ApiError && error.code === "ERR_AUTH_REQUEST_EXPIRED";
}

export function sameOrigin(path: string | undefined) {
  if (!path) return null;
  try {
    const url = new URL(path, window.location.origin);
    return url.origin === window.location.origin ? url.href : null;
  } catch {
    return null;
  }
}

export function SignInFlow({
  locale,
  authRequest,
  app,
  appLogo,
  expired: requestExpired,
  next,
  signedIn,
  allowed,
  emailSignIn,
  challenge,
  signUp,
  terms,
  loginHint,
  route,
}: {
  locale: Locale;
  authRequest?: string;
  app: string | null;
  appLogo: string | null;
  expired: boolean;
  next?: string;
  signedIn: boolean;
  allowed: string[];
  emailSignIn: Settings["emailSignIn"];
  challenge: Challenge | null;
  signUp: boolean;
  terms?: string;
  loginHint?: string;
  route: SignInProvider | null;
}) {
  const messages = authMessages(locale);
  const optionsId = useId();
  const emailRef = useRef<HTMLInputElement>(null);
  const attempt = useRef<AbortController | null>(null);
  const checked = useRef(false);
  const solver = useRef<{ resolve: (token: string) => void; reject: (error: unknown) => void } | null>(null);
  const supported = usePasskeySupport();
  const providers = useSignInProviders();
  const failure = useSearchParams().get("error");
  const [step, setStep] = useState<Step>(
    terms ? "terms" : requestExpired || failure === "ERR_AUTH_REQUEST_EXPIRED" ? "expired" : authRequest && signedIn && !failure ? "loading" : route && !failure ? "redirect" : "start",
  );
  const [termsCeremony, setTermsCeremony] = useState(terms ?? "");
  const [email, setEmail] = useState(loginHint && EMAIL.test(loginHint) ? loginHint : "");
  const [routed, setRouted] = useState(route?.name ?? "");
  const [methods, setMethods] = useState<Choice[]>([]);
  const [error, setError] = useState<ReactNode>(null);
  const [notice, setNotice] = useState<string | null>(() => federationProblem(failure));
  const [options, setOptions] = useState(false);
  const [focus, setFocus] = useState<"passkey" | "email" | null>(null);
  const [busy, setBusy] = useState(false);
  const [recovery, setRecovery] = useState("");
  const [recoveryError, setRecoveryError] = useState<ReactNode>(null);
  const [password, setPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [passwordError, setPasswordError] = useState<ReactNode>(null);
  const [sending, setSending] = useState(false);
  const [cooldown, setCooldown] = useState(0);
  const [resent, setResent] = useState(false);
  const [checking, setChecking] = useState(false);
  const [certifying, setCertifying] = useState(false);
  const [negotiating, setNegotiating] = useState(false);
  const windows = allowed.includes("kerberos");
  const untried = useRef(windows && step === "start" && !failure);
  const home = methods.length > 0 ? "choose" : "start";
  const emailed = emailSignIn === "link" ? "link" : "code";
  const changing = step === "new-password";
  const pass = step === "access-pass";

  useEffect(() => {
    if (cooldown <= 0) return;
    const timer = setTimeout(() => setCooldown((seconds) => seconds - 1), 1000);
    return () => clearTimeout(timer);
  }, [cooldown]);

  useEffect(() => () => settle(), [step]);

  useEffect(() => {
    if (step !== "loading" || checked.current) return;
    checked.current = true;
    api<SignedIn>("/auth/continue", { body: { authRequest } })
      .then(finish)
      .catch((error) => {
        if (expired(error)) return setStep("expired");
        const unauthenticated = error instanceof ApiError && error.status === 401;
        if (unauthenticated && route) return follow(route, true);
        if (!unauthenticated) setNotice(describeFailure(error));
        setStep("start");
      });
  }, [step, authRequest]);

  useEffect(() => {
    if (step === "redirect" && !checked.current && route) follow(route, true);
  }, [step, route]);

  useEffect(() => {
    if (step !== "start" || !untried.current) return;
    untried.current = false;
    void windowsSignIn(true);
  }, [step]);

  function follow(provider: SignInProvider, replace = false) {
    checked.current = true;
    setRouted(provider.name);
    setStep("redirect");
    const url = providerStartUrl(provider.id, authRequest, next);
    if (replace) window.location.replace(url);
    else window.location.assign(url);
  }

  function settle(token?: string) {
    const pending = solver.current;
    solver.current = null;
    setChecking(false);
    if (token) pending?.resolve(token);
    else pending?.reject(new DOMException(messages.completeCheck, "AbortError"));
  }

  function solve() {
    setChecking(true);
    return new Promise<string>((resolve, reject) => {
      solver.current = { resolve, reject };
    });
  }

  async function guarded<T>(call: (challenge?: string) => Promise<T>, always = false): Promise<T> {
    if (!challenge) return call();
    try {
      return await call(always ? await solve() : undefined);
    } catch (error) {
      if (!(error instanceof ApiError && error.code === "ERR_CHALLENGE_REQUIRED")) throw error;
      return call(await solve());
    }
  }

  function finish(result: SignedIn) {
    if (result.terms) {
      setTermsCeremony(result.terms);
      setStep("terms");
      return;
    }
    setStep("done");
    window.location.assign(authRequest ? result.redirect : (sameOrigin(next) ?? result.redirect));
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
      finish(await api<SignedIn>("/auth/passkey/finish", { body: { ceremony, credential, authRequest } }));
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
      await guarded((token) => api("/auth/magic-link", { body: { email: email.trim(), authRequest, challenge: token } }), true);
      setResent(again);
      setCooldown(30);
      setStep("inbox");
    } catch (error) {
      if (expired(error)) return setStep("expired");
      if (!cancelled(error)) setNotice(describeFailure(error));
    } finally {
      setSending(false);
    }
  }

  async function certificate() {
    setCertifying(true);
    setNotice(null);
    try {
      finish(await api<SignedIn>("/auth/certificate", { body: { authRequest } }));
    } catch (error) {
      if (expired(error)) return setStep("expired");
      setNotice(describeFailure(error));
    } finally {
      setCertifying(false);
    }
  }

  async function windowsSignIn(automatic = false) {
    setNegotiating(true);
    if (!automatic) setNotice(null);
    try {
      const result = await api<SignedIn | undefined>("/auth/kerberos", { body: { authRequest, automatic } });
      if (result) finish(result);
    } catch (error) {
      if (expired(error)) return setStep("expired");
      if (!automatic || !(error instanceof ApiError && error.code === "ERR_KERBEROS_NO_TICKET")) setNotice(describeFailure(error));
    } finally {
      setNegotiating(false);
    }
  }

  function pick(kind: Choice) {
    if (kind === "passkey" || kind === "security-key") return passkey(kind);
    if (kind === "certificate") return certificate();
    if (kind === "magic-link") return sendLink();
    if (kind === "password") {
      setPassword("");
      setNewPassword("");
      setPasswordError(null);
      return setStep("password");
    }
    setRecovery("");
    setRecoveryError(null);
    setStep(kind === "totp" ? "code" : kind === "access-pass" ? "access-pass" : "recovery");
  }

  function validEmail() {
    const problem = emailProblem(email, messages);
    setError(problem);
    if (problem) emailRef.current?.focus();
    else setEmail(email.trim());
    return !problem;
  }

  function other(kind: Choice) {
    if (kind === "security-key" || kind === "certificate" || validEmail()) pick(kind);
  }

  async function identify(event: FormEvent) {
    event.preventDefault();
    if (!validEmail()) return;
    setBusy(true);
    try {
      const result = await api<{ methods: Choice[]; provider?: SignInProvider }>("/auth/identify", { body: { email: email.trim() } });
      if (result.provider) return follow(result.provider);
      setMethods(result.methods.filter((kind) => supported || (kind !== "passkey" && kind !== "security-key" && kind !== "access-pass")));
      setNotice(null);
      setStep("choose");
    } catch (error) {
      setError(<span lang="en">{describeFailure(error)}</span>);
    } finally {
      setBusy(false);
    }
  }

  async function signInWithCode(path: "/auth/totp" | "/auth/recovery" | "/auth/magic-link/code", code: string) {
    try {
      finish(await guarded((token) => api<SignedIn>(path, { body: { email, code, authRequest, challenge: token } })));
    } catch (error) {
      if (expired(error)) setStep("expired");
      throw error;
    }
  }

  async function redeemPass(code: string) {
    const { redirect } = await guarded((token) => api<SignedIn>("/auth/access-pass", { body: { email, code, challenge: token } }));
    setStep("enroll");
    window.location.assign(redirect);
  }

  async function submitRecovery(event: FormEvent) {
    event.preventDefault();
    if (!recovery.trim()) {
      setRecoveryError(pass ? messages.accessPassRequired : messages.recoveryRequired);
      return;
    }
    setBusy(true);
    try {
      if (pass) await redeemPass(recovery.trim());
      else await signInWithCode("/auth/recovery", recovery.trim());
    } catch (error) {
      setRecoveryError(<span lang="en">{describeFailure(error)}</span>);
    } finally {
      setBusy(false);
    }
  }

  async function signInWithPassword(code = "") {
    try {
      finish(await guarded((token) => api<SignedIn>("/auth/password", { body: { email, password, newPassword, code, authRequest, challenge: token } })));
    } catch (error) {
      if (expired(error)) return setStep("expired");
      if (!(error instanceof ApiError) || !["ERR_PASSWORD_CHANGE_REQUIRED", "ERR_SECOND_FACTOR_REQUIRED"].includes(error.code)) throw error;
      setPasswordError(null);
      setStep(error.code === "ERR_PASSWORD_CHANGE_REQUIRED" ? "new-password" : "second-factor");
    }
  }

  async function submitPassword(event: FormEvent) {
    event.preventDefault();
    if (!(changing ? newPassword : password)) {
      setPasswordError(changing ? messages.newPasswordRequired : messages.passwordRequired);
      return;
    }
    setBusy(true);
    try {
      await signInWithPassword();
    } catch (error) {
      setPasswordError(<span lang="en">{describeFailure(error)}</span>);
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
        {messages.notYou}
      </button>
    </div>
  );

  const alert = notice ? (
    <div role="alert" lang="en" className="flex items-start gap-3 rounded-md border border-danger/20 bg-danger-container p-4">
      <CircleAlert aria-hidden="true" size={20} strokeWidth={1.75} className="mt-px shrink-0 text-danger" />
      <p className="text-body-sm text-fg-2">{notice}</p>
    </div>
  ) : null;

  const passkeys = supported && allowed.includes("passkey");
  const permitted = (kind: Choice) => kind === "access-pass" || allowed.includes(kind);
  const otherKinds: Choice[] = (supported ? (["security-key", "certificate", "totp", "password", "recovery-codes", "magic-link"] as Choice[]) : (["certificate", "totp", "password", "recovery-codes", "magic-link"] as Choice[])).filter(permitted);
  const choices: Choice[] = ([...methods, "magic-link"] as Choice[]).filter(permitted);

  return (
    <div key={step} lang={locale} className="flex animate-enter flex-col gap-8">
      {step === "loading" || step === "enroll" || step === "redirect" ? (
        <div role="status" className="flex items-center justify-center gap-3 text-body-sm text-fg-3">
          <Spinner size={20} />
          {step === "enroll" ? messages.passkeySetup : step === "redirect" ? messages.continuingTo(routed) : app ? messages.continuingTo(app) : messages.checkingSession}
        </div>
      ) : null}

      {step === "start" ? (
        <>
          <div className="flex flex-col gap-3 text-center">
            {app ? <Avatar name={app} src={appLogo} size="lg" square className="self-center" /> : null}
            <h1 className="text-h3 text-fg">{app ? messages.welcomeTo(app) : messages.welcome}</h1>
            {supported ? null : <p className="text-body-sm text-fg-3">{messages.passkeyUnavailable}</p>}
          </div>
          {alert}
          <div className="flex flex-col gap-6">
            {passkeys || windows || providers.length > 0 ? (
              <>
                <div className="flex flex-col gap-2">
                  {passkeys ? (
                    <Button variant="primary" size="lg" className="w-full" autoFocus={focus === "passkey"} onClick={() => passkey("passkey")}>
                      <Fingerprint aria-hidden="true" size={20} strokeWidth={1.75} />
                      {messages.usePasskey}
                    </Button>
                  ) : null}
                  {windows ? (
                    <Button variant={passkeys ? "secondary" : "primary"} size="lg" className="w-full" loading={negotiating} onClick={() => windowsSignIn()}>
                      <Monitor aria-hidden="true" size={20} strokeWidth={1.75} className={passkeys ? "text-fg-3" : undefined} />
                      {messages.useWindows}
                    </Button>
                  ) : null}
                  <ProviderButtons providers={providers} authRequest={authRequest} next={next} label={messages.useProvider} />
                </div>
                <div className="flex items-center gap-3 text-caption text-fg-3">
                  <span aria-hidden="true" className="h-px flex-1 bg-border" />
                  {messages.or}
                  <span aria-hidden="true" className="h-px flex-1 bg-border" />
                </div>
              </>
            ) : null}
            <form noValidate className="flex flex-col gap-4" onSubmit={identify}>
              <Field label={messages.email} error={error}>
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
                {messages.continue}
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
                {messages.options}
                <ChevronDown aria-hidden="true" size={16} strokeWidth={1.75} className={cn("transition-transform duration-fast ease-brand", options && "rotate-180")} />
              </Button>
              {options ? (
                <ul id={optionsId} className="flex animate-enter flex-col gap-2">
                  {otherKinds.map((kind) => {
                    const Icon = METHOD[kind].icon;
                    return (
                      <li key={kind}>
                        <Button size="lg" className="w-full" loading={(kind === "magic-link" && sending) || (kind === "certificate" && certifying)} onClick={() => other(kind)}>
                          <Icon aria-hidden="true" size={20} strokeWidth={1.75} className="text-fg-3" />
                          {choiceLabel(kind, emailed, messages)}
                        </Button>
                      </li>
                    );
                  })}
                </ul>
              ) : null}
            </div>
            {signUp ? (
              <Link href="/sign-up" className={linkButton}>
                {messages.signUp}
              </Link>
            ) : null}
          </div>
        </>
      ) : null}

      {step === "choose" ? (
        <>
          <div className="flex flex-col gap-4">
            <h1 className="text-center text-h3 text-fg">{messages.choose}</h1>
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
                    {choiceLabel(kind, emailed, messages)}
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
            <h1 className="text-center text-h3 text-fg">{messages.inbox}</h1>
            {account}
          </div>
          {alert}
          <p className="text-center text-body text-fg-3">
            {emailSignIn === "both"
              ? messages.sentBoth
              : messages.sent(emailed)}
            {app && emailSignIn !== "code" ? messages.openLink(app) : null}
          </p>
          {emailSignIn === "link" ? null : (
            <VerifyCode verify={(code) => signInWithCode("/auth/magic-link/code", code)} hint={messages.emailCodeHint} label={messages.codeLabel} accepted={messages.codeAccepted} />
          )}
          <div className="flex flex-col gap-2">
            <Button size="lg" className="w-full" loading={sending} disabled={cooldown > 0} onClick={sendLink}>
              {messages.resend(emailed, cooldown)}
            </Button>
            <p aria-live="polite" className="text-center text-caption text-fg-3">
              {resent ? messages.resent(emailed) : null}
            </p>
          </div>
          <button type="button" onClick={differentMethod} className={linkButton}>
            {messages.differentMethod}
          </button>
        </>
      ) : null}

      {step === "passkey" || step === "security-key" ? (
        <>
          <div className="flex flex-col gap-3 text-center">
            <h1 className="text-h3 text-fg">{step === "passkey" ? messages.passkeyWaiting : messages.securityKeyWaiting}</h1>
            <p className="text-body text-fg-3">
              {step === "passkey"
                ? messages.passkeyHint
                : messages.securityKeyHint}
            </p>
          </div>
          <div role="status" className="flex items-center justify-center gap-3 rounded-lg border border-border bg-surface p-4 text-body-sm text-fg-2">
            <Spinner size={20} />
            {messages.browserWaiting}
          </div>
          <Button size="lg" className="w-full" autoFocus onClick={cancelPasskey}>
            {messages.cancel}
          </Button>
        </>
      ) : null}

      {step === "code" ? (
        <>
          <div className="flex flex-col gap-4">
            <h1 className="text-center text-h3 text-fg">{messages.authenticatorCode}</h1>
            {account}
          </div>
          <VerifyCode
            verify={(code) => signInWithCode("/auth/totp", code)}
            hint={messages.authenticatorHint} label={messages.codeLabel} accepted={messages.codeAccepted}
          />
          <button type="button" onClick={differentMethod} className={linkButton}>
            {messages.differentMethod}
          </button>
        </>
      ) : null}

      {step === "recovery" || pass ? (
        <>
          <div className="flex flex-col gap-4">
            <h1 className="text-center text-h3 text-fg">{pass ? messages.accessPassTitle : messages.recoveryTitle}</h1>
            {account}
          </div>
          <form noValidate className="flex flex-col gap-4" onSubmit={submitRecovery}>
            <Field
              label={pass ? messages.accessPass : messages.recoveryCode}
              hint={pass ? messages.accessPassHint : messages.recoveryHint}
              error={recoveryError}
            >
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
              {pass ? messages.continue : messages.signIn}
            </Button>
          </form>
          <button type="button" onClick={differentMethod} className={linkButton}>
            {messages.differentMethod}
          </button>
        </>
      ) : null}

      {step === "password" || step === "new-password" ? (
        <>
          <div className="flex flex-col gap-4">
            <h1 className="text-center text-h3 text-fg">{changing ? messages.newPasswordTitle : messages.passwordTitle}</h1>
            {account}
          </div>
          {changing ? (
            <p className="text-center text-body text-fg-3">{messages.temporaryPasswordHint}</p>
          ) : null}
          <form noValidate className="flex flex-col gap-4" onSubmit={submitPassword}>
            <input type="email" name="email" autoComplete="username" value={email} readOnly hidden />
            <Field
              label={changing ? messages.newPassword : messages.password}
              hint={changing ? "At least 15 characters. A few unrelated words make a password that is long and easy to remember." : undefined}
              error={passwordError}
            >
              {(field) => (
                <PasswordInput
                  {...field}
                  autoFocus
                  visibilityLabels={{ show: messages.showPassword, hide: messages.hidePassword }}
                  name={changing ? "new-password" : "password"}
                  autoComplete={changing ? "new-password" : "current-password"}
                  value={changing ? newPassword : password}
                  onChange={(event) => {
                    (changing ? setNewPassword : setPassword)(event.target.value);
                    if (passwordError) setPasswordError(null);
                  }}
                  className="h-12"
                />
              )}
            </Field>
            <Button type="submit" variant="primary" size="lg" className="w-full" loading={busy}>
              {changing ? messages.savePassword : messages.signIn}
            </Button>
          </form>
          <button type="button" onClick={differentMethod} className={linkButton}>
            {messages.differentMethod}
          </button>
        </>
      ) : null}

      {step === "second-factor" ? (
        <>
          <div className="flex flex-col gap-4">
            <h1 className="text-center text-h3 text-fg">{messages.authenticatorCode}</h1>
            {account}
          </div>
          <VerifyCode
            verify={signInWithPassword}
            hint={messages.secondFactorHint} label={messages.codeLabel} accepted={messages.codeAccepted}
          />
          <button type="button" onClick={differentMethod} className={linkButton}>
            {messages.differentMethod}
          </button>
        </>
      ) : null}

      {step === "terms" ? <div lang="en"><TermsStep key={termsCeremony} ceremony={termsCeremony} app={app} onAccepted={finish} onLeave={forget} /></div> : null}

      {step === "done" ? (
        <div role="status" className="flex flex-col items-center gap-3 text-center">
          <CircleCheck aria-hidden="true" size={24} strokeWidth={1.75} className="text-success" />
          <h1 className="text-h3 text-fg">{messages.signedIn}</h1>
          <p className="text-body text-fg-3">{app ? messages.returning(app) : messages.openingAccount}</p>
        </div>
      ) : null}

      {step === "expired" ? (
        <div className="flex flex-col items-center gap-3 text-center">
          <Clock aria-hidden="true" size={24} strokeWidth={1.75} className="text-fg-3" />
          <h1 className="text-h3 text-fg">{messages.expired}</h1>
          <p className="text-body text-fg-3">{messages.expiredHint}</p>
        </div>
      ) : null}

      {checking && challenge ? <ChallengeCheck locale={locale} challenge={challenge} onToken={settle} onCancel={() => settle()} /> : null}
    </div>
  );
}
