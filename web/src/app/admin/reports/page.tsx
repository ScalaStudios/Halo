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
import { formatDate, isoDay } from "@/lib/format";
import { frameMessages } from "@/lib/i18n/console/frame";
import { governanceMessages } from "@/lib/i18n/console/governance";
import { monitoringMessages } from "@/lib/i18n/console/monitoring";
import type { Locale } from "@/lib/i18n/locale";
import { requestLocale } from "@/lib/i18n/server";
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

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.reports.label };
}

type Params = { report?: string; days?: string; filter?: string; group?: string };

const REPORTS = ["methods", "usage", "inactive", "applications"] as const;

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

function periodText(period: ReportPeriod, days: number, locale: Locale) {
  const t = governanceMessages[locale].reports;
  const range = t.range(formatDate(period.from, locale), formatDate(period.to, locale));
  return period.limitedByRetention && days > period.retentionDays ? t.retention(range, period.retentionDays) : `${range}.`;
}

export default async function ReportsPage({ searchParams }: { searchParams: Promise<Params> }) {
  const [params, locale] = await Promise.all([searchParams, requestLocale()]);
  const t = governanceMessages[locale].reports;
  const title = frameMessages[locale].nav.items.reports.label;
  const report = REPORTS.some((r) => r === params.report) ? params.report! : "methods";
  return (
    <div className="flex flex-col gap-8">
      <PageHeader title={title} description={t.description} />
      <TabNav label={title} tabs={REPORTS.map((id) => ({ id, label: t.tabs[id] }))} active={report} hrefFor={(id) => href({ report: id === "methods" ? undefined : id })} />
      <div key={report} className="flex animate-enter flex-col gap-8">
        {report === "methods" ? <Registration params={params} /> : null}
        {report === "usage" ? <MethodUsage params={params} /> : null}
        {report === "inactive" ? <InactivePeople params={params} /> : null}
        {report === "applications" ? <ApplicationUsage params={params} /> : null}
      </div>
    </div>
  );
}

async function PeriodFilters({ report, days }: { report: string; days: number }) {
  const t = governanceMessages[await requestLocale()].reports;
  return <Filters label={t.period} items={[7, 30, 90, 365].map((n) => ({ label: t.days(n), href: href({ report, days: String(n) }), current: n === days }))} />;
}

async function Registration({ params }: { params: Params }) {
  const locale = await requestLocale();
  const g = governanceMessages[locale];
  const t = g.reports;
  const filter = params.filter && params.filter in t.filters ? (params.filter as RegistrationFilter) : undefined;
  const query = new URLSearchParams(Object.entries({ filter, group: params.group }).filter((entry): entry is [string, string] => Boolean(entry[1])));
  const [data, groups] = await Promise.all([apiGet<MethodRegistrationReport>(`/reports/method-registration?${query}`), apiGet<Group[]>("/groups")]);
  const total = data.counts.people;
  const share = (n: number) => `${t.share(n, total)}${total ? ` (${Math.round((n / total) * 100)}%)` : ""}`;
  return (
    <>
      <Card>
        <CardHeader
          title={t.registered}
          description={t.registeredDescription(total, Boolean(params.group), data.counts.filters["no-passkey"] ?? 0)}
        />
        <CardBody>
          <BarList label={t.perMethod} max={total} items={data.methods.map((m) => ({ label: monitoringMessages[locale].methods[m.method], value: m.people, detail: share(m.people) }))} />
        </CardBody>
      </Card>
      <div className="flex flex-wrap items-center justify-between gap-4">
        <Filters
          label={t.registered}
          items={[
            { label: g.everyone, count: total, href: href({ group: params.group }), current: !filter },
            ...Object.entries(t.filters).map(([id, label]) => ({
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
  const [data, locale] = await Promise.all([apiGet<MethodUsageReport>(`/reports/method-usage?${query}`), requestLocale()]);
  const t = governanceMessages[locale].reports;
  const methods = [...data.methods].sort((a, b) => b.signIns - a.signIns);
  const total = methods.reduce((sum, m) => sum + m.signIns, 0);
  return (
    <>
      <PeriodFilters report="usage" days={days} />
      <Card>
        <CardHeader
          title={t.byMethod}
          description={t.successful(total, periodText(data.period, days, locale))}
          actions={<ExportButton path={`/reports/method-usage?${query}&format=csv`} label={t.exportCsv} variant="secondary" size="sm" recorded={false} />}
        />
        <CardBody>
          <BarList
            label={t.successfulPerMethod}
            max={Math.max(0, ...methods.map((m) => m.signIns))}
            items={methods.map((m) => ({
              label: monitoringMessages[locale].methods[m.method],
              value: m.signIns,
              detail: m.signIns ? t.usage(m.signIns, m.people) : t.notUsed,
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
  const [data, groups, me, locale] = await Promise.all([apiGet<InactivePeopleReport>(`/reports/inactive-people?${query}`), apiGet<Group[]>("/groups"), apiGet<User>("/me"), requestLocale()]);
  const t = governanceMessages[locale].reports;
  const never = data.people.filter((p) => !p.lastSignInAt).length;
  return (
    <>
      <div className="flex flex-wrap items-center justify-between gap-4">
        <Filters label={t.noSignInFor} items={[30, 60, 90, 180].map((n) => ({ label: t.days(n), href: href({ report: "inactive", days: String(n), group: params.group }), current: n === days }))} />
        <GroupFilter groups={groups} value={params.group ?? ""} query={{ report: "inactive", days: String(days) }} />
      </div>
      <p className="text-body-sm text-fg-3">{t.inactive(data.people.length, days, never)}</p>
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
  const [data, locale] = await Promise.all([apiGet<ApplicationUsageReport>(`/reports/application-usage?${query}`), requestLocale()]);
  const unused = data.applications.filter((a) => a.signIns === 0).length;
  return (
    <>
      <PeriodFilters report="applications" days={days} />
      <p className="text-body-sm text-fg-3">{governanceMessages[locale].reports.applications(periodText(data.period, days, locale), unused)}</p>
      <ApplicationsTable key={query} applications={data.applications} exportPath={`/reports/application-usage?${query}&format=csv`} />
    </>
  );
}
