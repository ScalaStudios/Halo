"use client";

import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { PeoplePicker } from "@/components/governance/people-picker";
import type { Person } from "@/components/governance/shared";
import { ActionError } from "@/components/ui/action-error";
import { Badge, Tag } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input, Textarea } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import { formatDateTime } from "@/lib/format";
import type { AuthorizationAnswer, AuthorizationModel } from "@/lib/types";

const EXAMPLE = {
  folder: { viewer: { subjects: ["user", "group"] } },
  document: {
    parent: { subjects: ["folder"] },
    owner: { subjects: ["user"] },
    editor: { subjects: ["user", "group"], union: ["owner"] },
    viewer: { subjects: ["user", "group"], union: ["editor"], from: [{ via: "parent", relation: "viewer" }] },
  },
};

export function AuthorizationModelForm({ appId, appName, model }: { appId: string; appName: string; model: AuthorizationModel }) {
  const saved = model.updatedAt ? JSON.stringify(model.types, null, 2) : JSON.stringify(EXAMPLE, null, 2);
  const [text, setText] = useState(saved);
  const [parseError, setParseError] = useState<string>();
  const [removing, setRemoving] = useState(false);
  const { pending, run, errorFor, clearError, toast } = useMutation();

  function save() {
    let types: unknown;
    try {
      types = JSON.parse(text);
    } catch {
      setParseError("The model isn't valid JSON. Check for a missing comma or quote.");
      return;
    }
    setParseError(undefined);
    void run(
      "save",
      () => api<AuthorizationModel>(`/applications/${appId}/authorization`, { method: "PUT", body: { types } }),
      (next) => {
        setText(JSON.stringify(next.types, null, 2));
        toast({ title: "Model saved", description: `${appName}'s next checks use it. Recorded in the audit log.` });
      },
      "inline",
    );
  }

  return (
    <Card>
      <CardHeader
        title="Model"
        description="Types by name, each with its relations. A relation is held by the subjects tuples name (user, group, a parent type or type#relation), by the holders of the relations in union, by the application roles in roles, and through a relation on parents in from."
        actions={model.updatedAt ? <Badge tone="success">Saved {formatDateTime(model.updatedAt)}</Badge> : <Badge>Not saved yet</Badge>}
      />
      <CardBody>
        <form
          noValidate
          className="flex flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            save();
          }}
        >
          <Field label="Types" hint={model.updatedAt ? undefined : "An example to start from. Nothing is saved until you save it."} error={parseError}>
            {(props) => <Textarea {...props} mono rows={18} spellCheck={false} value={text} onChange={(event) => setText(event.target.value)} />}
          </Field>
          <ActionError message={errorFor("save")} />
          <div className="flex flex-wrap items-center gap-3">
            <Button type="submit" variant="primary" loading={pending === "save"} disabled={model.updatedAt !== null && text === saved}>
              Save model
            </Button>
            {model.updatedAt ? (
              <Button type="button" variant="ghost" onClick={() => setRemoving(true)}>
                Remove model
              </Button>
            ) : null}
          </div>
        </form>
      </CardBody>
      <Dialog
        open={removing}
        onOpenChange={(open) => {
          if (open) return;
          setRemoving(false);
          clearError();
        }}
        error={errorFor("remove")}
        title="Remove the model and every tuple?"
        description={`${appName} can't check or write tuples until a new model is saved, and the tuples it wrote are deleted. It can't be undone.`}
        footer={
          <>
            <Button variant="secondary" onClick={() => setRemoving(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              loading={pending === "remove"}
              onClick={() =>
                run(
                  "remove",
                  () => api(`/applications/${appId}/authorization`, { method: "DELETE" }),
                  () => {
                    setRemoving(false);
                    setText(JSON.stringify(EXAMPLE, null, 2));
                    toast({ title: "Model removed", description: "Its tuples were deleted. Recorded in the audit log." });
                  },
                  "inline",
                )
              }
            >
              Remove model
            </Button>
          </>
        }
      />
    </Card>
  );
}

export function AuthorizationCheckForm({ appId, people }: { appId: string; people: Person[] }) {
  const [object, setObject] = useState("");
  const [relation, setRelation] = useState("");
  const [person, setPerson] = useState<string[]>([]);
  const [answer, setAnswer] = useState<AuthorizationAnswer>();
  const { pending, run, errorFor } = useMutation();

  return (
    <Card>
      <CardHeader title="Check access" description="Ask what the application would get from POST /api/v1/authz/check, and see the relations that granted it." />
      <CardBody>
        <form
          noValidate
          className="flex flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            setAnswer(undefined);
            void run(
              "check",
              () => api<AuthorizationAnswer>(`/applications/${appId}/authorization/check`, { body: { object: object.trim(), relation: relation.trim(), subject: `user:${person[0] ?? ""}` } }),
              setAnswer,
              "inline",
            );
          }}
        >
          <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
            <Field label="Object">
              {(props) => <Input {...props} mono placeholder="document:readme" autoComplete="off" spellCheck={false} value={object} onChange={(event) => setObject(event.target.value)} />}
            </Field>
            <Field label="Relation">
              {(props) => <Input {...props} mono placeholder="viewer" autoComplete="off" spellCheck={false} value={relation} onChange={(event) => setRelation(event.target.value)} />}
            </Field>
          </div>
          <Field label="Person">{(props) => <PeoplePicker {...props} people={people} value={person} onChange={(ids) => setPerson(ids.slice(-1))} />}</Field>
          <ActionError message={errorFor("check")} />
          <div className="flex flex-wrap items-center gap-3">
            <Button type="submit" variant="primary" loading={pending === "check"} disabled={!object.trim() || !relation.trim() || person.length === 0}>
              Run check
            </Button>
            <div aria-live="polite" className="flex flex-wrap items-center gap-2">
              {answer ? <Badge tone={answer.allowed ? "success" : "danger"}>{answer.allowed ? "Allowed" : "Denied"}</Badge> : null}
              {answer?.path.map((step) => (
                <Tag key={step}>{step}</Tag>
              ))}
            </div>
          </div>
        </form>
      </CardBody>
    </Card>
  );
}
