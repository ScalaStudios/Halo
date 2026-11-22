import { StatusDot, Tag, type Tone } from "@/components/ui/badge";
import { CardBody } from "@/components/ui/card";
import { RelativeTime } from "@/components/ui/relative-time";
import { formatDateTime } from "@/lib/format";
import { provisioningMessages } from "@/lib/i18n/console/infrastructure/provisioning";
import type { Locale } from "@/lib/i18n/locale";
import type { ApiKey, KeyScope, RoleKey, ServiceAccount } from "@/lib/provisioning-types";
import type { AuditEvent } from "@/lib/types";

export function rolesFor(locale: Locale): Record<RoleKey, { label: string; description: string }> {
  return provisioningMessages[locale].role;
}

export const ROLES = rolesFor("en");

export function scopesFor(locale: Locale): Record<KeyScope, { label: string; description: string }> {
  return provisioningMessages[locale].scope;
}

export const SCOPES = scopesFor("en");

export const ACCOUNT_STATUS: Record<ServiceAccount["status"], Tone> = { active: "success", suspended: "neutral" };

type KeyState = "active" | "expired" | "revoked";

export const KEY_STATE: Record<KeyState, Tone> = { active: "success", expired: "warning", revoked: "neutral" };

export function keyState(key: ApiKey): KeyState {
  if (key.revokedAt) return "revoked";
  if (key.expiresAt && new Date(key.expiresAt).getTime() <= Date.now()) return "expired";
  return "active";
}

export function activeKeys(keys: ApiKey[]) {
  return keys.filter((key) => keyState(key) === "active");
}

export function lastUsed(keys: { lastUsedAt: string | null }[]): string | null {
  return (
    keys
      .map((key) => key.lastUsedAt)
      .filter((time): time is string => time !== null)
      .sort()
      .at(-1) ?? null
  );
}

export function canProvision(account: ServiceAccount) {
  return account.roles.includes("user_admin") || account.roles.includes("global_admin");
}

export function StatusLabel({ label, tone }: { label: string; tone: Tone }) {
  return (
    <span className="inline-flex items-center gap-2 whitespace-nowrap">
      <StatusDot tone={tone} />
      {label}
    </span>
  );
}

export function RateLimitHits({ hits, locale }: { hits: number; locale: Locale }) {
  return hits ? <span className="tnum text-warning">{hits.toLocaleString(locale)}</span> : <span className="text-fg-3">{provisioningMessages[locale].none}</span>;
}

export function ScopeTags({ scopes }: { scopes: KeyScope[] }) {
  return (
    <span className="flex flex-wrap gap-2">
      {scopes.map((scope) => (
        <Tag key={scope}>{scope}</Tag>
      ))}
    </span>
  );
}

export function ActivityList({ events, actorNames, empty, locale }: { events: AuditEvent[]; actorNames: Record<string, string>; empty: string; locale: Locale }) {
  if (events.length === 0) {
    return (
      <CardBody>
        <p className="text-body-sm text-fg-3">{empty}</p>
      </CardBody>
    );
  }
  return (
    <ul>
      {events.map((event) => (
        <li key={event.id} className="flex flex-col gap-1 border-t border-border px-6 py-3">
          <span className="text-body-sm text-fg-2">
            <span className="font-medium text-fg">{event.target}</span> · {event.summary}
          </span>
          <span className="flex flex-wrap items-center gap-2 text-caption text-fg-3">
            <Tag>{event.action}</Tag>
            <span title={formatDateTime(event.time, locale)}>
              {event.actorId ? (actorNames[event.actorId] ?? event.actorId) : "Halo"} · <RelativeTime iso={event.time} />
            </span>
          </span>
        </li>
      ))}
    </ul>
  );
}
