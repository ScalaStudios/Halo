"use client";

import { useRouter } from "next/navigation";
import { Download, ListOrdered, Plus, RectangleEllipsis, RefreshCw, ShieldCheck, TriangleAlert } from "lucide-react";
import { useEffect, useId, useRef, useState, type FormEvent } from "react";
import { encode } from "uqr";
import { cancelled, describeFailure, deviceLabel, type Ceremony } from "@/components/auth/passkey";
import { VerifyCode } from "@/components/auth/verify-code";
import { LinkedAccounts } from "@/components/federation/linked-accounts";
import { MethodList } from "@/components/identity/method-list";
import { joinList } from "@/components/policy/shared";
import { Button } from "@/components/ui/button";
import { CopyButton } from "@/components/ui/copy-button";
import { CopyField } from "@/components/ui/copy-field";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input } from "@/components/ui/input";
import { PasswordInput } from "@/components/ui/password-input";
import { SectionTitle } from "@/components/ui/section-title";
import { Spinner } from "@/components/ui/spinner";
import { useToast } from "@/components/ui/toast";
import { api } from "@/lib/api/client";
import { formatDate, now } from "@/lib/format";
import { accountMessages } from "@/lib/i18n/account";
import { authMessages } from "@/lib/i18n/auth";
import { useLocale } from "@/lib/i18n/client";
import { monitoringMessages } from "@/lib/i18n/console/monitoring";
import { METHOD } from "@/lib/labels";
import type { AuthMethod } from "@/lib/types";
import { createCredential } from "@/lib/webauthn";

type Enrollable = "passkey" | "security-key" | "totp";
type TotpSetup = { ceremony: string; secret: string; uri: string };
export type PasskeyRules = { groups: string[]; deviceBound: boolean; attestation: boolean; approvedModels: boolean; minimum: number; keys: number };

const SECTIONS: Enrollable[] = ["passkey", "security-key", "totp"];

export function SecuritySettings({
  methods,
  email,
  password,
  rules,
}: {
  methods: AuthMethod[];
  email: string;
  password: { allowed: boolean; secondFactor: boolean };
  rules: PasskeyRules;
}) {
  const router = useRouter();
  const toast = useToast();
  const locale = useLocale();
  const copy = accountMessages(locale);
  const t = copy.security;
  const [adding, setAdding] = useState<"passkey" | "security-key" | null>(null);
  const [totp, setTotp] = useState<TotpSetup | null>(null);
  const [codes, setCodes] = useState<{ codes: string[]; replaced: boolean } | null>(null);
  const [busy, setBusy] = useState<"totp" | "recovery" | null>(null);
  const [changingPassword, setChangingPassword] = useState(false);
  const recovery = methods.find((m) => m.kind === "recovery-codes");
  const current = methods.find((m) => m.kind === "password");
  const passwordNote = !password.allowed ? t.passwordOff : password.secondFactor && !methods.some((m) => m.kind === "totp") ? t.passwordNeedsCode : null;

  function fail(error: unknown) {
    toast({ title: copy.sessions.failed, description: describeFailure(error), tone: "danger" });
  }

  function changed() {
    setAdding(null);
    setTotp(null);
    setChangingPassword(false);
    router.refresh();
  }

  async function startTotp() {
    setBusy("totp");
    try {
      setTotp(await api<TotpSetup>("/me/totp/begin", { method: "POST" }));
    } catch (error) {
      fail(error);
    } finally {
      setBusy(null);
    }
  }

  async function generate() {
    setBusy("recovery");
    try {
      const result = await api<{ codes: string[] }>("/me/recovery-codes", { method: "POST" });
      setCodes({ codes: result.codes, replaced: recovery !== undefined });
      router.refresh();
    } catch (error) {
      fail(error);
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="flex flex-col gap-12">
      {rules.groups.length > 0 ? <PasskeyRulesNote rules={rules} /> : null}
      {SECTIONS.map((kind) => {
        const section = { kind, ...t.sections[kind] };
        const list = methods.filter((m) => m.kind === section.kind);
        return (
          <section key={section.kind} className="flex flex-col gap-4">
            <SectionTitle
              title={section.title}
              description={section.description}
              action={
                <Button
                  size="sm"
                  loading={section.kind === "totp" && busy === "totp"}
                  onClick={() => (section.kind === "totp" ? startTotp() : setAdding(section.kind))}
                >
                  <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
                  {section.action}
                </Button>
              }
            />
            {list.length > 0 ? (
              <MethodList
                key={list.map((m) => m.id).join()}
                methods={list}
                strongElsewhere={methods.filter((m) => m.kind !== section.kind && METHOD[m.kind].phishingResistant).length}
              />
            ) : (
              <p className="rounded-lg border border-border px-6 py-4 text-body-sm text-fg-3">{section.empty}</p>
            )}
          </section>
        );
      })}

      {password.allowed || current ? (
        <section className="flex flex-col gap-4">
          <SectionTitle
            title={t.password}
            description={t.passwordHint}
            action={
              password.allowed ? (
                <Button size="sm" onClick={() => setChangingPassword(true)}>
                  <RectangleEllipsis aria-hidden="true" size={16} strokeWidth={1.75} />
                  {current ? t.changePassword : t.setPassword}
                </Button>
              ) : undefined
            }
          />
          {current ? (
            <MethodList key={current.addedAt} methods={[current]} />
          ) : (
            <p className="rounded-lg border border-border px-6 py-4 text-body-sm text-fg-3">{t.noPassword}</p>
          )}
          {passwordNote ? (
            <p className="flex items-start gap-2 text-body-sm text-warning">
              <TriangleAlert aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0" />
              {passwordNote}
            </p>
          ) : null}
        </section>
      ) : null}

      <section className="flex flex-col gap-4">
        <SectionTitle
          title={t.recovery}
          description={t.recoveryHint}
          action={
            <Button size="sm" loading={busy === "recovery"} onClick={generate}>
              <RefreshCw aria-hidden="true" size={16} strokeWidth={1.75} />
              {t.generate}
            </Button>
          }
        />
        <div className="flex items-center gap-4 rounded-lg border border-border bg-surface p-4 sm:px-6">
          <span className="grid size-10 shrink-0 place-items-center rounded-md border border-border bg-sunken text-fg-2">
            <ListOrdered aria-hidden="true" size={20} strokeWidth={1.75} />
          </span>
          <div className="flex min-w-0 flex-col gap-1">
            <p className="tnum text-body-sm font-semibold text-fg">{recovery ? (recovery.detail ?? t.recoveryReady) : t.noRecovery}</p>
            <p className="text-caption text-fg-3">
              {recovery ? t.generated(formatDate(recovery.addedAt, locale)) : t.generateHint}
            </p>
          </div>
        </div>
      </section>

      <LinkedAccounts />

      {adding ? <EnrollDialog kind={adding} onClose={() => setAdding(null)} onAdded={changed} /> : null}
      {totp ? <AuthenticatorDialog setup={totp} email={email} onClose={() => setTotp(null)} onAdded={changed} /> : null}
      {changingPassword ? <PasswordDialog email={email} replacing={current !== undefined} onClose={() => setChangingPassword(false)} onSaved={changed} /> : null}
      {codes ? <RecoveryCodesDialog codes={codes.codes} replaced={codes.replaced} email={email} onClose={() => setCodes(null)} /> : null}
    </div>
  );
}

function PasskeyRulesNote({ rules }: { rules: PasskeyRules }) {
  const titleId = useId();
  const t = accountMessages(useLocale()).security;
  const needs = [rules.deviceBound && t.needs.deviceBound, rules.attestation && t.needs.attestation, rules.approvedModels && t.needs.approvedModels].filter(
    (need): need is string => typeof need === "string",
  );
  const short = rules.minimum - rules.keys;
  return (
    <section aria-labelledby={titleId} className="flex items-start gap-4 rounded-lg border border-border bg-surface p-4 sm:px-6">
      <span className="grid size-10 shrink-0 place-items-center rounded-md border border-border bg-sunken text-fg-2">
        <ShieldCheck aria-hidden="true" size={20} strokeWidth={1.75} />
      </span>
      <div className="flex min-w-0 flex-col gap-1">
        <h2 id={titleId} className="text-body-sm font-semibold text-fg">
          {t.rulesFor(joinList(rules.groups, t.and))}
        </h2>
        {needs.length ? <p className="text-body-sm text-fg-3">{t.mustMeet(joinList(needs, t.and))}</p> : null}
        {rules.minimum > 0 ? (
          <p className="tnum text-body-sm text-fg-3">{t.keepAtLeast(rules.minimum)}</p>
        ) : null}
        {short > 0 ? (
          <p className="flex items-start gap-2 text-body-sm text-warning">
            <TriangleAlert aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0" />
            <span className="tnum">{t.short(rules.keys, short)}</span>
          </p>
        ) : null}
      </div>
    </section>
  );
}

function EnrollDialog({ kind, onClose, onAdded }: { kind: "passkey" | "security-key"; onClose: () => void; onAdded: () => void }) {
  const toast = useToast();
  const locale = useLocale();
  const copy = accountMessages(locale);
  const t = copy.security;
  const method = monitoringMessages[locale].methods[kind];
  const formId = useId();
  const cancelRef = useRef<HTMLButtonElement>(null);
  const attempt = useRef<AbortController | null>(null);
  const [name, setName] = useState(() => (kind === "passkey" ? deviceLabel(authMessages(locale)) : method));
  const [error, setError] = useState(false);
  const [waiting, setWaiting] = useState(false);
  const passkey = kind === "passkey";

  useEffect(() => () => attempt.current?.abort(), []);

  async function submit(event: FormEvent) {
    event.preventDefault();
    const label = name.trim();
    if (!label) {
      setError(true);
      return;
    }
    const controller = new AbortController();
    attempt.current = controller;
    setWaiting(true);
    cancelRef.current?.focus();
    try {
      const { ceremony, options } = await api<Ceremony>("/me/passkeys/begin", { body: { kind } });
      const credential = await createCredential(options, controller.signal);
      await api("/me/passkeys/finish", { body: { ceremony, credential, label } });
      toast({ title: t.added(method), description: t.addedHint(label) });
      onAdded();
    } catch (error) {
      if (controller.signal.aborted) return;
      setWaiting(false);
      if (!cancelled(error)) toast({ title: t.addFailed(method), description: describeFailure(error), tone: "danger" });
    }
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={passkey ? t.addPasskey : t.addSecurityKey}
      description={passkey ? t.passkeyIntro : t.securityKeyIntro}
      footer={
        <>
          <Button ref={cancelRef} variant="secondary" onClick={onClose}>
            {copy.cancel}
          </Button>
          {waiting ? null : (
            <Button variant="primary" type="submit" form={formId}>
              {copy.continue}
            </Button>
          )}
        </>
      }
    >
      {waiting ? (
        <div role="status" className="flex items-start gap-4 rounded-lg border border-border bg-sunken p-4">
          <Spinner size={20} className="mt-px shrink-0" />
          <div className="flex flex-col gap-1">
            <p className="text-body-sm font-semibold text-fg">{t.waiting}</p>
            <p className="text-body-sm text-fg-3">{passkey ? t.passkeyWaiting : t.securityKeyWaiting}</p>
          </div>
        </div>
      ) : (
        <form id={formId} noValidate onSubmit={submit}>
          <Field
            label={t.name}
            hint={t.nameHint}
            error={error ? t.nameRequired : undefined}
          >
            {(field) => (
              <Input
                {...field}
                autoFocus
                value={name}
                onChange={(event) => {
                  setName(event.target.value);
                  setError(false);
                }}
              />
            )}
          </Field>
        </form>
      )}
    </Dialog>
  );
}

function PasswordDialog({ email, replacing, onClose, onSaved }: { email: string; replacing: boolean; onClose: () => void; onSaved: () => void }) {
  const toast = useToast();
  const copy = accountMessages(useLocale());
  const t = copy.security;
  const formId = useId();
  const [value, setValue] = useState("");
  const [problem, setProblem] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!value) {
      setProblem(t.passwordRequired);
      return;
    }
    setSaving(true);
    try {
      await api("/me/password", { method: "PUT", body: { password: value } });
      toast({ title: replacing ? t.passwordChanged : t.passwordSet, description: t.passwordSavedHint });
      onSaved();
    } catch (error) {
      setProblem(describeFailure(error));
      setSaving(false);
    }
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={replacing ? t.changeTitle : t.setTitle}
      description={t.passwordDialogHint}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {copy.cancel}
          </Button>
          <Button variant="primary" type="submit" form={formId} loading={saving}>
            {t.savePassword}
          </Button>
        </>
      }
    >
      <form id={formId} noValidate onSubmit={submit}>
        <input type="email" name="email" autoComplete="username" value={email} readOnly hidden />
        <Field label={t.newPassword} error={problem}>
          {(field) => (
            <PasswordInput
              {...field}
              autoFocus
              name="new-password"
              autoComplete="new-password"
              value={value}
              onChange={(event) => {
                setValue(event.target.value);
                setProblem(null);
              }}
            />
          )}
        </Field>
      </form>
    </Dialog>
  );
}

function AuthenticatorDialog({ setup, email, onClose, onAdded }: { setup: TotpSetup; email: string; onClose: () => void; onAdded: () => void }) {
  const toast = useToast();
  const locale = useLocale();
  const copy = accountMessages(locale);
  const t = copy.security;
  const qr = encode(setup.uri, { border: 4 });
  const modules = qr.data.flatMap((row, y) => row.map((dark, x) => (dark ? `M${x} ${y}h1v1h-1z` : ""))).join("");

  async function verify(code: string) {
    await api("/me/totp/confirm", { body: { ceremony: setup.ceremony, code } });
    toast({ title: t.authenticatorAdded, description: t.authenticatorAddedHint });
    onAdded();
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={t.authenticatorTitle}
      description={t.authenticatorIntro}
      footer={
        <Button variant="secondary" onClick={onClose}>
          {copy.cancel}
        </Button>
      }
    >
      <div className="flex flex-col gap-8">
        <div className="flex flex-col items-center gap-6 sm:flex-row sm:items-start">
          <svg
            viewBox={`0 0 ${qr.size} ${qr.size}`}
            role="img"
            aria-label={t.qrCode(email)}
            shapeRendering="crispEdges"
            className="size-40 shrink-0 rounded-md bg-white text-ink"
          >
            <path fill="currentColor" d={modules} />
          </svg>
          <CopyField label={t.setupKey} value={setup.secret} hint={t.setupKeyHint} className="w-full min-w-0 flex-1" />
        </div>
        <div className="flex flex-col items-center gap-3 border-t border-border pt-6">
          <p className="text-label text-fg">{t.enterCode}</p>
          <VerifyCode label={t.codeLabel} hint={t.codeHint} accepted={authMessages(locale).codeAccepted} verify={verify} />
        </div>
      </div>
    </Dialog>
  );
}

function RecoveryCodesDialog({ codes, replaced, email, onClose }: { codes: string[]; replaced: boolean; email: string; onClose: () => void }) {
  const locale = useLocale();
  const t = accountMessages(locale).security;

  function download() {
    const text = [...t.codesFile(email, formatDate(now().toISOString(), locale)), "", ...codes, ""].join("\n");
    const url = URL.createObjectURL(new Blob([text], { type: "text/plain" }));
    const link = document.createElement("a");
    link.href = url;
    link.download = "halo-recovery-codes.txt";
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={t.codesTitle}
      description={t.codesIntro}
      footer={
        <Button variant="primary" onClick={onClose}>
          {t.codesSaved}
        </Button>
      }
    >
      <div className="flex flex-col gap-6">
        <div className="flex items-start gap-3 rounded-md border border-warning/20 bg-warning-container p-4">
          <TriangleAlert aria-hidden="true" size={20} strokeWidth={1.75} className="mt-px shrink-0 text-warning" />
          <p className="text-body-sm text-fg-2">
            {replaced ? t.oldCodesStopped : ""}
            {t.saveNow}
          </p>
        </div>
        <ol aria-label={t.codes} className="grid grid-cols-2 gap-x-6 gap-y-3 rounded-lg border border-border bg-sunken p-6 font-mono text-code-sm text-fg">
          {codes.map((code) => (
            <li key={code}>{code}</li>
          ))}
        </ol>
        <div className="flex flex-wrap gap-2">
          <CopyButton value={codes.join("\n")} label={t.copyAll} />
          <Button size="sm" variant="quiet" onClick={download}>
            <Download aria-hidden="true" size={16} strokeWidth={1.75} />
            {t.downloadTxt}
          </Button>
        </div>
      </div>
    </Dialog>
  );
}
