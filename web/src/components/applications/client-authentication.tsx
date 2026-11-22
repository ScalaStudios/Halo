"use client";

import { useState, type FormEvent } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ActionError } from "@/components/ui/action-error";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Field, Input, Select, Textarea } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import { useMessages } from "@/lib/i18n/client";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import type { Application } from "@/lib/types";

type ClientApp = Pick<Application, "id" | "name" | "type" | "tokenEndpointAuthMethod" | "jwks" | "jwksUri" | "fapi2Profile">;

export function ClientAuthenticationForm({ app }: { app: ClientApp }) {
  const [method, setMethod] = useState(app.tokenEndpointAuthMethod);
  const [source, setSource] = useState<"url" | "jwks">(app.jwks ? "jwks" : "url");
  const [jwksUri, setJwksUri] = useState(app.jwksUri);
  const [jwks, setJwks] = useState(app.jwks ? JSON.stringify(app.jwks, null, 2) : "");
  const [fapi, setFapi] = useState(app.fapi2Profile);
  const [jwksError, setJwksError] = useState<string>();
  const { pending, run, errorFor, toast } = useMutation();
  const t = useMessages(applicationsMessages);
  const c = t.clientAuth;
  const keyed = method === "private_key_jwt";
  const profile = keyed && fapi;

  function submit(event: FormEvent) {
    event.preventDefault();
    let keys: unknown = null;
    if (keyed && source === "jwks") {
      try {
        keys = JSON.parse(jwks);
      } catch {
        setJwksError(c.invalidJwks);
        return;
      }
    }
    setJwksError(undefined);
    const body = keyed ? { tokenEndpointAuthMethod: method, jwks: keys, jwksUri: source === "url" ? jwksUri.trim() : "", fapi2Profile: profile } : { tokenEndpointAuthMethod: method, fapi2Profile: false };
    void run(
      "client-authentication",
      () => api(`/applications/${app.id}`, { method: "PATCH", body }),
      () => toast({ title: c.saved, description: c.savedHint(app.name) }),
      "inline",
    );
  }

  return (
    <Card>
      <CardHeader
        title={c.title}
        description={c.description(app.name)}
      />
      <form onSubmit={submit} noValidate>
        <CardBody className="grid grid-cols-1 gap-6 md:grid-cols-2">
          <Field label={c.method} hint={keyed ? c.methodKeyed : c.methodSecret}>
            {(props) => (
              <Select {...props} value={method} onChange={(event) => setMethod(event.target.value as ClientApp["tokenEndpointAuthMethod"])}>
                <option value="client_secret_basic">{c.clientSecret}</option>
                <option value="private_key_jwt">{c.privateKeyJwt}</option>
              </Select>
            )}
          </Field>
          <Field
            label={c.fapi}
            hint={!keyed ? c.fapiNeedsKey : profile ? (app.type === "web" ? c.fapiWeb : c.fapiService) : c.fapiOff}
          >
            {(props) => (
              <Select {...props} disabled={!keyed} value={profile ? "on" : "off"} onChange={(event) => setFapi(event.target.value === "on")}>
                <option value="on">{t.common.on}</option>
                <option value="off">{t.common.off}</option>
              </Select>
            )}
          </Field>
          {keyed ? (
            <Field label={c.publicKeys} hint={source === "url" ? c.publicKeysUrl : c.publicKeysPasted}>
              {(props) => (
                <Select {...props} value={source} onChange={(event) => setSource(event.target.value as "url" | "jwks")}>
                  <option value="url">{c.fromUrl}</option>
                  <option value="jwks">{c.pasted}</option>
                </Select>
              )}
            </Field>
          ) : null}
          {keyed && source === "url" ? (
            <Field label={c.jwksUrl}>
              {(props) => <Input {...props} type="url" mono spellCheck={false} value={jwksUri} onChange={(event) => setJwksUri(event.target.value)} placeholder="https://app.example.com/.well-known/jwks.json" />}
            </Field>
          ) : null}
          {keyed && source === "jwks" ? (
            <Field label="JWKS" error={jwksError} className="md:col-span-2">
              {(props) => <Textarea {...props} mono rows={8} spellCheck={false} value={jwks} onChange={(event) => setJwks(event.target.value)} placeholder='{"keys": [{"kty": "EC", "crv": "P-256", "x": "…", "y": "…", "kid": "…"}]}' />}
            </Field>
          ) : null}
        </CardBody>
        <div className="flex flex-wrap items-center justify-end gap-4 border-t border-border px-6 py-4">
          <ActionError message={errorFor("client-authentication")} />
          <Button type="submit" variant="primary" loading={pending === "client-authentication"}>
            {c.save}
          </Button>
        </div>
      </form>
    </Card>
  );
}
