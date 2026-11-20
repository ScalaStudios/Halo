"use client";

import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import type { Challenge } from "@/lib/settings-types";

type WidgetApi = { render: (container: HTMLElement, params: Record<string, unknown>) => string; remove?: (id: string) => void };

const SCRIPTS: Record<Challenge["provider"], string> = {
  turnstile: "https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit&onload=haloChallengeReady",
  hcaptcha: "https://js.hcaptcha.com/1/api.js?render=explicit&onload=haloChallengeReady",
};

let script: Promise<WidgetApi> | null = null;

function load(provider: Challenge["provider"]) {
  script ??= new Promise<WidgetApi>((resolve, reject) => {
    const scope = window as unknown as Record<string, unknown>;
    scope.haloChallengeReady = () => resolve(scope[provider] as WidgetApi);
    const tag = Object.assign(document.createElement("script"), { src: SCRIPTS[provider], async: true });
    tag.onerror = () => {
      script = null;
      tag.remove();
      reject(new Error(`${provider} did not load`));
    };
    document.head.append(tag);
  });
  return script;
}

export function ChallengeCheck({ challenge, onToken, onCancel }: { challenge: Challenge; onToken: (token: string) => void; onCancel: () => void }) {
  const panel = useRef<HTMLDivElement>(null);
  const host = useRef<HTMLDivElement>(null);
  const solved = useRef(onToken);
  const [ready, setReady] = useState(false);
  const [problem, setProblem] = useState<string | null>(null);

  useEffect(() => {
    solved.current = onToken;
  });

  useEffect(() => panel.current?.focus(), []);

  useEffect(() => {
    let active = true;
    let remove = () => {};
    load(challenge.provider)
      .then((api) => {
        const element = host.current;
        if (!active || !element) return;
        const id = api.render(element, {
          sitekey: challenge.siteKey,
          theme: "dark",
          size: (element.parentElement?.clientWidth ?? 0) < 300 ? "compact" : "normal",
          callback: (token: string) => solved.current(token),
          "error-callback": () => {
            setProblem("The check couldn't finish. Try it again, or use a different way to sign in.");
            return true;
          },
        });
        remove = () => api.remove?.(id);
        setReady(true);
      })
      .catch(() => active && setProblem("The check didn't load. Check your connection, or use a different way to sign in."));
    return () => {
      active = false;
      remove();
    };
  }, [challenge.provider, challenge.siteKey]);

  return (
    <div
      ref={panel}
      tabIndex={-1}
      role="group"
      aria-label="Check that you're a person"
      className="flex animate-enter flex-col items-center gap-3 rounded-lg border border-border bg-surface p-4 text-center outline-none"
    >
      <p role="status" className={problem ? "text-body-sm text-danger" : "text-body-sm text-fg-2"}>
        {problem ?? "Confirm you're a person to continue."}
      </p>
      <div className="flex min-h-16 w-full items-center justify-center">
        {ready || problem ? null : <Spinner size={20} />}
        <div ref={host} />
      </div>
      <Button variant="quiet" size="sm" onClick={onCancel}>
        Cancel
      </Button>
    </div>
  );
}
