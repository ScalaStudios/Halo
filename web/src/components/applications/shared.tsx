import { AppWindow, PanelsTopLeft, ServerCog, Terminal, TriangleAlert, type LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import type { Tone } from "@/components/ui/badge";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { CopyButton } from "@/components/ui/copy-button";
import { CopyField } from "@/components/ui/copy-field";
import { cn } from "@/lib/cn";
import { daysUntil, formatDate, pluralize } from "@/lib/format";
import type { SetupGuide } from "@/lib/setup-guides";
import type { Application, AppType } from "@/lib/types";

export const APP_STATUS: Record<Application["status"] | "awaiting", { label: string; tone: Tone }> = {
  active: { label: "Active", tone: "success" },
  disabled: { label: "Disabled", tone: "neutral" },
  awaiting: { label: "Awaiting approval", tone: "warning" },
};

export function statusKey(app: Pick<Application, "status" | "awaitingApproval">) {
  return app.awaitingApproval ? "awaiting" : app.status;
}

export const PROTOCOL_LABEL = { oidc: "OpenID Connect", saml: "SAML 2.0", oauth: "OAuth 2.0", "forward-auth": "Forward auth", radius: "RADIUS", ldap: "LDAP", bookmark: "Bookmark" } as const;
export const TYPE_LABEL = { web: "Web", spa: "Single-page", native: "Native / CLI", service: "Service" } as const;

export const TYPE_ICON: Record<AppType, LucideIcon> = { web: AppWindow, spa: PanelsTopLeft, native: Terminal, service: ServerCog };

export function soonestExpiry(app: Application): string | null {
  return app.credentials.map((c) => c.expiresAt).sort()[0] ?? null;
}

export function credentialState(app: Application): "expired" | "expiring" | "ok" | "none" {
  const iso = soonestExpiry(app);
  if (!iso) return "none";
  const days = daysUntil(iso);
  return days < 0 ? "expired" : days <= 30 ? "expiring" : "ok";
}

export function ExpiryText({ iso }: { iso: string }) {
  const days = daysUntil(iso);
  const tone = days <= 14 ? "text-danger" : days <= 30 ? "text-warning" : "text-fg-3";
  return (
    <span className={cn("tnum whitespace-nowrap", tone)}>
      {days < 0 ? "Expired" : days <= 30 ? `Expires in ${pluralize(days, "day")}` : `Expires ${formatDate(iso)}`}
    </span>
  );
}

export function SecretWarning({ children }: { children: ReactNode }) {
  return (
    <div className="flex gap-3 rounded-md border border-warning/20 bg-warning-container p-4">
      <TriangleAlert aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0 text-warning" />
      <p className="text-body-sm text-fg-2">{children}</p>
    </div>
  );
}

export function RadiusServerFields({ issuer, port }: { issuer: string; port?: number }) {
  if (!port) return <SecretWarning>Halo&apos;s RADIUS server is off. Set HALO_RADIUS_LISTEN, for example to :1812, and restart Halo.</SecretWarning>;
  return (
    <>
      <CopyField label="Server" value={new URL(issuer).hostname} hint="The address of the Halo server itself. RADIUS runs over UDP, so HTTP proxies and ingresses don't carry it." />
      <CopyField label="Port" value={String(port)} hint="For authentication and accounting." />
    </>
  );
}

export function LdapConnectionFields({ ldap }: { ldap: NonNullable<Application["ldap"]> }) {
  return (
    <>
      {ldap.url ? (
        <CopyField
          label="Server"
          value={ldap.url}
          hint={ldap.url.startsWith("ldaps://") ? undefined : "Turn on StartTLS in the application. Halo refuses binds without TLS unless HALO_LDAP_TLS is none."}
        />
      ) : (
        <SecretWarning>The LDAP server is off. Set HALO_LDAP_LISTEN, with HALO_LDAP_CERT_FILE and HALO_LDAP_KEY_FILE for TLS, and restart Halo.</SecretWarning>
      )}
      <CopyField label="Base DN" value={ldap.baseDn} />
      <CopyField label="Bind DN" value={ldap.bindDn} hint="The application binds as this DN with its bind password to search for people and groups." />
      <CopyField
        label="User search base"
        value={ldap.peopleDn}
        hint="inetOrgPerson entries: uid and mail hold the email address, cn the name and memberOf the groups. People bind as their own DN with their Halo password; people without one can't."
      />
      <CopyField label="Group search base" value={ldap.groupsDn} hint="groupOfNames entries, with a member attribute for each person." />
    </>
  );
}

export function SetupGuideCard({ guide, className }: { guide: SetupGuide; className?: string }) {
  return (
    <Card className={className}>
      <CardHeader title={guide.title} description={guide.description} />
      <CardBody>
        <div className="overflow-hidden rounded-md border border-border bg-sunken">
          <div className="flex h-10 items-center justify-between gap-4 border-b border-border pr-1 pl-4">
            <span className="truncate font-mono text-code-sm text-fg-3">{guide.file}</span>
            <CopyButton value={guide.code} />
          </div>
          <pre className="overflow-x-auto p-4 font-mono text-code-sm text-fg-2">
            <code>{guide.code}</code>
          </pre>
        </div>
      </CardBody>
    </Card>
  );
}
