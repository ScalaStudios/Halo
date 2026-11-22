"use client";

import { Fingerprint } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Spinner } from "@/components/ui/spinner";
import { api, handleReauth } from "@/lib/api/client";
import { authMessages } from "@/lib/i18n/auth";
import { useLocale } from "@/lib/i18n/client";
import { getAssertion } from "@/lib/webauthn";
import { cancelled, describeFailure, type Ceremony } from "./passkey";
import { VerifyCode } from "./verify-code";

type Begun = { method: "totp" } | ({ method: "passkey" } & Ceremony);

function Status({ title, children }: { title: string; children?: string }) {
  return (
    <div role="status" className="flex items-start gap-4 rounded-lg border border-border bg-sunken p-4">
      <Spinner size={20} className="mt-px shrink-0" />
      <div className="flex flex-col gap-1">
        <p className="text-body-sm font-semibold text-fg">{title}</p>
        {children ? <p className="text-body-sm text-fg-3">{children}</p> : null}
      </div>
    </div>
  );
}

export function ReauthPrompt() {
  const messages = authMessages(useLocale());
  const t = messages.reauth;
  const settle = useRef<((confirmed: boolean) => void) | null>(null);
  const attempt = useRef<AbortController | null>(null);
  const [open, setOpen] = useState(false);
  const [begun, setBegun] = useState<Begun | null>(null);
  const [waiting, setWaiting] = useState(false);
  const [error, setError] = useState<string>();

  async function begin() {
    try {
      setBegun(await api<Begun>("/me/reauthenticate/begin", { method: "POST" }));
    } catch (failure) {
      setError(describeFailure(failure));
    }
  }

  useEffect(() => {
    handleReauth(
      () =>
        new Promise<boolean>((resolve) => {
          settle.current = resolve;
          setBegun(null);
          setError(undefined);
          setWaiting(false);
          setOpen(true);
          void begin();
        }),
    );
    return () => handleReauth(null);
  }, []);

  function close(confirmed: boolean) {
    attempt.current?.abort();
    attempt.current = null;
    settle.current?.(confirmed);
    settle.current = null;
    setOpen(false);
  }

  async function confirmWithPasskey() {
    if (begun?.method !== "passkey") return;
    const controller = new AbortController();
    attempt.current = controller;
    setError(undefined);
    setWaiting(true);
    try {
      const credential = await getAssertion(begun.options, controller.signal);
      await api("/me/reauthenticate", { body: { ceremony: begun.ceremony, credential } });
      close(true);
    } catch (failure) {
      if (controller.signal.aborted) return;
      setWaiting(false);
      if (cancelled(failure)) return;
      setError(describeFailure(failure));
      void begin();
    }
  }

  const passkey = begun?.method === "passkey";
  const totp = begun?.method === "totp";

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) close(false);
      }}
      title={t.title}
      description={passkey ? t.passkey : totp ? t.totp : t.generic}
      error={error}
      footer={
        <>
          <Button variant="secondary" onClick={() => close(false)}>
            {messages.cancel}
          </Button>
          {passkey && !waiting ? (
            <Button variant="primary" onClick={confirmWithPasskey}>
              <Fingerprint aria-hidden="true" size={16} strokeWidth={1.75} />
              {t.usePasskey}
            </Button>
          ) : null}
        </>
      }
    >
      {totp ? (
        <VerifyCode
          label={t.codeLabel}
          hint={t.codeHint}
          accepted={messages.codeAccepted}
          verify={(code) => api("/me/reauthenticate", { body: { code } }).then(() => close(true))}
        />
      ) : waiting ? (
        <Status title={t.waiting}>{t.waitingHint}</Status>
      ) : !begun && !error ? (
        <Status title={t.preparing} />
      ) : null}
    </Dialog>
  );
}
