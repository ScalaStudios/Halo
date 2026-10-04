import Link from "next/link";
import { ArrowLeft, MonitorSmartphone, ScrollText, UsersRound } from "lucide-react";
import { AppActions, DangerZone } from "@/components/applications/app-actions";
import { CredentialList } from "@/components/applications/credentials";
import { AuthenticationForm, ScopesForm } from "@/components/applications/edit-application";
import { SamlAttributesForm, SamlConnectionFields, SamlSettingsForm } from "@/components/applications/saml";
import { APP_STATUS, PROTOCOL_LABEL, SetupGuideCard, TYPE_ICON, TYPE_LABEL } from "@/components/applications/shared";
import { AddToGroup, RemoveFromGroup } from "@/components/directory/add-to-group";
import { SessionsTable } from "@/components/monitoring/sessions-table";
import { SignInRows } from "@/components/monitoring/sign-in-rows";
import { Badge, StatusDot, Tag } from "@/components/ui/badge";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { DescriptionList } from "@/components/ui/description-list";
import { EmptyState } from "@/components/ui/empty-state";
import { SectionTitle } from "@/components/ui/section-title";
import { SimpleTable } from "@/components/ui/simple-table";
import { TabNav } from "@/components/ui/tab-nav";
import { apiGet } from "@/lib/api/server";
import { formatDuration, pluralize } from "@/lib/format";
import type { SamlApplication } from "@/lib/saml-types";
import { endpoints, setupGuide } from "@/lib/setup-guides";
import type { Application, Group, Organization, Session, SignInEvent, User } from "@/lib/types";

export async function generateMetadata({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return { title: (await apiGet<Application>(`/applications/${encodeURIComponent(id)}`)).name };
}

export default async function ApplicationPage({ params, searchParams }: { params: Promise<{ id: string }>; searchParams: Promise<{ tab?: string }> }) {
  const { id } = await params;
  const { tab = "overview" } = await searchParams;
  const [app, groups, users, sessions, appSignIns, org] = await Promise.all([
    apiGet<SamlApplication>(`/applications/${encodeURIComponent(id)}`),
    apiGet<Group[]>("/groups"),
    apiGet<User[]>("/users"),
    apiGet<Session[]>("/sessions"),
    apiGet<SignInEvent[]>(`/sign-ins?app=${encodeURIComponent(id)}`),
    apiGet<Organization>("/organization"),
  ]);

  const saml = app.protocol === "saml";
  const service = app.type === "service";
  const owner = users.find((u) => u.id === app.owner);
  const appGroups = groups.filter((g) => app.groupIds.includes(g.id));
  const assigned = new Set(users.filter((u) => u.appIds.includes(app.id)).map((u) => u.id));
  const appSessions = service ? [] : sessions.filter((s) => assigned.has(s.userId)).sort((a, b) => b.lastActiveAt.localeCompare(a.lastActiveAt));
  const TypeIcon = TYPE_ICON[app.type];

  const tabs = [
    { id: "overview", label: "Overview" },
    { id: "authentication", label: "Authentication" },
    { id: "permissions", label: "Permissions", count: app.scopes.length },
    { id: "claims", label: saml ? "Attributes" : "Claims", count: app.claims.length },
    { id: "users", label: "Users & groups", count: appGroups.length },
    { id: "credentials", label: "Credentials", count: app.credentials.length },
    { id: "sessions", label: "Sessions", count: appSessions.length },
    { id: "logs", label: "Logs", count: appSignIns.length },
  ];

  return (
    <div className="flex animate-page flex-col gap-8">
      <div className="flex flex-col gap-6">
        <Link href="/admin/applications" className="inline-flex w-fit items-center gap-2 rounded-sm text-body-sm text-fg-3 transition-colors duration-fast hover:text-fg">
          <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
          Applications
        </Link>
        <header className="flex flex-col gap-6 lg:flex-row lg:items-start lg:justify-between">
          <div className="flex min-w-0 items-center gap-4">
            <span className="grid size-16 shrink-0 place-items-center rounded-lg border border-border bg-sunken text-fg-3">
              <TypeIcon aria-hidden="true" size={24} strokeWidth={1.75} />
            </span>
            <div className="flex min-w-0 flex-col gap-1">
              <div className="flex flex-wrap items-center gap-3">
                <h1 className="text-h2 text-fg">{app.name}</h1>
                <Badge tone={APP_STATUS[app.status].tone} dot>
                  {APP_STATUS[app.status].label}
                </Badge>
              </div>
              <p className="text-body text-fg-3">
                {PROTOCOL_LABEL[app.protocol]}
                {app.description ? ` · ${app.description}` : null}
              </p>
            </div>
          </div>
          <AppActions
            app={{
              id: app.id,
              name: app.name,
              description: app.description,
              owner: app.owner,
              clientId: app.clientId,
              homepage: app.homepage,
              saml,
              disabled: app.status === "disabled",
              hasSecret: !saml && (app.type === "web" || app.type === "service"),
            }}
            directory={users}
          />
        </header>
        <DescriptionList
          className="grid-cols-2 rounded-lg border border-border bg-surface p-6 md:grid-cols-3 xl:grid-cols-6"
          items={[
            { label: "Protocol", value: PROTOCOL_LABEL[app.protocol] },
            { label: "Type", value: TYPE_LABEL[app.type] },
            {
              label: "Status",
              value: (
                <span className="inline-flex items-center gap-2">
                  <StatusDot tone={APP_STATUS[app.status].tone} />
                  {APP_STATUS[app.status].label}
                </span>
              ),
            },
            { label: "Users", value: <span className="tnum">{app.userCount}</span> },
            {
              label: "Sign-ins (7 days)",
              value: (
                <span className="tnum">
                  {app.signIns7d.toLocaleString("en-US")} <span className="text-fg-3">· {app.failureRate7d}% failed</span>
                </span>
              ),
            },
            {
              label: "Owner",
              value: owner ? (
                <Link href={`/admin/users/${owner.id}`} className="text-link underline underline-offset-3 hover:text-ember">
                  {owner.name}
                </Link>
              ) : (
                "—"
              ),
            },
          ]}
        />
      </div>

      <TabNav label="Application sections" tabs={tabs} active={tab} hrefFor={(t) => `/admin/applications/${app.id}${t === "overview" ? "" : `?tab=${t}`}`} />

      <div key={tab} className="animate-enter">
        {tab === "overview" ? <OverviewTab app={app} issuer={org.issuer} /> : null}
        {tab === "authentication" ? <AuthenticationTab app={app} /> : null}
        {tab === "permissions" ? (
          <div className="flex flex-col gap-6">
            <SectionTitle title="Scopes" description={saml ? undefined : `Scopes ${app.name} can request. Requests for any other scope are rejected.`} />
            {!saml ? (
              <ScopesForm key={app.scopes.join()} app={app} />
            ) : (
              <EmptyState title="SAML applications don't use scopes" description={`${app.name} receives the attributes listed on the Attributes tab in every assertion.`} />
            )}
          </div>
        ) : null}
        {tab === "claims" && app.saml ? (
          <div className="flex flex-col gap-6">
            <SectionTitle title="Attributes" description={`Sent in every SAML assertion to ${app.name}. Rename them to match what ${app.name} expects.`} />
            <SamlAttributesForm appId={app.id} appName={app.name} saml={app.saml} />
          </div>
        ) : null}
        {tab === "claims" && !app.saml ? (
          <div className="flex flex-col gap-6">
            <SectionTitle
              title={saml ? "Attributes" : "Claims"}
              description={
                saml
                  ? `Sent in every SAML assertion to ${app.name}.`
                  : service
                    ? `Included in access tokens issued to ${app.name}.`
                    : `Included in ID tokens and the userinfo response for ${app.name}.`
              }
            />
            <SimpleTable
              caption={saml ? "Attributes" : "Claims"}
              head={[saml ? "Attribute" : "Claim", "Source"]}
              rows={app.claims.map((claim) => [
                <span key="n" className="font-mono text-code-sm break-all text-fg">
                  {claim.name}
                </span>,
                <span key="s" className="font-mono text-code-sm text-fg-2">
                  {claim.source}
                </span>,
              ])}
            />
          </div>
        ) : null}
        {tab === "users" ? (
          <div className="flex flex-col gap-6">
            <SectionTitle
              title="Assigned groups"
              description={
                service
                  ? `${app.name} authenticates as itself with client credentials. No people or groups are assigned.`
                  : `${pluralize(app.userCount, "person", "people")} can sign in to ${app.name} through ${pluralize(appGroups.length, "group")}.`
              }
              action={
                service ? undefined : (
                  <AddToGroup name={app.name} appId={app.id} assigned={app.groupIds} groups={groups.filter((g) => !app.groupIds.includes(g.id))} />
                )
              }
            />
            {appGroups.length ? (
              <SimpleTable
                caption="Assigned groups"
                head={["Group", "Membership", "Source", "Members", "Actions"]}
                rows={appGroups.map((group) => [
                  <span key="n" className="flex flex-col">
                    <span className="font-medium text-fg">{group.name}</span>
                    <span className="text-caption text-fg-3">{group.description}</span>
                  </span>,
                  group.kind === "dynamic" ? <Tag key="k">{group.rule}</Tag> : "Assigned",
                  group.source,
                  <span key="m" className="tnum">
                    {group.memberCount}
                  </span>,
                  <RemoveFromGroup key="r" name={app.name} appId={app.id} assigned={app.groupIds} group={group} />,
                ])}
              />
            ) : (
              <EmptyState
                icon={UsersRound}
                title="No groups assigned"
                description={service ? "Service applications get access through scopes, not group membership." : `Nobody can sign in to ${app.name} until a group is assigned.`}
              />
            )}
          </div>
        ) : null}
        {tab === "credentials" ? (
          <div className="flex flex-col gap-6">
            <SectionTitle
              title="Credentials"
              description={
                app.credentials.length === 0
                  ? undefined
                  : saml
                    ? `Certificates recorded for ${app.name}. Halo signs its assertions with the organization's SAML signing certificate, shown on the Overview tab.`
                    : `${app.name} uses these secrets to authenticate to Halo. Rotate a secret before it expires; the old one keeps working for 24 hours.`
              }
            />
            <CredentialList
              appId={app.id}
              appName={app.name}
              credentials={app.credentials}
              empty={
                saml
                  ? `Halo signs assertions to ${app.name} with the organization's SAML signing certificate, shown on the Overview tab. SAML applications need no other credentials.`
                  : app.type === "native" || app.type === "spa"
                    ? `${app.name} is a public client. It signs in with PKCE, so there is no secret to rotate.`
                    : `${app.name} has no active credentials and cannot authenticate to Halo.`
              }
            />
          </div>
        ) : null}
        {tab === "sessions" ? (
          <div className="flex flex-col gap-6">
            <SectionTitle
              title="Sessions"
              description={
                service
                  ? undefined
                  : `Halo sessions of people assigned to ${app.name}, most recently active first. Revoking a session signs that device out of Halo.`
              }
            />
            {service ? (
              <EmptyState
                icon={MonitorSmartphone}
                title="No user sessions"
                description={`${app.name} authenticates as itself with client credentials. Its access tokens last ${formatDuration(app.tokenPolicy.accessTokenTtl)} and it never holds a user session.`}
              />
            ) : (
              <SessionsTable sessions={appSessions} userNames={Object.fromEntries(users.map((u) => [u.id, u.name]))} />
            )}
          </div>
        ) : null}
        {tab === "logs" ? (
          <div className="flex flex-col gap-6">
            <SectionTitle title="Sign-ins" description="Newest first." />
            {appSignIns.length ? (
              <Card>
                <SignInRows events={appSignIns} appNames={{ [app.id]: app.name }} />
              </Card>
            ) : (
              <EmptyState
                icon={ScrollText}
                title="No sign-ins yet"
                description={
                  service
                    ? `${app.name} requests tokens with client credentials. Those requests are not sign-ins, so none appear here.`
                    : app.status === "disabled"
                      ? `${app.name} is disabled, so nobody can sign in to it.`
                      : `Sign-ins to ${app.name} appear here within a minute.`
                }
              />
            )}
          </div>
        ) : null}
      </div>
    </div>
  );
}

function OverviewTab({ app, issuer }: { app: SamlApplication; issuer: string }) {
  const saml = app.protocol === "saml";
  const urls = endpoints(issuer);
  return (
    <div className="grid grid-cols-1 gap-6 xl:grid-cols-12">
      <Card className="min-w-0 xl:col-span-5">
        <CardHeader
          title="Connection details"
          description={saml ? `What ${app.name} needs to trust Halo as its identity provider.` : `What ${app.name} needs to sign in with Halo.`}
        />
        <CardBody className="flex flex-col gap-4">
          {app.saml ? <SamlConnectionFields idp={app.saml.idp} /> : null}
          <CopyField label={saml ? "SP entity ID" : "Client ID"} value={app.clientId} />
          {saml ? null : (
            <>
              <CopyField label="Issuer" value={urls.issuer} />
              <CopyField label="Discovery endpoint" value={urls.discovery} />
            </>
          )}
          {app.type === "service" ? <CopyField label="Token endpoint" value={urls.token} /> : null}
          {app.redirectUris.map((uri, index) => (
            <CopyField
              key={uri}
              label={saml ? "ACS URL" : app.redirectUris.length > 1 ? `Redirect URI ${index + 1}` : "Redirect URI"}
              value={uri}
            />
          ))}
        </CardBody>
      </Card>
      <div id="setup-guide" className="min-w-0 scroll-mt-24 xl:col-span-7">
        <SetupGuideCard guide={setupGuide(app, issuer)} />
      </div>
      <DangerZone id={app.id} name={app.name} disabled={app.status === "disabled"} className="xl:col-span-12" />
    </div>
  );
}

function AuthenticationTab({ app }: { app: SamlApplication }) {
  if (app.saml) {
    return <SamlSettingsForm appId={app.id} appName={app.name} saml={app.saml} />;
  }

  const service = app.type === "service";
  const publicClient = app.type === "spa" || app.type === "native";
  const grants = service ? ["client_credentials"] : app.tokenPolicy.refreshTokenTtl > 0 ? ["authorization_code", "refresh_token"] : ["authorization_code"];

  return (
    <AuthenticationForm key={JSON.stringify([app.redirectUris, app.postLogoutUris, app.tokenPolicy])} app={app}>
      <Card>
        <CardHeader title="Grants" description="How the application gets tokens from Halo." />
        <CardBody>
          <DescriptionList
            items={[
              {
                label: "Grant types",
                value: (
                  <span className="flex flex-wrap gap-2">
                    {grants.map((grant) => (
                      <Tag key={grant}>{grant}</Tag>
                    ))}
                  </span>
                ),
              },
              { label: "PKCE", value: service ? "Not used. Client credentials has no browser redirect." : "Required, S256" },
              { label: "Client authentication", value: publicClient ? "None. Public client without a secret." : "Client secret, client_secret_basic" },
            ]}
          />
        </CardBody>
      </Card>
    </AuthenticationForm>
  );
}
