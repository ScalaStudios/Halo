"use client";

import { useState, type FormEvent } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ActionError } from "@/components/ui/action-error";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Field, Input, Select, Textarea } from "@/components/ui/input";
import { api } from "@/lib/api/client";
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
  const keyed = method === "private_key_jwt";
  const profile = keyed && fapi;

  function submit(event: FormEvent) {
    event.preventDefault();
    let keys: unknown = null;
    if (keyed && source === "jwks") {
      try {
        keys = JSON.parse(jwks);
      } catch {
        setJwksError("Paste a JSON Web Key Set: an object with a keys array.");
        return;
      }
    }
    setJwksError(undefined);
    const body = keyed ? { tokenEndpointAuthMethod: method, jwks: keys, jwksUri: source === "url" ? jwksUri.trim() : "", fapi2Profile: profile } : { tokenEndpointAuthMethod: method, fapi2Profile: false };
    void run(
      "client-authentication",
      () => api(`/applications/${app.id}`, { method: "PATCH", body }),
      () => toast({ title: "Client authentication saved", description: `${app.name} must authenticate this way from its next request. Recorded in the audit log.` }),
      "inline",
    );
  }

  return (
    <Card>
      <CardHeader
        title="Client authentication"
        description={`How ${app.name} proves who it is at the token, pushed request, introspection and revocation endpoints. With a private key JWT, nothing Halo stores lets anyone else act as ${app.name}.`}
      />
      <form onSubmit={submit} noValidate>
        <CardBody className="grid grid-cols-1 gap-6 md:grid-cols-2">
          <Field label="Method" hint={keyed ? "Signs a short-lived JWT with its own private key for every request. Its client secrets stop working at these endpoints." : "Sends one of its client secrets with HTTP Basic or in the form body."}>
            {(props) => (
              <Select {...props} value={method} onChange={(event) => setMethod(event.target.value as ClientApp["tokenEndpointAuthMethod"])}>
                <option value="client_secret_basic">Client secret</option>
                <option value="private_key_jwt">Private key JWT</option>
              </Select>
            )}
          </Field>
          <Field
            label="FAPI 2.0 security profile"
            hint={
              !keyed
                ? "Needs a private key JWT."
                : profile
                  ? app.type === "web"
                    ? "Requires pushed requests, PKCE, DPoP, https redirect URIs and ES256 or EdDSA signatures. Codes expire a minute after they are issued."
                    : "Requires DPoP and ES256 or EdDSA signatures."
                  : "Each requirement is set on its own, under Request and token protection."
            }
          >
            {(props) => (
              <Select {...props} disabled={!keyed} value={profile ? "on" : "off"} onChange={(event) => setFapi(event.target.value === "on")}>
                <option value="on">On</option>
                <option value="off">Off</option>
              </Select>
            )}
          </Field>
          {keyed ? (
            <Field label="Public keys" hint={source === "url" ? "Halo fetches the keys, keeps them for an hour and fetches them again when a JWT names a key it hasn't seen." : "Paste public keys only. Halo refuses private and symmetric keys."}>
              {(props) => (
                <Select {...props} value={source} onChange={(event) => setSource(event.target.value as "url" | "jwks")}>
                  <option value="url">From a JWKS URL</option>
                  <option value="jwks">Pasted JWKS</option>
                </Select>
              )}
            </Field>
          ) : null}
          {keyed && source === "url" ? (
            <Field label="JWKS URL">
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
            Save authentication
          </Button>
        </div>
      </form>
    </Card>
  );
}
