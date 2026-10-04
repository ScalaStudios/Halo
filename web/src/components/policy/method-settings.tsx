"use client";

import { TriangleAlert } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { Checkbox } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { api } from "@/lib/api/client";
import { pluralize } from "@/lib/format";
import { METHOD } from "@/lib/labels";
import type { MethodSetting } from "@/lib/policy-types";
import type { MethodKind } from "@/lib/types";
import { METHOD_PLURAL } from "./shared";

const DESCRIPTION: Record<MethodKind, string> = {
  passkey: "Face ID, Touch ID, Windows Hello or a phone screen lock. Bound to this domain, so a fake sign-in page can't use them.",
  "security-key": "Hardware keys such as a YubiKey or Titan Key. Phishing-resistant and portable between computers.",
  totp: "Six-digit codes from apps such as 1Password, Google Authenticator or Aegis. A convincing fake sign-in page can capture them.",
  "magic-link": "A single-use sign-in link sent by email. Only as safe as the mailbox it goes to.",
  "recovery-codes": "Single-use backup codes for people who lost their other methods.",
  federated: "Sign-in through Google, Microsoft Entra, GitHub or another OpenID Connect provider. Only as strong as that provider's sign-in.",
};

const HAVE: Record<MethodKind, string> = {
  passkey: "a passkey",
  "security-key": "a security key",
  totp: "an authenticator app",
  "magic-link": "",
  "recovery-codes": "recovery codes",
  federated: "a linked identity provider account",
};

function people(n: number) {
  return pluralize(n, "active person", "active people");
}

function usage(m: MethodSetting) {
  if (m.method === "magic-link") return `${people(m.users)} ${m.users === 1 ? "has" : "have"} no passkey, security key or authenticator app.`;
  return `${people(m.users)} ${m.users === 1 ? "has" : "have"} ${HAVE[m.method]}.`;
}

function warning(m: MethodSetting) {
  if (m.method === "magic-link") return `${pluralize(m.exclusive, "person relies", "people rely")} on magic links because nothing else they enrolled is turned on.`;
  return `${pluralize(m.exclusive, "person has", "people have")} no other passkey, security key or authenticator app that is turned on.`;
}

export function MethodSettings({ methods }: { methods: MethodSetting[] }) {
  const { pending, run, toast } = useMutation();
  const [confirming, setConfirming] = useState<MethodSetting | null>(null);
  const magicLinks = methods.find((m) => m.method === "magic-link")?.enabled ?? false;

  function apply(m: MethodSetting, enabled: boolean) {
    const plural = METHOD_PLURAL[m.method];
    void run(m.method, () => api(`/methods/${m.method}`, { method: "PUT", body: { enabled } }), () => {
      setConfirming(null);
      toast({
        title: `${plural[0]!.toUpperCase()}${plural.slice(1)} turned ${enabled ? "on" : "off"}`,
        description: enabled ? "People can sign in with them again." : "Sign-ins that use them are blocked from now on.",
      });
    });
  }

  function change(m: MethodSetting, enabled: boolean) {
    if (!enabled && m.exclusive > 0) setConfirming(m);
    else apply(m, enabled);
  }

  function consequence(m: MethodSetting) {
    const who = `${people(m.exclusive)} ${m.exclusive === 1 ? "has" : "have"}`;
    if (m.method === "magic-link") {
      return `${who} no passkey, security key or authenticator app that is turned on. They can't sign in until you turn magic links back on or send them a setup link to enroll a passkey.`;
    }
    const fallback = magicLinks
      ? "They'll have to sign in with a magic link until they enroll another method."
      : "They can't sign in until you turn this back on or send them a new setup link.";
    return `${who} no other passkey, security key or authenticator app that is turned on. ${fallback}`;
  }

  return (
    <>
      <Card>
        <ul>
          {methods.map((m) => {
            const meta = METHOD[m.method];
            const Icon = meta.icon;
            const id = `method-${m.method}`;
            return (
              <li key={m.method} className="flex flex-col gap-4 border-b border-border px-4 py-6 last:border-b-0 sm:flex-row sm:items-start sm:justify-between sm:gap-8 sm:px-6">
                <div className="flex min-w-0 gap-4">
                  <span className="grid size-10 shrink-0 place-items-center rounded-md border border-border bg-sunken text-fg-3">
                    <Icon aria-hidden="true" size={20} strokeWidth={1.75} />
                  </span>
                  <div className="flex min-w-0 flex-col gap-1">
                    <span className="flex flex-wrap items-center gap-2">
                      <span id={id} className="font-medium text-fg">
                        {meta.label}
                      </span>
                      {meta.phishingResistant ? <Badge tone="success">Phishing-resistant</Badge> : null}
                      {m.enabled ? null : <Badge tone="neutral">Off</Badge>}
                    </span>
                    <p className="text-body-sm text-fg-3">{DESCRIPTION[m.method]}</p>
                    <p className="tnum text-body-sm text-fg-2">{usage(m)}</p>
                    {m.enabled && m.exclusive > 0 ? (
                      <p className="flex items-start gap-2 text-body-sm text-warning">
                        <TriangleAlert aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0" />
                        <span className="tnum">{warning(m)}</span>
                      </p>
                    ) : null}
                  </div>
                </div>
                <label className="flex h-8 shrink-0 cursor-pointer items-center gap-2 pl-14 text-body-sm text-fg sm:pl-0">
                  {pending === m.method ? (
                    <Spinner />
                  ) : (
                    <Checkbox aria-labelledby={`${id} ${id}-state`} checked={m.enabled} disabled={pending !== null} onChange={(event) => change(m, event.target.checked)} />
                  )}
                  <span id={`${id}-state`}>Allowed</span>
                </label>
              </li>
            );
          })}
        </ul>
      </Card>
      <p className="text-body-sm text-fg-3">
        Changes apply from the next sign-in. Enrolled methods are kept when you turn one off, so turning it back on restores them. People who are already signed in stay signed in.
      </p>
      <Dialog
        open={confirming !== null}
        onOpenChange={(open) => {
          if (!open) setConfirming(null);
        }}
        title={confirming ? `Turn off ${METHOD_PLURAL[confirming.method]}?` : "Turn off this method?"}
        description={confirming ? consequence(confirming) : undefined}
        footer={
          <>
            <Button variant="secondary" onClick={() => setConfirming(null)}>
              Cancel
            </Button>
            <Button variant="destructive" loading={confirming !== null && pending === confirming.method} onClick={() => confirming && apply(confirming, false)}>
              Turn off {confirming ? METHOD_PLURAL[confirming.method] : ""}
            </Button>
          </>
        }
      />
    </>
  );
}
