import Link from "next/link";
import { hasRole } from "@/components/settings/roles";
import { ApplicationsTable, Bar, GroupFilter, InactiveTable, RegistrationTable } from "@/components/monitoring/reports";
import { ExportButton } from "@/components/settings/export-button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { LinkProgress } from "@/components/ui/navigation-progress";
import { PageHeader } from "@/components/ui/page-header";
import { TabNav } from "@/components/ui/tab-nav";
import { apiGet } from "@/lib/api/server";
import { cn } from "@/lib/cn";
import { formatDate, isoDay, pluralize } from "@/lib/format";
import { METHOD } from "@/lib/labels";
import type {
  ApplicationUsageReport,
  Group,
  InactivePeopleReport,
  MethodRegistrationReport,
  MethodUsageReport,
  RegistrationFilter,
  ReportPeriod,
  User,
} from "@/lib/types";

export const metadata = { title: "Reports" };

type Params = { report?: string; days?: string; filter?: string; group?: string };

const REPORTS = [
  { id: "methods", label: "Sign-in methods" },
  { id: "usage", label: "Method usage" },
  { id: "inactive", label: "Inactive people" },
  { id: "applications", label: "Application usage" },
];

const FILTERS: Record<RegistrationFilter, string> = {
  "no-passkey": "No passkey",
  "password-only": "Password only",
  "authenticator-only": "Authenticator app only",
  "no-method": "Email or identity provider only",
};

function href(params: Params) {
  const query = new URLSearchParams(Object.entries(params).filter((entry): entry is [string, string] => Boolean(entry[1])));
  return `/admin/reports${query.size ? `?${query}` : ""}`;
}

function pick(raw: string | undefined, options: number[], fallback: number) {
  return options.find((n) => String(n) === raw) ?? fallback;
}

function Filters({ label, items }: { label: string; items: { label: string; href: string; current: boolean; count?: number }[] }) {
  return (
    <nav aria-label={label} className="-mx-1 flex min-w-0 items-center gap-1 overflow-x-auto px-1">
      {items.map((item) => (
        <Link
          key={item.href}
          href={item.href}
          scroll={false}
          aria-current={item.current ? "true" : undefined}
          className={cn(
            "flex h-8 shrink-0 items-center gap-2 rounded-md px-3 text-body-sm font-medium whitespace-nowrap transition-colors duration-fast ease-brand",
            item.current ? "bg-press text-fg" : "text-fg-3 hover:bg-hover hover:text-fg",
          )}
        >
          {item.label}
          {item.count !== undefined ? <span className="tnum text-caption text-fg-3">{item.count}</span> : null}
          <LinkProgress />
        </Link>
      ))}
    </nav>
  );
}

function BarList({ label, items, max }: { label: string; items: { label: string; value: number; detail: string }[]; max: number }) {
  return (
    <ul aria-label={label} className="flex flex-col gap-4">
      {items.map((item) => (
        <li key={item.label} className="flex flex-col gap-2">
          <span className="flex flex-wrap items-baseline justify-between gap-x-4 text-body-sm">
            <span className="text-fg">{item.label}</span>
            <span className="tnum text-fg-3">{item.detail}</span>
          </span>
          <Bar value={item.value} max={max} />
        </li>
      ))}
    </ul>
  );
}

function periodText(period: ReportPeriod, days: number) {
  const range = `from ${formatDate(period.from)} to ${formatDate(period.to)}`;
  return period.limitedByRetention && days > period.retentionDays
    ? `${range}. The sign-in log keeps ${pluralize(period.retentionDays, "day")}, so older sign-ins are not counted.`
    : `${range}.`;
}

export default async function ReportsPage({ searchParams }: { searchParams: Promise<Params> }) {
  const params = await searchParams;
  const report = REPORTS.some((r) => r.id === params.report) ? params.report! : "methods";
  return (
    <div className="flex flex-col gap-8">
      <PageHeader title="Reports" description="Who signs in, with which methods and to which applications. Each report downloads as CSV and is available in the API." />
      <TabNav label="Reports" tabs={REPORTS} active={report} hrefFor={(id) => href({ report: id === "methods" ? undefined : id })} />
      <div key={report} className="flex animate-enter flex-col gap-8">
        {report === "methods" ? <Registration params={params} /> : null}
        {report === "usage" ? <MethodUsage params={params} /> : null}
        {report === "inactive" ? <InactivePeople params={params} /> : null}
        {report === "applications" ? <ApplicationUsage params={params} /> : null}
      </div>
    </div>
  );
}

function PeriodFilters({ report, days }: { report: string; days: number }) {
  return <Filters label="Period" items={[7, 30, 90, 365].map((n) => ({ label: `${n} days`, href: href({ report, days: String(n) }), current: n === days }))} />;
}

async function Registration({ params }: { params: Params }) {
  const filter = params.filter && params.filter in FILTERS ? (params.filter as RegistrationFilter) : undefined;
  const query = new URLSearchParams(Object.entries({ filter, group: params.group }).filter((entry): entry is [string, string] => Boolean(entry[1])));
  const [data, groups] = await Promise.all([apiGet<MethodRegistrationReport>(`/reports/method-registration?${query}`), apiGet<Group[]>("/groups")]);
  const total = data.counts.people;
  const share = (n: number) => `${n} of ${total}${total ? ` (${Math.round((n / total) * 100)}%)` : ""}`;
  return (
    <>
      <Card>
        <CardHeader
          title="Registered methods"
          description={`${pluralize(total, "active person", "active people")}${params.group ? " in this group" : ""}. ${pluralize(data.counts.filters["no-passkey"] ?? 0, "person has", "people have")} no passkey or security key.`}
        />
        <CardBody>
          <BarList label="People per registered method" max={total} items={data.methods.map((m) => ({ label: METHOD[m.method].label, value: m.people, detail: share(m.people) }))} />
        </CardBody>
      </Card>
      <div className="flex flex-wrap items-center justify-between gap-4">
        <Filters
          label="Registered methods"
          items={[
            { label: "Everyone", count: total, href: href({ group: params.group }), current: !filter },
            ...Object.entries(FILTERS).map(([id, label]) => ({
              label,
              count: data.counts.filters[id as RegistrationFilter] ?? 0,
              href: href({ filter: id, group: params.group }),
              current: id === filter,
            })),
          ]}
        />
        <GroupFilter groups={groups} value={params.group ?? ""} query={filter ? { filter } : {}} />
      </div>
      <RegistrationTable key={query.toString()} people={data.people} exportPath={`/reports/method-registration?${query}&format=csv`} />
    </>
  );
}

async function MethodUsage({ params }: { params: Params }) {
  const days = pick(params.days, [7, 30, 90, 365], 30);
  const query = `from=${isoDay(-days)}`;
  const data = await apiGet<MethodUsageReport>(`/reports/method-usage?${query}`);
  const methods = [...data.methods].sort((a, b) => b.signIns - a.signIns);
  const total = methods.reduce((sum, m) => sum + m.signIns, 0);
  return (
    <>
      <PeriodFilters report="usage" days={days} />
      <Card>
        <CardHeader
          title="Sign-ins by method"
          description={`${pluralize(total, "successful sign-in")} ${periodText(data.period, days)}`}
          actions={<ExportButton path={`/reports/method-usage?${query}&format=csv`} label="Export CSV" variant="secondary" size="sm" recorded={false} />}
        />
        <CardBody>
          <BarList
            label="Successful sign-ins per method"
            max={Math.max(0, ...methods.map((m) => m.signIns))}
            items={methods.map((m) => ({
              label: METHOD[m.method].label,
              value: m.signIns,
              detail: m.signIns ? `${pluralize(m.signIns, "sign-in")} by ${pluralize(m.people, "person", "people")}` : "Not used",
            }))}
          />
        </CardBody>
      </Card>
    </>
  );
}

async function InactivePeople({ params }: { params: Params }) {
  const days = pick(params.days, [30, 60, 90, 180], 90);
  const query = new URLSearchParams({ days: String(days), ...(params.group ? { group: params.group } : {}) });
  const [data, groups, me] = await Promise.all([apiGet<InactivePeopleReport>(`/reports/inactive-people?${query}`), apiGet<Group[]>("/groups"), apiGet<User>("/me")]);
  const never = data.people.filter((p) => !p.lastSignInAt).length;
  return (
    <>
      <div className="flex flex-wrap items-center justify-between gap-4">
        <Filters label="No sign-in for" items={[30, 60, 90, 180].map((n) => ({ label: `${n} days`, href: href({ report: "inactive", days: String(n), group: params.group }), current: n === days }))} />
        <GroupFilter groups={groups} value={params.group ?? ""} query={{ report: "inactive", days: String(days) }} />
      </div>
      <p className="text-body-sm text-fg-3">
        {pluralize(data.people.length, "active or invited person", "active or invited people")} without a sign-in in the last {days} days, including {never} who never signed in. People who never signed in
        count from when they were added.
      </p>
      <InactiveTable
        key={query.toString()}
        people={data.people}
        exportPath={`/reports/inactive-people?${query}&format=csv`}
        canRevoke={hasRole(me, "user_admin", "helpdesk_admin", "security_admin")}
        canSuspend={hasRole(me, "user_admin", "security_admin")}
      />
    </>
  );
}

async function ApplicationUsage({ params }: { params: Params }) {
  const days = pick(params.days, [7, 30, 90, 365], 30);
  const query = `from=${isoDay(-days)}`;
  const data = await apiGet<ApplicationUsageReport>(`/reports/application-usage?${query}`);
  const unused = data.applications.filter((a) => a.signIns === 0).length;
  return (
    <>
      <PeriodFilters report="applications" days={days} />
      <p className="text-body-sm text-fg-3">
        Successful sign-ins and the people who made them {periodText(data.period, days)} {pluralize(unused, "application")} had no sign-ins. Bookmarks and service applications are not listed.
      </p>
      <ApplicationsTable key={query} applications={data.applications} exportPath={`/reports/application-usage?${query}&format=csv`} />
    </>
  );
}
