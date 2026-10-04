"use client";

import { Plus } from "lucide-react";
import { useMutation } from "@/components/console/use-mutation";
import { Button } from "@/components/ui/button";
import { Menu, MenuContent, MenuItem, MenuLabel, MenuTrigger } from "@/components/ui/menu";
import { api } from "@/lib/api/client";

type Target = { name: string; userId?: string; appId?: string; assigned?: string[] };
type GroupRef = { id: string; name: string };

export function AddToGroup({ groups, ...target }: Target & { groups: GroupRef[] }) {
  const { pending, run, toast } = useMutation();

  function add(group: GroupRef) {
    void run(
      "add",
      () =>
        target.appId
          ? api(`/applications/${target.appId}/groups`, { method: "PUT", body: { groupIds: [...(target.assigned ?? []), group.id] } })
          : api(`/groups/${group.id}/members`, { body: { userId: target.userId } }),
      () =>
        toast(
          target.appId
            ? { title: `${group.name} assigned`, description: `Members of ${group.name} can now sign in to ${target.name}.` }
            : { title: `Added to ${group.name}`, description: `${target.name} gets the group's applications at next sign-in.` },
        ),
    );
  }

  return (
    <Menu>
      <MenuTrigger asChild>
        <Button size="sm" variant="secondary" loading={pending === "add"} disabled={groups.length === 0}>
          <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
          {target.appId ? "Assign group" : "Add to group"}
        </Button>
      </MenuTrigger>
      <MenuContent>
        <MenuLabel>{target.appId ? "Groups" : "Assigned groups"}</MenuLabel>
        {groups.map((group) => (
          <MenuItem key={group.id} onSelect={() => add(group)}>
            {group.name}
          </MenuItem>
        ))}
      </MenuContent>
    </Menu>
  );
}

export function RemoveFromGroup({ group, ...target }: Target & { group: GroupRef }) {
  const { pending, run, toast } = useMutation();

  function remove() {
    void run(
      "remove",
      () =>
        target.appId
          ? api(`/applications/${target.appId}/groups`, { method: "PUT", body: { groupIds: (target.assigned ?? []).filter((id) => id !== group.id) } })
          : api(`/groups/${group.id}/members/${target.userId}`, { method: "DELETE" }),
      () =>
        toast(
          target.appId
            ? { title: `${group.name} unassigned`, description: `Members of ${group.name} can no longer sign in to ${target.name} through it.` }
            : { title: `Removed from ${group.name}`, description: `${target.name} loses the group's applications at next sign-in.` },
        ),
    );
  }

  return (
    <Button size="sm" variant="quiet" loading={pending === "remove"} onClick={remove}>
      Remove
    </Button>
  );
}
