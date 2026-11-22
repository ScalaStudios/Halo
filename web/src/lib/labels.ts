import { Fingerprint, IdCard, KeyRound, ListOrdered, Mail, Monitor, Network, RectangleEllipsis, Smartphone, TicketCheck, type LucideIcon } from "lucide-react";
import type { Tone } from "@/components/ui/badge";
import { authMessages } from "./i18n/auth";
import type { Locale } from "./i18n/locale";
import type { LogExportProtocol, NotificationDestination } from "./settings-types";
import type { AuthStrength, MethodKind, RiskLevel, Scope, SignInEvent, SignInResult, UserStatus } from "./types";

export const STATUS: Record<UserStatus, { tone: Tone }> = {
  active: { tone: "success" },
  suspended: { tone: "warning" },
  invited: { tone: "info" },
  deprovisioned: { tone: "neutral" },
};

export const STRENGTH: Record<AuthStrength, { tone: Tone }> = {
  "phishing-resistant": { tone: "success" },
  "multi-factor": { tone: "warning" },
  "single-factor": { tone: "danger" },
};

export const METHOD: Record<MethodKind, { icon: LucideIcon; phishingResistant: boolean }> = {
  passkey: { icon: Fingerprint, phishingResistant: true },
  "security-key": { icon: KeyRound, phishingResistant: true },
  totp: { icon: Smartphone, phishingResistant: false },
  "magic-link": { icon: Mail, phishingResistant: false },
  "recovery-codes": { icon: ListOrdered, phishingResistant: false },
  password: { icon: RectangleEllipsis, phishingResistant: false },
  federated: { icon: Network, phishingResistant: false },
  "access-pass": { icon: TicketCheck, phishingResistant: false },
  certificate: { icon: IdCard, phishingResistant: false },
  kerberos: { icon: Monitor, phishingResistant: false },
};

export const RESULT: Record<SignInResult, { tone: Tone }> = {
  success: { tone: "success" },
  failure: { tone: "danger" },
  interrupted: { tone: "warning" },
  ended: { tone: "neutral" },
};

export const RISK: Record<RiskLevel, { tone: Tone }> = {
  none: { tone: "neutral" },
  low: { tone: "info" },
  medium: { tone: "warning" },
  high: { tone: "danger" },
};

export const PROTOCOLS: Record<LogExportProtocol, { placeholder: string }> = {
  "syslog-tls": { placeholder: "siem.example.com:6514" },
  "syslog-udp": { placeholder: "syslog.lab.internal:514" },
  "otlp-http": { placeholder: "https://otel.example.com:4318/v1/logs" },
};

export const DESTINATIONS: Record<NotificationDestination, { label: string; placeholder: string }> = {
  slack: { label: "Slack", placeholder: "https://hooks.slack.com/services/…" },
  discord: { label: "Discord", placeholder: "https://discord.com/api/webhooks/…" },
  webhook: { label: "Webhook", placeholder: "https://hooks.example.com/halo" },
};

export function signInUser(event: SignInEvent, names: Record<string, string>): string {
  return (event.userId && names[event.userId]) || event.email;
}

export function signInApp(event: SignInEvent, names: Record<string, string>, unknown: string): string {
  return event.appId ? (names[event.appId] ?? unknown) : "Halo";
}

export function scopeLabel(scope: Scope, locale: Locale) {
  return (authMessages(locale).scopes as Record<string, string>)[scope.name] ?? (scope.description || scope.name);
}
