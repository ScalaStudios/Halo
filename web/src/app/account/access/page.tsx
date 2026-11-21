import type { Metadata } from "next";
import Link from "next/link";
import { empty, list, row } from "@/components/account/list";
import { ActivateRole, CancelRequest, DeactivateRole, ExtendAccess, RequestAccess } from "@/components/governance/request-access";
import { GuestAccessButton } from "@/components/directory/guest-access";
import { PendingRequests } from "@/components/governance/requests";
import { approversLabel, dueLabel, grantLabel, joinNames, REQUEST_STATUS, requestDurationLabel, ReviewProgress } from "@/components/governance/shared";
import { ROLES } from "@/components/provisioning/shared";
import { keysOf } from "@/components/settings/roles";
import { Badge } from "@/components/ui/badge";
import { PageHeader } from "@/components/ui/page-header";
import { SectionTitle } from "@/components/ui/section-title";
import { apiGet } from "@/lib/api/server";
import { formatDateTime, formatRelative } from "@/lib/format";
import { STATUS } from "@/lib/labels";
import type { AccessPackage, AccessRequest, AccessReview, NamedRef } from "@/lib/governance-types";
import type { Organization, User } from "@/lib/types";

export const metadata: Metadata = { title: "Access" };

function requestDetail(request: AccessRequest, userId: string): string {
  const who =
    request.requester.id !== userId ? `For ${request.requester.name} · ` : request.requestedBy && request.requestedBy.id !== userId ? `Asked by ${request.requestedBy.name} · ` : "";
  switch (request.status) {
    case "pending":
      return `${who}${requestDurationLabel(request)}${request.stages > 1 ? ` · Stage ${request.stage} of ${request.stages}` : ""} · Waiting on ${approversLabel(request.approvers)} · Asked ${formatRelative(request.createdAt).toLowerCase()}`;
    case "approved":
      return `${who}Approved by ${request.decidedBy?.name ?? "an approver"} · ${grantLabel(request)}`;
    case "denied":
      return `${who}Denied by ${request.decidedBy?.name ?? "an approver"}`;
    case "cancelled":
      return `${who}Cancelled`;
    case "expired":
      return `${who}Nobody decided in time, so Halo closed it`;
  }
}

export default async function AccessPage() {
  const [user, org, packages, requests, grants, approvals, reviews, guests, reports] = await Promise.all([
    apiGet<User>("/me"),
    apiGet<Organization>("/organization"),
    apiGet<AccessPackage[]>("/me/access-packages"),
    apiGet<AccessRequest[]>("/me/access-requests"),
    apiGet<AccessRequest[]>("/me/access-grants"),
    apiGet<AccessRequest[]>("/me/approvals"),
    apiGet<AccessReview[]>("/me/reviews"),
    apiGet<User[]>("/me/guests"),
    apiGet<NamedRef[]>("/me/direct-reports"),
  ]);
  const pendingIds = requests.filter((r) => r.status === "pending" && r.requester.id === user.id).map((r) => r.package.id);
  const admin = user.roles.length > 0;
  const standing = keysOf(user);
  const eligible = user.eligibleRoles.filter((role) => !standing.includes(role));

  return (
    <div className="flex flex-col gap-12">
      <PageHeader
        size="large"
        title="Access"
        description="What you can reach and why. Groups decide which applications you see; roles decide what you can change in Halo."
        actions={packages.length ? <RequestAccess packages={packages} pendingIds={pendingIds} reports={reports} /> : null}
      />

      {packages.length === 0 ? (
        <p className={empty}>
          There&apos;s nothing to request in Halo yet. If you need an application or group you don&apos;t have, ask a Halo administrator at {org.name}.
        </p>
      ) : null}

      {approvals.length ? (
        <section className="flex flex-col gap-4">
          <SectionTitle
            title="Waiting for your approval"
            description="You approve requests for these packages. The requester gets an email with your decision."
          />
          <div className="rounded-lg border border-border bg-surface p-6">
            <PendingRequests requests={approvals} />
          </div>
        </section>
      ) : null}

      {reviews.length ? (
        <section className="flex flex-col gap-4">
          <SectionTitle title="Reviews assigned to you" description="Confirm or remove each member's access before the due date." />
          <ul className={list}>
            {reviews.map((review) => (
              <li key={review.id} className="flex flex-col gap-4 border-t border-border px-6 py-4 first:border-t-0 sm:flex-row sm:items-center sm:gap-6">
                <div className="flex min-w-0 flex-1 flex-col gap-1">
                  <p className="flex flex-wrap items-center gap-2 text-body-sm font-semibold text-fg">
                    {review.name}
                    {review.status === "overdue" ? <Badge tone="danger">Overdue</Badge> : null}
                  </p>
                  <p className="text-body-sm text-fg-3">
                    {review.group.name} · {dueLabel(review)}
                  </p>
                </div>
                <ReviewProgress review={review} className="w-full sm:w-40" />
                <Link
                  href={admin ? `/admin/access-reviews/${review.id}` : `/account/reviews/${review.id}`}
                  className="shrink-0 text-body-sm text-link underline underline-offset-3 hover:text-ember-hover"
                >
                  Open review
                </Link>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      {guests.length ? (
        <section className="flex flex-col gap-4">
          <SectionTitle title="Guests you sponsor" description="You're responsible for these external accounts. Halo emails you two weeks before access ends and suspends the account then unless you extend it." />
          <ul className={list}>
            {guests.map((guest) => (
              <li key={guest.id} className={row}>
                <div className="flex min-w-0 flex-col gap-1">
                  <p className="flex flex-wrap items-center gap-2 text-body-sm font-semibold text-fg">
                    {guest.name}
                    {guest.status === "active" || guest.status === "invited" ? null : <Badge tone={STATUS[guest.status].tone}>{STATUS[guest.status].label}</Badge>}
                  </p>
                  <p className="text-body-sm text-fg-3">
                    {guest.email}
                    {guest.expiresAt ? ` · Access ends ${formatDateTime(guest.expiresAt)}` : null}
                  </p>
                </div>
                {guest.status === "active" || guest.status === "invited" ? <GuestAccessButton guest={guest} /> : null}
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      <section className="flex flex-col gap-4">
        <SectionTitle title="Your requests" description="You get an email when an approver decides." />
        {requests.length ? (
          <ul className={list}>
            {requests.map((request) => (
              <li key={request.id} className={row}>
                <div className="flex min-w-0 flex-col gap-1">
                  <p className="flex flex-wrap items-center gap-2 text-body-sm font-semibold text-fg">
                    {request.package.name}
                    <Badge tone={REQUEST_STATUS[request.status].tone}>{REQUEST_STATUS[request.status].label}</Badge>
                    {request.extends ? <Badge tone="neutral">Extension</Badge> : null}
                  </p>
                  <p className="text-body-sm text-fg-3">{requestDetail(request, user.id)}</p>
                  {request.decisionNote ? <p className="text-body-sm text-fg-3">“{request.decisionNote}”</p> : null}
                </div>
                {request.status === "pending" ? (
                  <CancelRequest request={request} />
                ) : (
                  <span className="shrink-0 text-caption text-fg-3">{formatRelative(request.decidedAt ?? request.createdAt)}</span>
                )}
              </li>
            ))}
          </ul>
        ) : (
          <p className={empty}>You haven&apos;t requested access yet.</p>
        )}
      </section>

      <section className="flex flex-col gap-4">
        <SectionTitle title="Access you were granted" description="Approved packages. Halo removes the access when it ends." />
        {grants.length ? (
          <ul className={list}>
            {grants.map((grant) => {
              const pkg = packages.find((p) => p.id === grant.package.id);
              const asked = requests.some((r) => r.status === "pending" && r.extends === grant.id);
              return (
                <li key={grant.id} className={row}>
                  <div className="flex min-w-0 flex-col gap-1">
                    <p className="text-body-sm font-semibold text-fg">{grant.package.name}</p>
                    <p className="text-body-sm text-fg-3">Groups: {joinNames(grant.groups)}</p>
                  </div>
                  <div className="flex shrink-0 flex-wrap items-center gap-3">
                    <span className="text-caption text-fg-3">{asked ? `${grantLabel(grant)} · Extension requested` : grantLabel(grant)}</span>
                    {grant.expiresAt && pkg && !asked ? <ExtendAccess grant={grant} pkg={pkg} /> : null}
                  </div>
                </li>
              );
            })}
          </ul>
        ) : (
          <p className={empty}>No approved access right now.</p>
        )}
      </section>

      <section className="flex flex-col gap-4">
        <SectionTitle
          title="Your roles"
          description="Administrative roles in Halo. Most people have none. Roles you're eligible for stay off until you activate them."
        />
        {standing.length + eligible.length > 0 ? (
          <ul className={list}>
            {standing.map((role) => (
              <li key={role} className={row}>
                <div className="flex min-w-0 flex-col gap-1">
                  <p className="text-body-sm font-semibold text-fg">{ROLES[role].label}</p>
                  <p className="text-body-sm text-fg-3">Lets you manage Halo for {org.name}.</p>
                </div>
                <Link href="/admin" className="shrink-0 text-body-sm text-link underline underline-offset-3 hover:text-ember-hover">
                  Open Halo administration
                </Link>
              </li>
            ))}
            {eligible.map((role) => {
              const active = user.activations.find((a) => a.role === role);
              return (
                <li key={`eligible-${role}`} className={row}>
                  <div className="flex min-w-0 flex-col gap-1">
                    <p className="flex flex-wrap items-center gap-2 text-body-sm font-semibold text-fg">
                      {ROLES[role].label}
                      <Badge tone={active ? "success" : "neutral"}>{active ? "Active" : "Eligible"}</Badge>
                    </p>
                    <p className="text-body-sm text-fg-3">
                      {active ? `Until ${formatDateTime(active.expiresAt)} · “${active.justification}”` : "Activate it for up to 8 hours when you need it."}
                    </p>
                  </div>
                  {active ? <DeactivateRole activation={active} /> : <ActivateRole role={role} />}
                </li>
              );
            })}
          </ul>
        ) : (
          <p className={empty}>You don&apos;t have an administrative role in Halo.</p>
        )}
      </section>
    </div>
  );
}
