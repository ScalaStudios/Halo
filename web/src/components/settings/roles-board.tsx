"use client";

import { Check, Plus, X } from "lucide-react";
import { type ReactNode, useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ROLES } from "@/components/provisioning/shared";
import { Avatar } from "@/components/ui/avatar";
import { Badge, StatusDot } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { Field, Select } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import { formatTime, pluralize } from "@/lib/format";
import type { RoleKey } from "@/lib/provisioning-types";
import type { User } from "@/lib/types";
import { CAPABILITIES, keysOf, ROLE_ORDER } from "./roles";

type Person = Pick<User, "id" | "name" | "email" | "roles" | "status" | "eligibleRoles" | "activations">;

type Adding = { role: RoleKey; eligible: boolean };

export function RolesBoard({ people, meId, canEdit }: { people: Person[]; meId: string; canEdit: boolean }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [adding, setAdding] = useState<Adding | null>(null);

  function setRoles(person: Person, roles: RoleKey[], title: string, eligible = false) {
    void run(
      adding ? "add" : `${eligible ? "eligible" : "member"}-${person.id}`,
      () => api(`/users/${person.id}/${eligible ? "eligible-roles" : "roles"}`, { method: "PUT", body: { roles } }),
      () => {
        setAdding(null);
        toast({ title, description: "Takes effect on their next request. Recorded in the audit log." });
      },
      adding ? "inline" : undefined,
    );
  }

  return (
    <>
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-2">
        {ROLE_ORDER.map((role) => {
          const members = people.filter((p) => keysOf(p).includes(role));
          const eligible = people.filter((p) => p.eligibleRoles.includes(role));
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
                        <PersonRow key={person.id} person={person}>
                          {canEdit && !(role === "global_admin" && person.id === meId) ? (
                            <Button
                              size="icon-sm"
                              variant="quiet"
                              aria-label={`Remove ${person.name} from ${ROLES[role].label}`}
                              loading={pending === `member-${person.id}`}
                              onClick={() => setRoles(person, keysOf(person).filter((key) => key !== role), `${person.name} is no longer a ${ROLES[role].label.toLowerCase()}`)}
                            >
                              <X aria-hidden="true" size={16} strokeWidth={1.75} />
                            </Button>
                          ) : null}
                        </PersonRow>
                      ))}
                    </ul>
                  ) : (
                    <p className="text-body-sm text-fg-3">Nobody holds this role.</p>
                  )}
                  {canEdit ? (
                    <div>
                      <Button size="sm" variant="secondary" onClick={() => setAdding({ role, eligible: false })}>
                        <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
                        Add member
                      </Button>
                    </div>
                  ) : null}
                </div>
                <div className="flex flex-col gap-3">
                  <h3 className="text-label text-fg-3">Eligible</h3>
                  {eligible.length ? (
                    <ul className="flex flex-col rounded-md border border-border">
                      {eligible.map((person) => {
                        const active = person.activations.find((a) => a.role === role);
                        return (
                          <PersonRow
                            key={person.id}
                            person={person}
                            note={
                              active ? (
                                <span className="flex items-center gap-2 text-caption text-fg-2">
                                  <StatusDot tone="success" />
                                  Active until {formatTime(active.expiresAt)}
                                </span>
                              ) : null
                            }
                          >
                            {canEdit && person.id !== meId ? (
                              <Button
                                size="icon-sm"
                                variant="quiet"
                                aria-label={`Remove ${person.name}'s eligibility for ${ROLES[role].label}`}
                                loading={pending === `eligible-${person.id}`}
                                onClick={() =>
                                  setRoles(
                                    person,
                                    person.eligibleRoles.filter((key) => key !== role),
                                    `${person.name} is no longer eligible for ${ROLES[role].label.toLowerCase()}`,
                                    true,
                                  )
                                }
                              >
                                <X aria-hidden="true" size={16} strokeWidth={1.75} />
                              </Button>
                            ) : null}
                          </PersonRow>
                        );
                      })}
                    </ul>
                  ) : (
                    <p className="text-body-sm text-fg-3">Nobody can activate this role.</p>
                  )}
                  {canEdit ? (
                    <div>
                      <Button size="sm" variant="secondary" onClick={() => setAdding({ role, eligible: true })}>
                        <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
                        Add eligible person
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
          role={adding.role}
          eligible={adding.eligible}
          candidates={people.filter(
            (p) => p.status === "active" && p.id !== meId && !(adding.eligible ? p.eligibleRoles : keysOf(p)).includes(adding.role),
          )}
          saving={pending === "add"}
          failure={errorFor("add")}
          onAdd={(person) =>
            adding.eligible
              ? setRoles(person, [...person.eligibleRoles, adding.role], `${person.name} can now activate ${ROLES[adding.role].label.toLowerCase()}`, true)
              : setRoles(person, [...keysOf(person), adding.role], `${person.name} is now a ${ROLES[adding.role].label.toLowerCase()}`)
          }
          onClose={() => {
            clearError();
            setAdding(null);
          }}
        />
      ) : null}
    </>
  );
}

function PersonRow({ person, note, children }: { person: Person; note?: ReactNode; children: ReactNode }) {
  return (
    <li className="flex items-center gap-3 border-b border-border px-4 py-2 last:border-b-0">
      <Avatar name={person.name} />
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="truncate text-body-sm text-fg">{person.name}</span>
        <span className="truncate text-caption text-fg-3">{person.email}</span>
        {note}
      </span>
      {children}
    </li>
  );
}

function AddMemberDialog({
  role,
  eligible,
  candidates,
  saving,
  failure,
  onAdd,
  onClose,
}: {
  role: RoleKey;
  eligible: boolean;
  candidates: Person[];
  saving: boolean;
  failure?: string;
  onAdd: (person: Person) => void;
  onClose: () => void;
}) {
  const [personId, setPersonId] = useState("");
  const [error, setError] = useState<string>();
  const person = candidates.find((p) => p.id === personId);
  const label = ROLES[role].label.toLowerCase();
  const copy = eligible
    ? {
        title: `Make someone eligible for ${label}`,
        description: `They can activate the role for up to 8 hours with a reason, under Access in their account portal. Halo removes it when the time ends.`,
        submit: "Make eligible",
        missing: "Choose the person to make eligible.",
        hint: "Only active people who aren't eligible yet are listed. You can't make yourself eligible.",
        none: "Everyone active is already eligible",
      }
    : {
        title: `Add a ${label}`,
        description: `${ROLES[role].description} Their other roles stay as they are.`,
        submit: "Add member",
        missing: "Choose the person to give this role.",
        hint: "Only active people who don't already hold this role are listed. You can't give yourself a role.",
        none: "Everyone active already holds this role",
      };

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      error={failure}
      title={copy.title}
      description={copy.description}
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
              else setError(copy.missing);
            }}
          >
            {copy.submit}
          </Button>
        </>
      }
    >
      <Field label="Person" error={error} hint={candidates.length ? copy.hint : undefined}>
        {(props) => (
          <Select {...props} autoFocus value={personId} disabled={!candidates.length} onChange={(event) => setPersonId(event.target.value)}>
            <option value="">{candidates.length ? "Choose a person" : copy.none}</option>
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
