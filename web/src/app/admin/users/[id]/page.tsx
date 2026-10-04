import Link from "next/link";
import type { ReactNode } from "react";
import { ArrowLeft, Laptop, ShieldAlert, ShieldCheck, ShieldX, UsersRound } from "lucide-react";
import { Avatar } from "@/components/ui/avatar";
import { Badge, StatusDot, Tag } from "@/components/ui/badge";
import { buttonClasses } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { DescriptionList } from "@/components/ui/description-list";
import { EmptyState } from "@/components/ui/empty-state";
import { TabNav } from "@/components/ui/tab-nav";
import { UserActions } from "@/components/directory/user-actions";
import { EditProfileButton, type ProfileEdit } from "@/components/directory/edit-profile";
import { AddToGroup, RemoveFromGroup } from "@/components/directory/add-to-group";
import { PROTOCOL_LABEL } from "@/components/applications/shared";
import { MethodList } from "@/components/identity/method-list";
import { SignInRows } from "@/components/monitoring/sign-in-rows";
import { SectionTitle } from "@/components/ui/section-title";
import { SimpleTable } from "@/components/ui/simple-table";
import { SessionList } from "@/components/identity/session-list";
import { DevicesTable } from "@/components/policy/devices-table";
import { apiGet } from "@/lib/api/server";
import { formatDate, formatDateTime, formatRelative } from "@/lib/format";
import { METHOD, STATUS, STRENGTH } from "@/lib/labels";
import type { Device } from "@/lib/policy-types";
import type { Application, AuditEvent, Group, Session, SignInEvent, User } from "@/lib/types";

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
  const [user, userSessions, userSignIns, groups, applications, audit, userDevices, directory, me] = await Promise.all([
    apiGet<User>(path),
    apiGet<Session[]>(`${path}/sessions`),
    apiGet<SignInEvent[]>(`${path}/sign-ins`),
    apiGet<Group[]>("/groups"),
    apiGet<Application[]>("/applications"),
    apiGet<AuditEvent[]>("/audit"),
    apiGet<Device[]>(`/devices?user=${encodeURIComponent(id)}`),
    apiGet<User[]>("/users"),
    apiGet<User>("/me"),
  ]);
  const manager = user.managerId ? await apiGet<User>(`/users/${encodeURIComponent(user.managerId)}`) : undefined;

  const userGroups = groups.filter((g) => user.groupIds.includes(g.id));
  const userApps = applications.filter((a) => user.appIds.includes(a.id));
  const appNames = Object.fromEntries(applications.map((a) => [a.id, a.name]));
  const StrengthIcon = strengthIcon[user.strength];
  const signInMethods = user.methods.filter((m) => m.kind !== "recovery-codes");
  const global = me.roles.includes("Global administrator");
  const access = user.roles.length > 0 && !global ? null : global || me.roles.includes("User administrator") ? "all" : me.roles.includes("Helpdesk administrator") ? "org" : null;
  const edit: ProfileEdit | undefined = access ? { user, directory, access } : undefined;

  const tabs = [
    { id: "overview", label: "Overview" },
    { id: "authentication", label: "Authentication", count: signInMethods.length },
    { id: "access", label: "Access" },
    { id: "applications", label: "Applications", count: userApps.length },
    { id: "groups", label: "Groups", count: userGroups.length },
    { id: "devices", label: "Devices", count: userDevices.length },
    { id: "sessions", label: "Sessions", count: userSessions.length },
    { id: "activity", label: "Activity" },
  ];

  return (
    <div className="flex animate-page flex-col gap-8">
      <div className="flex flex-col gap-6">
        <Link href="/admin/users" className="inline-flex w-fit items-center gap-2 rounded-sm text-body-sm text-fg-3 transition-colors duration-fast hover:text-fg">
          <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
          Users
        </Link>
        <header className="flex flex-col gap-6 lg:flex-row lg:items-start lg:justify-between">
          <div className="flex min-w-0 items-center gap-4">
            <Avatar name={user.name} size="xl" />
            <div className="flex min-w-0 flex-col gap-1">
              <h1 className="truncate text-h2 text-fg">{user.name}</h1>
              <p className="flex min-w-0 items-center gap-2 text-body text-fg-3">
                <span className="truncate">{user.email}</span>
                <Badge tone={user.emailVerified ? "success" : "neutral"}>{user.emailVerified ? "Verified" : "Not verified"}</Badge>
              </p>
            </div>
          </div>
          <UserActions id={user.id} name={user.name} suspended={user.status === "suspended"} edit={edit} />
        </header>
        <dl className="grid grid-cols-2 gap-x-8 gap-y-4 rounded-lg border border-border bg-surface p-6 md:grid-cols-3 xl:grid-cols-6">
          <Fact label="Status">
            <span className="inline-flex items-center gap-2">
              <StatusDot tone={STATUS[user.status].tone} />
              {STATUS[user.status].label}
            </span>
          </Fact>
          <Fact label="Authentication strength">
            <span className="inline-flex items-center gap-2">
              <StrengthIcon aria-hidden="true" size={16} strokeWidth={1.75} className={strengthColor[user.strength]} />
              {STRENGTH[user.strength].label}
            </span>
          </Fact>
          <Fact label="Admin role">{user.roles[0] ?? "None"}</Fact>
          <Fact label="Last sign-in">{user.lastSignInAt ? formatRelative(user.lastSignInAt) : "Never"}</Fact>
          <Fact label="Source">{user.source}</Fact>
          <Fact label="Member since">{formatDate(user.createdAt)}</Fact>
        </dl>
      </div>

      <TabNav label="User sections" tabs={tabs} active={tab} hrefFor={(t) => `/admin/users/${user.id}${t === "overview" ? "" : `?tab=${t}`}`} />

      <div key={tab} className="animate-enter">
        {tab === "overview" ? <OverviewTab user={user} manager={manager} /> : null}
        {tab === "authentication" ? (
          <div className="flex flex-col gap-6">
            <SectionTitle title="Sign-in methods" description={STRENGTH[user.strength].description} />
            <MethodList
              key={user.methods.map((m) => m.id).join()}
              methods={user.methods}
              userId={user.id}
              empty={user.status === "invited" ? "Invited. No method is enrolled until they open their setup link." : "No sign-in methods enrolled."}
            />
          </div>
        ) : null}
        {tab === "access" ? <AccessTab user={user} /> : null}
        {tab === "applications" ? (
          <div className="flex flex-col gap-6">
            <SectionTitle title="Applications" description="Assigned through group membership. Remove access by changing groups." />
            {userApps.length === 0 ? (
              <EmptyState title="No applications" description="Applications assigned to this user's groups appear here." />
            ) : (
              <SimpleTable
                caption="Applications"
                head={["Application", "Protocol", "Assigned through", "Last sign-in"]}
                rows={userApps.map((app) => {
                  const via = userGroups.find((g) => app.groupIds.includes(g.id));
                  const last = userSignIns.find((e) => e.appId === app.id && e.result === "success");
                  return [
                    <Link key="n" href={`/admin/applications/${app.id}`} className="font-medium text-fg hover:underline">
                      {app.name}
                    </Link>,
                    PROTOCOL_LABEL[app.protocol],
                    via?.name ?? "Direct assignment",
                    last ? formatRelative(last.time) : "—",
                  ];
                })}
              />
            )}
          </div>
        ) : null}
        {tab === "groups" ? (
          <div className="flex flex-col gap-6">
            <SectionTitle
              title="Groups"
              description="Dynamic groups follow their rule. Assigned groups are managed here or by provisioning."
              action={<AddToGroup name={user.name} userId={user.id} groups={groups.filter((g) => g.kind === "assigned" && !user.groupIds.includes(g.id))} />}
            />
            {userGroups.length === 0 ? (
              <EmptyState icon={UsersRound} title="No groups" description="Add this person to an assigned group to give them its applications." />
            ) : (
              <SimpleTable
                caption="Groups"
                head={["Group", "Membership", "Source", "Members", "Actions"]}
                rows={userGroups.map((group) => [
                  <span key="n" className="flex flex-col">
                    <span className="font-medium text-fg">{group.name}</span>
                    <span className="text-caption text-fg-3">{group.description}</span>
                  </span>,
                  group.kind === "dynamic" ? <Tag key="k">{group.rule}</Tag> : "Assigned",
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
            <SectionTitle title="Devices" description="Browsers this person has signed in from. Trusted devices satisfy device conditions in policies; blocked devices can't sign in." />
            {userDevices.length === 0 ? (
              <EmptyState icon={Laptop} title="No devices yet" description="A device appears here after its first sign-in to Halo." />
            ) : (
              <DevicesTable devices={userDevices} owners={{ [user.id]: { name: user.name, email: user.email } }} />
            )}
          </div>
        ) : null}
        {tab === "sessions" ? (
          <div className="flex flex-col gap-6">
            <SectionTitle title="Sessions" description="Halo sessions and the application sessions that depend on them." />
            <SessionList key={userSessions.map((session) => session.id).join()} sessions={userSessions} userId={user.id} />
          </div>
        ) : null}
        {tab === "activity" ? <ActivityTab user={user} /> : null}
      </div>
    </div>
  );

  function OverviewTab({ user, manager }: { user: User; manager?: User }) {
    return (
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-12">
        <Card className="xl:col-span-7">
          <CardHeader title="Profile" actions={edit ? <EditProfileButton {...edit} /> : undefined} />
          <CardBody>
            <DescriptionList
              columns={2}
              items={[
                { label: "Email", value: user.email },
                { label: "Title", value: user.title },
                { label: "Department", value: user.department },
                { label: "Location", value: user.location },
                {
                  label: "Manager",
                  value: manager ? (
                    <Link href={`/admin/users/${manager.id}`} className="text-link underline underline-offset-3 hover:text-ember">
                      {manager.name}
                    </Link>
                  ) : (
                    "—"
                  ),
                },
                { label: "User ID", value: user.id, mono: true },
              ]}
            />
          </CardBody>
        </Card>
        <Card className="xl:col-span-5">
          <CardHeader
            title="Authentication"
            actions={
              <Link href={`/admin/users/${user.id}?tab=authentication`} scroll={false} className={buttonClasses("quiet", "sm")}>
                Manage
              </Link>
            }
          />
          <CardBody className="flex flex-col gap-4">
            <p className="text-body-sm text-fg-2">{STRENGTH[user.strength].description}</p>
            {user.roles.length > 0 && user.strength !== "phishing-resistant" ? (
              <div className="flex gap-3 rounded-md border border-warning/20 bg-warning-container p-4">
                <ShieldAlert aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0 text-warning" />
                <p className="text-body-sm text-fg-2">
                  {user.roles[0]} should sign in with a passkey or security key. Until then this account can be phished.
                </p>
              </div>
            ) : null}
            <ul className="flex flex-col gap-2">
              {signInMethods.map((m) => {
                const Icon = METHOD[m.kind].icon;
                return (
                  <li key={m.id} className="flex items-center gap-3 text-body-sm">
                    <Icon aria-hidden="true" size={16} strokeWidth={1.75} className="text-fg-3" />
                    <span className="flex-1 truncate text-fg">{m.label}</span>
                    <span className="shrink-0 text-caption text-fg-3">{m.lastUsedAt ? formatRelative(m.lastUsedAt) : "Never used"}</span>
                  </li>
                );
              })}
              {signInMethods.length === 0 ? <li className="text-body-sm text-fg-3">No methods enrolled.</li> : null}
            </ul>
          </CardBody>
        </Card>
        <Card className="xl:col-span-12">
          <CardHeader
            title="Recent sign-ins"
            actions={
              <Link href={`/admin/users/${user.id}?tab=activity`} scroll={false} className={buttonClasses("quiet", "sm")}>
                All activity
              </Link>
            }
          />
          {userSignIns.length === 0 ? (
            <CardBody>
              <p className="text-body-sm text-fg-3">No sign-ins recorded yet.</p>
            </CardBody>
          ) : (
            <SignInRows events={userSignIns.slice(0, 5)} appNames={appNames} />
          )}
        </Card>
      </div>
    );
  }

  function AccessTab({ user }: { user: User }) {
    return (
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-2">
        <Card>
          <CardHeader title="Administrative roles" description="What this person can change in Halo." />
          <CardBody>
            {user.roles.length ? (
              <ul className="flex flex-col gap-3">
                {user.roles.map((role) => (
                  <li key={role} className="flex items-center justify-between gap-4 rounded-md border border-border p-4">
                    <span className="flex flex-col">
                      <span className="text-body-sm font-semibold text-fg">{role}</span>
                      <span className="text-caption text-fg-3">Assigned directly · No expiry</span>
                    </span>
                    <Badge tone="ember">Privileged</Badge>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="text-body-sm text-fg-3">No administrative roles. This person can only manage their own account.</p>
            )}
          </CardBody>
        </Card>
      </div>
    );
  }

  function ActivityTab({ user }: { user: User }) {
    const changes = audit.filter((event) => (event.targetType === "user" && event.targetId === user.id) || event.actorId === user.id);
    return (
      <div className="flex flex-col gap-8">
        <div className="flex flex-col gap-4">
          <SectionTitle title="Sign-ins" description="Newest first." />
          <Card>{userSignIns.length ? <SignInRows events={userSignIns} appNames={appNames} /> : <CardBody className="pt-6">No sign-ins recorded yet.</CardBody>}</Card>
        </div>
        <div className="flex flex-col gap-4">
          <SectionTitle title="Administrative changes" description="Changes made to or by this person." />
          {changes.length ? (
            <SimpleTable
              caption="Administrative changes"
              head={["Change", "Target", "Action", "Time"]}
              rows={changes.map((event) => [
                <span key="s" className="text-fg">
                  {event.summary}
                </span>,
                event.target,
                <Tag key="a">{event.action}</Tag>,
                formatDateTime(event.time),
              ])}
            />
          ) : (
            <p className="text-body-sm text-fg-3">No administrative changes recorded.</p>
          )}
        </div>
      </div>
    );
  }
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex min-w-0 flex-col gap-1">
      <dt className="text-label text-fg-3">{label}</dt>
      <dd className="truncate text-body-sm text-fg">{children}</dd>
    </div>
  );
}

