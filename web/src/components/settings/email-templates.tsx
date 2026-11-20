"use client";

import { CircleAlert, RotateCcw } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ActionError } from "@/components/ui/action-error";
import { Badge, Tag } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Field, Input, Textarea } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import { ApiError, api } from "@/lib/api/client";
import { cn } from "@/lib/cn";
import type { EmailTemplate } from "@/lib/settings-types";
import { ReadOnlyNote } from "./shared";

type Preview = { subject: string; text: string };

export function EmailTemplateEditor({ template, canEdit }: { template: EmailTemplate; canEdit: boolean }) {
  const { pending, run, errorFor, toast } = useMutation();
  const variablesId = useId();
  const saved = template.custom ?? template.default;
  const [subject, setSubject] = useState(saved.subject);
  const [body, setBody] = useState(saved.body);
  const [preview, setPreview] = useState<Preview>();
  const [problem, setProblem] = useState<string>();
  const [previewing, setPreviewing] = useState(true);
  const latest = useRef(0);
  const dirty = subject.trim() !== saved.subject || body.trim() !== saved.body;

  useEffect(() => {
    const request = ++latest.current;
    setPreviewing(true);
    const timer = setTimeout(async () => {
      let result: Preview | undefined;
      let failure: string | undefined;
      try {
        result = await api<Preview>(`/email-templates/${template.kind}/preview`, { body: { subject, body } });
      } catch (error) {
        failure = error instanceof ApiError ? error.message : "Halo could not reach the server to update the preview. Try again.";
      }
      if (request !== latest.current) return;
      if (result) setPreview(result);
      setProblem(failure);
      setPreviewing(false);
    }, 300);
    return () => clearTimeout(timer);
  }, [template.kind, subject, body]);

  function save() {
    void run(
      "save",
      () => api<EmailTemplate>(`/email-templates/${template.kind}`, { method: "PUT", body: { subject, body } }),
      () => toast({ title: "Email template saved", description: `New ${template.label.toLowerCase()} emails use it within 30 seconds.` }),
      "inline",
    );
  }

  function reset() {
    void run(
      "reset",
      () => api<EmailTemplate>(`/email-templates/${template.kind}`, { method: "DELETE" }),
      () => {
        setSubject(template.default.subject);
        setBody(template.default.body);
        toast({ title: "Email template reset", description: `New ${template.label.toLowerCase()} emails use Halo's default within 30 seconds.` });
      },
      "inline",
    );
  }

  return (
    <Card>
      <CardHeader
        title={template.label}
        description={template.description}
        actions={template.custom ? <Badge tone="ember">Custom</Badge> : <Badge>Halo&apos;s default</Badge>}
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
          <div className="grid grid-cols-1 gap-8 lg:grid-cols-2">
            <div className="flex min-w-0 flex-col gap-6">
              <Field label="Subject">
                {(props) => (
                  <Input {...props} mono maxLength={200} spellCheck={false} autoComplete="off" disabled={!canEdit} value={subject} onChange={(event) => setSubject(event.target.value)} />
                )}
              </Field>
              <Field
                label="Body"
                hint={`${body.length.toLocaleString("en")} of 10,000 characters. Plain text. Write {{.Variable}} to insert a value and {{if .Variable}}…{{else}}…{{end}} for text that depends on one.`}
              >
                {(props) => (
                  <Textarea {...props} mono rows={14} maxLength={10000} spellCheck={false} disabled={!canEdit} value={body} onChange={(event) => setBody(event.target.value)} />
                )}
              </Field>
              <section aria-labelledby={variablesId} className="flex flex-col gap-3">
                <h3 id={variablesId} className="text-label text-fg">
                  Variables
                </h3>
                <dl className="flex flex-col gap-3">
                  {template.variables.map((variable) => (
                    <div key={variable.name} className="flex flex-col gap-1">
                      <dt className="flex flex-wrap items-center gap-2">
                        <Tag>{`{{.${variable.name}}}`}</Tag>
                        {variable.name === template.required ? <Badge tone="info">Required</Badge> : null}
                      </dt>
                      <dd className="text-body-sm text-fg-3">
                        {variable.description} Example: {variable.example}
                      </dd>
                    </div>
                  ))}
                </dl>
              </section>
            </div>
            <figure className="flex min-w-0 flex-col gap-2">
              <div aria-busy={previewing} className="flex flex-col gap-4 rounded-lg border border-border bg-sunken p-6">
                <div className="flex items-center justify-between gap-4">
                  <span className="text-label text-fg-3">Preview</span>
                  {previewing ? <Spinner /> : null}
                </div>
                <div>
                  <div aria-live="polite">
                    {problem ? (
                      <p className="mb-4 flex items-start gap-2 text-body-sm text-danger">
                        <CircleAlert aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0" />
                        {problem}
                      </p>
                    ) : null}
                  </div>
                  {preview ? (
                    <div className={cn("flex flex-col gap-3", problem && "opacity-45")}>
                      <p className="text-h4 text-fg [overflow-wrap:anywhere]">{preview.subject}</p>
                      <p className="text-body-sm whitespace-pre-wrap text-fg-2 [overflow-wrap:anywhere]">{preview.text}</p>
                    </div>
                  ) : (
                    <div className="flex flex-col gap-3">
                      <Skeleton className="h-6 w-2/3 rounded-md" />
                      <Skeleton className="h-32 w-full rounded-md" />
                    </div>
                  )}
                </div>
              </div>
              <figcaption className="text-caption text-fg-3">Shown with the example values and your organization name. Changes apply when you save.</figcaption>
            </figure>
          </div>
          {canEdit ? (
            <div className="flex flex-wrap items-center gap-3">
              <Button type="submit" variant="primary" loading={pending === "save"} disabled={!dirty || pending === "reset"}>
                Save template
              </Button>
              <Button variant="quiet" loading={pending === "reset"} disabled={!template.custom || pending === "save"} onClick={reset}>
                <RotateCcw aria-hidden="true" size={16} strokeWidth={1.75} />
                Reset to Halo&apos;s default
              </Button>
              <ActionError message={errorFor("save") ?? errorFor("reset")} className="basis-full" />
            </div>
          ) : (
            <ReadOnlyNote>Only a global administrator can change email templates.</ReadOnlyNote>
          )}
        </form>
      </CardBody>
    </Card>
  );
}
