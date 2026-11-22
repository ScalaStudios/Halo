"use client";

import { useMutation } from "@/components/console/use-mutation";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api/client";
import { formatRelative } from "@/lib/format";
import { accountMessages } from "@/lib/i18n/account";
import { useLocale } from "@/lib/i18n/client";
import type { NamedRef } from "@/lib/governance-types";
import type { JoinableGroup, JoinRequest } from "@/lib/types";
import { list, row } from "./list";

export function JoinGroup({ group }: { group: JoinableGroup }) {
  const { pending, run, toast } = useMutation();
  const t = accountMessages(useLocale()).groups;

  function join() {
    void run(
      "join",
      () => api<{ status: "joined" | "requested" }>(`/me/groups/${group.id}/join`, { method: "POST" }),
      ({ status }) =>
        toast(
          status === "joined"
            ? { title: t.joined(group.name), description: t.joinedHint }
            : { title: t.requestSent, description: t.requestSentHint(group.name) },
        ),
    );
  }

  return (
    <Button size="sm" variant="secondary" className="shrink-0 self-start" aria-label={group.joinPolicy === "open" ? t.joinGroup(group.name) : t.askToJoinGroup(group.name)} loading={pending === "join"} onClick={join}>
      {group.joinPolicy === "open" ? t.join : t.askToJoin}
    </Button>
  );
}

export function JoinRequests({ groupId, groupName, requests }: { groupId: string; groupName: string; requests: JoinRequest[] }) {
  const { pending, run, toast } = useMutation();
  const locale = useLocale();
  const t = accountMessages(locale).groups;

  function decide(user: NamedRef, decision: "approve" | "deny") {
    void run(
      `${decision}:${user.id}`,
      () => api(`/groups/${groupId}/join-requests/${user.id}/${decision}`, { method: "POST" }),
      () =>
        toast(
          decision === "approve"
            ? { title: t.added(user.name), description: t.addedHint(groupName) }
            : { title: t.declined(user.name), description: t.recorded },
        ),
    );
  }

  return (
    <ul className={list}>
      {requests.map(({ user, createdAt }) => (
        <li key={user.id} className={row}>
          <div className="flex min-w-0 flex-col gap-1">
            <p className="text-body-sm font-semibold text-fg">{user.name}</p>
            <p className="text-body-sm text-fg-3">
              {user.email} · {t.asked(formatRelative(createdAt, locale).toLowerCase())}
            </p>
          </div>
          <div className="flex shrink-0 gap-2">
            <Button size="sm" variant="secondary" aria-label={t.declinePerson(user.name)} loading={pending === `deny:${user.id}`} onClick={() => decide(user, "deny")}>
              {t.decline}
            </Button>
            <Button size="sm" variant="primary" aria-label={t.approvePerson(user.name)} loading={pending === `approve:${user.id}`} onClick={() => decide(user, "approve")}>
              {t.approve}
            </Button>
          </div>
        </li>
      ))}
    </ul>
  );
}

export function OwnedMembers({ groupId, groupName, members }: { groupId: string; groupName: string; members: NamedRef[] }) {
  const { pending, run, toast } = useMutation();
  const t = accountMessages(useLocale()).groups;

  function remove(user: NamedRef) {
    void run(
      `remove:${user.id}`,
      () => api(`/groups/${groupId}/members/${user.id}`, { method: "DELETE" }),
      () => toast({ title: t.removed(user.name), description: t.removedHint(groupName) }),
    );
  }

  return (
    <ul className={list}>
      {members.map((user) => (
        <li key={user.id} className={row}>
          <div className="flex min-w-0 flex-col gap-1">
            <p className="text-body-sm font-semibold text-fg">{user.name}</p>
            <p className="truncate text-body-sm text-fg-3">{user.email}</p>
          </div>
          <Button size="sm" variant="quiet" className="shrink-0 self-start" aria-label={t.removePerson(user.name)} loading={pending === `remove:${user.id}`} onClick={() => remove(user)}>
            {t.remove}
          </Button>
        </li>
      ))}
    </ul>
  );
}
