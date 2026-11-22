import type { Metadata } from "next";
import Link from "next/link";
import { empty, list, row } from "@/components/account/list";
import { ActivateRole, CancelRequest, DeactivateRole, ExtendAccess, RequestAccess } from "@/components/governance/request-access";
import { GuestAccessButton } from "@/components/directory/guest-access";
import { PendingRequests } from "@/components/governance/requests";
import { approversLabel, dueLabel, grantLabel, joinNames, REQUEST_STATUS, requestDurationLabel, ReviewProgress } from "@/components/governance/shared";
import { rolesFor } from "@/components/provisioning/shared";
import { keysOf } from "@/components/settings/roles";
import { Badge } from "@/components/ui/badge";
import { PageHeader } from "@/components/ui/page-header";
import { SectionTitle } from "@/components/ui/section-title";
import { apiGet } from "@/lib/api/server";
import { formatDateTime, formatRelative } from "@/lib/format";
import { accountMessages } from "@/lib/i18n/account";
import { governanceMessages } from "@/lib/i18n/console/governance";
import { monitoringMessages } from "@/lib/i18n/console/monitoring";
import type { Locale } from "@/lib/i18n/locale";
import { requestLocale } from "@/lib/i18n/server";
import { STATUS } from "@/lib/labels";
import type { AccessPackage, AccessRequest, AccessReview, NamedRef } from "@/lib/governance-types";
import type { Organization, User } from "@/lib/types";

export async function generateMetadata(): Promise<Metadata> {
  return { title: accountMessages(await requestLocale()).nav.access };
}

function requestDetail(request: AccessRequest, userId: string, locale: Locale): string {
  const t = accountMessages(locale).access;
  const who =
    request.requester.id !== userId ? t.forPerson(request.requester.name) : request.requestedBy && request.requestedBy.id !== userId ? t.askedBy(request.requestedBy.name) : "";
  switch (request.status) {
    case "pending":
      return `${who}${requestDurationLabel(request, locale)}${request.stages > 1 ? ` · ${governanceMessages[locale].requests.stageOf(request.stage, request.stages)}` : ""} · ${t.waitingOn(approversLabel(request.approvers, locale))} · ${t.asked(formatRelative(request.createdAt, locale).toLowerCase())}`;
    case "approved":
      return `${who}${t.approvedBy(request.decidedBy?.name ?? t.anApprover)} · ${grantLabel(request, locale)}`;
    case "denied":
      return `${who}${t.deniedBy(request.decidedBy?.name ?? t.anApprover)}`;
    case "cancelled":
      return `${who}${t.cancelled}`;
    case "expired":
      return `${who}${t.expired}`;
  }
}

export default async function AccessPage() {
  const [user, org, packages, requests, grants, approvals, reviews, guests, reports, locale] = await Promise.all([
    apiGet<User>("/me"),
    apiGet<Organization>("/organization"),
    apiGet<AccessPackage[]>("/me/access-packages"),
    apiGet<AccessRequest[]>("/me/access-requests"),
    apiGet<AccessRequest[]>("/me/access-grants"),
    apiGet<AccessRequest[]>("/me/approvals"),
    apiGet<AccessReview[]>("/me/reviews"),
    apiGet<User[]>("/me/guests"),
    apiGet<NamedRef[]>("/me/direct-reports"),
    requestLocale(),
  ]);
  const account = accountMessages(locale);
  const t = account.access;
  const governance = governanceMessages[locale];
  const roles = rolesFor(locale);
  const pendingIds = requests.filter((r) => r.status === "pending" && r.requester.id === user.id).map((r) => r.package.id);
  const admin = user.roles.length > 0;
  const standing = keysOf(user);
  const eligible = user.eligibleRoles.filter((role) => !standing.includes(role));

  return (
    <div className="flex flex-col gap-12">
      <PageHeader
        size="large"
        title={account.nav.access}
        description={t.description}
        actions={packages.length ? <RequestAccess packages={packages} pendingIds={pendingIds} reports={reports} /> : null}
      />

      {packages.length === 0 ? (
        <p className={empty}>{t.nothingToRequest(org.name)}</p>
      ) : null}

      {approvals.length ? (
        <section className="flex flex-col gap-4">
          <SectionTitle
            title={t.approvals}
            description={t.approvalsHint}
          />
          <div className="rounded-lg border border-border bg-surface p-6">
            <PendingRequests requests={approvals} />
          </div>
        </section>
      ) : null}

      {reviews.length ? (
        <section className="flex flex-col gap-4">
          <SectionTitle title={t.reviews} description={t.reviewsHint} />
          <ul className={list}>
            {reviews.map((review) => (
              <li key={review.id} className="flex flex-col gap-4 border-t border-border px-6 py-4 first:border-t-0 sm:flex-row sm:items-center sm:gap-6">
                <div className="flex min-w-0 flex-1 flex-col gap-1">
                  <p className="flex flex-wrap items-center gap-2 text-body-sm font-semibold text-fg">
                    {review.name}
                    {review.status === "overdue" ? <Badge tone="danger">{governance.reviewStatus.overdue}</Badge> : null}
                  </p>
                  <p className="text-body-sm text-fg-3">
                    {review.group.name} · {dueLabel(review, locale)}
                  </p>
                </div>
                <ReviewProgress review={review} className="w-full sm:w-40" locale={locale} />
                <Link
                  href={admin ? `/admin/access-reviews/${review.id}` : `/account/reviews/${review.id}`}
                  className="shrink-0 text-body-sm text-link underline underline-offset-3 hover:text-ember-hover"
                >
                  {t.openReview}
                </Link>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      {guests.length ? (
        <section className="flex flex-col gap-4">
          <SectionTitle title={t.guests} description={t.guestsHint} />
          <ul className={list}>
            {guests.map((guest) => (
              <li key={guest.id} className={row}>
                <div className="flex min-w-0 flex-col gap-1">
                  <p className="flex flex-wrap items-center gap-2 text-body-sm font-semibold text-fg">
                    {guest.name}
                    {guest.status === "active" || guest.status === "invited" ? null : <Badge tone={STATUS[guest.status].tone}>{monitoringMessages[locale].statuses[guest.status]}</Badge>}
                  </p>
                  <p className="text-body-sm text-fg-3">
                    {guest.email}
                    {guest.expiresAt ? t.guestEnds(formatDateTime(guest.expiresAt, locale)) : null}
                  </p>
                </div>
                {guest.status === "active" || guest.status === "invited" ? <GuestAccessButton guest={guest} /> : null}
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      <section className="flex flex-col gap-4">
        <SectionTitle title={t.requests} description={t.requestsHint} />
        {requests.length ? (
          <ul className={list}>
            {requests.map((request) => (
              <li key={request.id} className={row}>
                <div className="flex min-w-0 flex-col gap-1">
                  <p className="flex flex-wrap items-center gap-2 text-body-sm font-semibold text-fg">
                    {request.package.name}
                    <Badge tone={REQUEST_STATUS[request.status].tone}>{governance.requestStatus[request.status]}</Badge>
                    {request.extends ? <Badge tone="neutral">{t.extension}</Badge> : null}
                  </p>
                  <p className="text-body-sm text-fg-3">{requestDetail(request, user.id, locale)}</p>
                  {request.decisionNote ? <p className="text-body-sm text-fg-3">“{request.decisionNote}”</p> : null}
                </div>
                {request.status === "pending" ? (
                  <CancelRequest request={request} />
                ) : (
                  <span className="shrink-0 text-caption text-fg-3">{formatRelative(request.decidedAt ?? request.createdAt, locale)}</span>
                )}
              </li>
            ))}
          </ul>
        ) : (
          <p className={empty}>{t.noRequests}</p>
        )}
      </section>

      <section className="flex flex-col gap-4">
        <SectionTitle title={t.grants} description={t.grantsHint} />
        {grants.length ? (
          <ul className={list}>
            {grants.map((grant) => {
              const pkg = packages.find((p) => p.id === grant.package.id);
              const asked = requests.some((r) => r.status === "pending" && r.extends === grant.id);
              return (
                <li key={grant.id} className={row}>
                  <div className="flex min-w-0 flex-col gap-1">
                    <p className="text-body-sm font-semibold text-fg">{grant.package.name}</p>
                    <p className="text-body-sm text-fg-3">{t.groups(joinNames(grant.groups, locale))}</p>
                  </div>
                  <div className="flex shrink-0 flex-wrap items-center gap-3">
                    <span className="text-caption text-fg-3">{asked ? t.extensionRequested(grantLabel(grant, locale)) : grantLabel(grant, locale)}</span>
                    {grant.expiresAt && pkg && !asked ? <ExtendAccess grant={grant} pkg={pkg} /> : null}
                  </div>
                </li>
              );
            })}
          </ul>
        ) : (
          <p className={empty}>{t.noGrants}</p>
        )}
      </section>

      <section className="flex flex-col gap-4">
        <SectionTitle
          title={t.roles}
          description={t.rolesHint}
        />
        {standing.length + eligible.length > 0 ? (
          <ul className={list}>
            {standing.map((role) => (
              <li key={role} className={row}>
                <div className="flex min-w-0 flex-col gap-1">
                  <p className="text-body-sm font-semibold text-fg">{roles[role].label}</p>
                  <p className="text-body-sm text-fg-3">{t.manage(org.name)}</p>
                </div>
                <Link href="/admin" className="shrink-0 text-body-sm text-link underline underline-offset-3 hover:text-ember-hover">
                  {t.openAdmin}
                </Link>
              </li>
            ))}
            {eligible.map((role) => {
              const active = user.activations.find((a) => a.role === role);
              return (
                <li key={`eligible-${role}`} className={row}>
                  <div className="flex min-w-0 flex-col gap-1">
                    <p className="flex flex-wrap items-center gap-2 text-body-sm font-semibold text-fg">
                      {roles[role].label}
                      <Badge tone={active ? "success" : "neutral"}>{active ? t.active : t.eligible}</Badge>
                    </p>
                    <p className="text-body-sm text-fg-3">
                      {active ? t.activeUntil(formatDateTime(active.expiresAt, locale), active.justification) : t.activateHint}
                    </p>
                  </div>
                  {active ? <DeactivateRole activation={active} /> : <ActivateRole role={role} />}
                </li>
              );
            })}
          </ul>
        ) : (
          <p className={empty}>{t.noRoles}</p>
        )}
      </section>
    </div>
  );
}
