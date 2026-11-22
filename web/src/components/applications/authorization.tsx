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
import { useLocale, useMessages } from "@/lib/i18n/client";
import { applicationsMessages } from "@/lib/i18n/console/applications";
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
  const locale = useLocale();
  const t = applicationsMessages[locale];
  const a = t.authorization;

  function save() {
    let types: unknown;
    try {
      types = JSON.parse(text);
    } catch {
      setParseError(a.invalidJson);
      return;
    }
    setParseError(undefined);
    void run(
      "save",
      () => api<AuthorizationModel>(`/applications/${appId}/authorization`, { method: "PUT", body: { types } }),
      (next) => {
        setText(JSON.stringify(next.types, null, 2));
        toast({ title: a.saved, description: a.savedHint(appName) });
      },
      "inline",
    );
  }

  return (
    <Card>
      <CardHeader
        title={a.model}
        description={a.modelHint}
        actions={model.updatedAt ? <Badge tone="success">{a.savedAt(formatDateTime(model.updatedAt, locale))}</Badge> : <Badge>{a.notSaved}</Badge>}
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
          <Field label={a.types} hint={model.updatedAt ? undefined : a.example} error={parseError}>
            {(props) => <Textarea {...props} mono rows={18} spellCheck={false} value={text} onChange={(event) => setText(event.target.value)} />}
          </Field>
          <ActionError message={errorFor("save")} />
          <div className="flex flex-wrap items-center gap-3">
            <Button type="submit" variant="primary" loading={pending === "save"} disabled={model.updatedAt !== null && text === saved}>
              {a.save}
            </Button>
            {model.updatedAt ? (
              <Button type="button" variant="ghost" onClick={() => setRemoving(true)}>
                {a.remove}
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
        title={a.removeTitle}
        description={a.removeHint(appName)}
        footer={
          <>
            <Button variant="secondary" onClick={() => setRemoving(false)}>
              {t.common.cancel}
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
                    toast({ title: a.removed, description: a.removedHint });
                  },
                  "inline",
                )
              }
            >
              {a.remove}
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
  const a = useMessages(applicationsMessages).authorization;

  return (
    <Card>
      <CardHeader title={a.check} description={a.checkHint} />
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
            <Field label={a.object}>
              {(props) => <Input {...props} mono placeholder="document:readme" autoComplete="off" spellCheck={false} value={object} onChange={(event) => setObject(event.target.value)} />}
            </Field>
            <Field label={a.relation}>
              {(props) => <Input {...props} mono placeholder="viewer" autoComplete="off" spellCheck={false} value={relation} onChange={(event) => setRelation(event.target.value)} />}
            </Field>
          </div>
          <Field label={a.person}>{(props) => <PeoplePicker {...props} people={people} value={person} onChange={(ids) => setPerson(ids.slice(-1))} />}</Field>
          <ActionError message={errorFor("check")} />
          <div className="flex flex-wrap items-center gap-3">
            <Button type="submit" variant="primary" loading={pending === "check"} disabled={!object.trim() || !relation.trim() || person.length === 0}>
              {a.run}
            </Button>
            <div aria-live="polite" className="flex flex-wrap items-center gap-2">
              {answer ? <Badge tone={answer.allowed ? "success" : "danger"}>{answer.allowed ? a.allowed : a.denied}</Badge> : null}
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
