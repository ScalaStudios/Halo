import Link from "next/link";
import { Bot } from "lucide-react";
import { OutboundTable } from "@/components/provisioning/outbound";
import { ActivityList, activeKeys, canProvision, lastUsed } from "@/components/provisioning/shared";
import { buttonClasses } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { DescriptionList } from "@/components/ui/description-list";
import { EmptyState } from "@/components/ui/empty-state";
import { PageHeader } from "@/components/ui/page-header";
import { SectionTitle } from "@/components/ui/section-title";
import { apiGet } from "@/lib/api/server";
import { formatRelative, pluralize } from "@/lib/format";
import type { AppProvisioning, ServiceAccount } from "@/lib/provisioning-types";
import type { Application, AuditEvent, Organization } from "@/lib/types";

export const metadata = { title: "Provisioning" };

export default async function ProvisioningPage() {
  const [org, accounts, audit, applications, configs] = await Promise.all([
    apiGet<Organization>("/organization"),
    apiGet<ServiceAccount[]>("/service-accounts"),
    apiGet<AuditEvent[]>("/audit"),
    apiGet<Application[]>("/applications"),
    apiGet<AppProvisioning[]>("/provisioning/applications"),
  ]);
  const connectors = accounts
    .map((account) => ({ account, keys: activeKeys(account.keys).filter((key) => key.scopes.includes("scim")) }))
    .filter(({ keys }) => keys.length > 0);
  const actorNames = Object.fromEntries(accounts.map((a) => [a.id, a.name]));
  const activity = audit.filter((event) => event.action.startsWith("scim.")).slice(0, 10);
  const byApp = Object.fromEntries(configs.map((c) => [c.appId, c]));
  const apps = applications
    .filter((a) => (a.protocol === "oidc" || a.protocol === "saml") && a.type !== "service")
    .map((a) => ({ id: a.id, name: a.name, protocol: a.protocol, config: byApp[a.id] ?? null }));
  const on = configs.filter((c) => c.enabled).length;

  return (
    <div className="flex animate-page flex-col gap-8">
      <PageHeader
        title="Provisioning"
        description="SCIM 2.0 in both directions: your HR system or identity provider keeps people in Halo current, and Halo keeps accounts in your applications current."
      />

      <section className="flex flex-col gap-6">
        <SectionTitle title="Inbound" description="Workday, BambooHR, Okta or Microsoft Entra ID create, update and deactivate people and groups in Halo." />
        <div className="grid grid-cols-1 gap-6 xl:grid-cols-12">
          <Card className="min-w-0 xl:col-span-5">
            <CardHeader title="Connection details" description="Enter these in the provisioning settings of the source system." />
            <CardBody className="flex flex-col gap-6">
              <CopyField label="SCIM base URL" value={`${org.issuer}/scim/v2`} />
              <DescriptionList
                items={[
                  { label: "Authentication", value: "Bearer token: an API key with the scim scope" },
                  { label: "Required role", value: "User administrator on the key's service account" },
                  { label: "Supported", value: "Users and assigned groups, PATCH, filters on userName, externalId and emails, enterprise department and manager" },
                ]}
              />
            </CardBody>
          </Card>
          <Card className="min-w-0 xl:col-span-7">
            <CardHeader
              title="Connectors"
              description="Service accounts with an active SCIM key."
              actions={
                <Link href="/admin/service-accounts" className={buttonClasses("quiet", "sm")}>
                  Manage
                </Link>
              }
            />
            {connectors.length === 0 ? (
              <EmptyState
                icon={Bot}
                title="No connectors yet"
                description="Create a service account with the User administrator role, then give it an API key with the scim scope."
                action={
                  <Link href="/admin/service-accounts" className={buttonClasses("secondary", "sm")}>
                    Open service accounts
                  </Link>
                }
              />
            ) : (
              <ul>
                {connectors.map(({ account, keys }) => {
                  const used = lastUsed(keys);
                  return (
                    <li key={account.id} className="flex items-center gap-4 border-t border-border px-6 py-3">
                      <span className="flex min-w-0 flex-1 flex-col">
                        <Link href={`/admin/service-accounts/${account.id}`} className="truncate font-medium text-fg hover:underline">
                          {account.name}
                        </Link>
                        {canProvision(account) ? (
                          <span className="text-caption text-fg-3">{pluralize(keys.length, "SCIM key")}</span>
                        ) : (
                          <span className="text-caption text-warning">Missing the User administrator role, so its requests are rejected</span>
                        )}
                      </span>
                      <span className="shrink-0 text-caption text-fg-3">{used ? `Used ${formatRelative(used).toLowerCase()}` : "Never used"}</span>
                    </li>
                  );
                })}
              </ul>
            )}
          </Card>
        </div>
        <Card>
          <CardHeader
            title="Recent SCIM changes"
            description="The latest changes connectors made in Halo."
            actions={
              <Link href="/admin/audit" className={buttonClasses("quiet", "sm")}>
                Audit log
              </Link>
            }
          />
          <ActivityList events={activity} actorNames={actorNames} empty="No SCIM changes yet. They appear here as soon as a connector creates or updates someone." />
        </Card>
      </section>

      <section className="flex flex-col gap-6">
        <SectionTitle
          title="Outbound"
          description={`Every 2 minutes Halo creates accounts for people assigned to an application, updates changed profiles and deactivates people who lose access or are suspended. ${on} of ${pluralize(apps.length, "application")} on.`}
        />
        <OutboundTable apps={apps} />
      </section>
    </div>
  );
}
