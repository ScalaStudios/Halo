"use client";

import { Check, Plus, X } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ROLES } from "@/components/provisioning/shared";
import { Avatar } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { Field, Select } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import { pluralize } from "@/lib/format";
import type { RoleKey } from "@/lib/provisioning-types";
import type { User } from "@/lib/types";
import { CAPABILITIES, ROLE_ORDER } from "./roles";

type Person = Pick<User, "id" | "name" | "email" | "roles" | "status">;

const KEY_BY_LABEL = Object.fromEntries(ROLE_ORDER.map((key) => [ROLES[key].label, key])) as Record<string, RoleKey>;

function keysOf(person: Person) {
  return person.roles.map((label) => KEY_BY_LABEL[label]).filter((key): key is RoleKey => Boolean(key));
}

export function RolesBoard({ people, meId, canEdit }: { people: Person[]; meId: string; canEdit: boolean }) {
  const { pending, run, toast } = useMutation();
  const [adding, setAdding] = useState<RoleKey | null>(null);

  function setRoles(person: Person, roles: RoleKey[], title: string) {
    void run(`${person.id}`, () => api(`/users/${person.id}/roles`, { method: "PUT", body: { roles } }), () => {
      setAdding(null);
      toast({ title, description: "Takes effect on their next request. Recorded in the audit log." });
    });
  }

  return (
    <>
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-2">
        {ROLE_ORDER.map((role) => {
          const members = people.filter((p) => keysOf(p).includes(role));
          return (
            <Card key={role} className="flex flex-col">
              <CardHeader
                title={ROLES[role].label}
                description={ROLES[role].description}
                actions={
                  <Badge tone={members.length ? "neutral" : "warning"}>
                    <span className="tnum">{pluralize(members.length, "member")}</span>
                  </Badge>
                }
              />
              <CardBody className="flex flex-1 flex-col gap-6">
                <ul className="flex flex-col gap-2">
                  {CAPABILITIES[role].map((item) => (
                    <li key={item} className="flex items-start gap-2 text-body-sm text-fg-2">
                      <Check aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0 text-fg-3" />
                      {item}
                    </li>
                  ))}
                </ul>
                <div className="mt-auto flex flex-col gap-3">
                  <h3 className="text-label text-fg-3">Members</h3>
                  {members.length ? (
                    <ul className="flex flex-col rounded-md border border-border">
                      {members.map((person) => (
                        <li key={person.id} className="flex items-center gap-3 border-b border-border px-4 py-2 last:border-b-0">
                          <Avatar name={person.name} />
                          <span className="flex min-w-0 flex-1 flex-col">
                            <span className="truncate text-body-sm text-fg">{person.name}</span>
                            <span className="truncate text-caption text-fg-3">{person.email}</span>
                          </span>
                          {canEdit && !(role === "global_admin" && person.id === meId) ? (
                            <Button
                              size="icon-sm"
                              variant="quiet"
                              aria-label={`Remove ${person.name} from ${ROLES[role].label}`}
                              loading={pending === person.id}
                              onClick={() => setRoles(person, keysOf(person).filter((key) => key !== role), `${person.name} is no longer a ${ROLES[role].label.toLowerCase()}`)}
                            >
                              <X aria-hidden="true" size={16} strokeWidth={1.75} />
                            </Button>
                          ) : null}
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <p className="text-body-sm text-fg-3">Nobody holds this role.</p>
                  )}
                  {canEdit ? (
                    <div>
                      <Button size="sm" variant="secondary" onClick={() => setAdding(role)}>
                        <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
                        Add member
                      </Button>
                    </div>
                  ) : null}
                </div>
              </CardBody>
            </Card>
          );
        })}
      </div>
      {adding ? (
        <AddMemberDialog
          role={adding}
          candidates={people.filter((p) => p.status === "active" && !keysOf(p).includes(adding))}
          saving={pending !== null}
          onAdd={(person) => setRoles(person, [...keysOf(person), adding], `${person.name} is now a ${ROLES[adding].label.toLowerCase()}`)}
          onClose={() => setAdding(null)}
        />
      ) : null}
    </>
  );
}

function AddMemberDialog({ role, candidates, saving, onAdd, onClose }: { role: RoleKey; candidates: Person[]; saving: boolean; onAdd: (person: Person) => void; onClose: () => void }) {
  const [personId, setPersonId] = useState("");
  const [error, setError] = useState<string>();
  const person = candidates.find((p) => p.id === personId);

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={`Add a ${ROLES[role].label.toLowerCase()}`}
      description={`${ROLES[role].description} Their other roles stay as they are.`}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="primary"
            loading={saving}
            onClick={() => {
              if (person) onAdd(person);
              else setError("Choose the person to give this role.");
            }}
          >
            Add member
          </Button>
        </>
      }
    >
      <Field label="Person" error={error} hint={candidates.length ? "Only active people who don't already hold this role are listed." : undefined}>
        {(props) => (
          <Select {...props} autoFocus value={personId} disabled={!candidates.length} onChange={(event) => setPersonId(event.target.value)}>
            <option value="">{candidates.length ? "Choose a person" : "Everyone active already holds this role"}</option>
            {candidates.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name} · {p.email}
              </option>
            ))}
          </Select>
        )}
      </Field>
    </Dialog>
  );
}
