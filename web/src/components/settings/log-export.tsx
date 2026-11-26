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
import { useMessages } from "@/lib/i18n/client";
import { settingsMessages } from "@/lib/i18n/console/settings";
import { PROTOCOLS } from "@/lib/labels";
import type { LogExportProtocol, LogExportResponse } from "@/lib/settings-types";
import { ReadOnlyNote } from "./shared";

export function LogExportStatus({ data, canEdit }: { data: LogExportResponse; canEdit: boolean }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [deleting, setDeleting] = useState(false);
  const { logExport: t, common } = useMessages(settingsMessages);
  const exporter = data.exporter;
  if (!exporter) return null;

  return (
    <Card>
      <CardHeader
        title={t.delivery}
        description={t.deliveryHint}
        actions={
          canEdit ? (
            <>
              <Button
                variant="secondary"
                loading={pending === "test"}
                onClick={() =>
                  void run("test", () => api("/log-export/test", { method: "POST" }), () =>
                    toast({ title: t.testSent, description: t.testSentHint(exporter.endpoint) }),
                  )
                }
              >
                <Send aria-hidden="true" size={16} strokeWidth={1.75} />
                {common.sendTestEvent}
              </Button>
              {data.environment ? null : (
                <Button size="icon" variant="secondary" aria-label={t.stop} onClick={() => setDeleting(true)}>
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
            { label: t.lastSent, value: exporter.lastSentAt ? <RelativeTime iso={exporter.lastSentAt} /> : t.never },
            { label: t.waiting, value: t.eventCount(data.queued) },
          ]}
        />
        {exporter.lastError ? (
          <p role="status" className="text-body-sm text-danger">
            {exporter.failures > 0 ? t.failures(exporter.failures) : null}
            <span lang="en">{exporter.lastError}</span>
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
        title={t.stopTitle}
        description={t.stopDescription}
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(false)}>
              {common.cancel}
            </Button>
            <Button
              variant="destructive"
              loading={pending === "delete"}
              onClick={() =>
                void run("delete", () => api("/log-export", { method: "DELETE" }), () => {
                  setDeleting(false);
                  toast({ title: t.stopped, description: common.recorded });
                }, "inline")
              }
            >
              {t.stop}
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
  const t = useMessages(settingsMessages).logExport;
  const editable = canEdit && !data.environment;
  const endpointError = endpoint.trim() ? undefined : t.endpointError;
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
        toast({ title: saved ? t.saved : t.started, description: t.savedHint });
      },
      "inline",
    );
  }

  return (
    <Card>
      <CardHeader title={t.destination} description={t.destinationHint} />
      <CardBody>
        <form
          noValidate
          className="flex max-w-xl flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <Field label={t.protocol} hint={t.protocols[protocol].hint}>
            {(props) => (
              <Select {...props} disabled={!editable} value={protocol} onChange={(event) => setProtocol(event.target.value as LogExportProtocol)}>
                {Object.keys(PROTOCOLS).map((value) => (
                  <option key={value} value={value}>
                    {t.protocols[value as LogExportProtocol].label}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label={protocol === "otlp-http" ? t.collector : t.server} hint={protocol === "otlp-http" ? undefined : t.serverHint} error={attempted ? endpointError : undefined}>
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
              label={t.headers}
              hint={saved?.headersSet && protocol === saved.protocol && endpoint.trim() === saved.endpoint ? t.headersKeep : t.headersHint}
            >
              {(props) => <Textarea {...props} mono rows={3} spellCheck={false} disabled={!editable} value={headers} onChange={(event) => setHeaders(event.target.value)} />}
            </Field>
          ) : null}
          <label className="flex cursor-pointer items-start gap-3">
            <Checkbox className="mt-0.5" disabled={!editable} checked={allowPrivate} onChange={(event) => setAllowPrivate(event.target.checked)} />
            <span className="flex flex-col">
              <span className="text-body-sm text-fg">{t.allowPrivate}</span>
              <span className="text-caption text-fg-3">{t.allowPrivateHint}</span>
            </span>
          </label>
          {data.environment ? (
            <ReadOnlyNote>{t.environment}</ReadOnlyNote>
          ) : canEdit ? (
            <div className="flex flex-wrap items-center gap-4">
              <Button type="submit" variant="primary" loading={pending === "save"} disabled={!dirty}>
                {saved ? t.save : t.start}
              </Button>
              <ActionError message={errorFor("save")} />
            </div>
          ) : (
            <ReadOnlyNote>{t.readOnly}</ReadOnlyNote>
          )}
        </form>
      </CardBody>
    </Card>
  );
}
