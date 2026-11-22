import Link from "next/link";
import { ArrowLeft, MonitorSmartphone, ScrollText, UsersRound } from "lucide-react";
import { AvatarEditor } from "@/components/account/avatar-editor";
import { AppActions, DangerZone } from "@/components/applications/app-actions";
import { AuthorizationCheckForm, AuthorizationModelForm } from "@/components/applications/authorization";
import { ClaimMappingsForm } from "@/components/applications/claim-mappings";
import { ClientAuthenticationForm } from "@/components/applications/client-authentication";
import { CredentialList } from "@/components/applications/credentials";
import { AuthenticationForm, BackchannelLogoutForm, ProtectedAddressesForm, ScopesForm, SenderConstraintForm, SignInFrequencyForm, WindowsSignInForm } from "@/components/applications/edit-application";
import { HookForm } from "@/components/applications/hook-form";
import { RolesTable } from "@/components/applications/roles";
import { RemoteSessions } from "@/components/remote/sessions";
import { RemoteSettingsForm } from "@/components/remote/settings";
import { hasRole } from "@/components/settings/roles";
import { SamlAttributesForm, SamlConnectionFields, SamlSettingsForm } from "@/components/applications/saml";
import { SharedSignals } from "@/components/applications/shared-signals";
import { APP_STATUS, LdapConnectionFields, RadiusServerFields, SetupGuideCard, statusKey } from "@/components/applications/shared";
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
import { applicationsMessages } from "@/lib/i18n/console/applications";
import { remoteMessages } from "@/lib/i18n/console/infrastructure/remote";
import { requestLocale } from "@/lib/i18n/server";
import type { ApiResource, Outpost, RemoteSession } from "@/lib/infra-types";
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
  const [app, groups, users, sessions, appSignIns, org, resources, hook, attributes, logouts, signals, model, locale] = await Promise.all([
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
    requestLocale(),
  ]);
  const t = applicationsMessages[locale];
  const d = t.detail;

  const saml = app.protocol === "saml";
  const bookmark = app.protocol === "bookmark";
  const forward = app.protocol === "forward-auth";
  const radius = app.protocol === "radius";
  const ldap = app.protocol === "ldap";
  const remote = app.protocol === "remote";
  const service = app.type === "service";
  const hookable = !saml && !bookmark && !forward && !radius && !ldap && !remote && !service && app.clientId !== "halo-cli";
  const roleable = !bookmark && !radius && !ldap && !remote && !service && app.clientId !== "halo-cli";
  const [outposts, remoteSessions, me] =
    remote && !org.featuresOff.includes("remote-access")
      ? await Promise.all([apiGet<Outpost[]>("/outposts"), apiGet<RemoteSession[]>(`/remote-sessions?app=${encodeURIComponent(id)}`), apiGet<User>("/me")])
      : [[], [], null];
  const modelled = (app.protocol === "oidc" || app.protocol === "oauth") && (app.type === "web" || service) && app.clientId !== "halo-cli";
  const constrainable = (app.protocol === "oidc" || app.protocol === "oauth") && app.clientId !== "halo-cli";
  const windows = !bookmark && !radius && !ldap && !remote && !service && !org.featuresOff.includes("kerberos");
  const owner = users.find((u) => u.id === app.owner);
  const appGroups = groups.filter((g) => app.groupIds.includes(g.id));
  const assigned = new Set(users.filter((u) => u.appIds.includes(app.id)).map((u) => u.id));
  const kind = saml ? "attribute" : forward ? "header" : "claim";
  const mapping = t.mappings.kinds[kind];
  const appSessions = service ? [] : sessions.filter((s) => assigned.has(s.userId)).sort((a, b) => b.lastActiveAt.localeCompare(a.lastActiveAt));

  const tabs = bookmark
    ? [
        { id: "overview", label: d.tabs.overview },
        { id: "users", label: d.tabs.users, count: appGroups.length },
      ]
    : radius
      ? [
          { id: "overview", label: d.tabs.overview },
          { id: "authentication", label: d.tabs.authentication },
          { id: "users", label: d.tabs.users, count: appGroups.length },
          { id: "credentials", label: d.tabs.credentials, count: app.credentials.length },
          { id: "logs", label: d.tabs.logs, count: appSignIns.length },
        ]
    : ldap
      ? [
          { id: "overview", label: d.tabs.overview },
          { id: "users", label: d.tabs.users, count: appGroups.length },
          { id: "logs", label: d.tabs.logs, count: appSignIns.length },
        ]
    : remote
      ? [
          { id: "overview", label: d.tabs.overview },
          { id: "authentication", label: d.tabs.connection },
          { id: "users", label: d.tabs.users, count: appGroups.length },
          { id: "sessions", label: d.tabs.sessions, count: remoteSessions.length },
          { id: "logs", label: d.tabs.logs, count: appSignIns.length },
        ]
    : forward
      ? [
          { id: "overview", label: d.tabs.overview },
          { id: "authentication", label: d.tabs.authentication },
          { id: "claims", label: mapping.tab, count: app.claims.length },
          { id: "users", label: d.tabs.users, count: appGroups.length },
          { id: "roles", label: d.tabs.roles, count: app.roles.length },
          { id: "sessions", label: d.tabs.sessions, count: appSessions.length },
          { id: "logs", label: d.tabs.logs, count: appSignIns.length },
        ]
      : [
        { id: "overview", label: d.tabs.overview },
        { id: "authentication", label: d.tabs.authentication },
        { id: "permissions", label: d.tabs.permissions, count: app.scopes.length },
        { id: "claims", label: mapping.tab, count: app.claims.length },
        { id: "users", label: d.tabs.users, count: appGroups.length },
        ...(roleable ? [{ id: "roles", label: d.tabs.roles, count: app.roles.length }] : []),
        ...(modelled ? [{ id: "authorization", label: d.tabs.authorization }] : []),
        { id: "credentials", label: d.tabs.credentials, count: app.credentials.length },
        { id: "sessions", label: d.tabs.sessions, count: appSessions.length },
        { id: "logs", label: d.tabs.logs, count: appSignIns.length },
      ];

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-6">
        <Link href="/admin/applications" className="inline-flex w-fit items-center gap-2 rounded-sm text-body-sm text-fg-3 transition-colors duration-fast hover:text-fg">
          <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
          {t.common.applications}
        </Link>
        <header className="flex flex-col gap-6 2xl:flex-row 2xl:items-center 2xl:justify-between">
          <AvatarEditor name={app.name} avatarUrl={app.logoUrl} endpoint={`/api/v1/applications/${app.id}/logo`} size="xl" logo className="min-w-0 flex-1">
            <div className="flex min-w-0 flex-col gap-1">
              <div className="flex flex-wrap items-center gap-3">
                <h1 className="text-h2 text-fg">{app.name}</h1>
                <Badge tone={APP_STATUS[statusKey(app)]} dot>
                  {t.status[statusKey(app)]}
                </Badge>
                {app.registeredDynamically ? <Badge tone="info">{d.registeredDynamically}</Badge> : null}
              </div>
              <p className="text-body text-fg-3">
                {t.protocols[app.protocol]}
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
              remote,
              disabled: app.status === "disabled",
              awaitingApproval: app.awaitingApproval,
              redirectUris: app.redirectUris,
              postLogoutUris: app.postLogoutUris,
              hasSecret: ldap || (!saml && !bookmark && !forward && !remote && (app.type === "web" || app.type === "service")),
            }}
            directory={users}
          />
        </header>
        <DescriptionList
          className={`grid-cols-2 rounded-lg border border-border bg-surface p-6 md:grid-cols-3 ${bookmark ? "xl:grid-cols-4" : forward || radius || ldap || remote ? "xl:grid-cols-5" : "xl:grid-cols-6"}`}
          items={[
            { label: t.list.protocol, value: t.protocols[app.protocol] },
            ...(bookmark || forward || radius || ldap || remote ? [] : [{ label: t.list.type, value: t.types[app.type] }]),
            {
              label: t.common.status,
              value: (
                <span className="inline-flex items-center gap-2">
                  <StatusDot tone={APP_STATUS[statusKey(app)]} />
                  {t.status[statusKey(app)]}
                </span>
              ),
            },
            { label: t.list.users, value: <span className="tnum">{app.userCount}</span> },
            ...(bookmark
              ? []
              : [
                  {
                    label: d.signIns7d,
                    value: (
                      <span className="tnum">
                        {app.signIns7d.toLocaleString(locale)} <span className="text-fg-3">· {d.failed(app.failureRate7d)}</span>
                      </span>
                    ),
                  },
                ]),
            {
              label: t.common.owner,
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

      <TabNav label={d.sections} tabs={tabs} active={tab} hrefFor={(t) => `/admin/applications/${app.id}${t === "overview" ? "" : `?tab=${t}`}`} />

      <div key={tab} className="animate-enter">
        {tab === "overview" ? <OverviewTab app={app} issuer={org.issuer} outposts={outposts} /> : null}
        {tab === "authentication" && remote && app.remote ? (
          <div className="flex flex-col gap-6">
            <RemoteSettingsForm key={JSON.stringify(app.remote)} appId={app.id} appName={app.name} target={app.remote} outposts={outposts} />
            <SignInFrequencyForm key={app.maxAuthAge} app={app} />
          </div>
        ) : null}
        {tab === "authentication" && !remote ? (
          <div className="flex flex-col gap-6">
            {forward || radius ? <ProtectedAddressesForm key={app.redirectUris.join()} app={app} /> : <AuthenticationTab app={app} resources={resources} />}
            {modelled ? <ClientAuthenticationForm key={`${app.tokenEndpointAuthMethod}:${app.jwksUri}:${JSON.stringify(app.jwks)}:${app.fapi2Profile}`} app={app} /> : null}
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
                <SectionTitle title={d.logouts} description={d.logoutsHint(logouts.length)} />
                <DeliveryLog deliveries={logouts} payloadNote={d.logoutNote} />
              </section>
            ) : null}
          </div>
        ) : null}
        {tab === "permissions" ? (
          <div className="flex flex-col gap-6">
            <SectionTitle title={d.scopes} description={saml ? undefined : d.scopesHint(app.name)} />
            {!saml ? (
              <ScopesForm key={`${app.scopes.join()}:${app.requiresConsent}`} app={app} />
            ) : (
              <EmptyState title={d.samlScopes} description={d.samlScopesHint(app.name)} />
            )}
          </div>
        ) : null}
        {tab === "claims" && app.saml ? (
          <div className="flex flex-col gap-6">
            <SectionTitle title={t.mappings.kinds.attribute.tab} description={d.samlAttributesHint(app.name)} />
            <SamlAttributesForm appId={app.id} appName={app.name} saml={app.saml} />
          </div>
        ) : null}
        {tab === "claims" && !app.saml ? (
          <div className="flex flex-col gap-6">
            <SectionTitle
              title={mapping.tab}
              description={
                saml
                  ? d.claimsSaml(app.name)
                  : forward
                    ? d.claimsForward(app.name)
                    : service
                    ? d.claimsService(app.name)
                    : d.claimsDefault(app.name)
              }
            />
            <SimpleTable
              caption={mapping.tab}
              head={[mapping.column, t.common.source]}
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
              title={mapping.title}
              description={mapping.hint(app.name)}
            />
            <ClaimMappingsForm
              key={JSON.stringify(app.claimMappings)}
              appId={app.id}
              appName={app.name}
              kind={kind}
              mappings={app.claimMappings}
              attributes={attributes}
            />
          </div>
        ) : null}
        {tab === "users" ? (
          <div className="flex flex-col gap-6">
            <SectionTitle
              title={d.assignedGroups}
              description={service ? d.serviceGroups(app.name) : d.groupsHint(app.userCount, app.name, appGroups.length, bookmark)}
              action={
                service ? undefined : (
                  <AddToGroup name={app.name} appId={app.id} assigned={app.groupIds} groups={groups} bookmark={bookmark} />
                )
              }
            />
            {appGroups.length ? (
              <SimpleTable
                caption={d.assignedGroups}
                head={[d.group, d.membership, t.common.source, d.members, t.common.actions]}
                rows={appGroups.map((group) => [
                  <span key="n" className="flex flex-col">
                    <span className="font-medium text-fg">{group.name}</span>
                    <span className="text-caption text-fg-3">{group.description}</span>
                  </span>,
                  group.kind === "dynamic" ? <Tag key="k">{group.rule}</Tag> : d.assigned,
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
                title={d.noGroups}
                description={service ? d.noGroupsService : bookmark ? d.noGroupsBookmark(app.name) : d.noGroupsDefault(app.name)}
              />
            )}
          </div>
        ) : null}
        {tab === "roles" && roleable ? (
          <div className="flex flex-col gap-6">
            <SectionTitle
              title={d.roles}
              description={d.rolesHint(app.name, kind)}
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
              title={d.tabs.authorization}
              description={d.authorizationHint(app.name)}
            />
            <CopyField label={d.authorizationApi} value={`${org.issuer}/api/v1/authz`} />
            <AuthorizationModelForm appId={app.id} appName={app.name} model={model} />
            {model.updatedAt ? (
              <AuthorizationCheckForm appId={app.id} people={users.filter((u) => u.status !== "deprovisioned").map(({ id, name, email }) => ({ id, name, email }))} />
            ) : null}
          </div>
        ) : null}
        {tab === "credentials" ? (
          <div className="flex flex-col gap-6">
            <SectionTitle
              title={d.tabs.credentials}
              description={
                app.credentials.length === 0
                  ? undefined
                  : saml
                    ? d.credentialsSaml(app.name)
                    : radius
                      ? d.credentialsRadius(app.name)
                      : d.credentialsDefault(app.name)
              }
            />
            <CredentialList
              appId={app.id}
              appName={app.name}
              credentials={app.credentials}
              secret={radius ? "shared" : undefined}
              empty={
                saml
                  ? d.noCredentialsSaml(app.name)
                  : app.type === "native" || app.type === "spa"
                    ? d.noCredentialsPublic(app.name)
                    : radius
                      ? d.noCredentialsRadius(app.name)
                      : d.noCredentialsDefault(app.name)
              }
            />
          </div>
        ) : null}
        {tab === "sessions" && remote ? (
          <div className="flex flex-col gap-6">
            <SectionTitle title={d.tabs.sessions} description={d.remoteSessions(app.name)} />
            <RemoteSessions
              sessions={remoteSessions}
              showApplication={false}
              canEnd={me ? hasRole(me, "security_admin", "app_admin") : false}
              canDownload={me ? hasRole(me, "security_admin", "auditor") : false}
              empty={d.remoteSessionsEmpty(app.name)}
            />
          </div>
        ) : null}
        {tab === "sessions" && !remote ? (
          <div className="flex flex-col gap-6">
            <SectionTitle
              title={d.tabs.sessions}
              description={service ? undefined : d.sessionsHint(app.name)}
            />
            {service ? (
              <EmptyState
                icon={MonitorSmartphone}
                title={d.noSessions}
                description={d.noSessionsHint(app.name, t.duration(app.tokenPolicy.accessTokenTtl))}
              />
            ) : (
              <SessionsTable sessions={appSessions} userNames={Object.fromEntries(users.map((u) => [u.id, u.name]))} />
            )}
          </div>
        ) : null}
        {tab === "logs" ? (
          <div className="flex flex-col gap-6">
            <SectionTitle title={d.signIns} description={d.newestFirst} />
            {appSignIns.length ? (
              <Card>
                <SignInRows events={appSignIns} appNames={{ [app.id]: app.name }} />
              </Card>
            ) : (
              <EmptyState
                icon={ScrollText}
                title={d.noSignIns}
                description={service ? d.noSignInsService(app.name) : app.status === "disabled" ? d.noSignInsDisabled(app.name) : d.noSignInsDefault(app.name)}
              />
            )}
          </div>
        ) : null}
      </div>
    </div>
  );
}

async function OverviewTab({ app, issuer, outposts }: { app: SamlApplication; issuer: string; outposts: Outpost[] }) {
  const locale = await requestLocale();
  const t = applicationsMessages[locale];
  const d = t.detail;
  const saml = app.protocol === "saml";
  const urls = endpoints(issuer);
  if (app.remote) {
    const target = app.remote;
    const outpost = outposts.find((o) => o.id === target.outpostId);
    return (
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-12">
        <Card className="min-w-0 xl:col-span-7">
          <CardHeader
            title={d.tabs.connection}
            description={d.connectionHint(app.name)}
          />
          <CardBody>
            <DescriptionList
              columns={2}
              items={[
                { label: d.host, value: `${target.host}:${target.port}`, mono: true },
                { label: d.outpost, value: outpost ? `${outpost.name} · ${outpost.connected ? d.connected : d.notConnected}` : d.unknown },
                { label: d.signsInWith, value: remoteMessages[locale].settings.auth[target.auth] },
                { label: d.account, value: target.username || (target.protocol === "ssh" ? d.firstPrincipal : d.passwordOnly) },
                { label: d.hostKey, value: target.hostKey ? d.pinned(target.hostKey.split(" ")[0]) : d.hostCertificate },
                { label: d.timeLimit, value: t.duration(target.sessionLimit) },
                { label: d.recording, value: target.record ? d.recorded : t.common.off },
              ]}
            />
          </CardBody>
        </Card>
        <DangerZone id={app.id} name={app.name} disabled={app.status === "disabled"} className="xl:col-span-12" />
      </div>
    );
  }
  if (app.protocol === "bookmark") {
    return (
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-12">
        <Card className="min-w-0 xl:col-span-5">
          <CardHeader title={t.protocols.bookmark} description={d.bookmarkHint(app.name)} />
          <CardBody>
            <CopyField label={t.common.url} value={app.homepage} />
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
            title={d.connectionDetails}
            description={d.radiusHint(app.name)}
          />
          <CardBody className="flex flex-col gap-4">
            <RadiusServerFields issuer={issuer} port={app.radiusPort} locale={locale} />
            {app.redirectUris.map((address, index) => (
              <CopyField key={address} label={app.redirectUris.length > 1 ? `${t.fields.clientAddress} ${index + 1}` : t.fields.clientAddress} value={address} />
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
          <CardHeader title={d.connectionDetails} description={d.ldapHint(app.name)} />
          <CardBody className="flex flex-col gap-4">
            <LdapConnectionFields ldap={app.ldap} locale={locale} />
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
          <CardHeader title={d.connectionDetails} description={d.forwardHint(app.name)} />
          <CardBody className="flex flex-col gap-4">
            <CopyField label={d.verifyEndpoint} value={`${issuer}/forward-auth/${app.id}/verify`} hint={d.verifyHint} />
            {app.redirectUris.map((uri, index) => (
              <CopyField key={uri} label={app.redirectUris.length > 1 ? `${t.fields.protectedAddress} ${index + 1}` : t.fields.protectedAddress} value={uri} />
            ))}
            <CopyField label={d.signOutPath} value="/_halo/sign-out" hint={d.signOutHint(app.name)} />
          </CardBody>
        </Card>
        <div id="setup-guide" className="flex min-w-0 scroll-mt-24 flex-col gap-6 xl:col-span-7">
          {proxyGuides(app, issuer, locale).map((guide) => (
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
          title={d.connectionDetails}
          description={saml ? d.samlHint(app.name) : d.oidcHint(app.name)}
        />
        <CardBody className="flex flex-col gap-4">
          {app.saml ? <SamlConnectionFields idp={app.saml.idp} /> : null}
          <CopyField label={saml ? t.fields.spEntityId : t.fields.clientId} value={app.clientId} />
          {saml ? null : (
            <>
              <CopyField label={t.fields.issuer} value={urls.issuer} />
              <CopyField label={t.fields.discovery} value={urls.discovery} />
            </>
          )}
          {app.type === "service" ? <CopyField label={t.fields.tokenEndpoint} value={urls.token} /> : null}
          {app.redirectUris.map((uri, index) => (
            <CopyField
              key={uri}
              label={saml ? t.fields.acsUrl : app.redirectUris.length > 1 ? `${t.fields.redirectUri} ${index + 1}` : t.fields.redirectUri}
              value={uri}
            />
          ))}
        </CardBody>
      </Card>
      <div id="setup-guide" className="min-w-0 scroll-mt-24 xl:col-span-7">
        <SetupGuideCard guide={setupGuide(app, issuer, locale)} />
      </div>
      <DangerZone id={app.id} name={app.name} disabled={app.status === "disabled"} className="xl:col-span-12" />
    </div>
  );
}

async function AuthenticationTab({ app, resources }: { app: SamlApplication; resources: ApiResource[] }) {
  const d = applicationsMessages[await requestLocale()].detail;
  if (app.saml) {
    return <SamlSettingsForm appId={app.id} appName={app.name} saml={app.saml} />;
  }

  const service = app.type === "service";
  const publicClient = app.type === "spa" || app.type === "native";
  const grants = service ? ["client_credentials"] : app.tokenPolicy.refreshTokenTtl > 0 ? ["authorization_code", "refresh_token"] : ["authorization_code"];

  return (
    <AuthenticationForm key={JSON.stringify([app.redirectUris, app.postLogoutUris, app.tokenPolicy])} app={app} resources={resources}>
      <Card>
        <CardHeader title={d.grants} description={d.grantsHint} />
        <CardBody>
          <DescriptionList
            items={[
              {
                label: d.grantTypes,
                value: (
                  <span className="flex flex-wrap gap-2">
                    {grants.map((grant) => (
                      <Tag key={grant}>{grant}</Tag>
                    ))}
                  </span>
                ),
              },
              { label: "PKCE", value: service ? d.pkceService : d.pkceRequired },
              { label: d.clientAuthentication, value: publicClient ? d.publicClient : d.secretBasic },
              ...(app.tokenPolicy.exchange ? [{ label: d.tokenExchange, value: "urn:ietf:params:oauth:grant-type:token-exchange", mono: true }] : []),
            ]}
          />
        </CardBody>
      </Card>
    </AuthenticationForm>
  );
}
