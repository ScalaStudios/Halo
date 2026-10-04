"use client";

import { RefreshCw, Trash2 } from "lucide-react";
import { useState } from "react";
import { SecretWarning } from "@/components/applications/shared";
import { useMutation } from "@/components/console/use-mutation";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { formatDate } from "@/lib/format";
import type { SigningKeys as Keys } from "@/lib/settings-types";
import { ReadOnlyNote } from "./shared";

type Rotation = "oidc" | "saml";

const ROTATION: Record<Rotation, { path: string; title: string; description: string; confirm: string; done: string; result: string }> = {
  oidc: {
    path: "/signing-keys/rotate",
    title: "Rotate the token signing key?",
    description:
      "Halo signs every new ID token and access token with a new key from now on. The current key stays published at /oauth2/keys for 7 days, so tokens it already signed keep verifying, and then Halo retires it. Applications pick up the new key from /oauth2/keys on their own.",
    confirm: "Rotate signing key",
    done: "Signing key rotated",
    result: "New tokens use the new key. The previous key stays published for 7 days.",
  },
  saml: {
    path: "/saml/certificate/rotate",
    title: "Rotate the SAML certificate?",
    description: "Halo signs every SAML response with a new certificate from now on. The current certificate stays listed here until you remove it.",
    confirm: "Rotate certificate",
    done: "SAML certificate rotated",
    result: "Upload the new certificate to every SAML application. Their sign-ins fail until you do.",
  },
};

export function SigningKeys({ keys, canRotate }: { keys: Keys; canRotate: boolean }) {
  const { pending, run, toast } = useMutation();
  const [confirming, setConfirming] = useState<Rotation | null>(null);
  const rotation = confirming ? ROTATION[confirming] : null;

  function rotate(kind: Rotation) {
    void run(kind, () => api<Keys>(ROTATION[kind].path, { method: "POST" }), () => {
      setConfirming(null);
      toast({ title: ROTATION[kind].done, description: ROTATION[kind].result });
    });
  }

  function remove(id: string) {
    void run(`remove-${id}`, () => api(`/saml/certificates/${id}`, { method: "DELETE" }), () =>
      toast({ title: "Previous certificate removed", description: "Recorded in the audit log." }),
    );
  }

  const rotateButton = (kind: Rotation, label: string) =>
    canRotate ? (
      <Button size="sm" variant="secondary" onClick={() => setConfirming(kind)}>
        <RefreshCw aria-hidden="true" size={16} strokeWidth={1.75} />
        {label}
      </Button>
    ) : null;

  return (
    <Card>
      <CardHeader title="Signing keys" description="The keys Halo signs OpenID Connect tokens and SAML responses with. Every rotation is recorded in the audit log." />
      <CardBody className="flex flex-col gap-8">
        <section aria-labelledby="oidc-keys" className="flex flex-col gap-4">
          <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
            <div className="flex min-w-0 flex-col gap-1">
              <h3 id="oidc-keys" className="text-body font-semibold text-fg">
                OpenID Connect
              </h3>
              <p className="text-body-sm text-fg-3">
                Published at <code className="font-mono text-code-sm text-fg-2">/oauth2/keys</code>. Previous keys stay published for 7 days after a rotation.
              </p>
            </div>
            {rotateButton("oidc", "Rotate key")}
          </div>
          {keys.oidc.length === 0 ? (
            <p className="rounded-lg border border-border px-6 py-4 text-body-sm text-fg-3">
              No signing key yet. Halo creates the first one when it signs a token or publishes <code className="font-mono text-code-sm text-fg-2">/oauth2/keys</code>.
            </p>
          ) : (
            <SimpleTable
              caption="OpenID Connect signing keys"
              head={["Key ID", "Algorithm", "Created", "Status"]}
              rows={keys.oidc.map((key) => [
                <span key="k" className="font-mono text-code-sm text-fg">
                  {key.kid}
                </span>,
                key.algorithm,
                <span key="c" className="tnum whitespace-nowrap">
                  {formatDate(key.createdAt)}
                </span>,
                key.active ? (
                  <Badge key="s" tone="success" dot>
                    Signing
                  </Badge>
                ) : key.retiredAt ? (
                  <span key="s" className="whitespace-nowrap text-fg-3">
                    Retired {formatDate(key.retiredAt)}
                  </span>
                ) : (
                  <Badge key="s" tone="info" dot>
                    Published until {formatDate(key.retiresAt ?? key.createdAt)}
                  </Badge>
                ),
              ])}
            />
          )}
        </section>

        <section aria-labelledby="saml-certificates" className="flex flex-col gap-4">
          <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
            <div className="flex min-w-0 flex-col gap-1">
              <h3 id="saml-certificates" className="text-body font-semibold text-fg">
                SAML
              </h3>
              <p className="text-body-sm text-fg-3">SAML applications trust only the certificate they were given. After a rotation, upload the new one to each of them.</p>
            </div>
            {rotateButton("saml", "Rotate certificate")}
          </div>
          {keys.saml.length === 0 ? (
            <p className="rounded-lg border border-border px-6 py-4 text-body-sm text-fg-3">
              No SAML certificate yet. Halo creates one when the first SAML application or <code className="font-mono text-code-sm text-fg-2">/saml/metadata</code> needs it.
            </p>
          ) : (
            <SimpleTable
              caption="SAML signing certificates"
              head={canRotate ? ["SHA-256 fingerprint", "Expires", "Status", "Actions"] : ["SHA-256 fingerprint", "Expires", "Status"]}
              rows={keys.saml.map((cert) => {
                const row = [
                  <span key="f" className="block max-w-80 truncate font-mono text-code-sm text-fg" title={cert.fingerprint}>
                    {cert.fingerprint}
                  </span>,
                  <span key="e" className="tnum whitespace-nowrap">
                    {formatDate(cert.notAfter)}
                  </span>,
                  cert.retiredAt ? (
                    <span key="s" className="whitespace-nowrap text-fg-3">
                      Replaced {formatDate(cert.retiredAt)}
                    </span>
                  ) : (
                    <Badge key="s" tone="success" dot>
                      Signing
                    </Badge>
                  ),
                ];
                if (canRotate) {
                  row.push(
                    cert.retiredAt ? (
                      <Button key="a" size="sm" variant="quiet" loading={pending === `remove-${cert.id}`} onClick={() => remove(cert.id)}>
                        <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
                        Remove
                      </Button>
                    ) : (
                      <a key="a" href="/saml/certificate" className="text-body-sm whitespace-nowrap text-link hover:underline">
                        Download .pem
                      </a>
                    ),
                  );
                }
                return row;
              })}
            />
          )}
        </section>

        {canRotate ? null : <ReadOnlyNote>Only a global administrator can rotate signing keys.</ReadOnlyNote>}
      </CardBody>

      <Dialog
        open={rotation !== null}
        onOpenChange={(open) => {
          if (!open) setConfirming(null);
        }}
        title={rotation?.title ?? ""}
        description={rotation?.description}
        footer={
          <>
            <Button variant="secondary" onClick={() => setConfirming(null)}>
              Cancel
            </Button>
            <Button variant="primary" loading={confirming !== null && pending === confirming} onClick={() => confirming && rotate(confirming)}>
              {rotation?.confirm}
            </Button>
          </>
        }
      >
        {confirming === "saml" ? (
          <SecretWarning>
            Sign-ins to every SAML application fail until you upload the new certificate, or its new metadata, to that application. Download it from /saml/certificate after rotating.
          </SecretWarning>
        ) : null}
      </Dialog>
    </Card>
  );
}
