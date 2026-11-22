import { AppWindow, PanelsTopLeft, ServerCog, Terminal, TriangleAlert, type LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import type { Tone } from "@/components/ui/badge";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { CopyButton } from "@/components/ui/copy-button";
import { CopyField } from "@/components/ui/copy-field";
import { cn } from "@/lib/cn";
import { daysUntil, formatDate } from "@/lib/format";
import { useLocale } from "@/lib/i18n/client";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import type { Locale } from "@/lib/i18n/locale";
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

export const PROTOCOL_LABEL = { oidc: "OpenID Connect", saml: "SAML 2.0", oauth: "OAuth 2.0", "forward-auth": "Forward auth", radius: "RADIUS", ldap: "LDAP", remote: "Remote access", bookmark: "Bookmark" } as const;
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
  const locale = useLocale();
  const t = applicationsMessages[locale].expiry;
  const days = daysUntil(iso);
  const tone = days <= 14 ? "text-danger" : days <= 30 ? "text-warning" : "text-fg-3";
  return (
    <span className={cn("tnum whitespace-nowrap", tone)}>
      {days < 0 ? t.expired : days <= 30 ? t.in(days) : t.on(formatDate(iso, locale))}
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

export function RadiusServerFields({ issuer, port, locale }: { issuer: string; port?: number; locale: Locale }) {
  const t = applicationsMessages[locale].fields;
  if (!port) return <SecretWarning>{t.radiusOff}</SecretWarning>;
  return (
    <>
      <CopyField label={t.server} value={new URL(issuer).hostname} hint={t.radiusServerHint} />
      <CopyField label={t.port} value={String(port)} hint={t.portHint} />
    </>
  );
}

export function LdapConnectionFields({ ldap, locale }: { ldap: NonNullable<Application["ldap"]>; locale: Locale }) {
  const t = applicationsMessages[locale].fields;
  return (
    <>
      {ldap.url ? (
        <CopyField
          label={t.server}
          value={ldap.url}
          hint={ldap.url.startsWith("ldaps://") ? undefined : t.ldapTlsHint}
        />
      ) : (
        <SecretWarning>{t.ldapOff}</SecretWarning>
      )}
      <CopyField label={t.baseDn} value={ldap.baseDn} />
      <CopyField label={t.bindDn} value={ldap.bindDn} hint={t.bindDnHint} />
      <CopyField
        label={t.userBase}
        value={ldap.peopleDn}
        hint={t.userBaseHint}
      />
      <CopyField label={t.groupBase} value={ldap.groupsDn} hint={t.groupBaseHint} />
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
