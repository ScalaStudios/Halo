"use client";

import { Fingerprint } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Spinner } from "@/components/ui/spinner";
import { api, handleReauth } from "@/lib/api/client";
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
      title="Confirm it's you"
      description={
        passkey
          ? "Sensitive changes need a recent sign-in. Use your passkey or security key to continue."
          : totp
            ? "Sensitive changes need a recent sign-in. Enter the code from your authenticator app to continue."
            : "Sensitive changes need a recent sign-in."
      }
      error={error}
      footer={
        <>
          <Button variant="secondary" onClick={() => close(false)}>
            Cancel
          </Button>
          {passkey && !waiting ? (
            <Button variant="primary" onClick={confirmWithPasskey}>
              <Fingerprint aria-hidden="true" size={16} strokeWidth={1.75} />
              Use passkey
            </Button>
          ) : null}
        </>
      }
    >
      {totp ? (
        <VerifyCode
          label="Authenticator app code"
          hint="Enter the six-digit code from your authenticator app."
          verify={(code) => api("/me/reauthenticate", { body: { code } }).then(() => close(true))}
        />
      ) : waiting ? (
        <Status title="Waiting for your device">Follow the prompt from your browser and confirm with your fingerprint, face, screen lock or security key.</Status>
      ) : !begun && !error ? (
        <Status title="Getting ready" />
      ) : null}
    </Dialog>
  );
}
