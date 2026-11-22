import Link from "next/link";
import { Bot } from "lucide-react";
import { LDAPDirectories } from "@/components/provisioning/ldap-directories";
import { OutboundTable } from "@/components/provisioning/outbound";
import {
  ActivityList,
  activeKeys,
  canProvision,
  lastUsed,
} from "@/components/provisioning/shared";
import { buttonClasses } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { DescriptionList } from "@/components/ui/description-list";
import { EmptyState } from "@/components/ui/empty-state";
import { PageHeader } from "@/components/ui/page-header";
import { SectionTitle } from "@/components/ui/section-title";
import { apiGet } from "@/lib/api/server";
import { formatRelative } from "@/lib/format";
import { frameMessages } from "@/lib/i18n/console/frame";
import { provisioningMessages } from "@/lib/i18n/console/infrastructure/provisioning";
import { requestLocale } from "@/lib/i18n/server";
import type {
  AppProvisioning,
  LDAPDirectory,
  ServiceAccount,
} from "@/lib/provisioning-types";
import type { OrganizationProfile } from "@/lib/settings-types";
import type { Application, Attribute, AuditEvent, Group } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.provisioning.label };
}

export default async function ProvisioningPage() {
  const [org, locale] = await Promise.all([apiGet<OrganizationProfile>("/organization"), requestLocale()]);
  const nav = frameMessages[locale].nav.items;
  const t = provisioningMessages[locale];
  const inbound = !org.featuresOff.includes("scim");
  const outbound = !org.featuresOff.includes("provisioning");
  const [
    accounts,
    audit,
    applications,
    configs,
    groups,
    directories,
    attributes,
  ] = await Promise.all([
    apiGet<ServiceAccount[]>("/service-accounts"),
    apiGet<AuditEvent[]>("/audit"),
    apiGet<Application[]>("/applications"),
    outbound ? apiGet<AppProvisioning[]>("/provisioning/applications") : [],
    apiGet<Group[]>("/groups"),
    apiGet<LDAPDirectory[]>("/ldap-directories"),
    apiGet<Attribute[]>("/attributes", []),
  ]);
  const connectors = accounts
    .map((account) => ({
      account,
      keys: activeKeys(account.keys).filter((key) =>
        key.scopes.includes("scim"),
      ),
    }))
    .filter(({ keys }) => keys.length > 0);
  const actorNames = Object.fromEntries(accounts.map((a) => [a.id, a.name]));
  const activity = audit
    .filter((event) => event.action.startsWith("scim."))
    .slice(0, 10);
  const byApp = Object.fromEntries(configs.map((c) => [c.appId, c]));
  const apps = applications
    .filter(
      (a) =>
        (a.protocol === "oidc" || a.protocol === "saml") &&
        a.type !== "service",
    )
    .map((a) => ({
      id: a.id,
      name: a.name,
      protocol: a.protocol,
      config: byApp[a.id] ?? null,
    }));
  const on = configs.filter((c) => c.enabled).length;

  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title={nav.provisioning.label}
        description={t.page.description}
      />

      {inbound ? (
        <section className="flex flex-col gap-6">
          <SectionTitle
            title={t.page.inbound}
            description={t.page.inboundHint}
          />
          <div className="grid grid-cols-1 gap-6 xl:grid-cols-12">
            <Card className="min-w-0 xl:col-span-5">
              <CardHeader
                title={t.page.connection}
                description={t.page.connectionHint}
              />
              <CardBody className="flex flex-col gap-6">
                <CopyField
                  label={t.page.scimBaseUrl}
                  value={`${org.issuer}/scim/v2`}
                />
                <DescriptionList
                  items={[
                    {
                      label: t.page.authentication,
                      value: t.page.authenticationValue,
                    },
                    {
                      label: t.page.requiredRole,
                      value: t.page.requiredRoleValue,
                    },
                    {
                      label: t.page.supported,
                      value: t.page.supportedValue,
                    },
                  ]}
                />
              </CardBody>
            </Card>
            <Card className="min-w-0 xl:col-span-7">
              <CardHeader
                title={t.page.connectors}
                description={t.page.connectorsHint}
                actions={
                  <Link
                    href="/admin/service-accounts"
                    className={buttonClasses("quiet", "sm")}
                  >
                    {t.page.manage}
                  </Link>
                }
              />
              {connectors.length === 0 ? (
                <EmptyState
                  icon={Bot}
                  title={t.page.noConnectors}
                  description={t.page.noConnectorsHint}
                  action={
                    <Link
                      href="/admin/service-accounts"
                      className={buttonClasses("secondary", "sm")}
                    >
                      {t.page.openServiceAccounts}
                    </Link>
                  }
                />
              ) : (
                <ul>
                  {connectors.map(({ account, keys }) => {
                    const used = lastUsed(keys);
                    return (
                      <li
                        key={account.id}
                        className="flex items-center gap-4 border-t border-border px-6 py-3"
                      >
                        <span className="flex min-w-0 flex-1 flex-col">
                          <Link
                            href={`/admin/service-accounts/${account.id}`}
                            className="truncate font-medium text-fg hover:underline"
                          >
                            {account.name}
                          </Link>
                          {canProvision(account) ? (
                            <span className="text-caption text-fg-3">
                              {t.page.scimKeys(keys.length)}
                            </span>
                          ) : (
                            <span className="text-caption text-warning">
                              {t.page.missingRole}
                            </span>
                          )}
                        </span>
                        <span className="shrink-0 text-caption text-fg-3">
                          {used
                            ? t.page.used(formatRelative(used, locale).toLowerCase())
                            : t.page.neverUsed}
                        </span>
                      </li>
                    );
                  })}
                </ul>
              )}
            </Card>
          </div>
          <Card>
            <CardHeader
              title={t.page.recent}
              description={t.page.recentHint}
              actions={
                <Link
                  href="/admin/audit"
                  className={buttonClasses("quiet", "sm")}
                >
                  {nav.audit.label}
                </Link>
              }
            />
            <ActivityList
              events={activity}
              actorNames={actorNames}
              empty={t.page.noChanges}
              locale={locale}
            />
          </Card>
        </section>
      ) : null}

      <LDAPDirectories directories={directories} attributes={attributes} />

      {outbound ? (
        <section className="flex flex-col gap-6">
          <SectionTitle
            title={t.page.outbound}
            description={t.page.outboundHint(on, apps.length)}
          />
          <OutboundTable apps={apps} groups={groups} />
        </section>
      ) : null}
    </div>
  );
}
