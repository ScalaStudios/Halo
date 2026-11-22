"use client";

import { useEffect, useState } from "react";
import { buttonClasses } from "@/components/ui/button";
import { api } from "@/lib/api/client";
import type { SignInProvider } from "@/lib/federation-types";
import type { SignInMessages } from "@/lib/i18n/auth";
import { METHOD } from "@/lib/labels";

export function federationProblem(code: string | null, messages: SignInMessages) {
  if (!code) return null;
  return messages.federation[code] ?? messages.federation.fallback;
}

export function useSignInProviders() {
  const [providers, setProviders] = useState<SignInProvider[]>([]);
  useEffect(() => {
    let live = true;
    api<SignInProvider[]>("/auth/providers").then(
      (list) => live && setProviders(list),
      () => {},
    );
    return () => {
      live = false;
    };
  }, []);
  return providers;
}

export function providerStartUrl(id: string, authRequest?: string, next?: string) {
  const params = new URLSearchParams();
  if (authRequest) params.set("authRequest", authRequest);
  if (next) params.set("next", next);
  const query = params.toString();
  return `/api/v1/auth/federated/${encodeURIComponent(id)}/start${query ? `?${query}` : ""}`;
}

export function ProviderButtons({ providers, authRequest, next, label }: { providers: SignInProvider[]; authRequest?: string; next?: string; label: (name: string) => string }) {
  const Icon = METHOD.federated.icon;
  return providers.map((provider) => (
    <a key={provider.id} href={providerStartUrl(provider.id, authRequest, next)} className={buttonClasses("secondary", "lg", "w-full animate-enter")}>
      <Icon aria-hidden="true" size={20} strokeWidth={1.75} className="text-fg-3" />
      {label(provider.name)}
    </a>
  ));
}
