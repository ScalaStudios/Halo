import { StatusDot, Tag, type Tone } from "@/components/ui/badge";
import { CardBody } from "@/components/ui/card";
import { RelativeTime } from "@/components/ui/relative-time";
import { formatDateTime } from "@/lib/format";
import type { ApiKey, KeyScope, RoleKey, ServiceAccount } from "@/lib/provisioning-types";
import type { AuditEvent } from "@/lib/types";

export const ROLES: Record<RoleKey, { label: string; description: string }> = {
  global_admin: { label: "Global administrator", description: "Every permission in Halo, including roles, service accounts and API keys." },
  security_admin: { label: "Security administrator", description: "Suspend people, end sessions, and manage service accounts and API keys." },
  user_admin: { label: "User administrator", description: "Invite, suspend and restore people, and manage groups. Required for SCIM provisioning." },
  helpdesk_admin: { label: "Helpdesk administrator", description: "Reset authentication methods and end sessions." },
  app_admin: { label: "Application administrator", description: "Register applications, rotate secrets, assign groups and set up provisioning." },
  auditor: { label: "Auditor", description: "Read every administration page and the audit log without changing anything." },
};

export const SCOPES: Record<KeyScope, { label: string; description: string }> = {
  api: { label: "Management API", description: "Everything the service account's roles allow under /api/v1." },
  scim: { label: "SCIM provisioning", description: "Create, update and deactivate people and groups under /scim/v2." },
};

export const ACCOUNT_STATUS: Record<ServiceAccount["status"], { label: string; tone: Tone }> = {
  active: { label: "Active", tone: "success" },
  suspended: { label: "Disabled", tone: "neutral" },
};

type KeyState = "active" | "expired" | "revoked";

export const KEY_STATE: Record<KeyState, { label: string; tone: Tone }> = {
  active: { label: "Active", tone: "success" },
  expired: { label: "Expired", tone: "warning" },
  revoked: { label: "Revoked", tone: "neutral" },
};

export function keyState(key: ApiKey): KeyState {
  if (key.revokedAt) return "revoked";
  if (key.expiresAt && new Date(key.expiresAt).getTime() <= Date.now()) return "expired";
  return "active";
}

export function activeKeys(keys: ApiKey[]) {
  return keys.filter((key) => keyState(key) === "active");
}

export function lastUsed(keys: ApiKey[]): string | null {
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

export function ScopeTags({ scopes }: { scopes: KeyScope[] }) {
  return (
    <span className="flex flex-wrap gap-2">
      {scopes.map((scope) => (
        <Tag key={scope}>{scope}</Tag>
      ))}
    </span>
  );
}

export function ActivityList({ events, actorNames, empty }: { events: AuditEvent[]; actorNames: Record<string, string>; empty: string }) {
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
            <span title={formatDateTime(event.time)}>
              {(event.actorId && actorNames[event.actorId]) || "Halo"} · <RelativeTime iso={event.time} />
            </span>
          </span>
        </li>
      ))}
    </ul>
  );
}
