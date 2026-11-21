"use client";

import { Send, Trash2 } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ActionError } from "@/components/ui/action-error";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { DescriptionList } from "@/components/ui/description-list";
import { Dialog } from "@/components/ui/dialog";
import { Checkbox, Field, Input, Select, Textarea } from "@/components/ui/input";
import { RelativeTime } from "@/components/ui/relative-time";
import { api } from "@/lib/api/client";
import { pluralize } from "@/lib/format";
import { PROTOCOLS } from "@/lib/labels";
import type { LogExportProtocol, LogExportResponse } from "@/lib/settings-types";
import { ReadOnlyNote } from "./shared";

export function LogExportStatus({ data, canEdit }: { data: LogExportResponse; canEdit: boolean }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [deleting, setDeleting] = useState(false);
  const exporter = data.exporter;
  if (!exporter) return null;

  return (
    <Card>
      <CardHeader
        title="Delivery"
        description="Halo sends new events every 10 seconds. While the collector is down they wait in the database, and Halo retries with a growing delay of up to 5 minutes."
        actions={
          canEdit ? (
            <>
              <Button
                variant="secondary"
                loading={pending === "test"}
                onClick={() =>
                  void run("test", () => api("/log-export/test", { method: "POST" }), () =>
                    toast({ title: "Test event sent", description: `A log_export.test event went to ${exporter.endpoint}.` }),
                  )
                }
              >
                <Send aria-hidden="true" size={16} strokeWidth={1.75} />
                Send test event
              </Button>
              {data.environment ? null : (
                <Button size="icon" variant="secondary" aria-label="Stop streaming" onClick={() => setDeleting(true)}>
                  <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
                </Button>
              )}
            </>
          ) : null
        }
      />
      <CardBody className="flex flex-col gap-4">
        <DescriptionList
          columns={2}
          items={[
            { label: "Last successful send", value: exporter.lastSentAt ? <RelativeTime iso={exporter.lastSentAt} /> : "Never" },
            { label: "Waiting to be sent", value: pluralize(data.queued, "event") },
          ]}
        />
        {exporter.lastError ? (
          <p role="status" className="text-body-sm text-danger">
            {exporter.failures > 0 ? `${pluralize(exporter.failures, "failed send")} in a row. ` : null}
            {exporter.lastError}
          </p>
        ) : null}
      </CardBody>
      <Dialog
        open={deleting}
        onOpenChange={(next) => {
          clearError();
          setDeleting(next);
        }}
        error={errorFor("delete")}
        title="Stop streaming?"
        description="Halo stops sending sign-in and audit events and forgets the saved headers. Events still waiting are not sent."
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              loading={pending === "delete"}
              onClick={() =>
                void run("delete", () => api("/log-export", { method: "DELETE" }), () => {
                  setDeleting(false);
                  toast({ title: "Streaming stopped", description: "Recorded in the audit log." });
                }, "inline")
              }
            >
              Stop streaming
            </Button>
          </>
        }
      />
    </Card>
  );
}

export function LogExportForm({ data, canEdit }: { data: LogExportResponse; canEdit: boolean }) {
  const { pending, run, errorFor, toast } = useMutation();
  const saved = data.exporter;
  const [protocol, setProtocol] = useState<LogExportProtocol>(saved?.protocol ?? "syslog-tls");
  const [endpoint, setEndpoint] = useState(saved?.endpoint ?? "");
  const [allowPrivate, setAllowPrivate] = useState(saved?.allowPrivate ?? false);
  const [headers, setHeaders] = useState("");
  const [attempted, setAttempted] = useState(false);
  const editable = canEdit && !data.environment;
  const endpointError = endpoint.trim() ? undefined : "Enter where Halo sends the events.";
  const dirty = !saved || protocol !== saved.protocol || endpoint.trim() !== saved.endpoint || allowPrivate !== saved.allowPrivate || headers.trim() !== "";

  function submit() {
    setAttempted(true);
    if (endpointError) return;
    void run(
      "save",
      () => api<LogExportResponse>("/log-export", { method: "PUT", body: { protocol, endpoint: endpoint.trim(), allowPrivate, ...(headers.trim() ? { headers: headers.trim() } : {}) } }),
      (result) => {
        setHeaders("");
        setEndpoint(result.exporter?.endpoint ?? endpoint.trim());
        toast({ title: saved ? "Log streaming saved" : "Log streaming started", description: "New sign-in and audit events are sent within 10 seconds." });
      },
      "inline",
    );
  }

  return (
    <Card>
      <CardHeader title="Destination" description="Each event is the same JSON as a webhook's, with secret values already redacted from audit changes." />
      <CardBody>
        <form
          noValidate
          className="flex max-w-xl flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <Field label="Protocol" hint={PROTOCOLS[protocol].hint}>
            {(props) => (
              <Select {...props} disabled={!editable} value={protocol} onChange={(event) => setProtocol(event.target.value as LogExportProtocol)}>
                {Object.entries(PROTOCOLS).map(([value, p]) => (
                  <option key={value} value={value}>
                    {p.label}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label={protocol === "otlp-http" ? "Collector URL" : "Server"} hint={protocol === "otlp-http" ? undefined : "Host name and port."} error={attempted ? endpointError : undefined}>
            {(props) => (
              <Input
                {...props}
                mono
                autoComplete="off"
                spellCheck={false}
                disabled={!editable}
                placeholder={PROTOCOLS[protocol].placeholder}
                value={endpoint}
                onChange={(event) => setEndpoint(event.target.value)}
              />
            )}
          </Field>
          {protocol === "otlp-http" ? (
            <Field
              label="Headers"
              hint={
                saved?.headersSet
                  ? "Leave empty to keep the saved headers. Halo stores them encrypted and never shows them again."
                  : "Optional name=value pairs, one per line, such as Authorization=Bearer <token>. Stored encrypted."
              }
            >
              {(props) => <Textarea {...props} mono rows={3} spellCheck={false} disabled={!editable} value={headers} onChange={(event) => setHeaders(event.target.value)} />}
            </Field>
          ) : null}
          <label className="flex cursor-pointer items-start gap-3">
            <Checkbox className="mt-0.5" disabled={!editable} checked={allowPrivate} onChange={(event) => setAllowPrivate(event.target.checked)} />
            <span className="flex flex-col">
              <span className="text-body-sm text-fg">Allow private addresses</span>
              <span className="text-caption text-fg-3">Send to a collector on your own network, and allow http:// for OpenTelemetry. Otherwise Halo only connects to public addresses.</span>
            </span>
          </label>
          {data.environment ? (
            <ReadOnlyNote>Set by HALO_LOG_EXPORT_* environment variables. Change them and restart Halo.</ReadOnlyNote>
          ) : canEdit ? (
            <div className="flex flex-wrap items-center gap-4">
              <Button type="submit" variant="primary" loading={pending === "save"} disabled={!dirty}>
                {saved ? "Save destination" : "Start streaming"}
              </Button>
              <ActionError message={errorFor("save")} />
            </div>
          ) : (
            <ReadOnlyNote>Only security and global administrators can change log streaming.</ReadOnlyNote>
          )}
        </form>
      </CardBody>
    </Card>
  );
}
