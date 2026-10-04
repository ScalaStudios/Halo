"use client";

import { useEffect, useState } from "react";
import { buttonClasses } from "@/components/ui/button";
import { api } from "@/lib/api/client";
import type { SignInProvider } from "@/lib/federation-types";
import { METHOD } from "@/lib/labels";

const PROBLEMS: Record<string, string> = {
  ERR_PROVIDER_UNAVAILABLE: "That sign-in option is turned off. Choose another way to sign in.",
  ERR_PROVIDER_UNREACHABLE: "Halo could not reach the identity provider. Try again in a few minutes, or sign in another way.",
  ERR_FEDERATION_STATE: "That sign-in attempt expired or was already used. Start again.",
  ERR_FEDERATION_CANCELLED: "The identity provider did not sign you in. Try again, or choose another way to sign in.",
  ERR_FEDERATION_FAILED: "Halo could not verify the response from the identity provider. Try again; if it keeps failing, ask your administrator to check the provider settings.",
  ERR_FEDERATION_EMAIL: "The identity provider did not confirm your email address, so Halo could not find your account. Sign in another way, or ask your administrator for help.",
  ERR_FEDERATION_DOMAIN: "Your email domain is not allowed with this identity provider. Use your work account, or ask your administrator to allow your domain.",
  ERR_FEDERATION_NO_ACCOUNT: "No Halo account matches that account. Ask your administrator to invite you, then sign in again.",
  ERR_ACCOUNT_SUSPENDED: "This account is suspended. Contact your administrator to restore access.",
  ERR_NOT_ASSIGNED: "You don't have access to this application. Ask your administrator to assign it to you.",
  ERR_BLOCKED_BY_POLICY: "Your organization's access policy blocked this sign-in. If you think this is wrong, contact your administrator.",
  ERR_STRONGER_AUTH_REQUIRED: "This application requires a passkey or security key. Sign in with one of those instead.",
};

export function federationProblem(code: string | null) {
  if (!code) return null;
  return PROBLEMS[code] ?? "Halo could not complete the sign-in. Try again, or choose another way to sign in.";
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

export function ProviderButtons({ providers, authRequest, next }: { providers: SignInProvider[]; authRequest?: string; next?: string }) {
  const Icon = METHOD.federated.icon;
  return providers.map((provider) => (
    <a key={provider.id} href={providerStartUrl(provider.id, authRequest, next)} className={buttonClasses("secondary", "lg", "w-full animate-enter")}>
      <Icon aria-hidden="true" size={20} strokeWidth={1.75} className="text-fg-3" />
      Continue with {provider.name}
    </a>
  ));
}
