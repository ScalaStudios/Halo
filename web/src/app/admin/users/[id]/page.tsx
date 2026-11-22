import Link from "next/link";
import type { ReactNode } from "react";
import { ArrowLeft, Laptop, ShieldAlert, ShieldCheck, ShieldX, UsersRound } from "lucide-react";
import { AvatarEditor } from "@/components/account/avatar-editor";
import { Avatar } from "@/components/ui/avatar";
import { Badge, StatusDot, Tag } from "@/components/ui/badge";
import { buttonClasses } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { DescriptionList } from "@/components/ui/description-list";
import { EmptyState } from "@/components/ui/empty-state";
import { TabNav } from "@/components/ui/tab-nav";
import { AccessPassButton, TemporaryPasswordButton, UserActions } from "@/components/directory/user-actions";
import { ImpersonationNotices } from "@/components/directory/impersonation";
import { PersonalData } from "@/components/directory/personal-data";
import { EditProfileButton, type ProfileEdit } from "@/components/directory/edit-profile";
import { EditRolesButton, PasskeyWarning } from "@/components/directory/edit-roles";
import { limitedOnly, roleLabel, roleNames } from "@/components/settings/roles";
import { AddToGroup, RemoveFromGroup } from "@/components/directory/add-to-group";
import { GuestAccessButton } from "@/components/directory/guest-access";
import { LinuxAccountCard } from "@/components/directory/linux-account";
import { CheckIdentityButton, IdentityChecks } from "@/components/directory/identity-checks";
import { MethodList } from "@/components/identity/method-list";
import { SignInRows } from "@/components/monitoring/sign-in-rows";
import { SectionTitle } from "@/components/ui/section-title";
import { SimpleTable } from "@/components/ui/simple-table";
import { SessionList } from "@/components/identity/session-list";
import { DevicesTable } from "@/components/policy/devices-table";
import { joinList } from "@/components/policy/shared";
import { apiGet } from "@/lib/api/server";
import { formatAttribute, formatDate, formatDateTime, formatRelative } from "@/lib/format";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import { directoryMessages } from "@/lib/i18n/console/directory";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";
import { METHOD, STATUS } from "@/lib/labels";
import type { ComplianceSource, Device } from "@/lib/policy-types";
import type { SettingsResponse } from "@/lib/settings-types";
import type { Application, Attribute, AuditEvent, Group, IdentityProof, Impersonation, PersonRelationship, Session, SignInEvent, User } from "@/lib/types";

const strengthIcon = { "phishing-resistant": ShieldCheck, "multi-factor": ShieldAlert, "single-factor": ShieldX } as const;
const strengthColor = { "phishing-resistant": "text-success", "multi-factor": "text-warning", "single-factor": "text-danger" } as const;

export async function generateMetadata({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return { title: (await apiGet<User>(`/users/${encodeURIComponent(id)}`)).name };
}

export default async function UserPage({ params, searchParams }: { params: Promise<{ id: string }>; searchParams: Promise<{ tab?: string }> }) {
  const { id } = await params;
  const { tab = "overview" } = await searchParams;
  const path = `/users/${encodeURIComponent(id)}`;
  const [user, userSessions, userSignIns, groups, applications, audit, userDevices, directory, me, enabled, response, impersonations, sources, attributes, relationships, proofs, locale] = await Promise.all([
    apiGet<User>(path),
    apiGet<Session[]>(`${path}/sessions`),
    apiGet<SignInEvent[]>(`${path}/sign-ins`),
    apiGet<Group[]>("/groups"),
    apiGet<Application[]>("/applications", []),
    apiGet<AuditEvent[]>("/audit", []),
    apiGet<Device[]>(`/devices?user=${encodeURIComponent(id)}`, []),
    apiGet<User[]>("/users"),
    apiGet<User>("/me"),
    apiGet<{ methods: string[] } | null>("/auth/methods", null),
    apiGet<SettingsResponse | null>("/settings", null),
    apiGet<Impersonation[]>(`/impersonations?user=${encodeURIComponent(id)}`, []),
    apiGet<ComplianceSource[]>("/compliance-sources", []),
    apiGet<Attribute[]>("/attributes", []),
    tab === "applications" ? apiGet<PersonRelationship[]>(`${path}/relationships`, []) : [],
    tab === "authentication" ? apiGet<IdentityProof[]>(`${path}/identity-proofs`, []) : [],
    requestLocale(),
  ]);
  const t = directoryMessages[locale];
  const copy = t.user;
  const settings = response?.settings;
  const limited = limitedOnly(me);
  const manager = user.managerId ? await apiGet<User>(`/users/${encodeURIComponent(user.managerId)}`) : undefined;

  const userGroups = groups.filter((g) => user.groupIds.includes(g.id));
  const userApps = applications.filter((a) => user.appIds.includes(a.id));
  const appNames = Object.fromEntries(applications.map((a) => [a.id, a.name]));
  const StrengthIcon = strengthIcon[user.strength];
  const signInMethods = user.methods.filter((m) => m.kind !== "recovery-codes");
  const global = me.roles.includes("Global administrator");
  const access = user.roles.length + user.eligibleRoles.length > 0 && !global ? null : global || me.roles.includes("User administrator") ? "all" : me.roles.includes("Helpdesk administrator") ? "org" : null;
  const edit: ProfileEdit | undefined = access ? { user, directory, access, attributes } : undefined;
  const profile = user.directory ? undefined : edit;
  const impersonate =
    (global || me.roles.includes("Helpdesk administrator")) &&
    user.id !== me.id &&
    user.status === "active" &&
    user.roles.length + user.eligibleRoles.length === 0 &&
    Boolean(settings?.impersonationGroupIds.some((g) => user.groupIds.includes(g)));
  const awaitingCheck =
    user.status === "invited" &&
    Boolean(settings?.identityProofingInvites) &&
    (settings?.identityProofingGroupIds.length === 0 || Boolean(settings?.identityProofingGroupIds.some((g) => user.groupIds.includes(g)))) &&
    !proofs.some((p) => p.status === "verified");
  const canCheckIdentity = Boolean(access) && user.id !== me.id && user.status !== "deprovisioned";
  const passwordGroups = settings?.passwordGroupIds.map((id) => groups.find((g) => g.id === id)?.name ?? copy.deletedGroup) ?? [];
  const passwordWarning =
    !settings || !enabled
      ? undefined
      : !enabled.methods.includes("password")
        ? copy.passwordsOff(user.name)
        : passwordGroups.length > 0 && !settings.passwordGroupIds.some((id) => user.groupIds.includes(id))
          ? copy.passwordGroups(joinList(passwordGroups, t.or), user.name)
          : settings.passwordSecondFactor && !user.methods.some((m) => m.kind === "totp")
            ? copy.passwordSecondFactor(user.name)
            : undefined;

  const tabs = [
    { id: "overview", label: copy.tabs.overview },
    { id: "authentication", label: copy.tabs.authentication, count: signInMethods.length },
    { id: "access", label: copy.tabs.access },
    { id: "applications", label: copy.tabs.applications, count: userApps.length },
    { id: "groups", label: copy.tabs.groups, count: userGroups.length },
    { id: "devices", label: copy.tabs.devices, count: userDevices.length },
    { id: "sessions", label: copy.tabs.sessions, count: userSessions.length },
    { id: "activity", label: copy.tabs.activity },
  ];

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-6">
        <Link href="/admin/users" className="inline-flex w-fit items-center gap-2 rounded-sm text-body-sm text-fg-3 transition-colors duration-fast hover:text-fg">
          <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
          {frameMessages[locale].nav.items.users.label}
        </Link>
        <header className="flex flex-col gap-6 lg:flex-row lg:items-start lg:justify-between">
          {edit ? (
            <AvatarEditor name={user.name} avatarUrl={user.avatarUrl} endpoint={`/api/v1${path}/avatar`} size="xl">
              <ProfileHeading user={user} t={t} />
            </AvatarEditor>
          ) : (
            <div className="flex min-w-0 items-center gap-4">
              <Avatar name={user.name} src={user.avatarUrl} size="xl" />
              <ProfileHeading user={user} t={t} />
            </div>
          )}
          <UserActions id={user.id} name={user.name} suspended={user.status === "suspended"} edit={profile} impersonate={impersonate} />
        </header>
        <ImpersonationNotices impersonations={impersonations} meId={me.id} />
        <dl className="grid grid-cols-2 gap-x-8 gap-y-4 rounded-lg border border-border bg-surface p-6 md:grid-cols-3 xl:grid-cols-6">
          <Fact label={t.fields.status}>
            <span className="inline-flex items-center gap-2">
              <StatusDot tone={STATUS[user.status].tone} />
              {t.status[user.status]}
            </span>
          </Fact>
          <Fact label={copy.authenticationStrength}>
            <span className="inline-flex items-center gap-2">
              <StrengthIcon aria-hidden="true" size={16} strokeWidth={1.75} className={strengthColor[user.strength]} />
              {t.strength[user.strength].label}
            </span>
          </Fact>
          <Fact label={t.fields.adminRole} action={global && !user.guest ? <EditRolesButton user={user} /> : undefined}>
            {(limited ? user.roles.map((role) => roleLabel(role, locale)) : roleNames(user, groups, locale)).join(", ") || t.none}
          </Fact>
          <Fact label={t.fields.lastSignIn}>{user.lastSignInAt ? formatRelative(user.lastSignInAt, locale) : t.never}</Fact>
          <Fact label={t.fields.source}>{user.source}</Fact>
          <Fact label={copy.memberSince}>{formatDate(user.createdAt, locale)}</Fact>
        </dl>
      </div>

      <TabNav label={copy.sections} tabs={tabs} active={tab} hrefFor={(t) => `/admin/users/${user.id}${t === "overview" ? "" : `?tab=${t}`}`} />

      <div key={tab} className="animate-enter">
        {tab === "overview" ? <OverviewTab user={user} manager={manager} /> : null}
        {tab === "authentication" ? (
          <div className="flex flex-col gap-6">
            <SectionTitle
              title={copy.signInMethods}
              description={t.strength[user.strength].description}
              action={
                edit ? (
                  <div className="flex flex-wrap gap-2">
                    {me.id === user.id ? null : <AccessPassButton user={{ id: user.id, name: user.name }} />}
                    <TemporaryPasswordButton user={{ id: user.id, name: user.name }} warning={passwordWarning} />
                  </div>
                ) : undefined
              }
            />
            <MethodList
              key={user.methods.map((m) => m.id).join()}
              methods={user.methods}
              userId={user.id}
              empty={user.status === "invited" ? copy.invitedEmpty : copy.noMethodsEnrolled}
            />
            <SectionTitle
              title={copy.identityChecks}
              description={awaitingCheck ? copy.awaitingCheck(user.name) : copy.checksDescription}
              action={canCheckIdentity ? <CheckIdentityButton name={user.name} endpoint={`${path}/identity-proofs`} /> : undefined}
            />
            <IdentityChecks
              proofs={proofs}
              providerName={settings?.identityProofingName ?? ""}
              empty={awaitingCheck ? copy.awaitingCheckEmpty : copy.noChecks}
            />
          </div>
        ) : null}
        {tab === "access" ? <AccessTab user={user} /> : null}
        {tab === "applications" ? (
          <div className="flex flex-col gap-6">
            <SectionTitle title={t.fields.applications} description={copy.applicationsDescription} />
            {userApps.length === 0 ? (
              <EmptyState title={copy.noApplications} description={copy.noApplicationsDescription} />
            ) : (
              <SimpleTable
                caption={t.fields.applications}
                head={[t.fields.application, t.fields.protocol, copy.assignedThrough, t.fields.lastSignIn]}
                rows={userApps.map((app) => {
                  const via = userGroups.find((g) => app.groupIds.includes(g.id));
                  const last = userSignIns.find((e) => e.appId === app.id && e.result === "success");
                  return [
                    <Link key="n" href={`/admin/applications/${app.id}`} className="font-medium text-fg hover:underline">
                      {app.name}
                    </Link>,
                    applicationsMessages[locale].protocols[app.protocol],
                    via?.name ?? copy.directAssignment,
                    last ? formatRelative(last.time, locale) : "—",
                  ];
                })}
              />
            )}
            <SectionTitle title={copy.relationships} description={copy.relationshipsDescription} />
            {relationships.length === 0 ? (
              <EmptyState title={copy.noRelationships} description={copy.noRelationshipsDescription} />
            ) : (
              <SimpleTable
                caption={copy.relationships}
                head={[t.fields.application, copy.object, copy.relation, copy.through, copy.since]}
                rows={relationships.map((t) => [
                  <Link key="a" href={`/admin/applications/${t.applicationId}?tab=authorization`} className="font-medium text-fg hover:underline">
                    {t.application}
                  </Link>,
                  <Tag key="o">{t.object}</Tag>,
                  <Tag key="r">{t.relation}</Tag>,
                  t.groupId ? (userGroups.find((g) => g.id === t.groupId)?.name ?? t.groupId) : copy.direct,
                  formatDate(t.createdAt, locale),
                ])}
              />
            )}
          </div>
        ) : null}
        {tab === "groups" ? (
          <div className="flex flex-col gap-6">
            <SectionTitle
              title={t.fields.groups}
              description={copy.groupsDescription}
              action={<AddToGroup name={user.name} userId={user.id} assigned={user.groupIds} groups={groups} />}
            />
            {userGroups.length === 0 ? (
              <EmptyState icon={UsersRound} title={copy.noGroups} description={copy.noGroupsDescription} />
            ) : (
              <SimpleTable
                caption={t.fields.groups}
                head={[copy.group, t.fields.membership, t.fields.source, t.fields.members, copy.actions]}
                rows={userGroups.map((group) => [
                  <span key="n" className="flex flex-col">
                    <span className="font-medium text-fg">{group.name}</span>
                    <span className="text-caption text-fg-3">{group.description}</span>
                  </span>,
                  group.kind === "dynamic" ? <Tag key="k">{group.rule}</Tag> : t.groupKind.assigned,
                  group.source,
                  <span key="m" className="tnum">
                    {group.memberCount}
                  </span>,
                  group.kind === "assigned" ? <RemoveFromGroup key="r" name={user.name} userId={user.id} group={group} /> : null,
                ])}
              />
            )}
          </div>
        ) : null}
        {tab === "devices" ? (
          <div className="flex flex-col gap-6">
            <SectionTitle title={copy.tabs.devices} description={copy.devicesDescription} />
            {userDevices.length === 0 ? (
              <EmptyState icon={Laptop} title={t.devices.emptyTitle} description={copy.noDevicesDescription} />
            ) : (
              <DevicesTable devices={userDevices} sources={sources} owners={{ [user.id]: { name: user.name, email: user.email } }} />
            )}
          </div>
        ) : null}
        {tab === "sessions" ? (
          <div className="flex flex-col gap-6">
            <SectionTitle title={copy.tabs.sessions} description={copy.sessionsDescription} />
            <SessionList key={userSessions.map((session) => session.id).join()} sessions={userSessions} userId={user.id} locale={locale} />
          </div>
        ) : null}
        {tab === "activity" ? <ActivityTab user={user} /> : null}
      </div>
    </div>
  );

  function OverviewTab({ user, manager }: { user: User; manager?: User }) {
    return (
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-12">
        {user.guest ? <GuestCard user={user} /> : null}
        <Card className="xl:col-span-7">
          <CardHeader
            title={copy.profile}
            description={user.directory ? copy.synced(user.directory.name) : undefined}
            actions={profile ? <EditProfileButton {...profile} /> : undefined}
          />
          <CardBody>
            <DescriptionList
              columns={2}
              items={[
                { label: t.fields.email, value: user.email },
                { label: t.fields.title, value: user.title },
                { label: t.fields.department, value: user.department },
                { label: t.fields.location, value: user.location },
                {
                  label: t.fields.manager,
                  value: manager ? (
                    <Link href={`/admin/users/${manager.id}`} className="text-link underline underline-offset-3 hover:text-ember">
                      {manager.name}
                    </Link>
                  ) : (
                    "—"
                  ),
                },
                ...attributes.map((a) => ({ label: a.label, value: formatAttribute(a, user.attributes[a.name], locale) })),
                { label: t.fields.userId, value: user.id, mono: true },
              ]}
            />
          </CardBody>
        </Card>
        <Card className="xl:col-span-5">
          <CardHeader
            title={t.fields.authentication}
            actions={
              <Link href={`/admin/users/${user.id}?tab=authentication`} scroll={false} className={buttonClasses("quiet", "sm")}>
                {copy.manage}
              </Link>
            }
          />
          <CardBody className="flex flex-col gap-4">
            <p className="text-body-sm text-fg-2">{t.strength[user.strength].description}</p>
            {user.roles.length > 0 && user.strength !== "phishing-resistant" ? <PasskeyWarning role={user.roles[0]} /> : null}
            <ul className="flex flex-col gap-2">
              {signInMethods.map((m) => {
                const Icon = METHOD[m.kind].icon;
                return (
                  <li key={m.id} className="flex items-center gap-3 text-body-sm">
                    <Icon aria-hidden="true" size={16} strokeWidth={1.75} className="text-fg-3" />
                    <span className="flex-1 truncate text-fg">{m.label}</span>
                    <span className="shrink-0 text-caption text-fg-3">{m.lastUsedAt ? formatRelative(m.lastUsedAt, locale) : copy.neverUsed}</span>
                  </li>
                );
              })}
              {signInMethods.length === 0 ? <li className="text-body-sm text-fg-3">{copy.noMethods}</li> : null}
            </ul>
          </CardBody>
        </Card>
        <Card className="xl:col-span-12">
          <CardHeader
            title={copy.recentSignIns}
            actions={
              <Link href={`/admin/users/${user.id}?tab=activity`} scroll={false} className={buttonClasses("quiet", "sm")}>
                {copy.allActivity}
              </Link>
            }
          />
          {userSignIns.length === 0 ? (
            <CardBody>
              <p className="text-body-sm text-fg-3">{copy.noSignIns}</p>
            </CardBody>
          ) : (
            <SignInRows events={userSignIns.slice(0, 5)} appNames={appNames} />
          )}
        </Card>
        {global ? (
          <div className="xl:col-span-12">
            <PersonalData user={{ id: user.id, name: user.name, email: user.email }} self={user.id === me.id} />
          </div>
        ) : null}
      </div>
    );
  }

  function GuestCard({ user }: { user: User }) {
    const sponsor = directory.find((u) => u.id === user.sponsor?.id);
    const sponsorActive = sponsor && !sponsor.guest && sponsor.status === "active";
    return (
      <Card className="xl:col-span-12">
        <CardHeader
          title={copy.guestAccess}
          description={copy.guestDescription}
          actions={global || me.roles.includes("User administrator") ? <GuestAccessButton guest={user} directory={directory} /> : undefined}
        />
        <CardBody className="flex flex-col gap-4">
          <DescriptionList
            columns={2}
            items={[
              {
                label: t.guestAccess.sponsor,
                value: user.sponsor ? (
                  <Link href={`/admin/users/${user.sponsor.id}`} className="text-link underline underline-offset-3 hover:text-ember">
                    {user.sponsor.name}
                  </Link>
                ) : (
                  t.none
                ),
              },
              { label: t.guestAccess.accessEnds, value: user.expiresAt ? `${formatDateTime(user.expiresAt, locale)} · ${formatRelative(user.expiresAt, locale)}` : "—" },
            ]}
          />
          {sponsorActive ? null : (
            <p className="rounded-md border border-warning/20 bg-warning-container p-4 text-body-sm text-fg-2">
              {copy.sponsorGone(user.sponsor?.name)}
            </p>
          )}
        </CardBody>
      </Card>
    );
  }

  function AccessTab({ user }: { user: User }) {
    return (
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-2">
        <Card>
          <CardHeader title={copy.adminRoles} description={copy.adminRolesDescription} />
          <CardBody>
            {user.roles.length ? (
              <ul className="flex flex-col gap-3">
                {user.roles.map((role) => (
                  <li key={role} className="flex items-center justify-between gap-4 rounded-md border border-border p-4">
                    <span className="flex flex-col">
                      <span className="text-body-sm font-semibold text-fg">{roleLabel(role, locale)}</span>
                      <span className="text-caption text-fg-3">{copy.assignedDirectly}</span>
                    </span>
                    <Badge tone="ember">{copy.privileged}</Badge>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="text-body-sm text-fg-3">
                {user.guest ? copy.guestsNoRoles : copy.noRoles}
              </p>
            )}
          </CardBody>
        </Card>
        <LinuxAccountCard user={user} editable={access === "all"} />
      </div>
    );
  }

  function ActivityTab({ user }: { user: User }) {
    const changes = audit.filter((event) => (event.targetType === "user" && event.targetId === user.id) || event.actorId === user.id);
    return (
      <div className="flex flex-col gap-8">
        <div className="flex flex-col gap-4">
          <SectionTitle title={copy.signIns} description={copy.newestFirst} />
          <Card>{userSignIns.length ? <SignInRows events={userSignIns} appNames={appNames} /> : <CardBody className="pt-6">{copy.noSignIns}</CardBody>}</Card>
        </div>
        <div className="flex flex-col gap-4">
          <SectionTitle title={copy.changes} description={copy.changesDescription} />
          {changes.length ? (
            <SimpleTable
              caption={copy.changes}
              head={[copy.change, copy.target, copy.action, copy.time]}
              rows={changes.map((event) => [
                <span key="s" className="text-fg">
                  {event.summary}
                </span>,
                event.target,
                <Tag key="a">{event.action}</Tag>,
                formatDateTime(event.time, locale),
              ])}
            />
          ) : (
            <p className="text-body-sm text-fg-3">{copy.noChanges}</p>
          )}
        </div>
      </div>
    );
  }
}

function ProfileHeading({ user, t }: { user: User; t: (typeof directoryMessages)["en"] }) {
  return (
    <div className="flex min-w-0 flex-col gap-1">
      <h1 className="flex min-w-0 items-center gap-3 text-h2 text-fg">
        <span className="truncate">{user.name}</span>
        {user.guest ? <Badge tone="info">{t.guest}</Badge> : null}
      </h1>
      <p className="flex min-w-0 items-center gap-2 text-body text-fg-3">
        <span className="truncate">{user.email}</span>
        <Badge tone={user.emailVerified ? "success" : "neutral"}>{user.emailVerified ? t.user.verified : t.user.notVerified}</Badge>
      </p>
    </div>
  );
}

function Fact({ label, action, children }: { label: string; action?: ReactNode; children: ReactNode }) {
  return (
    <div className="flex min-w-0 flex-col gap-1">
      <dt className="flex items-center gap-2 text-label text-fg-3">
        {label}
        {action}
      </dt>
      <dd className="truncate text-body-sm text-fg">{children}</dd>
    </div>
  );
}

