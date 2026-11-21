"use client";

import { useState, type FormEvent } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ActionError } from "@/components/ui/action-error";
import { Badge, Tag, type Tone } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { DescriptionList } from "@/components/ui/description-list";
import { Dialog } from "@/components/ui/dialog";
import { Field, Select } from "@/components/ui/input";
import { SectionTitle } from "@/components/ui/section-title";
import { DeliveryLog } from "@/components/settings/webhooks";
import { api } from "@/lib/api/client";
import { formatDateTime, pluralize } from "@/lib/format";
import type { WebhookDelivery } from "@/lib/settings-types";
import type { SignalReceiver, SignalStream, SubjectFormat } from "@/lib/types";

const FORMATS: Record<SubjectFormat, string> = {
  iss_sub: "Issuer and subject (iss_sub)",
  email: "Email address (email)",
  opaque: "Opaque ID (opaque)",
};

const STREAM_STATUS: Record<SignalStream["status"], { label: string; tone: Tone }> = {
  enabled: { label: "Enabled", tone: "success" },
  paused: { label: "Paused", tone: "warning" },
  disabled: { label: "Disabled", tone: "neutral" },
};

const eventName = (uri: string) => uri.slice(uri.lastIndexOf("/") + 1);

export function SharedSignals({ appId, appName, issuer, receiver, deliveries }: { appId: string; appName: string; issuer: string; receiver: SignalReceiver | null; deliveries: WebhookDelivery[] }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [format, setFormat] = useState<SubjectFormat>(receiver?.subjectFormat ?? "iss_sub");
  const [removing, setRemoving] = useState(false);
  const stream = receiver?.stream;

  function submit(event: FormEvent) {
    event.preventDefault();
    void run(
      "save",
      () => api(`/applications/${appId}/shared-signals`, { method: "PUT", body: { subjectFormat: format } }),
      () =>
        toast({
          title: receiver ? "Subject format saved" : "Shared signals turned on",
          description: receiver ? `Events not yet delivered to ${appName} name people by ${format}. Recorded in the audit log.` : `${appName} can now create its stream. Recorded in the audit log.`,
        }),
      "inline",
    );
  }

  return (
    <>
      <Card>
        <CardHeader
          title="Shared signals"
          description={`Halo sends ${appName} signed security events about every person under the OpenID Shared Signals Framework: ended sessions, credential and device compliance changes (CAEP), and suspensions, restores, deletions and email changes (RISC). ${appName} creates and manages its stream with a client credentials token.`}
        />
        <form onSubmit={submit} noValidate>
          <CardBody className="flex flex-col gap-6">
            <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
              <Field label="Subject format" hint="How each event names the person. Email changes are sent only with email subjects.">
                {(props) => (
                  <Select {...props} value={format} onChange={(event) => setFormat(event.target.value as SubjectFormat)}>
                    {Object.entries(FORMATS).map(([value, label]) => (
                      <option key={value} value={value}>
                        {label}
                      </option>
                    ))}
                  </Select>
                )}
              </Field>
              <CopyField label="Transmitter configuration" value={`${issuer}/.well-known/ssf-configuration`} hint={`Give this URL to ${appName}. It lists the stream, status, verification and key endpoints.`} />
            </div>
            {stream ? (
              <DescriptionList
                columns={2}
                items={[
                  {
                    label: "Stream",
                    value: (
                      <Badge tone={STREAM_STATUS[stream.status].tone} dot>
                        {STREAM_STATUS[stream.status].label}
                        {stream.reason ? ` · ${stream.reason}` : ""}
                      </Badge>
                    ),
                  },
                  { label: "Delivery", value: stream.deliveryMethod === "urn:ietf:rfc:8935" ? `Pushed to ${new URL(stream.endpointUrl).host}` : `Polled by ${appName}` },
                  {
                    label: "Events requested",
                    value: stream.eventsRequested.length ? (
                      <span className="flex flex-wrap gap-2">
                        {stream.eventsRequested.map((uri) => (
                          <Tag key={uri}>{eventName(uri)}</Tag>
                        ))}
                      </span>
                    ) : (
                      "None"
                    ),
                  },
                  { label: "Created", value: formatDateTime(stream.createdAt) },
                ]}
              />
            ) : receiver ? (
              <p className="text-body-sm text-fg-3">{appName} has not created its stream yet.</p>
            ) : null}
          </CardBody>
          <div className="flex flex-wrap items-center justify-end gap-4 border-t border-border px-6 py-4">
            <ActionError message={errorFor("save")} />
            {receiver ? (
              <Button variant="secondary" onClick={() => setRemoving(true)}>
                Turn off
              </Button>
            ) : null}
            <Button type="submit" variant="primary" loading={pending === "save"} disabled={receiver?.subjectFormat === format}>
              {receiver ? "Save subject format" : "Turn on shared signals"}
            </Button>
          </div>
        </form>
        <Dialog
          open={removing}
          onOpenChange={(next) => {
            clearError();
            setRemoving(next);
          }}
          error={errorFor("remove")}
          title="Turn off shared signals?"
          description={`Halo deletes ${appName}'s stream with its undelivered events and refuses its stream requests until shared signals are turned on again.`}
          footer={
            <>
              <Button variant="secondary" onClick={() => setRemoving(false)}>
                Cancel
              </Button>
              <Button
                variant="destructive"
                loading={pending === "remove"}
                onClick={() =>
                  void run(
                    "remove",
                    () => api(`/applications/${appId}/shared-signals`, { method: "DELETE" }),
                    () => {
                      setRemoving(false);
                      toast({ title: "Shared signals turned off", description: "Recorded in the audit log." });
                    },
                    "inline",
                  )
                }
              >
                Turn off
              </Button>
            </>
          }
        />
      </Card>
      {deliveries.length > 0 ? (
        <section className="flex flex-col gap-6">
          <SectionTitle title="Security event deliveries" description={`The latest ${pluralize(deliveries.length, "security event token")}, newest first. The event ID is the token's jti.`} />
          <DeliveryLog
            deliveries={deliveries.map((d) => ({ ...d, eventType: eventName(d.eventType) }))}
            payloadNote="The person and event Halo signs a Security Event Token from. Retries and polls carry the same jti."
          />
        </section>
      ) : null}
    </>
  );
}
