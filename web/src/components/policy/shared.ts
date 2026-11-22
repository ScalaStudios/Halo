import { Ban, Check, FileCheck, KeyRound, ShieldCheck, type LucideIcon } from "lucide-react";
import type { Tone } from "@/components/ui/badge";
import { METHOD } from "@/lib/labels";
import type { policyMessages } from "@/lib/i18n/console/policy";
import type { AccessPolicy, ComplianceState, DeviceTrust, NetworkZone, PolicyEffect, PolicyMode, PolicyWithActivity } from "@/lib/policy-types";
import type { MethodKind, RiskLevel } from "@/lib/types";

export type PolicyCopy = (typeof policyMessages)["en"];

export const HALO_APP = "halo";

export const EFFECT: Record<PolicyEffect, { icon: LucideIcon; color: string }> = {
  allow: { icon: Check, color: "text-success" },
  "require-phishing-resistant": { icon: KeyRound, color: "text-warning" },
  "require-terms": { icon: FileCheck, color: "text-info" },
  "require-context": { icon: ShieldCheck, color: "text-warning" },
  block: { icon: Ban, color: "text-danger" },
};

export function policyState(p: { enabled: boolean; mode: PolicyMode }, t: PolicyCopy): { label: string; tone: Tone } {
  if (!p.enabled) return { label: t.labels.state.off, tone: "neutral" };
  return p.mode === "enforce" ? { label: t.labels.state.enforced, tone: "success" } : { label: t.labels.state.report, tone: "info" };
}

export const COMPLIANCE: Record<ComplianceState, { tone: Tone }> = {
  compliant: { tone: "success" },
  "non-compliant": { tone: "danger" },
  unknown: { tone: "neutral" },
};

export const TRUST: Record<DeviceTrust, { tone: Tone }> = {
  unknown: { tone: "neutral" },
  trusted: { tone: "success" },
  blocked: { tone: "danger" },
};

export const ZONE_KIND: Record<NetworkZone["kind"], { tone: Tone }> = {
  trusted: { tone: "success" },
  risky: { tone: "danger" },
};

export const RISK_LEVELS: RiskLevel[] = ["none", "low", "medium", "high"];

export const POLICY_METHODS = (Object.keys(METHOD) as MethodKind[]).filter((kind) => kind !== "access-pass");

export type Names = {
  groups: Record<string, string>;
  users: Record<string, string>;
  apps: Record<string, string>;
  zones: Record<string, string>;
  terms: Record<string, string>;
  contexts: Record<string, string>;
};

export function joinList(items: string[], joiner = "or"): string {
  if (items.length <= 1) return items[0] ?? "";
  return `${items.slice(0, -1).join(", ")} ${joiner} ${items[items.length - 1]}`;
}

function people(groupIds: string[], userIds: string[], names: Names, joiner: string, t: PolicyCopy): string {
  const groups = groupIds.map((id) => names.groups[id] ?? t.describe.deletedGroup);
  return joinList([...(groups.length ? [t.describe.membersOf(joinList(groups, joiner))] : []), ...userIds.map((id) => names.users[id] ?? t.describe.deletedUser)], joiner);
}

export function describePolicy(p: Pick<AccessPolicy, "effect" | "termsId" | "contextId" | "conditions">, names: Names, t: PolicyCopy): string {
  const c = p.conditions;
  const d = t.describe;
  const who = c.allUsers ? d.anyone : people(c.groupIds, c.userIds, names, t.or, t) || d.nobody;
  const excluded = people(c.excludeGroupIds, c.excludeUserIds, names, t.and, t);
  const apps = c.allApps ? d.anyApp : joinList(c.appIds.map((id) => (id === HALO_APP ? "Halo" : (names.apps[id] ?? d.deletedApp))), t.or) || d.noApp;
  const zones = joinList(c.zoneIds.map((id) => d.quote(names.zones[id] ?? d.deletedNetwork)), t.or);
  const parts = [d.by(who, excluded), d.to(apps)];
  if (c.network === "in") parts.push(d.from(zones || d.selectedNetworks));
  if (c.network === "not-in") parts.push(d.fromOutside(zones || d.selectedNetworks));
  if (c.device === "trusted") parts.push(d.trusted);
  if (c.device === "untrusted") parts.push(d.untrusted);
  if (c.compliance === "compliant") parts.push(d.compliant);
  if (c.compliance === "not-compliant") parts.push(d.notCompliant);
  if (c.risk.length) parts.push(d.risk(joinList(RISK_LEVELS.filter((level) => c.risk.includes(level)).map((level) => d.riskLevel[level]), t.or)));
  if (c.methods.length) parts.push(d.methods(joinList(c.methods.map((m) => t.labels.methodPlural[m]), t.or)));
  const verb =
    p.effect === "require-terms"
      ? d.requireTerms((p.termsId && names.terms[p.termsId]) || d.termsOfUse)
      : p.effect === "require-context" && p.contextId
        ? d.requireContext(names.contexts[p.contextId] ?? p.contextId)
        : d.verb[p.effect];
  return d.sentence(verb, parts.join(" "));
}

export function activityText(p: PolicyWithActivity, t: PolicyCopy): string {
  const { matched, stopped } = p.activity;
  const report = p.mode === "report";
  if (matched === 0) return t.activity.none;
  if (p.effect === "block") return t.activity.blocked(report, stopped);
  if (p.effect === "require-phishing-resistant") return t.activity.required(report, stopped, matched);
  if (p.effect === "require-context") return t.activity.context(report, stopped, matched);
  if (p.effect === "require-terms") return t.activity.terms(report, stopped, matched);
  return t.activity.matched(matched);
}
