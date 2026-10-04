import { Ban, Check, KeyRound, type LucideIcon } from "lucide-react";
import type { Tone } from "@/components/ui/badge";
import { pluralize } from "@/lib/format";
import type {
  AccessPolicy,
  DeviceCondition,
  DeviceTrust,
  NetworkCondition,
  NetworkZone,
  PolicyEffect,
  PolicyMode,
  PolicyWithActivity,
  RiskEventType,
} from "@/lib/policy-types";
import type { MethodKind, RiskLevel } from "@/lib/types";

export const HALO_APP = "halo";

export const EFFECT: Record<PolicyEffect, { label: string; verb: string; icon: LucideIcon; color: string }> = {
  allow: { label: "Allow", verb: "Allow", icon: Check, color: "text-success" },
  "require-phishing-resistant": { label: "Require a passkey or security key", verb: "Require a passkey or security key for", icon: KeyRound, color: "text-warning" },
  block: { label: "Block", verb: "Block", icon: Ban, color: "text-danger" },
};

export const MODE: Record<PolicyMode, { label: string; description: string }> = {
  report: { label: "Report-only", description: "Halo records what the policy would do without changing any sign-in." },
  enforce: { label: "Enforced", description: "The policy decides sign-ins." },
};

export function policyState(p: { enabled: boolean; mode: PolicyMode }): { label: string; tone: Tone } {
  if (!p.enabled) return { label: "Off", tone: "neutral" };
  return p.mode === "enforce" ? { label: "Enforced", tone: "success" } : { label: "Report-only", tone: "info" };
}

export const NETWORK_CONDITION: Record<NetworkCondition, string> = {
  any: "Any network",
  in: "Inside selected networks",
  "not-in": "Outside selected networks",
};

export const DEVICE_CONDITION: Record<DeviceCondition, string> = {
  any: "Any device",
  trusted: "Trusted devices only",
  untrusted: "Devices that aren't trusted",
};

export const TRUST: Record<DeviceTrust, { label: string; tone: Tone }> = {
  unknown: { label: "Unknown", tone: "neutral" },
  trusted: { label: "Trusted", tone: "success" },
  blocked: { label: "Blocked", tone: "danger" },
};

export const ZONE_KIND: Record<NetworkZone["kind"], { label: string; tone: Tone; description: string }> = {
  trusted: { label: "Trusted", tone: "success", description: "Networks you control, such as offices and VPN egress." },
  risky: { label: "Risky", tone: "danger", description: "Sign-ins from these networks are rated high risk." },
};

export const RISK_TYPE: Record<RiskEventType, string> = {
  "risky-network": "Risky network",
  "failed-sign-ins": "Repeated failed sign-ins",
  "new-device": "New device",
};

export const RISK_LEVELS: RiskLevel[] = ["none", "low", "medium", "high"];

export const METHOD_PLURAL: Record<MethodKind, string> = {
  passkey: "passkeys",
  "security-key": "security keys",
  totp: "authenticator apps",
  "magic-link": "magic links",
  "recovery-codes": "recovery codes",
  federated: "identity providers",
};

export type Names = {
  groups: Record<string, string>;
  users: Record<string, string>;
  apps: Record<string, string>;
  zones: Record<string, string>;
};

export function joinList(items: string[], joiner = "or"): string {
  if (items.length <= 1) return items[0] ?? "";
  return `${items.slice(0, -1).join(", ")} ${joiner} ${items[items.length - 1]}`;
}

function people(groupIds: string[], userIds: string[], names: Names, joiner: string): string {
  const groups = groupIds.map((id) => names.groups[id] ?? "a deleted group");
  return joinList([...(groups.length ? [`members of ${joinList(groups, joiner)}`] : []), ...userIds.map((id) => names.users[id] ?? "a deleted user")], joiner);
}

export function describePolicy(p: Pick<AccessPolicy, "effect" | "conditions">, names: Names): string {
  const c = p.conditions;
  const who = c.allUsers ? "anyone" : people(c.groupIds, c.userIds, names, "or") || "nobody yet";
  const excluded = people(c.excludeGroupIds, c.excludeUserIds, names, "and");
  const apps = c.allApps ? "any application" : joinList(c.appIds.map((id) => (id === HALO_APP ? "Halo" : (names.apps[id] ?? "a deleted application")))) || "no application yet";
  const zones = joinList(c.zoneIds.map((id) => `“${names.zones[id] ?? "a deleted network"}”`));
  const parts = [`by ${who}${excluded ? ` except ${excluded}` : ""}`, `to ${apps}`];
  if (c.network === "in") parts.push(`from ${zones || "the selected networks"}`);
  if (c.network === "not-in") parts.push(`from outside ${zones || "the selected networks"}`);
  if (c.device === "trusted") parts.push("on trusted devices");
  if (c.device === "untrusted") parts.push("on devices that aren't trusted");
  if (c.risk.length) parts.push(`with ${joinList(RISK_LEVELS.filter((level) => c.risk.includes(level)))} risk`);
  if (c.methods.length) parts.push(`using ${joinList(c.methods.map((m) => METHOD_PLURAL[m]))}`);
  return `${EFFECT[p.effect].verb} sign-ins ${parts.join(" ")}.`;
}

export function activityText(p: PolicyWithActivity): string {
  const { matched, stopped } = p.activity;
  const report = p.mode === "report";
  if (matched === 0) return "No matching sign-ins in the last 24 hours.";
  if (p.effect === "block") return `${report ? "Would have blocked" : "Blocked"} ${pluralize(stopped, "sign-in")} in the last 24 hours.`;
  if (p.effect === "require-phishing-resistant") {
    return `${report ? "Would have required" : "Required"} a passkey or security key for ${stopped} of ${pluralize(matched, "matching sign-in")} in the last 24 hours.`;
  }
  return `Matched ${pluralize(matched, "sign-in")} in the last 24 hours.`;
}
