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
import { useLocale } from "@/lib/i18n/client";
import { settingsMessages } from "@/lib/i18n/console/settings";
import type { SigningKeys as Keys } from "@/lib/settings-types";
import { ReadOnlyNote } from "./shared";

type Rotation = "oidc" | "saml";

const PATHS: Record<Rotation, string> = { oidc: "/signing-keys/rotate", saml: "/saml/certificate/rotate" };

type Messages = (typeof settingsMessages)["en"]["signingKeys"];

function keyManager(ref: string, t: Messages) {
  if (ref.startsWith("arn:")) return "AWS KMS";
  if (ref.startsWith("projects/")) return "Google Cloud KMS";
  if (ref.includes("://")) return "Azure Key Vault";
  if (ref.startsWith("pkcs11:")) return t.pkcs11;
  return "Vault Transit";
}

function storedIn(externalKey: string | null, t: Messages) {
  return externalKey ? (
    <span key="l" className="whitespace-nowrap" title={externalKey}>
      {keyManager(externalKey, t)}
    </span>
  ) : (
    <span key="l" className="whitespace-nowrap text-fg-3">
      {t.database}
    </span>
  );
}

export function SigningKeys({ keys, canRotate }: { keys: Keys; canRotate: boolean }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [confirming, setConfirming] = useState<Rotation | null>(null);
  const locale = useLocale();
  const { signingKeys: t, common } = settingsMessages[locale];
  const rotation = confirming ? t.rotation[confirming] : null;

  function rotate(kind: Rotation) {
    void run(kind, () => api<Keys>(PATHS[kind], { method: "POST" }), () => {
      setConfirming(null);
      toast({ title: t.rotation[kind].done, description: t.rotation[kind].result });
    }, "inline");
  }

  function remove(id: string) {
    void run(`remove-${id}`, () => api(`/saml/certificates/${id}`, { method: "DELETE" }), () =>
      toast({ title: t.removed, description: common.recorded }),
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
      <CardHeader title={t.title} description={t.description} />
      <CardBody className="flex flex-col gap-8">
        <section aria-labelledby="oidc-keys" className="flex flex-col gap-4">
          <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
            <div className="flex min-w-0 flex-col gap-1">
              <h3 id="oidc-keys" className="text-body font-semibold text-fg">
                OpenID Connect
              </h3>
              <p className="text-body-sm text-fg-3">
                {t.oidcBefore}
                <code className="font-mono text-code-sm text-fg-2">/oauth2/keys</code>
                {t.oidcAfter}
              </p>
            </div>
            {rotateButton("oidc", t.rotateKeys)}
          </div>
          {keys.oidc.length === 0 ? (
            <p className="rounded-lg border border-border px-6 py-4 text-body-sm text-fg-3">
              {t.noKeyBefore}
              <code className="font-mono text-code-sm text-fg-2">/oauth2/keys</code>
              {t.noKeyAfter}
            </p>
          ) : (
            <SimpleTable
              caption={t.oidcCaption}
              head={[t.keyId, t.algorithm, t.storedIn, t.created, common.status]}
              rows={keys.oidc.map((key) => [
                <span key="k" className="font-mono text-code-sm text-fg">
                  {key.kid}
                </span>,
                key.algorithm,
                storedIn(key.externalKey, t),
                <span key="c" className="tnum whitespace-nowrap">
                  {formatDate(key.createdAt, locale)}
                </span>,
                key.active ? (
                  <Badge key="s" tone="success" dot>
                    {t.signing}
                  </Badge>
                ) : key.retiredAt ? (
                  <span key="s" className="whitespace-nowrap text-fg-3">
                    {t.retired(formatDate(key.retiredAt, locale))}
                  </span>
                ) : (
                  <Badge key="s" tone="info" dot>
                    {t.publishedUntil(formatDate(key.retiresAt ?? key.createdAt, locale))}
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
              <p className="text-body-sm text-fg-3">{t.samlHint}</p>
            </div>
            {rotateButton("saml", t.rotateCertificate)}
          </div>
          {keys.saml.length === 0 ? (
            <p className="rounded-lg border border-border px-6 py-4 text-body-sm text-fg-3">
              {t.noCertBefore}
              <code className="font-mono text-code-sm text-fg-2">/saml/metadata</code>
              {t.noCertAfter}
            </p>
          ) : (
            <SimpleTable
              caption={t.samlCaption}
              head={canRotate ? [t.fingerprint, t.storedIn, t.expires, common.status, common.actions] : [t.fingerprint, t.storedIn, t.expires, common.status]}
              rows={keys.saml.map((cert) => {
                const row = [
                  <span key="f" className="block max-w-80 truncate font-mono text-code-sm text-fg" title={cert.fingerprint}>
                    {cert.fingerprint}
                  </span>,
                  storedIn(cert.externalKey, t),
                  <span key="e" className="tnum whitespace-nowrap">
                    {formatDate(cert.notAfter, locale)}
                  </span>,
                  cert.retiredAt ? (
                    <span key="s" className="whitespace-nowrap text-fg-3">
                      {t.replaced(formatDate(cert.retiredAt, locale))}
                    </span>
                  ) : (
                    <Badge key="s" tone="success" dot>
                      {t.signing}
                    </Badge>
                  ),
                ];
                if (canRotate) {
                  row.push(
                    cert.retiredAt ? (
                      <Button key="a" size="sm" variant="quiet" loading={pending === `remove-${cert.id}`} onClick={() => remove(cert.id)}>
                        <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
                        {common.remove}
                      </Button>
                    ) : (
                      <a key="a" href="/saml/certificate" className="text-body-sm whitespace-nowrap text-link hover:underline">
                        {t.download}
                      </a>
                    ),
                  );
                }
                return row;
              })}
            />
          )}
        </section>

        {canRotate ? null : <ReadOnlyNote>{t.readOnly}</ReadOnlyNote>}
      </CardBody>

      <Dialog
        open={rotation !== null}
        onOpenChange={(open) => {
          if (open) return;
          setConfirming(null);
          clearError();
        }}
        error={confirming ? errorFor(confirming) : undefined}
        title={rotation?.title ?? ""}
        description={rotation?.description}
        footer={
          <>
            <Button variant="secondary" onClick={() => setConfirming(null)}>
              {common.cancel}
            </Button>
            <Button variant="primary" loading={confirming !== null && pending === confirming} onClick={() => confirming && rotate(confirming)}>
              {rotation?.confirm}
            </Button>
          </>
        }
      >
        {confirming === "saml" ? (
          <SecretWarning>
            {t.samlWarning}
          </SecretWarning>
        ) : null}
      </Dialog>
    </Card>
  );
}
