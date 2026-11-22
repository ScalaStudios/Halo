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
import { formatDateTime } from "@/lib/format";
import { useLocale } from "@/lib/i18n/client";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import type { WebhookDelivery } from "@/lib/settings-types";
import type { SignalReceiver, SignalStream, SubjectFormat } from "@/lib/types";

const STREAM_TONE: Record<SignalStream["status"], Tone> = { enabled: "success", paused: "warning", disabled: "neutral" };

const eventName = (uri: string) => uri.slice(uri.lastIndexOf("/") + 1);

export function SharedSignals({ appId, appName, issuer, receiver, deliveries }: { appId: string; appName: string; issuer: string; receiver: SignalReceiver | null; deliveries: WebhookDelivery[] }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [format, setFormat] = useState<SubjectFormat>(receiver?.subjectFormat ?? "iss_sub");
  const [removing, setRemoving] = useState(false);
  const stream = receiver?.stream;
  const locale = useLocale();
  const t = applicationsMessages[locale];
  const s = t.signals;

  function submit(event: FormEvent) {
    event.preventDefault();
    void run(
      "save",
      () => api(`/applications/${appId}/shared-signals`, { method: "PUT", body: { subjectFormat: format } }),
      () =>
        toast({
          title: receiver ? s.formatSaved : s.turnedOn,
          description: receiver ? s.formatHint(appName, format) : s.turnedOnHint(appName),
        }),
      "inline",
    );
  }

  return (
    <>
      <Card>
        <CardHeader
          title={s.title}
          description={s.description(appName)}
        />
        <form onSubmit={submit} noValidate>
          <CardBody className="flex flex-col gap-6">
            <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
              <Field label={s.subjectFormat} hint={s.subjectFormatHint}>
                {(props) => (
                  <Select {...props} value={format} onChange={(event) => setFormat(event.target.value as SubjectFormat)}>
                    {Object.entries(s.formats).map(([value, label]) => (
                      <option key={value} value={value}>
                        {label}
                      </option>
                    ))}
                  </Select>
                )}
              </Field>
              <CopyField label={s.transmitter} value={`${issuer}/.well-known/ssf-configuration`} hint={s.transmitterHint(appName)} />
            </div>
            {stream ? (
              <DescriptionList
                columns={2}
                items={[
                  {
                    label: s.stream,
                    value: (
                      <Badge tone={STREAM_TONE[stream.status]} dot>
                        {s.streamStatus[stream.status]}
                        {stream.reason ? ` · ${stream.reason}` : ""}
                      </Badge>
                    ),
                  },
                  { label: s.delivery, value: stream.deliveryMethod === "urn:ietf:rfc:8935" ? s.pushed(new URL(stream.endpointUrl).host) : s.polled(appName) },
                  {
                    label: s.events,
                    value: stream.eventsRequested.length ? (
                      <span className="flex flex-wrap gap-2">
                        {stream.eventsRequested.map((uri) => (
                          <Tag key={uri}>{eventName(uri)}</Tag>
                        ))}
                      </span>
                    ) : (
                      t.common.none
                    ),
                  },
                  { label: t.common.created, value: formatDateTime(stream.createdAt, locale) },
                ]}
              />
            ) : receiver ? (
              <p className="text-body-sm text-fg-3">{s.noStream(appName)}</p>
            ) : null}
          </CardBody>
          <div className="flex flex-wrap items-center justify-end gap-4 border-t border-border px-6 py-4">
            <ActionError message={errorFor("save")} />
            {receiver ? (
              <Button variant="secondary" onClick={() => setRemoving(true)}>
                {s.turnOff}
              </Button>
            ) : null}
            <Button type="submit" variant="primary" loading={pending === "save"} disabled={receiver?.subjectFormat === format}>
              {receiver ? s.saveFormat : s.turnOn}
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
          title={s.turnOffTitle}
          description={s.turnOffHint(appName)}
          footer={
            <>
              <Button variant="secondary" onClick={() => setRemoving(false)}>
                {t.common.cancel}
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
                      toast({ title: s.turnedOff, description: t.common.audit });
                    },
                    "inline",
                  )
                }
              >
                {s.turnOff}
              </Button>
            </>
          }
        />
      </Card>
      {deliveries.length > 0 ? (
        <section className="flex flex-col gap-6">
          <SectionTitle title={s.deliveries} description={s.deliveriesHint(deliveries.length)} />
          <DeliveryLog
            deliveries={deliveries.map((d) => ({ ...d, eventType: eventName(d.eventType) }))}
            payloadNote={s.payloadNote}
          />
        </section>
      ) : null}
    </>
  );
}
