import { useSyncExternalStore } from "react";
import { ApiError } from "@/lib/api/client";
import { describeWebAuthnError, passkeysSupported, type getAssertion } from "@/lib/webauthn";

export type Ceremony = { ceremony: string; options: Parameters<typeof getAssertion>[0] };

const subscribe = () => () => {};

export function usePasskeySupport() {
  return useSyncExternalStore(subscribe, passkeysSupported, () => true);
}

export function cancelled(error: unknown) {
  return error instanceof DOMException && (error.name === "NotAllowedError" || error.name === "AbortError");
}

export function describeFailure(error: unknown) {
  if (error instanceof ApiError) return error.message;
  if (error instanceof DOMException) return describeWebAuthnError(error);
  return "Halo could not reach the server. Try again.";
}

export function deviceLabel() {
  const ua = navigator.userAgent;
  const browser = /Edg\//.test(ua) ? "Edge" : /Firefox\//.test(ua) ? "Firefox" : /Chrome\//.test(ua) ? "Chrome" : /Safari\//.test(ua) ? "Safari" : null;
  const os = /Android/.test(ua) ? "Android" : /iPhone|iPad/.test(ua) ? "iOS" : /Mac OS X/.test(ua) ? "macOS" : /Windows/.test(ua) ? "Windows" : /Linux|X11/.test(ua) ? "Linux" : null;
  return browser && os ? `${browser} on ${os}` : (browser ?? os ?? "");
}
