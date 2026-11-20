"use client";

import { useMutation } from "@/components/console/use-mutation";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api/client";
import { formatRelative } from "@/lib/format";
import type { NamedRef } from "@/lib/governance-types";
import type { JoinableGroup, JoinRequest } from "@/lib/types";
import { list, row } from "./list";

export function JoinGroup({ group }: { group: JoinableGroup }) {
  const { pending, run, toast } = useMutation();

  function join() {
    void run(
      "join",
      () => api<{ status: "joined" | "requested" }>(`/me/groups/${group.id}/join`, { method: "POST" }),
      ({ status }) =>
        toast(
          status === "joined"
            ? { title: `You joined ${group.name}`, description: "You get its applications at your next sign-in." }
            : { title: "Request sent", description: `An owner of ${group.name} decides. The group moves to Your groups once they approve.` },
        ),
    );
  }

  return (
    <Button size="sm" variant="secondary" className="shrink-0 self-start" aria-label={`${group.joinPolicy === "open" ? "Join" : "Ask to join"} ${group.name}`} loading={pending === "join"} onClick={join}>
      {group.joinPolicy === "open" ? "Join" : "Ask to join"}
    </Button>
  );
}

export function JoinRequests({ groupId, groupName, requests }: { groupId: string; groupName: string; requests: JoinRequest[] }) {
  const { pending, run, toast } = useMutation();

  function decide(user: NamedRef, decision: "approve" | "deny") {
    void run(
      `${decision}:${user.id}`,
      () => api(`/groups/${groupId}/join-requests/${user.id}/${decision}`, { method: "POST" }),
      () =>
        toast(
          decision === "approve"
            ? { title: `Added ${user.name}`, description: `They get ${groupName}'s applications at their next sign-in.` }
            : { title: `Declined ${user.name}'s request`, description: "Recorded in the audit log." },
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
              {user.email} · Asked {formatRelative(createdAt).toLowerCase()}
            </p>
          </div>
          <div className="flex shrink-0 gap-2">
            <Button size="sm" variant="secondary" aria-label={`Decline ${user.name}`} loading={pending === `deny:${user.id}`} onClick={() => decide(user, "deny")}>
              Decline
            </Button>
            <Button size="sm" variant="primary" aria-label={`Approve ${user.name}`} loading={pending === `approve:${user.id}`} onClick={() => decide(user, "approve")}>
              Approve
            </Button>
          </div>
        </li>
      ))}
    </ul>
  );
}

export function OwnedMembers({ groupId, groupName, members }: { groupId: string; groupName: string; members: NamedRef[] }) {
  const { pending, run, toast } = useMutation();

  function remove(user: NamedRef) {
    void run(
      `remove:${user.id}`,
      () => api(`/groups/${groupId}/members/${user.id}`, { method: "DELETE" }),
      () => toast({ title: `Removed ${user.name}`, description: `They lose ${groupName}'s applications at their next sign-in.` }),
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
          <Button size="sm" variant="quiet" className="shrink-0 self-start" aria-label={`Remove ${user.name}`} loading={pending === `remove:${user.id}`} onClick={() => remove(user)}>
            Remove
          </Button>
        </li>
      ))}
    </ul>
  );
}
