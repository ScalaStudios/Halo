"use client";

import { useRouter } from "next/navigation";
import { Download, ListOrdered, Plus, RefreshCw, TriangleAlert } from "lucide-react";
import { useEffect, useId, useRef, useState, type FormEvent } from "react";
import { encode } from "uqr";
import { cancelled, describeFailure, deviceLabel, type Ceremony } from "@/components/auth/passkey";
import { VerifyCode } from "@/components/auth/verify-code";
import { LinkedAccounts } from "@/components/federation/linked-accounts";
import { MethodList } from "@/components/identity/method-list";
import { Button } from "@/components/ui/button";
import { CopyButton } from "@/components/ui/copy-button";
import { CopyField } from "@/components/ui/copy-field";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input } from "@/components/ui/input";
import { SectionTitle } from "@/components/ui/section-title";
import { Spinner } from "@/components/ui/spinner";
import { useToast } from "@/components/ui/toast";
import { api } from "@/lib/api/client";
import { formatDate, now } from "@/lib/format";
import { METHOD } from "@/lib/labels";
import type { AuthMethod } from "@/lib/types";
import { createCredential } from "@/lib/webauthn";

type Enrollable = "passkey" | "security-key" | "totp";
type TotpSetup = { ceremony: string; secret: string; uri: string };

const SECTIONS: { kind: Enrollable; title: string; description: string; action: string; empty: string }[] = [
  {
    kind: "passkey",
    title: "Passkeys",
    description: "Sign in with your fingerprint, face or screen lock. Passkeys can't be phished or reused on another site.",
    action: "Add passkey",
    empty: "Add a passkey to sign in without codes. It's the fastest and safest way into Halo.",
  },
  {
    kind: "security-key",
    title: "Security keys",
    description: "A hardware key such as a YubiKey. Works on any computer with a USB port or NFC reader.",
    action: "Add security key",
    empty: "A security key is a good backup if you lose your phone or laptop.",
  },
  {
    kind: "totp",
    title: "Authenticator apps",
    description: "Six-digit codes from an app like 1Password or Aegis. Useful as a backup, but codes can be phished.",
    action: "Set up authenticator app",
    empty: "No authenticator app set up. You don't need one while you have a passkey and a security key.",
  },
];

export function SecuritySettings({ methods, email }: { methods: AuthMethod[]; email: string }) {
  const router = useRouter();
  const toast = useToast();
  const [adding, setAdding] = useState<"passkey" | "security-key" | null>(null);
  const [totp, setTotp] = useState<TotpSetup | null>(null);
  const [codes, setCodes] = useState<{ codes: string[]; replaced: boolean } | null>(null);
  const [busy, setBusy] = useState<"totp" | "recovery" | null>(null);
  const recovery = methods.find((m) => m.kind === "recovery-codes");

  function fail(error: unknown) {
    toast({ title: "That didn't work", description: describeFailure(error), tone: "danger" });
  }

  function changed() {
    setAdding(null);
    setTotp(null);
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
      {SECTIONS.map((section) => {
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

      <section className="flex flex-col gap-4">
        <SectionTitle
          title="Recovery"
          description="One-time codes for when you can't use any passkey or security key. Keep them in a password manager or somewhere safe offline."
          action={
            <Button size="sm" loading={busy === "recovery"} onClick={generate}>
              <RefreshCw aria-hidden="true" size={16} strokeWidth={1.75} />
              Generate new codes
            </Button>
          }
        />
        <div className="flex items-center gap-4 rounded-lg border border-border bg-surface p-4 sm:px-6">
          <span className="grid size-10 shrink-0 place-items-center rounded-md border border-border bg-sunken text-fg-2">
            <ListOrdered aria-hidden="true" size={20} strokeWidth={1.75} />
          </span>
          <div className="flex min-w-0 flex-col gap-1">
            <p className="tnum text-body-sm font-semibold text-fg">{recovery ? (recovery.detail ?? "Recovery codes ready") : "No recovery codes yet"}</p>
            <p className="text-caption text-fg-3">
              {recovery ? `Generated ${formatDate(recovery.addedAt)} · Each code works once` : "Generate codes so you can get back in if you lose your devices."}
            </p>
          </div>
        </div>
      </section>

      <LinkedAccounts />

      {adding ? <EnrollDialog kind={adding} onClose={() => setAdding(null)} onAdded={changed} /> : null}
      {totp ? <AuthenticatorDialog setup={totp} email={email} onClose={() => setTotp(null)} onAdded={changed} /> : null}
      {codes ? <RecoveryCodesDialog codes={codes.codes} replaced={codes.replaced} email={email} onClose={() => setCodes(null)} /> : null}
    </div>
  );
}

function EnrollDialog({ kind, onClose, onAdded }: { kind: "passkey" | "security-key"; onClose: () => void; onAdded: () => void }) {
  const toast = useToast();
  const formId = useId();
  const cancelRef = useRef<HTMLButtonElement>(null);
  const attempt = useRef<AbortController | null>(null);
  const [name, setName] = useState(() => (kind === "passkey" ? deviceLabel() : "Security key"));
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
      toast({ title: `${METHOD[kind].label} added`, description: `You can now sign in to Halo with “${label}”.` });
      onAdded();
    } catch (error) {
      if (controller.signal.aborted) return;
      setWaiting(false);
      if (!cancelled(error)) toast({ title: `Couldn't add the ${METHOD[kind].label.toLowerCase()}`, description: describeFailure(error), tone: "danger" });
    }
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={passkey ? "Add a passkey" : "Add a security key"}
      description={
        passkey
          ? "Your device creates a passkey and keeps the private part to itself. Halo only stores a public key."
          : "Have your security key ready. You'll insert it and touch it to confirm."
      }
      footer={
        <>
          <Button ref={cancelRef} variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          {waiting ? null : (
            <Button variant="primary" type="submit" form={formId}>
              Continue
            </Button>
          )}
        </>
      }
    >
      {waiting ? (
        <div role="status" className="flex items-start gap-4 rounded-lg border border-border bg-sunken p-4">
          <Spinner size={20} className="mt-px shrink-0" />
          <div className="flex flex-col gap-1">
            <p className="text-body-sm font-semibold text-fg">Waiting for your device</p>
            <p className="text-body-sm text-fg-3">
              {passkey
                ? "Follow the prompt from your browser and confirm with your fingerprint, face or screen lock."
                : "Insert your security key and touch it when it flashes."}
            </p>
          </div>
        </div>
      ) : (
        <form id={formId} noValidate onSubmit={submit}>
          <Field
            label="Name"
            hint="Helps you tell your methods apart later, for example the browser and device."
            error={error ? "Give it a name you'll recognise, like “Firefox on Linux”." : undefined}
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

function AuthenticatorDialog({ setup, email, onClose, onAdded }: { setup: TotpSetup; email: string; onClose: () => void; onAdded: () => void }) {
  const toast = useToast();
  const qr = encode(setup.uri, { border: 4 });
  const modules = qr.data.flatMap((row, y) => row.map((dark, x) => (dark ? `M${x} ${y}h1v1h-1z` : ""))).join("");

  async function verify(code: string) {
    await api("/me/totp/confirm", { body: { ceremony: setup.ceremony, code } });
    toast({ title: "Authenticator app added", description: "Use it when you don't have a passkey or security key with you." });
    onAdded();
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title="Set up an authenticator app"
      description="Scan the code with 1Password, Aegis, Google Authenticator or any app that supports time-based codes."
      footer={
        <Button variant="secondary" onClick={onClose}>
          Cancel
        </Button>
      }
    >
      <div className="flex flex-col gap-8">
        <div className="flex flex-col items-center gap-6 sm:flex-row sm:items-start">
          <svg
            viewBox={`0 0 ${qr.size} ${qr.size}`}
            role="img"
            aria-label={`QR code for Halo, ${email}`}
            shapeRendering="crispEdges"
            className="size-40 shrink-0 rounded-md bg-white text-ink"
          >
            <path fill="currentColor" d={modules} />
          </svg>
          <CopyField label="Setup key" value={setup.secret} hint="Can't scan the code? Enter this key in the app instead." className="w-full min-w-0 flex-1" />
        </div>
        <div className="flex flex-col items-center gap-3 border-t border-border pt-6">
          <p className="text-label text-fg">Enter the 6-digit code from the app</p>
          <VerifyCode label="Code from your authenticator app" hint="This confirms the app is set up correctly." verify={verify} />
        </div>
      </div>
    </Dialog>
  );
}

function RecoveryCodesDialog({ codes, replaced, email, onClose }: { codes: string[]; replaced: boolean; email: string; onClose: () => void }) {
  function download() {
    const text = [`Halo recovery codes for ${email}`, `Generated ${formatDate(now().toISOString())}`, "Each code works once.", "", ...codes, ""].join("\n");
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
      title="Your new recovery codes"
      description="Each code signs you in once if you can't use any passkey or security key."
      footer={
        <Button variant="primary" onClick={onClose}>
          I saved my codes
        </Button>
      }
    >
      <div className="flex flex-col gap-6">
        <div className="flex items-start gap-3 rounded-md border border-warning/20 bg-warning-container p-4">
          <TriangleAlert aria-hidden="true" size={20} strokeWidth={1.75} className="mt-px shrink-0 text-warning" />
          <p className="text-body-sm text-fg-2">
            {replaced ? "Your old codes stopped working. " : ""}Save these now — Halo won&apos;t show them again.
          </p>
        </div>
        <ol aria-label="Recovery codes" className="grid grid-cols-2 gap-x-6 gap-y-3 rounded-lg border border-border bg-sunken p-6 font-mono text-code-sm text-fg">
          {codes.map((code) => (
            <li key={code}>{code}</li>
          ))}
        </ol>
        <div className="flex flex-wrap gap-2">
          <CopyButton value={codes.join("\n")} label="Copy all" />
          <Button size="sm" variant="quiet" onClick={download}>
            <Download aria-hidden="true" size={16} strokeWidth={1.75} />
            Download as .txt
          </Button>
        </div>
      </div>
    </Dialog>
  );
}
