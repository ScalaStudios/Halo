import Link from "next/link";
import type { ReactNode } from "react";
import { AppWindow, ArrowRight, ClipboardCheck, FileKey, Inbox, Laptop, LogIn, ShieldAlert, ShieldCheck, TriangleAlert, UserRound, UsersRound, type LucideIcon } from "lucide-react";
import { OrgLogo } from "@/components/console/org-logo";
import { PendingRequests } from "@/components/governance/requests";
import { ReportInsights } from "@/components/policy/policy-list";
import { hasRole } from "@/components/settings/roles";
import { Avatar } from "@/components/ui/avatar";
import { Badge, StatusDot } from "@/components/ui/badge";
import { buttonClasses } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { CopyButton } from "@/components/ui/copy-button";
import { EmptyState } from "@/components/ui/empty-state";
import { ListSkeleton, Skeleton, SkeletonText } from "@/components/ui/skeleton";
import { apiGet } from "@/lib/api/server";
import { cn } from "@/lib/cn";
import { formatRelative, pluralize } from "@/lib/format";
import { RESULT, signInApp, signInUser } from "@/lib/labels";
import type { AccessRequest, AccessReview } from "@/lib/governance-types";
import type { Device, PolicyWithActivity, RiskEvent } from "@/lib/policy-types";
import type { OrganizationProfile } from "@/lib/settings-types";
import type { Application, Group, Overview, User } from "@/lib/types";

function names(list: string[]) {
  return list.length > 5 ? `${list.slice(0, 5).join(", ")} and ${list.length - 5} more` : list.join(", ");
}

const shortcuts = [
  { label: "Invite a user", href: "/admin/users?invite=1" },
  { label: "Sign-in logs", href: "/admin/sign-ins" },
  { label: "Audit log", href: "/admin/audit" },
  { label: "Sign-in methods", href: "/admin/methods" },
  { label: "Suspended users", href: "/admin/users?view=suspended" },
  { label: "Access policies", href: "/admin/policies" },
  { label: "Network zones", href: "/admin/policies?tab=networks" },
  { label: "Risk events", href: "/admin/risk" },
  { label: "Sessions", href: "/admin/sessions" },
  { label: "Domains", href: "/admin/settings/domains" },
  { label: "Roles", href: "/admin/roles" },
];

export function Shortcuts() {
  return (
    <section className="flex flex-col gap-4">
      <h2 className="text-h4 text-fg">Shortcuts</h2>
      <ul className="flex flex-wrap gap-2">
        {shortcuts.map((shortcut) => (
          <li key={shortcut.label}>
            <Link href={shortcut.href} className={buttonClasses("secondary", "sm")}>
              {shortcut.label}
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}

export async function OrganizationCard({ org }: { org: OrganizationProfile }) {
  const [me, users, groups, devices, applications] = await Promise.all([apiGet<User>("/me"), apiGet<User[]>("/users"), apiGet<Group[]>("/groups"), apiGet<Device[]>("/devices"), apiGet<Application[]>("/applications")]);
  const directory: { value: number; label: string; action: string; href: string; icon: LucideIcon }[] = [
    { value: users.length, label: "Users", action: "View users", href: "/admin/users", icon: UserRound },
    { value: groups.length, label: "Groups", action: "View groups", href: "/admin/groups", icon: UsersRound },
    { value: devices.length, label: "Devices", action: "View devices", href: "/admin/devices", icon: Laptop },
    { value: applications.length, label: "Applications", action: "View apps", href: "/admin/applications", icon: AppWindow },
  ];
  return (
    <Card className="flex flex-col">
      <div className="px-6 pt-6">
        <OrgLogo name={org.name} logoUrl={org.logoUrl} canEdit={hasRole(me)} />
      </div>
      <div className="flex min-w-0 items-center gap-2 px-6 pt-4 text-body-sm">
        <span className="shrink-0 font-semibold text-fg">Issuer</span>
        <span className="min-w-0 truncate font-mono text-code-sm text-fg-2">{org.issuer}</span>
        <CopyButton value={org.issuer} label="Copy" className="ml-auto" />
      </div>
      <ul className="mx-6 mt-4 mb-6 grid grid-cols-2 gap-x-4 gap-y-6 border-t border-border pt-6">
        {directory.map((item) => {
          const Icon = item.icon;
          return (
            <li key={item.label} className="flex items-start gap-3">
              <Icon aria-hidden="true" size={24} strokeWidth={1.5} className="mt-1 shrink-0 text-fg-3" />
              <div className="flex min-w-0 flex-col items-start gap-2">
                <span className="tnum font-display text-h2 text-fg">
                  {item.value}
                  <span className="sr-only"> {item.label}</span>
                </span>
                <Link href={item.href} className={buttonClasses("secondary", "sm")}>
                  {item.action}
                </Link>
              </div>
            </li>
          );
        })}
      </ul>
    </Card>
  );
}

export function OrganizationCardSkeleton() {
  return (
    <Card className="flex flex-col">
      <div className="flex items-center gap-4 px-6 pt-6">
        <Skeleton className="size-12 shrink-0 rounded-md" />
        <p className="text-h3">
          <SkeletonText chars={16} />
        </p>
      </div>
      <div className="flex min-w-0 items-center gap-2 px-6 pt-4 text-body-sm">
        <span className="shrink-0 font-semibold text-fg">Issuer</span>
        <SkeletonText chars={24} />
      </div>
      <ul aria-hidden="true" className="mx-6 mt-4 mb-6 grid grid-cols-2 gap-x-4 gap-y-6 border-t border-border pt-6">
        {[0, 1, 2, 3].map((i) => (
          <li key={i} className="flex items-start gap-3">
            <Skeleton className="mt-1 size-6 shrink-0 rounded-md" />
            <div className="flex min-w-0 flex-col items-start gap-2">
              <span className="text-h2">
                <SkeletonText chars={3} />
              </span>
              <Skeleton className="h-8 w-24 rounded-md" />
            </div>
          </li>
        ))}
      </ul>
    </Card>
  );
}

export async function ProfileCard() {
  const me = await apiGet<User>("/me");
  return (
    <Card className="flex flex-col">
      <div className="flex items-center gap-4 px-6 pt-6">
        <Avatar name={me.name} src={me.avatarUrl} size="lg" />
        <div className="flex min-w-0 flex-col items-start gap-1">
          <h2 className="max-w-full truncate text-h3 text-fg">{me.name}</h2>
          {me.roles[0] ? <Badge tone="ember">{me.roles[0]}</Badge> : null}
        </div>
      </div>
      <div className="flex min-w-0 items-center gap-2 px-6 pt-4 text-body-sm">
        <span className="shrink-0 font-semibold text-fg">User ID</span>
        <span className="min-w-0 truncate font-mono text-code-sm text-fg-2">{me.id}</span>
        <CopyButton value={me.id} label="Copy" className="ml-auto" />
      </div>
      <div className="px-6 pt-1">
        <Link href="/account" className={buttonClasses("quiet", "sm", "-ml-2")}>
          Your account
          <ArrowRight aria-hidden="true" size={16} strokeWidth={1.75} />
        </Link>
      </div>
      <div className="mx-6 mt-4 flex flex-1 flex-col gap-3 border-t border-border pt-6 pb-6">
        <h3 className="text-body font-semibold text-fg">Your roles</h3>
        <ul className="flex flex-wrap gap-2">
          {me.roles.map((role) => (
            <li key={role}>
              <Badge>{role}</Badge>
            </li>
          ))}
        </ul>
        <Link href="/admin/roles" className={buttonClasses("secondary", "sm", "mt-auto self-start")}>
          Manage roles
        </Link>
      </div>
    </Card>
  );
}

export function ProfileCardSkeleton() {
  return (
    <Card className="flex flex-col">
      <div className="flex items-center gap-4 px-6 pt-6">
        <Skeleton className="size-12 shrink-0 rounded-full" />
        <div className="flex min-w-0 flex-col items-start gap-1">
          <p className="text-h3">
            <SkeletonText chars={10} />
          </p>
          <Skeleton className="h-5 w-32 rounded-full" />
        </div>
      </div>
      <div className="flex min-w-0 items-center gap-2 px-6 pt-4 text-body-sm">
        <span className="shrink-0 font-semibold text-fg">User ID</span>
        <SkeletonText chars={28} />
      </div>
      <div className="px-6 pt-1">
        <Link href="/account" className={buttonClasses("quiet", "sm", "-ml-2")}>
          Your account
          <ArrowRight aria-hidden="true" size={16} strokeWidth={1.75} />
        </Link>
      </div>
      <div className="mx-6 mt-4 flex flex-1 flex-col gap-3 border-t border-border pt-6 pb-6">
        <h3 className="text-body font-semibold text-fg">Your roles</h3>
        <Skeleton className="h-5 w-32 rounded-full" />
        <Link href="/admin/roles" className={buttonClasses("secondary", "sm", "mt-auto self-start")}>
          Manage roles
        </Link>
      </div>
    </Card>
  );
}

const riskCard = "flex flex-col lg:col-span-2 min-[1400px]:col-span-1";

export async function RiskCard() {
  const [risks, users] = await Promise.all([apiGet<RiskEvent[]>("/risk-events"), apiGet<User[]>("/users")]);
  const userNames = Object.fromEntries(users.map((u) => [u.id, u.name]));
  const openRisks = risks.filter((event) => event.status === "open");
  return (
    <Card className={riskCard}>
      <CardHeader title="Sign-ins at risk" description={openRisks.length ? pluralize(openRisks.length, "open risk event") : undefined} />
      <div className="flex flex-1 flex-col px-6 pb-6">
        {openRisks.length === 0 ? (
          <div className="flex flex-1 flex-col items-center justify-center gap-2 py-8 text-center">
            <ShieldCheck aria-hidden="true" size={24} strokeWidth={1.75} className="text-success" />
            <p className="text-body font-semibold text-fg">No open risk events</p>
            <p className="max-w-[32ch] text-body-sm text-fg-3">No medium- or high-risk sign-in is waiting for a security administrator.</p>
          </div>
        ) : (
          <ul className="flex flex-1 flex-col">
            {openRisks.slice(0, 4).map((event) => (
              <li key={event.id} className="flex items-start gap-3 border-b border-border py-3 last:border-b-0">
                <StatusDot tone={event.level === "high" ? "danger" : "warning"} className="mt-2" />
                <div className="flex min-w-0 flex-col">
                  <span className="truncate text-body-sm font-medium text-fg">{event.userId ? (userNames[event.userId] ?? "Unknown user") : "Unknown user"}</span>
                  <span className="truncate text-caption text-fg-3">
                    {event.detail} · {formatRelative(event.time)}
                  </span>
                </div>
              </li>
            ))}
          </ul>
        )}
        <Link href="/admin/risk" className={buttonClasses("secondary", "sm", "mt-4 self-start")}>
          View risk events
        </Link>
      </div>
    </Card>
  );
}

export function RiskCardSkeleton() {
  return (
    <Card className={riskCard}>
      <CardHeader title="Sign-ins at risk" description={<SkeletonText chars={18} />} />
      <div className="flex flex-1 flex-col px-6 pb-6">
        <ul aria-hidden="true" className="flex flex-1 flex-col">
          {[0, 1].map((i) => (
            <li key={i} className="flex flex-col gap-1 border-b border-border py-3 last:border-b-0">
              <span className="text-body-sm">
                <SkeletonText chars={14} />
              </span>
              <span className="text-caption">
                <SkeletonText chars={56} />
              </span>
            </li>
          ))}
        </ul>
        <Link href="/admin/risk" className={buttonClasses("secondary", "sm", "mt-4 self-start")}>
          View risk events
        </Link>
      </div>
    </Card>
  );
}

export async function Posture() {
  const [overview, pending, reviews] = await Promise.all([apiGet<Overview>("/overview"), apiGet<AccessRequest[]>("/access-requests?status=pending"), apiGet<AccessReview[]>("/access-reviews")]);
  const { weakAdmins, expiringCredentials: expiring } = overview;
  const overdue = reviews.filter((review) => review.status === "overdue");
  const posture: { value: number; label: string; href: string; icon: LucideIcon }[] = [
    { value: weakAdmins.length, label: "Admins without phishing-resistant authentication", href: "/admin/users?view=weak-admins", icon: ShieldAlert },
    { value: expiring.length, label: "Application secrets expiring within 30 days", href: "/admin/applications?view=expiring", icon: FileKey },
    { value: pending.length, label: "Pending access requests", href: "/admin/requests", icon: Inbox },
    { value: overdue.length, label: overdue.length === 1 ? "Overdue access review" : "Overdue access reviews", href: "/admin/access-reviews", icon: ClipboardCheck },
  ];
  return (
    <Card className="overflow-hidden">
      <h2 className="sr-only">Security posture</h2>
      <ul className="grid grid-cols-2 gap-px bg-border xl:grid-cols-4">
        {posture.map((item) => {
          const Icon = item.icon;
          const clear = item.value === 0;
          return (
            <li key={item.label} className="bg-surface">
              <Link href={item.href} className="group flex h-full flex-col gap-3 p-4 transition-colors sm:p-6 duration-fast ease-brand hover:bg-hover">
                <span className="flex items-center justify-between">
                  <Icon aria-hidden="true" size={20} strokeWidth={1.75} className={clear ? "text-success" : "text-warning"} />
                  <ArrowRight aria-hidden="true" size={16} strokeWidth={1.75} className="text-fg-3 opacity-0 transition-opacity duration-fast group-hover:opacity-100" />
                </span>
                <span className="tnum font-display text-h1 text-fg">{item.value}</span>
                <span className="text-body-sm text-fg-2">{item.label}</span>
              </Link>
            </li>
          );
        })}
      </ul>
    </Card>
  );
}

export function PostureSkeleton() {
  return (
    <Card className="overflow-hidden">
      <ul aria-hidden="true" className="grid grid-cols-2 gap-px bg-border xl:grid-cols-4">
        {[40, 36, 24, 22].map((chars) => (
          <li key={chars} className="flex flex-col gap-3 bg-surface p-4 sm:p-6">
            <Skeleton className="size-5 rounded-sm" />
            <span className="text-h1">
              <SkeletonText chars={2} />
            </span>
            <span className="text-body-sm">
              <SkeletonText chars={chars} />
            </span>
          </li>
        ))}
      </ul>
    </Card>
  );
}

export async function Attention() {
  const [overview, users, reviews, risks] = await Promise.all([apiGet<Overview>("/overview"), apiGet<User[]>("/users"), apiGet<AccessReview[]>("/access-reviews"), apiGet<RiskEvent[]>("/risk-events")]);
  const userNames = Object.fromEntries(users.map((u) => [u.id, u.name]));
  const { weakAdmins, expiringCredentials: expiring } = overview;
  const invited = users.filter((u) => u.status === "invited");
  const overdue = reviews.filter((review) => review.status === "overdue");
  const openRisks = risks.filter((event) => event.status === "open");
  const attention: { id: string; tone: "danger" | "warning"; title: string; detail: string; action: string; href: string }[] = [
    ...openRisks.map((event) => ({
      id: event.id,
      tone: event.level === "high" ? ("danger" as const) : ("warning" as const),
      title: `${event.level === "high" ? "High" : "Medium"}-risk sign-in for ${event.userId ? (userNames[event.userId] ?? "an unknown user") : "an unknown user"}`,
      detail: `${event.detail} · ${formatRelative(event.time)}`,
      action: event.signInId ? "View sign-in" : "Review risk",
      href: event.signInId ? `/admin/sign-ins?event=${event.signInId}` : "/admin/risk",
    })),
    ...(weakAdmins.length
      ? [
          {
            id: "weak-admins",
            tone: "warning" as const,
            title: `${pluralize(weakAdmins.length, "administrator")} can still sign in without a passkey or security key`,
            detail: `${names(weakAdmins.map((u) => u.name))}.`,
            action: "Review admins",
            href: "/admin/users?view=weak-admins",
          },
        ]
      : []),
    ...expiring.map((credential) => ({
      id: credential.credentialId,
      tone: credential.daysLeft <= 14 ? ("danger" as const) : ("warning" as const),
      title: `${credential.appName} client secret expires in ${pluralize(credential.daysLeft, "day")}`,
      detail: `“${credential.label}” ${credential.lastUsedAt ? `was last used ${formatRelative(credential.lastUsedAt).toLowerCase()}` : "has never been used"}. Rotate it before ${credential.appName} sign-in stops working.`,
      action: "Rotate secret",
      href: `/admin/applications/${credential.appId}?tab=credentials`,
    })),
    ...overdue.map((review) => ({
      id: review.id,
      tone: "warning" as const,
      title: `${review.name} is overdue`,
      detail: `${review.decided} of ${review.total} decisions made. Reviewers: ${names(review.reviewers.map((r) => r.name))}.`,
      action: "Open review",
      href: `/admin/access-reviews/${review.id}`,
    })),
    ...(invited.length
      ? [
          {
            id: "invited",
            tone: "warning" as const,
            title: `${pluralize(invited.length, "invited user")} ${invited.length === 1 ? "hasn't" : "haven't"} enrolled yet`,
            detail: `${names(invited.map((u) => u.name))}. They can't sign in until they open their setup link.`,
            action: "Review invitations",
            href: "/admin/users?view=invited",
          },
        ]
      : []),
  ];

  return (
    <Card>
      <CardHeader title="Needs attention" description={attention.length ? `${pluralize(attention.length, "item")}, most urgent first.` : undefined} />
      {attention.length === 0 ? (
        <EmptyState
          icon={ShieldCheck}
          title="Nothing needs attention"
          description="Admin authentication, expiring secrets, risky sign-ins, access reviews and pending invitations all look fine."
        />
      ) : null}
      <ul>
        {attention.map((item) => (
          <li key={item.id} className="flex flex-col gap-4 border-t border-border px-6 py-4 sm:flex-row sm:items-center">
            <span
              className={cn(
                "grid size-8 shrink-0 place-items-center rounded-md border",
                item.tone === "danger" ? "border-danger/20 bg-danger-container text-danger" : "border-warning/20 bg-warning-container text-warning",
              )}
            >
              <TriangleAlert aria-hidden="true" size={16} strokeWidth={1.75} />
              <span className="sr-only">{item.tone === "danger" ? "Urgent" : "Warning"}</span>
            </span>
            <div className="flex min-w-0 flex-1 flex-col gap-1">
              <p className="text-body-sm font-semibold text-fg">{item.title}</p>
              <p className="text-body-sm text-fg-3">{item.detail}</p>
            </div>
            <Link href={item.href} className={buttonClasses("secondary", "sm", "self-start sm:self-center")}>
              {item.action}
            </Link>
          </li>
        ))}
      </ul>
    </Card>
  );
}

export function AttentionSkeleton() {
  return (
    <Card>
      <CardHeader title="Needs attention" description={<SkeletonText chars={30} />} />
      <ListSkeleton media className="border-t border-border" />
    </Card>
  );
}

function RecentSignInsHeader() {
  return (
    <CardHeader
      title="Recent sign-ins"
      description="Across all applications, newest first."
      actions={
        <Link href="/admin/sign-ins" className={buttonClasses("quiet", "sm")}>
          View all
          <ArrowRight aria-hidden="true" size={16} strokeWidth={1.75} />
        </Link>
      }
    />
  );
}

export async function RecentSignIns() {
  const [overview, users, applications] = await Promise.all([apiGet<Overview>("/overview"), apiGet<User[]>("/users"), apiGet<Application[]>("/applications")]);
  const userNames = Object.fromEntries(users.map((u) => [u.id, u.name]));
  const appNames = Object.fromEntries(applications.map((a) => [a.id, a.name]));
  const recent = overview.recentSignIns;
  return (
    <Card>
      <RecentSignInsHeader />
      {recent.length === 0 ? (
        <EmptyState icon={LogIn} title="No sign-ins yet" description="Sign-ins to Halo and every application appear here." />
      ) : (
        <div className="relative overflow-x-auto">
          <table className="w-full text-left text-body-sm">
            <caption className="sr-only">Eight most recent sign-ins</caption>
            <thead>
              <tr className="border-y border-border text-label text-fg-3">
                <th scope="col" className="h-10 px-3 pl-6 font-semibold">User</th>
                <th scope="col" className="h-10 px-3 font-semibold">Application</th>
                <th scope="col" className="h-10 px-3 font-semibold">Result</th>
                <th scope="col" className="hidden h-10 px-3 font-semibold md:table-cell">Location</th>
                <th scope="col" className="h-10 px-3 pr-6 text-right font-semibold">Time</th>
              </tr>
            </thead>
            <tbody>
              {recent.map((event) => (
                <tr key={event.id} className="h-12 border-b border-border last:border-b-0">
                  <td className="px-3 pl-6">
                    {event.userId ? (
                      <Link href={`/admin/users/${event.userId}`} className="font-medium text-fg hover:underline">
                        {signInUser(event, userNames)}
                      </Link>
                    ) : (
                      <span className="font-medium text-fg">{event.email}</span>
                    )}
                  </td>
                  <td className="px-3 text-fg-2">{signInApp(event, appNames)}</td>
                  <td className="px-3">
                    <span className="inline-flex items-center gap-2 text-fg-2">
                      <StatusDot tone={RESULT[event.result].tone} />
                      {RESULT[event.result].label}
                    </span>
                  </td>
                  <td className="hidden px-3 text-fg-3 md:table-cell">{event.location}</td>
                  <td className="tnum px-3 pr-6 text-right whitespace-nowrap text-fg-3">{formatRelative(event.time)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  );
}

export function RecentSignInsSkeleton() {
  return (
    <Card>
      <RecentSignInsHeader />
      <ul aria-hidden="true" className="border-t border-border">
        {[0, 1, 2, 3, 4, 5, 6, 7].map((i) => (
          <li key={i} className="flex h-12 items-center gap-6 border-t border-border px-6 first:border-t-0">
            <Skeleton className="h-3 w-28 rounded-sm" />
            <Skeleton className="h-3 w-24 rounded-sm" />
            <Skeleton className="h-3 w-16 rounded-sm" />
            <Skeleton className="h-3 ml-auto w-16 rounded-sm" />
          </li>
        ))}
      </ul>
    </Card>
  );
}

function WaitingHeader({ description }: { description: ReactNode }) {
  return (
    <CardHeader
      title="Waiting on you"
      description={description}
      actions={
        <Link href="/admin/requests" className={buttonClasses("quiet", "sm")}>
          All requests
        </Link>
      }
    />
  );
}

export async function WaitingOnYou() {
  const pending = await apiGet<AccessRequest[]>("/access-requests?status=pending");
  return (
    <Card>
      <WaitingHeader description={pending.length ? pluralize(pending.length, "pending access request") : undefined} />
      <CardBody>
        <PendingRequests requests={pending.slice(0, 3)} linkPeople />
      </CardBody>
    </Card>
  );
}

export function WaitingOnYouSkeleton() {
  return (
    <Card>
      <WaitingHeader description={<SkeletonText chars={26} />} />
      <ListSkeleton media rows={2} className="pb-2" />
    </Card>
  );
}

export async function Insights() {
  const policies = await apiGet<PolicyWithActivity[]>("/policies");
  return <ReportInsights policies={policies} />;
}

export function InsightsSkeleton() {
  return (
    <Card>
      <CardHeader title="Report-only insights" description="What report-only policies would have done in the last 24 hours. None of these sign-ins were changed." />
      <ListSkeleton rows={2} className="border-t border-border" />
    </Card>
  );
}

export async function ApplicationHealth() {
  const overview = await apiGet<Overview>("/overview");
  return (
    <Card>
      <CardHeader title="Applications" description={`${overview.counts.activeApplications} active`} />
      <CardBody className="flex flex-col gap-3">
        {overview.failureRates.map((app) => (
          <Link key={app.appId} href={`/admin/applications/${app.appId}`} className="-mx-2 flex items-center justify-between gap-3 rounded-md px-2 py-2 transition-colors duration-fast hover:bg-hover">
            <span className="flex min-w-0 flex-col">
              <span className="truncate text-body-sm font-medium text-fg">{app.appName}</span>
              <span className="text-caption text-fg-3">Sign-in failure rate, 7 days</span>
            </span>
            <Badge tone="warning">{app.failureRate7d}%</Badge>
          </Link>
        ))}
        {overview.failureRates.length === 0 ? <p className="text-body-sm text-fg-3">No application failed 1% or more of its sign-ins in the last 7 days.</p> : null}
      </CardBody>
    </Card>
  );
}

export function ApplicationHealthSkeleton() {
  return (
    <Card>
      <CardHeader title="Applications" description={<SkeletonText chars={8} />} />
      <CardBody>
        <p className="text-body-sm">
          <SkeletonText chars={70} />
        </p>
      </CardBody>
    </Card>
  );
}
