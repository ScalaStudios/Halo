import { Fingerprint, KeyRound, ListOrdered, Mail, Network, Smartphone, type LucideIcon } from "lucide-react";
import type { Tone } from "@/components/ui/badge";
import type { AuthStrength, MethodKind, RiskLevel, SignInEvent, SignInResult, UserStatus } from "./types";

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
  federated: { label: "Identity provider", icon: Network, phishingResistant: false },
};

export const RESULT: Record<SignInResult, { label: string; tone: Tone }> = {
  success: { label: "Success", tone: "success" },
  failure: { label: "Failed", tone: "danger" },
  interrupted: { label: "Interrupted", tone: "warning" },
};

export const RISK: Record<RiskLevel, { label: string; tone: Tone }> = {
  none: { label: "None", tone: "neutral" },
  low: { label: "Low", tone: "info" },
  medium: { label: "Medium", tone: "warning" },
  high: { label: "High", tone: "danger" },
};

export function signInUser(event: SignInEvent, names: Record<string, string>): string {
  return (event.userId && names[event.userId]) || event.email;
}

export function signInApp(event: SignInEvent, names: Record<string, string>): string {
  return event.appId ? (names[event.appId] ?? "Unknown application") : "Halo";
}
