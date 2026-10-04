"use client";

import { Download } from "lucide-react";
import { useState, type FormEvent } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { Field, Input, Select, Textarea } from "@/components/ui/input";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { NAME_ID_FORMATS, type NameIdFormat, type SamlAttributes, type SamlConnection, type SamlSettings } from "@/lib/saml-types";
import { ExpiryText } from "./shared";

const SOURCES: { key: keyof SamlAttributes; label: string; source: string }[] = [
  { key: "email", label: "Email address", source: "user.email" },
  { key: "name", label: "Full name", source: "user.name" },
  { key: "givenName", label: "Given name", source: "user.name (first word)" },
  { key: "familyName", label: "Family name", source: "user.name (after the first word)" },
  { key: "groups", label: "Groups", source: "user.groups (every group)" },
];

export function SamlConnectionFields({ idp }: { idp: SamlConnection }) {
  return (
    <>
      <CopyField label="Metadata URL" value={idp.metadataUrl} />
      <CopyField label="SSO URL" value={idp.ssoUrl} />
      <CopyField label="IdP entity ID" value={idp.entityId} />
      <CopyField
        label="Signing certificate fingerprint (SHA-256)"
        value={idp.certificateFingerprint}
        hint={
          <span className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <a href={idp.certificateUrl} download className="inline-flex items-center gap-1 text-link underline underline-offset-3 hover:text-ember">
              <Download aria-hidden="true" size={16} strokeWidth={1.75} />
              Download certificate
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
  const { pending, run, toast } = useMutation();

  function submit(event: FormEvent) {
    event.preventDefault();
    const urls = acsUrls.split("\n").map((url) => url.trim()).filter(Boolean);
    void run(
      "settings",
      () => api(`/applications/${appId}/saml`, { method: "PUT", body: { entityId: entityId.trim(), acsUrls: urls, nameIdFormat: format, signResponse } }),
      () => toast({ title: "SAML settings saved", description: `${appName} gets them on its next sign-in. Recorded in the audit log.` }),
    );
  }

  return (
    <Card>
      <CardHeader title="SAML settings" description={`How Halo builds and signs assertions for ${appName}. Signatures use RSA-SHA256; single logout is not supported.`} />
      <form onSubmit={submit} noValidate>
        <CardBody className="grid grid-cols-1 gap-6 md:grid-cols-2">
          <Field label="Entity ID" hint="The service provider's issuer, from its SAML settings or metadata.">
            {(props) => <Input {...props} mono value={entityId} onChange={(event) => setEntityId(event.target.value)} />}
          </Field>
          <Field label="NameID format" hint={`Halo sends ${NAME_ID_FORMATS[format].value} as the NameID.`}>
            {(props) => (
              <Select {...props} value={format} onChange={(event) => setFormat(event.target.value as NameIdFormat)}>
                {(Object.keys(NAME_ID_FORMATS) as NameIdFormat[]).map((key) => (
                  <option key={key} value={key}>
                    {NAME_ID_FORMATS[key].label}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label="ACS URLs" hint="One per line. Halo only posts SAML responses to these addresses." className="md:col-span-2">
            {(props) => <Textarea {...props} mono rows={3} value={acsUrls} onChange={(event) => setAcsUrls(event.target.value)} />}
          </Field>
          <Field label="Signature" hint={signResponse ? "Halo signs the response and the assertion inside it." : "Most applications expect a signed assertion."}>
            {(props) => (
              <Select {...props} value={signResponse ? "response" : "assertion"} onChange={(event) => setSignResponse(event.target.value === "response")}>
                <option value="assertion">Sign the assertion</option>
                <option value="response">Sign the whole response</option>
              </Select>
            )}
          </Field>
        </CardBody>
        <div className="flex justify-end border-t border-border px-6 py-4">
          <Button type="submit" variant="primary" loading={pending === "settings"}>
            Save settings
          </Button>
        </div>
      </form>
    </Card>
  );
}

export function SamlAttributesForm({ appId, appName, saml }: { appId: string; appName: string; saml: SamlSettings }) {
  const [names, setNames] = useState<SamlAttributes>(saml.attributes);
  const { pending, run, toast } = useMutation();

  function submit(event: FormEvent) {
    event.preventDefault();
    void run(
      "attributes",
      () => api(`/applications/${appId}/saml`, { method: "PUT", body: { attributes: names } }),
      () => toast({ title: "Attributes saved", description: `${appName} receives them on its next sign-in. Recorded in the audit log.` }),
    );
  }

  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-6">
      <SimpleTable
        caption="Attributes"
        head={["Value", "Source", "SAML attribute name"]}
        rows={[
          [
            <span key="v" className="font-medium text-fg">
              NameID
            </span>,
            <span key="s" className="font-mono text-code-sm text-fg-2">
              {NAME_ID_FORMATS[saml.nameIdFormat].value}
            </span>,
            <span key="n" className="text-fg-3">
              Set by the NameID format on the Authentication tab
            </span>,
          ],
          ...SOURCES.map(({ key, label, source }) => [
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
              aria-label={`SAML attribute name for ${label.toLowerCase()}`}
              placeholder="Not sent"
              value={names[key]}
              onChange={(event) => setNames({ ...names, [key]: event.target.value })}
            />,
          ]),
        ]}
      />
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <p className="text-body-sm text-fg-3">Clear a name to stop sending that value. Empty values, like a missing family name, are never sent.</p>
        <Button type="submit" variant="primary" loading={pending === "attributes"} className="self-start sm:self-auto">
          Save attributes
        </Button>
      </div>
    </form>
  );
}
