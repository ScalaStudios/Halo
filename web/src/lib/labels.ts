import { Fingerprint, IdCard, KeyRound, ListOrdered, Mail, Monitor, Network, RectangleEllipsis, Smartphone, TicketCheck, type LucideIcon } from "lucide-react";
import type { Tone } from "@/components/ui/badge";
import type { RemoteAuth } from "./infra-types";
import type { LogExportProtocol, NotificationDestination } from "./settings-types";
import type { AttributeType, AuthStrength, Group, JoinPolicy, MethodKind, RiskLevel, Scope, SignInEvent, SignInResult, UserStatus } from "./types";

export const ATTRIBUTE_TYPE: Record<AttributeType, string> = {
  string: "Text",
  number: "Number",
  boolean: "Yes or no",
  date: "Date",
  list: "List",
};

export const STATUS: Record<UserStatus, { label: string; tone: Tone }> = {
  active: { label: "Active", tone: "success" },
  suspended: { label: "Suspended", tone: "warning" },
  invited: { label: "Invited", tone: "info" },
  deprovisioned: { label: "Deprovisioned", tone: "neutral" },
};

export const STRENGTH: Record<AuthStrength, { label: string; tone: Tone; description: string }> = {
  "phishing-resistant": { label: "Phishing-resistant", tone: "success", description: "Signs in with a passkey or security key bound to this domain." },
  "multi-factor": { label: "Multi-factor", tone: "warning", description: "Uses a second factor that can be phished, such as an authenticator code." },
  "single-factor": { label: "Single factor", tone: "danger", description: "Relies on one factor only, such as an email magic link." },
};

export const METHOD: Record<MethodKind, { label: string; icon: LucideIcon; phishingResistant: boolean }> = {
  passkey: { label: "Passkey", icon: Fingerprint, phishingResistant: true },
  "security-key": { label: "Security key", icon: KeyRound, phishingResistant: true },
  totp: { label: "Authenticator app", icon: Smartphone, phishingResistant: false },
  "magic-link": { label: "Magic link", icon: Mail, phishingResistant: false },
  "recovery-codes": { label: "Recovery codes", icon: ListOrdered, phishingResistant: false },
  password: { label: "Password", icon: RectangleEllipsis, phishingResistant: false },
  federated: { label: "Identity provider", icon: Network, phishingResistant: false },
  "access-pass": { label: "Temporary access pass", icon: TicketCheck, phishingResistant: false },
  certificate: { label: "Client certificate", icon: IdCard, phishingResistant: false },
  kerberos: { label: "Windows sign-in", icon: Monitor, phishingResistant: false },
};

export const SIGN_IN_METHOD: Record<SignInEvent["method"], { label: string }> = { ...METHOD, token: { label: "Token request" } };

export const RESULT: Record<SignInResult, { label: string; tone: Tone }> = {
  success: { label: "Success", tone: "success" },
  failure: { label: "Failed", tone: "danger" },
  interrupted: { label: "Interrupted", tone: "warning" },
  ended: { label: "Session ended", tone: "neutral" },
};

export const RISK: Record<RiskLevel, { label: string; tone: Tone }> = {
  none: { label: "None", tone: "neutral" },
  low: { label: "Low", tone: "info" },
  medium: { label: "Medium", tone: "warning" },
  high: { label: "High", tone: "danger" },
};

export const PROTOCOLS: Record<LogExportProtocol, { label: string; placeholder: string; hint: string }> = {
  "syslog-tls": { label: "Syslog over TLS", placeholder: "siem.example.com:6514", hint: "RFC 5424 messages over TCP with TLS, one per event." },
  "syslog-udp": { label: "Syslog over UDP", placeholder: "syslog.lab.internal:514", hint: "Unencrypted and without delivery confirmation. Use it only in a lab." },
  "otlp-http": { label: "OpenTelemetry (OTLP/HTTP)", placeholder: "https://otel.example.com:4318/v1/logs", hint: "OTLP log records as JSON. Halo adds /v1/logs when the address has no path." },
};

export const DESTINATIONS: Record<NotificationDestination, { label: string; placeholder: string }> = {
  slack: { label: "Slack", placeholder: "https://hooks.slack.com/services/…" },
  discord: { label: "Discord", placeholder: "https://discord.com/api/webhooks/…" },
  webhook: { label: "Webhook", placeholder: "https://hooks.example.com/halo" },
};

export const GROUP_KIND: Record<Group["kind"], string> = { assigned: "Assigned", dynamic: "Rule-based" };

export const REMOTE_AUTH: Record<RemoteAuth, string> = { certificate: "Certificate from Halo's SSH CA", password: "Stored password", key: "Stored private key" };

export const JOIN_POLICY: Record<JoinPolicy, { label: string; description: string }> = {
  closed: { label: "Closed", description: "owners add members" },
  request: { label: "On request", description: "people ask, owners approve" },
  open: { label: "Open", description: "anyone can join" },
};

export function signInUser(event: SignInEvent, names: Record<string, string>): string {
  return (event.userId && names[event.userId]) || event.email;
}

export function signInApp(event: SignInEvent, names: Record<string, string>): string {
  return event.appId ? (names[event.appId] ?? "Unknown application") : "Halo";
}

const SCOPE: Record<string, string> = {
  openid: "Confirm who you are",
  profile: "See your name, username and profile picture",
  email: "See your email address",
  groups: "See the names of the groups you belong to",
  offline_access: "Keep access while you're away",
};

export function scopeLabel(scope: Scope) {
  return SCOPE[scope.name] ?? (scope.description || scope.name);
}
