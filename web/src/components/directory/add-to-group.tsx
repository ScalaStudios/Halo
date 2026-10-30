"use client";

import { Command } from "cmdk";
import Link from "next/link";
import { Plus, Search, UsersRound } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Checkbox, Input } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import { pluralize } from "@/lib/format";
import type { Group } from "@/lib/types";

type Target = { name: string; userId?: string; appId?: string; assigned?: string[] };
type GroupRef = { id: string; name: string };

export function AddToGroup({ groups, assigned = [], ...target }: Target & { groups: Group[] }) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [picked, setPicked] = useState<string[]>([]);
  const { pending, run, toast } = useMutation();
  const words = query.toLowerCase().split(" ");
  const matches = groups.filter((g) => words.every((word) => `${g.name} ${g.description}`.toLowerCase().includes(word))).sort((a, b) => a.name.localeCompare(b.name));

  function close() {
    setOpen(false);
    setQuery("");
    setPicked([]);
  }

  function toggle(id: string) {
    setPicked((current) => (current.includes(id) ? current.filter((p) => p !== id) : [...current, id]));
  }

  function add() {
    const one = picked.length === 1 ? groups.find((g) => g.id === picked[0]) : undefined;
    const label = one ? one.name : pluralize(picked.length, "group");
    void run(
      "add",
      () =>
        Promise.all(
          target.appId
            ? [api(`/applications/${target.appId}/groups`, { method: "PUT", body: { groupIds: [...assigned, ...picked] } })]
            : picked.map((id) => api(`/groups/${id}/members`, { body: { userId: target.userId } })),
        ),
      () => {
        close();
        toast(
          target.appId
            ? { title: `${label} assigned`, description: `Members of ${label} can now sign in to ${target.name}.` }
            : { title: `Added to ${label}`, description: `${target.name} gets the ${one ? "group's" : "groups'"} applications at next sign-in.` },
        );
      },
    );
  }

  return (
    <>
      <Button size="sm" variant="secondary" onClick={() => setOpen(true)}>
        <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
        {target.appId ? "Assign group" : "Add to group"}
      </Button>
      <Dialog
        open={open}
        onOpenChange={(next) => (next ? setOpen(true) : close())}
        title={target.appId ? `Assign groups to ${target.name}` : `Add ${target.name} to groups`}
        description={
          target.appId
            ? `Members of the groups you pick can sign in to ${target.name}. Recorded in the audit log.`
            : "Rule-based groups follow people's profiles, so they can't be picked."
        }
        footer={
          <>
            <Button variant="secondary" onClick={close}>
              Cancel
            </Button>
            <Button variant="primary" disabled={picked.length === 0} loading={pending === "add"} onClick={add}>
              {target.appId ? "Assign" : "Add to"} {picked.length ? pluralize(picked.length, "group") : "groups"}
            </Button>
          </>
        }
      >
        {groups.length ? (
          <Command
            label="Search groups"
            loop
            shouldFilter={false}
            onKeyDown={(event) => {
              if (event.key !== " " || (query && event.target instanceof HTMLInputElement)) return;
              event.preventDefault();
              event.currentTarget.querySelector<HTMLElement>("[cmdk-item][data-selected=true]")?.click();
            }}
            className="flex flex-col gap-3 pt-1"
          >
            <Command.Input asChild value={query} onValueChange={setQuery}>
              <Input autoFocus placeholder="Search by name or description" leading={<Search size={16} strokeWidth={1.75} />} />
            </Command.Input>
            <Command.List label="Groups" className="h-80 max-h-[35dvh] overflow-y-auto rounded-md border border-border p-1">
              <Command.Empty className="px-2 py-8 text-center text-body-sm text-fg-3">No groups match “{query.trim()}”.</Command.Empty>
              {matches.map((group) => {
                const member = assigned.includes(group.id);
                const checked = member || picked.includes(group.id);
                return (
                  <Command.Item
                    key={group.id}
                    value={group.id}
                    disabled={member || (!target.appId && group.kind === "dynamic")}
                    onSelect={toggle}
                    aria-checked={checked}
                    className="group flex cursor-pointer items-center gap-3 rounded-sm px-3 py-2 data-[disabled=true]:cursor-default data-[selected=true]:bg-press"
                  >
                    <Checkbox tabIndex={-1} aria-hidden="true" readOnly checked={checked} className="pointer-events-none group-data-[disabled=true]:opacity-45" />
                    <span className="flex min-w-0 flex-1 flex-col group-data-[disabled=true]:opacity-45">
                      <span className="truncate text-body-sm text-fg">{group.name}</span>
                      <span className="truncate text-caption text-fg-3">
                        {[group.kind === "dynamic" && "Rule-based", pluralize(group.memberCount, "member"), group.description].filter(Boolean).join(" · ")}
                      </span>
                    </span>
                    {member ? <span className="shrink-0 text-caption text-fg-3">{target.appId ? "Already assigned" : "Already a member"}</span> : null}
                  </Command.Item>
                );
              })}
            </Command.List>
          </Command>
        ) : (
          <EmptyState
            icon={UsersRound}
            title="No groups yet"
            description={
              <>
                Create one on the{" "}
                <Link href="/admin/groups" className="text-link underline underline-offset-3 hover:text-ember">
                  Groups page
                </Link>{" "}
                first.
              </>
            }
          />
        )}
      </Dialog>
    </>
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
