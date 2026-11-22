"use client";

import { Download } from "lucide-react";
import { useState, type FormEvent } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ActionError } from "@/components/ui/action-error";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { Field, Input, Select, Textarea } from "@/components/ui/input";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { useMessages } from "@/lib/i18n/client";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import { NAME_ID_FORMATS, type NameIdFormat, type SamlAttributes, type SamlConnection, type SamlEncryption, type SamlSettings } from "@/lib/saml-types";
import { ExpiryText } from "./shared";

export function SamlConnectionFields({ idp }: { idp: SamlConnection }) {
  const s = useMessages(applicationsMessages).saml;
  return (
    <>
      <CopyField label={s.metadataUrl} value={idp.metadataUrl} />
      <CopyField label={s.ssoUrl} value={idp.ssoUrl} />
      <CopyField label={s.idpEntityId} value={idp.entityId} />
      <CopyField
        label={s.fingerprint}
        value={idp.certificateFingerprint}
        hint={
          <span className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <a href={idp.certificateUrl} download className="inline-flex items-center gap-1 text-link underline underline-offset-3 hover:text-ember">
              <Download aria-hidden="true" size={16} strokeWidth={1.75} />
              {s.download}
            </a>
            <ExpiryText iso={idp.certificateExpiresAt} />
          </span>
        }
      />
    </>
  );
}

export function SamlSettingsForm({ appId, appName, saml }: { appId: string; appName: string; saml: SamlSettings }) {
  const [entityId, setEntityId] = useState(saml.entityId);
  const [acsUrls, setAcsUrls] = useState(saml.acsUrls.join("\n"));
  const [format, setFormat] = useState<NameIdFormat>(saml.nameIdFormat);
  const [signResponse, setSignResponse] = useState(saml.signResponse);
  const [encryption, setEncryption] = useState<SamlEncryption>(saml.encryption);
  const [certificate, setCertificate] = useState(saml.encryptionCertificate);
  const { pending, run, errorFor, toast } = useMutation();
  const t = useMessages(applicationsMessages);
  const s = t.saml;

  function submit(event: FormEvent) {
    event.preventDefault();
    const urls = acsUrls.split("\n").map((url) => url.trim()).filter(Boolean);
    void run(
      "settings",
      () =>
        api(`/applications/${appId}/saml`, {
          method: "PUT",
          body: {
            entityId: entityId.trim(),
            acsUrls: urls,
            nameIdFormat: format,
            signResponse,
            encryption,
            encryptionCertificate: encryption === "off" ? undefined : certificate,
          },
        }),
      () => toast({ title: s.saved, description: s.savedHint(appName) }),
      "inline",
    );
  }

  return (
    <Card>
      <CardHeader title={s.title} description={s.description(appName)} />
      <form onSubmit={submit} noValidate>
        <CardBody className="grid grid-cols-1 gap-6 md:grid-cols-2">
          <Field label={t.setup.entityId} hint={t.setup.entityIdHint}>
            {(props) => <Input {...props} mono value={entityId} onChange={(event) => setEntityId(event.target.value)} />}
          </Field>
          <Field label={t.setup.nameIdFormat} hint={s.nameIdHint(NAME_ID_FORMATS[format].value)}>
            {(props) => (
              <Select {...props} value={format} onChange={(event) => setFormat(event.target.value as NameIdFormat)}>
                {(Object.keys(NAME_ID_FORMATS) as NameIdFormat[]).map((key) => (
                  <option key={key} value={key}>
                    {t.setup.nameIdFormats[key]}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label={s.acsUrls} hint={s.acsUrlsHint} className="md:col-span-2">
            {(props) => <Textarea {...props} mono rows={3} value={acsUrls} onChange={(event) => setAcsUrls(event.target.value)} />}
          </Field>
          <Field label={s.signature} hint={signResponse ? s.signResponse : s.signAssertion}>
            {(props) => (
              <Select {...props} value={signResponse ? "response" : "assertion"} onChange={(event) => setSignResponse(event.target.value === "response")}>
                <option value="assertion">{s.assertionOption}</option>
                <option value="response">{s.responseOption}</option>
              </Select>
            )}
          </Field>
          <Field label={s.encryption} hint={s.encryptions[encryption].hint}>
            {(props) => (
              <Select {...props} value={encryption} onChange={(event) => setEncryption(event.target.value as SamlEncryption)}>
                {(Object.keys(s.encryptions) as SamlEncryption[]).map((key) => (
                  <option key={key} value={key}>
                    {s.encryptions[key].label}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          {encryption === "off" ? null : (
            <Field
              label={s.certificate}
              hint={s.certificateHint}
              className="md:col-span-2"
            >
              {(props) => (
                <Textarea
                  {...props}
                  mono
                  rows={6}
                  spellCheck={false}
                  placeholder="-----BEGIN CERTIFICATE-----"
                  value={certificate}
                  onChange={(event) => setCertificate(event.target.value)}
                />
              )}
            </Field>
          )}
        </CardBody>
        <div className="flex flex-wrap items-center justify-end gap-4 border-t border-border px-6 py-4">
          <ActionError message={errorFor("settings")} />
          <Button type="submit" variant="primary" loading={pending === "settings"}>
            {s.save}
          </Button>
        </div>
      </form>
    </Card>
  );
}

export function SamlAttributesForm({ appId, appName, saml }: { appId: string; appName: string; saml: SamlSettings }) {
  const [names, setNames] = useState<SamlAttributes>(saml.attributes);
  const { pending, run, errorFor, toast } = useMutation();
  const t = useMessages(applicationsMessages);
  const s = t.saml;

  function submit(event: FormEvent) {
    event.preventDefault();
    void run(
      "attributes",
      () => api(`/applications/${appId}/saml`, { method: "PUT", body: { attributes: names } }),
      () => toast({ title: s.attributesSaved, description: s.attributesHint(appName) }),
      "inline",
    );
  }

  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-6">
      <SimpleTable
        caption={t.mappings.kinds.attribute.tab}
        head={[t.common.value, t.common.source, s.attributeName]}
        rows={[
          [
            <span key="v" className="font-medium text-fg">
              NameID
            </span>,
            <span key="s" className="font-mono text-code-sm text-fg-2">
              {NAME_ID_FORMATS[saml.nameIdFormat].value}
            </span>,
            <span key="n" className="text-fg-3">
              {s.nameIdNote}
            </span>,
          ],
          ...(Object.entries(s.sources) as [keyof SamlAttributes, { label: string; source: string }][]).map(([key, { label, source }]) => [
            <span key="v" className="font-medium whitespace-nowrap text-fg">
              {label}
            </span>,
            <span key="s" className="font-mono text-code-sm whitespace-nowrap text-fg-2">
              {source}
            </span>,
            <Input
              key="n"
              size="sm"
              mono
              className="min-w-48"
              aria-label={s.attributeFor(label)}
              placeholder={t.mappings.notSent}
              value={names[key]}
              onChange={(event) => setNames({ ...names, [key]: event.target.value })}
            />,
          ]),
        ]}
      />
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <p className="text-body-sm text-fg-3">{s.clearNote}</p>
        <div className="flex flex-col items-start gap-2 sm:items-end">
          <ActionError message={errorFor("attributes")} />
          <Button type="submit" variant="primary" loading={pending === "attributes"}>
            {s.saveAttributes}
          </Button>
        </div>
      </div>
    </form>
  );
}
