import Link from "next/link";
import { ArrowLeft, MonitorSmartphone, ScrollText, UsersRound } from "lucide-react";
import { AvatarEditor } from "@/components/account/avatar-editor";
import { AppActions, DangerZone } from "@/components/applications/app-actions";
import { AuthorizationCheckForm, AuthorizationModelForm } from "@/components/applications/authorization";
import { ClaimMappingsForm } from "@/components/applications/claim-mappings";
import { CredentialList } from "@/components/applications/credentials";
import { AuthenticationForm, BackchannelLogoutForm, ProtectedAddressesForm, ScopesForm, SenderConstraintForm, SignInFrequencyForm, WindowsSignInForm } from "@/components/applications/edit-application";
import { HookForm } from "@/components/applications/hook-form";
import { RolesTable } from "@/components/applications/roles";
import { SamlAttributesForm, SamlConnectionFields, SamlSettingsForm } from "@/components/applications/saml";
import { SharedSignals } from "@/components/applications/shared-signals";
import { APP_STATUS, LdapConnectionFields, PROTOCOL_LABEL, RadiusServerFields, SetupGuideCard, statusKey, TYPE_LABEL } from "@/components/applications/shared";
import { AddToGroup, RemoveFromGroup } from "@/components/directory/add-to-group";
import { SessionsTable } from "@/components/monitoring/sessions-table";
import { DeliveryLog } from "@/components/settings/webhooks";
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
import type { ApiResource } from "@/lib/infra-types";
import type { SamlApplication } from "@/lib/saml-types";
import type { OrganizationProfile, WebhookDelivery } from "@/lib/settings-types";
import { endpoints, proxyGuides, setupGuide } from "@/lib/setup-guides";
import type { Application, ApplicationHook, Attribute, AuthorizationModel, Group, Session, SignalReceiver, SignInEvent, User } from "@/lib/types";

export async function generateMetadata({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return { title: (await apiGet<Application>(`/applications/${encodeURIComponent(id)}`)).name };
}

export default async function ApplicationPage({ params, searchParams }: { params: Promise<{ id: string }>; searchParams: Promise<{ tab?: string }> }) {
  const { id } = await params;
  const { tab = "overview" } = await searchParams;
  const [app, groups, users, sessions, appSignIns, org, resources, hook, attributes, logouts, signals, model] = await Promise.all([
    apiGet<SamlApplication>(`/applications/${encodeURIComponent(id)}`),
    apiGet<Group[]>("/groups"),
    apiGet<User[]>("/users"),
    apiGet<Session[]>("/sessions"),
    apiGet<SignInEvent[]>(`/sign-ins?app=${encodeURIComponent(id)}`),
    apiGet<OrganizationProfile>("/organization"),
    tab === "authentication" ? apiGet<ApiResource[]>("/api-resources") : [],
    tab === "authentication" ? apiGet<ApplicationHook | null>(`/applications/${encodeURIComponent(id)}/hook`) : null,
    tab === "claims" ? apiGet<Attribute[]>("/attributes") : [],
    tab === "authentication" ? apiGet<WebhookDelivery[]>(`/applications/${encodeURIComponent(id)}/logout-deliveries`) : [],
    tab === "authentication" ? apiGet<{ receiver: SignalReceiver | null; deliveries: WebhookDelivery[] }>(`/applications/${encodeURIComponent(id)}/shared-signals`) : null,
    tab === "authorization" ? apiGet<AuthorizationModel>(`/applications/${encodeURIComponent(id)}/authorization`) : null,
  ]);

  const saml = app.protocol === "saml";
  const bookmark = app.protocol === "bookmark";
  const forward = app.protocol === "forward-auth";
  const radius = app.protocol === "radius";
  const ldap = app.protocol === "ldap";
  const service = app.type === "service";
  const hookable = !saml && !bookmark && !forward && !radius && !ldap && !service && app.clientId !== "halo-cli";
  const roleable = !bookmark && !radius && !ldap && !service && app.clientId !== "halo-cli";
  const modelled = (app.protocol === "oidc" || app.protocol === "oauth") && (app.type === "web" || service) && app.clientId !== "halo-cli";
  const constrainable = (app.protocol === "oidc" || app.protocol === "oauth") && app.clientId !== "halo-cli";
  const windows = !bookmark && !radius && !ldap && !service && !org.featuresOff.includes("kerberos");
  const owner = users.find((u) => u.id === app.owner);
  const appGroups = groups.filter((g) => app.groupIds.includes(g.id));
  const assigned = new Set(users.filter((u) => u.appIds.includes(app.id)).map((u) => u.id));
  const appSessions = service ? [] : sessions.filter((s) => assigned.has(s.userId)).sort((a, b) => b.lastActiveAt.localeCompare(a.lastActiveAt));

  const tabs = bookmark
    ? [
        { id: "overview", label: "Overview" },
        { id: "users", label: "Users & groups", count: appGroups.length },
      ]
    : radius
      ? [
          { id: "overview", label: "Overview" },
          { id: "authentication", label: "Authentication" },
          { id: "users", label: "Users & groups", count: appGroups.length },
          { id: "credentials", label: "Credentials", count: app.credentials.length },
          { id: "logs", label: "Logs", count: appSignIns.length },
        ]
    : ldap
      ? [
          { id: "overview", label: "Overview" },
          { id: "users", label: "Users & groups", count: appGroups.length },
          { id: "logs", label: "Logs", count: appSignIns.length },
        ]
    : forward
      ? [
          { id: "overview", label: "Overview" },
          { id: "authentication", label: "Authentication" },
          { id: "claims", label: "Headers", count: app.claims.length },
          { id: "users", label: "Users & groups", count: appGroups.length },
          { id: "roles", label: "Roles", count: app.roles.length },
          { id: "sessions", label: "Sessions", count: appSessions.length },
          { id: "logs", label: "Logs", count: appSignIns.length },
        ]
      : [
        { id: "overview", label: "Overview" },
        { id: "authentication", label: "Authentication" },
        { id: "permissions", label: "Permissions", count: app.scopes.length },
        { id: "claims", label: saml ? "Attributes" : "Claims", count: app.claims.length },
        { id: "users", label: "Users & groups", count: appGroups.length },
        ...(roleable ? [{ id: "roles", label: "Roles", count: app.roles.length }] : []),
        ...(modelled ? [{ id: "authorization", label: "Authorization" }] : []),
        { id: "credentials", label: "Credentials", count: app.credentials.length },
        { id: "sessions", label: "Sessions", count: appSessions.length },
        { id: "logs", label: "Logs", count: appSignIns.length },
      ];

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-6">
        <Link href="/admin/applications" className="inline-flex w-fit items-center gap-2 rounded-sm text-body-sm text-fg-3 transition-colors duration-fast hover:text-fg">
          <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
          Applications
        </Link>
        <header className="flex flex-col gap-6 2xl:flex-row 2xl:items-center 2xl:justify-between">
          <AvatarEditor name={app.name} avatarUrl={app.logoUrl} endpoint={`/api/v1/applications/${app.id}/logo`} size="xl" logo className="min-w-0 flex-1">
            <div className="flex min-w-0 flex-col gap-1">
              <div className="flex flex-wrap items-center gap-3">
                <h1 className="text-h2 text-fg">{app.name}</h1>
                <Badge tone={APP_STATUS[statusKey(app)].tone} dot>
                  {APP_STATUS[statusKey(app)].label}
                </Badge>
                {app.registeredDynamically ? <Badge tone="info">Registered dynamically</Badge> : null}
              </div>
              <p className="text-body text-fg-3">
                {PROTOCOL_LABEL[app.protocol]}
                {app.description ? ` · ${app.description}` : null}
              </p>
            </div>
          </AvatarEditor>
          <AppActions
            app={{
              id: app.id,
              name: app.name,
              description: app.description,
              owner: app.owner,
              clientId: app.clientId,
              homepage: app.homepage,
              saml,
              bookmark,
              radius,
              ldap,
              disabled: app.status === "disabled",
              awaitingApproval: app.awaitingApproval,
              redirectUris: app.redirectUris,
              postLogoutUris: app.postLogoutUris,
              hasSecret: ldap || (!saml && !bookmark && !forward && (app.type === "web" || app.type === "service")),
            }}
            directory={users}
          />
        </header>
        <DescriptionList
          className={`grid-cols-2 rounded-lg border border-border bg-surface p-6 md:grid-cols-3 ${bookmark ? "xl:grid-cols-4" : forward || radius || ldap ? "xl:grid-cols-5" : "xl:grid-cols-6"}`}
          items={[
            { label: "Protocol", value: PROTOCOL_LABEL[app.protocol] },
            ...(bookmark || forward || radius || ldap ? [] : [{ label: "Type", value: TYPE_LABEL[app.type] }]),
            {
              label: "Status",
              value: (
                <span className="inline-flex items-center gap-2">
                  <StatusDot tone={APP_STATUS[statusKey(app)].tone} />
                  {APP_STATUS[statusKey(app)].label}
                </span>
              ),
            },
            { label: "Users", value: <span className="tnum">{app.userCount}</span> },
            ...(bookmark
              ? []
              : [
                  {
                    label: "Sign-ins (7 days)",
                    value: (
                      <span className="tnum">
                        {app.signIns7d.toLocaleString("en-US")} <span className="text-fg-3">· {app.failureRate7d}% failed</span>
                      </span>
                    ),
                  },
                ]),
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
        {tab === "authentication" ? (
          <div className="flex flex-col gap-6">
            {forward || radius ? <ProtectedAddressesForm key={app.redirectUris.join()} app={app} /> : <AuthenticationTab app={app} resources={resources} />}
            {constrainable ? <SenderConstraintForm key={`${app.requirePar}:${app.requirePkce}:${app.requireDpop}`} app={app} /> : null}
            {service ? null : <SignInFrequencyForm key={app.maxAuthAge} app={app} />}
            {windows ? <WindowsSignInForm key={String(app.kerberosSignIn)} app={app} /> : null}
            {hookable ? <HookForm key={JSON.stringify(hook)} appId={app.id} appName={app.name} hook={hook} /> : null}
            {hookable ? <BackchannelLogoutForm key={`${app.backchannelLogoutUri}:${app.backchannelLogoutSessionRequired}`} app={app} /> : null}
            {signals && service ? (
              <SharedSignals key={JSON.stringify(signals.receiver)} appId={app.id} appName={app.name} issuer={org.issuer} receiver={signals.receiver} deliveries={signals.deliveries} />
            ) : null}
            {logouts.length > 0 ? (
              <section className="flex flex-col gap-6">
                <SectionTitle title="Logout deliveries" description={`The latest ${pluralize(logouts.length, "logout")}, newest first. Each one is a Halo session that ended.`} />
                <DeliveryLog deliveries={logouts} payloadNote="The session the logout token ends. Halo signs a new token with it on every attempt." />
              </section>
            ) : null}
          </div>
        ) : null}
        {tab === "permissions" ? (
          <div className="flex flex-col gap-6">
            <SectionTitle title="Scopes" description={saml ? undefined : `Scopes ${app.name} can request. Requests for any other scope are rejected.`} />
            {!saml ? (
              <ScopesForm key={`${app.scopes.join()}:${app.requiresConsent}`} app={app} />
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
              title={saml ? "Attributes" : forward ? "Headers" : "Claims"}
              description={
                saml
                  ? `Sent in every SAML assertion to ${app.name}.`
                  : forward
                    ? `Sent to the proxy with every request Halo lets through to ${app.name}. The proxy replaces any copy the browser sent and passes them on.`
                    : service
                    ? `Included in access tokens issued to ${app.name}.`
                    : `Included in ID tokens and the userinfo response for ${app.name}.`
              }
            />
            <SimpleTable
              caption={saml ? "Attributes" : forward ? "Headers" : "Claims"}
              head={[saml ? "Attribute" : forward ? "Header" : "Claim", "Source"]}
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
        {tab === "claims" && !service ? (
          <div className="mt-12 flex flex-col gap-6">
            <SectionTitle
              title={saml ? "Attribute mappings" : forward ? "Header mappings" : "Claim mappings"}
              description={
                saml
                  ? `Send a profile field, group names or a custom attribute to ${app.name} under the name it expects. A mapping replaces the attribute above with the same name, and Not sent leaves it out.`
                  : forward
                    ? `Send a profile field, group names or a custom attribute in the header ${app.name} expects, like Remote-User. A mapping replaces the header above with the same name, and Not sent leaves it out. Update the proxy configuration after you add a header.`
                    : `Add a profile field, group names or a custom attribute to ID tokens and userinfo for ${app.name}. A mapping replaces the claim above with the same name, and Not sent leaves it out. Halo's protocol claims, such as sub and iss, can't be mapped.`
              }
            />
            <ClaimMappingsForm
              key={JSON.stringify(app.claimMappings)}
              appId={app.id}
              appName={app.name}
              kind={saml ? "attribute" : forward ? "header" : "claim"}
              mappings={app.claimMappings}
              attributes={attributes}
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
                  : `${pluralize(app.userCount, "person", "people")} ${bookmark ? "see" : "can sign in to"} ${app.name} through ${pluralize(appGroups.length, "group")}.`
              }
              action={
                service ? undefined : (
                  <AddToGroup name={app.name} appId={app.id} assigned={app.groupIds} groups={groups} bookmark={bookmark} />
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
                  <RemoveFromGroup key="r" name={app.name} appId={app.id} assigned={app.groupIds} group={group} bookmark={bookmark} />,
                ])}
              />
            ) : (
              <EmptyState
                icon={UsersRound}
                title="No groups assigned"
                description={
                  service
                    ? "Service applications get access through scopes, not group membership."
                    : bookmark
                      ? `Nobody sees ${app.name} in their account portal until a group is assigned.`
                      : `Nobody can sign in to ${app.name} until a group is assigned.`
                }
              />
            )}
          </div>
        ) : null}
        {tab === "roles" && roleable ? (
          <div className="flex flex-col gap-6">
            <SectionTitle
              title="Roles"
              description={`Halo sends the keys of the roles a person holds to ${app.name} ${saml ? "in the roles attribute of every assertion" : forward ? "in the X-Halo-Roles header" : "in the roles claim of ID tokens, userinfo and access tokens"}. Rename it with a mapping on the ${saml ? "Attributes" : forward ? "Headers" : "Claims"} tab.`}
            />
            <RolesTable
              appId={app.id}
              appName={app.name}
              roles={app.roles}
              groups={groups}
              people={users.filter((u) => u.status !== "deprovisioned").map(({ id, name, email }) => ({ id, name, email }))}
            />
          </div>
        ) : null}
        {tab === "authorization" && modelled && model ? (
          <div className="flex flex-col gap-6">
            <SectionTitle
              title="Authorization"
              description={`${app.name} writes relationships such as document:readme#viewer@user:usr_… and asks Halo whether a person has a relation on an object. It calls the routes under this address with HTTP Basic: its client ID and one of its client secrets. Groups expand to their members, and deleting a person or group deletes the relationships that name them.`}
            />
            <CopyField label="Authorization API" value={`${org.issuer}/api/v1/authz`} />
            <AuthorizationModelForm appId={app.id} appName={app.name} model={model} />
            {model.updatedAt ? (
              <AuthorizationCheckForm appId={app.id} people={users.filter((u) => u.status !== "deprovisioned").map(({ id, name, email }) => ({ id, name, email }))} />
            ) : null}
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
                    : radius
                      ? `${app.name} signs its RADIUS requests with these shared secrets. Rotate a secret before it expires, or Halo stops answering ${app.name}; the old one keeps working for 24 hours.`
                      : `${app.name} uses these secrets to authenticate to Halo. Rotate a secret before it expires; the old one keeps working for 24 hours.`
              }
            />
            <CredentialList
              appId={app.id}
              appName={app.name}
              credentials={app.credentials}
              label={radius ? "Shared secret" : undefined}
              empty={
                saml
                  ? `Halo signs assertions to ${app.name} with the organization's SAML signing certificate, shown on the Overview tab. SAML applications need no other credentials.`
                  : app.type === "native" || app.type === "spa"
                    ? `${app.name} is a public client. It signs in with PKCE, so there is no secret to rotate.`
                    : radius
                      ? `${app.name} has no active shared secret, so Halo ignores its RADIUS requests. Rotate one from the actions above.`
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
  if (app.protocol === "bookmark") {
    return (
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-12">
        <Card className="min-w-0 xl:col-span-5">
          <CardHeader title="Bookmark" description={`Opens from the account portal of everyone assigned to ${app.name}. Nobody signs in to it through Halo.`} />
          <CardBody>
            <CopyField label="URL" value={app.homepage} />
          </CardBody>
        </Card>
        <DangerZone id={app.id} name={app.name} disabled={app.status === "disabled"} bookmark className="xl:col-span-12" />
      </div>
    );
  }
  if (app.protocol === "radius") {
    return (
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-12">
        <Card className="min-w-0 xl:col-span-5">
          <CardHeader
            title="Connection details"
            description={`Enter these values in the RADIUS settings of ${app.name}. Use PAP, and send a Message-Authenticator with every request: Halo ignores requests without one. The shared secret is on the Credentials tab.`}
          />
          <CardBody className="flex flex-col gap-4">
            <RadiusServerFields issuer={issuer} port={app.radiusPort} />
            {app.redirectUris.map((address, index) => (
              <CopyField key={address} label={app.redirectUris.length > 1 ? `Client address ${index + 1}` : "Client address"} value={address} />
            ))}
          </CardBody>
        </Card>
        <DangerZone id={app.id} name={app.name} disabled={app.status === "disabled"} className="xl:col-span-12" />
      </div>
    );
  }
  if (app.ldap) {
    return (
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-12">
        <Card className="min-w-0 xl:col-span-5">
          <CardHeader title="Connection details" description={`What ${app.name} needs to read its people and groups from Halo over LDAP. It sees only the people in its assigned groups.`} />
          <CardBody className="flex flex-col gap-4">
            <LdapConnectionFields ldap={app.ldap} />
          </CardBody>
        </Card>
        <DangerZone id={app.id} name={app.name} disabled={app.status === "disabled"} className="xl:col-span-12" />
      </div>
    );
  }
  if (app.protocol === "forward-auth") {
    return (
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-12">
        <Card className="min-w-0 xl:col-span-5">
          <CardHeader title="Connection details" description={`What the reverse proxy in front of ${app.name} needs to ask Halo before every request.`} />
          <CardBody className="flex flex-col gap-4">
            <CopyField
              label="Verify endpoint"
              value={`${issuer}/forward-auth/${app.id}/verify`}
              hint="Halo reads X-Forwarded-Host and X-Forwarded-Uri only from addresses in HALO_TRUSTED_PROXIES. With more than one protected address, point the proxy at Halo's server directly."
            />
            {app.redirectUris.map((uri, index) => (
              <CopyField key={uri} label={app.redirectUris.length > 1 ? `Protected address ${index + 1}` : "Protected address"} value={uri} />
            ))}
            <CopyField label="Sign-out path" value="/_halo/sign-out" hint={`Link to it on ${app.name}'s own address. It ends the person's Halo session.`} />
          </CardBody>
        </Card>
        <div id="setup-guide" className="flex min-w-0 scroll-mt-24 flex-col gap-6 xl:col-span-7">
          {proxyGuides(app, issuer).map((guide) => (
            <SetupGuideCard key={guide.title} guide={guide} />
          ))}
        </div>
        <DangerZone id={app.id} name={app.name} disabled={app.status === "disabled"} className="xl:col-span-12" />
      </div>
    );
  }
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

function AuthenticationTab({ app, resources }: { app: SamlApplication; resources: ApiResource[] }) {
  if (app.saml) {
    return <SamlSettingsForm appId={app.id} appName={app.name} saml={app.saml} />;
  }

  const service = app.type === "service";
  const publicClient = app.type === "spa" || app.type === "native";
  const grants = service ? ["client_credentials"] : app.tokenPolicy.refreshTokenTtl > 0 ? ["authorization_code", "refresh_token"] : ["authorization_code"];

  return (
    <AuthenticationForm key={JSON.stringify([app.redirectUris, app.postLogoutUris, app.tokenPolicy])} app={app} resources={resources}>
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
              ...(app.tokenPolicy.exchange ? [{ label: "Token exchange", value: "urn:ietf:params:oauth:grant-type:token-exchange", mono: true }] : []),
            ]}
          />
        </CardBody>
      </Card>
    </AuthenticationForm>
  );
}
