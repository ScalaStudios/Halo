"use client";

import { Plus, Trash2, Workflow } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Field, Input, Select, Textarea } from "@/components/ui/input";
import { RelativeTime } from "@/components/ui/relative-time";
import { SectionTitle } from "@/components/ui/section-title";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { useMessages } from "@/lib/i18n/client";
import { provisioningMessages } from "@/lib/i18n/console/infrastructure/provisioning";
import type { FederatedCredential, KeyScope, ServiceAccount } from "@/lib/provisioning-types";
import { ScopeFieldset } from "./account-actions";
import { ScopeTags } from "./shared";

const ISSUERS = {
  github: { issuer: "https://token.actions.githubusercontent.com", subject: "repo:acme/infra:ref:refs/heads/main" },
  gitlab: { issuer: "https://gitlab.com", subject: "project_path:acme/infra:ref_type:branch:ref:main" },
  kubernetes: { issuer: "", subject: "system:serviceaccount:ci:deployer" },
  other: { issuer: "", subject: "" },
};

type IssuerKind = keyof typeof ISSUERS;

export function FederatedCredentials({ account, issuer }: { account: ServiceAccount; issuer: string }) {
  const [adding, setAdding] = useState(false);
  const [removing, setRemoving] = useState<FederatedCredential | null>(null);
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const t = useMessages(provisioningMessages);
  const credentials = account.federatedCredentials;

  function remove() {
    const credential = removing;
    if (!credential) return;
    void run(
      "remove",
      () => api(`/service-accounts/${account.id}/federated-credentials/${credential.id}`, { method: "DELETE" }),
      () => {
        setRemoving(null);
        toast({ title: t.federated.removed(credential.name), description: t.federated.removedHint });
      },
      "inline",
    );
  }

  const add = (
    <Button size="sm" variant="secondary" onClick={() => setAdding(true)}>
      <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
      {t.federated.add}
    </Button>
  );

  return (
    <section className="flex flex-col gap-6">
      <SectionTitle
        title={t.federated.title}
        description={t.federated.hint}
        action={credentials.length ? add : null}
      />
      {credentials.length ? (
        <SimpleTable
          caption={t.federated.title}
          head={t.federated.head}
          rows={credentials.map((credential) => [
            <span key="n" className="flex flex-col">
              <span className="font-medium text-fg">{credential.name}</span>
              <span className="font-mono text-code-sm text-fg-3">{credential.issuer}</span>
            </span>,
            <span key="s" className="flex flex-col">
              <span className="font-mono text-code-sm text-fg">{credential.subject}</span>
              {Object.entries(credential.conditions).map(([claim, value]) => (
                <span key={claim} className="font-mono text-code-sm text-fg-3">
                  {claim} = {value}
                </span>
              ))}
            </span>,
            <span key="a" className="font-mono text-code-sm">
              {credential.audience}
            </span>,
            <ScopeTags key="t" scopes={credential.scopes} />,
            <span key="u" className="whitespace-nowrap">
              {credential.lastUsedAt ? <RelativeTime iso={credential.lastUsedAt} /> : t.never}
            </span>,
            <span key="r" className="flex justify-end">
              <Button size="icon-sm" variant="quiet" aria-label={t.federated.remove(credential.name)} onClick={() => setRemoving(credential)}>
                <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
            </span>,
          ])}
        />
      ) : (
        <Card>
          <EmptyState
            icon={Workflow}
            title={t.federated.empty}
            description={t.federated.emptyHint(account.name)}
            action={add}
          />
        </Card>
      )}
      <AddCredentialDialog account={account} issuer={issuer} open={adding} onOpenChange={setAdding} />
      <Dialog
        open={removing !== null}
        onOpenChange={(next) => {
          if (next) return;
          clearError();
          setRemoving(null);
        }}
        error={errorFor("remove")}
        title={t.federated.removeTitle(removing?.name ?? "")}
        description={t.federated.removeHint(account.name)}
        footer={
          <>
            <Button
              variant="secondary"
              onClick={() => {
                clearError();
                setRemoving(null);
              }}
            >
              {t.cancel}
            </Button>
            <Button variant="destructive" loading={pending === "remove"} onClick={remove}>
              {t.federated.removeConfirm}
            </Button>
          </>
        }
      />
    </section>
  );
}

function AddCredentialDialog({ account, issuer, open, onOpenChange }: { account: ServiceAccount; issuer: string; open: boolean; onOpenChange: (open: boolean) => void }) {
  const [kind, setKind] = useState<IssuerKind>("github");
  const [name, setName] = useState("");
  const [trusted, setTrusted] = useState(ISSUERS.github.issuer);
  const [subject, setSubject] = useState("");
  const [audience, setAudience] = useState(issuer);
  const [conditions, setConditions] = useState("");
  const [scopes, setScopes] = useState<KeyScope[]>([]);
  const [errors, setErrors] = useState<{ name?: string; issuer?: string; subject?: string; conditions?: string; scopes?: string }>({});
  const [created, setCreated] = useState<FederatedCredential | null>(null);
  const { pending, run, errorFor, clearError } = useMutation();
  const t = useMessages(provisioningMessages);

  function close() {
    onOpenChange(false);
    clearError();
    setKind("github");
    setName("");
    setTrusted(ISSUERS.github.issuer);
    setSubject("");
    setAudience(issuer);
    setConditions("");
    setScopes([]);
    setErrors({});
    setCreated(null);
  }

  function submit() {
    const lines = conditions
      .split("\n")
      .map((line) => line.trim())
      .filter(Boolean);
    const parsed = lines.map((line) => [line.slice(0, line.indexOf("=")).trim(), line.slice(line.indexOf("=") + 1).trim()]);
    const next = {
      name: name.trim() ? undefined : t.federated.nameError,
      issuer: trusted.trim() ? undefined : t.federated.issuerError,
      subject: subject.trim() ? undefined : t.federated.subjectError,
      conditions: lines.every((line) => line.indexOf("=") > 0) && parsed.every(([claim, value]) => claim && value) ? undefined : t.federated.conditionsError,
      scopes: scopes.length ? undefined : t.account.scopesError,
    };
    setErrors(next);
    if (Object.values(next).some(Boolean)) return;
    void run(
      "create",
      () =>
        api<FederatedCredential>(`/service-accounts/${account.id}/federated-credentials`, {
          body: { name: name.trim(), issuer: trusted.trim(), subject: subject.trim(), audience: audience.trim(), conditions: Object.fromEntries(parsed), scopes },
        }),
      setCreated,
      "inline",
    );
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => (next ? onOpenChange(true) : close())}
      error={errorFor("create")}
      title={created ? t.federated.added : t.federated.add}
      description={created ? t.federated.addedHint : t.federated.addHint(account.name)}
      footer={
        created ? (
          <Button variant="primary" onClick={close}>
            {t.done}
          </Button>
        ) : (
          <>
            <Button variant="secondary" onClick={close}>
              {t.cancel}
            </Button>
            <Button variant="primary" loading={pending === "create"} onClick={submit}>
              {t.federated.add}
            </Button>
          </>
        )
      }
    >
      {created ? (
        <div className="flex animate-enter flex-col gap-4">
          <CopyField label={t.federated.tokenEndpoint} value={`${issuer}/oauth2/token`} />
          <CopyField label="client_id" value={account.id} />
          <CopyField label={t.federated.audienceToRequest} value={created.audience} />
        </div>
      ) : (
        <form
          className="flex flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <Field label={t.name} error={errors.name}>
            {(props) => <Input {...props} autoFocus autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder={t.federated.namePlaceholder} />}
          </Field>
          <Field label={t.federated.workload}>
            {(props) => (
              <Select
                {...props}
                value={kind}
                onChange={(event) => {
                  const next = event.target.value as IssuerKind;
                  setKind(next);
                  setTrusted(ISSUERS[next].issuer);
                }}
              >
                {(Object.keys(ISSUERS) as IssuerKind[]).map((key) => (
                  <option key={key} value={key}>
                    {t.federated.issuers[key]}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label={t.federated.issuer} hint={t.federated.issuerHint} error={errors.issuer}>
            {(props) => <Input {...props} autoComplete="off" className="font-mono" value={trusted} onChange={(event) => setTrusted(event.target.value)} placeholder="https://kubernetes.example.com" />}
          </Field>
          <Field label={t.federated.subject} hint={t.federated.subjectHint} error={errors.subject}>
            {(props) => <Input {...props} autoComplete="off" className="font-mono" value={subject} onChange={(event) => setSubject(event.target.value)} placeholder={ISSUERS[kind].subject || "workload-name"} />}
          </Field>
          <Field label={t.federated.audience} hint={t.federated.audienceHint}>
            {(props) => <Input {...props} autoComplete="off" className="font-mono" value={audience} onChange={(event) => setAudience(event.target.value)} />}
          </Field>
          <Field label={t.federated.conditions} hint={t.federated.conditionsHint} error={errors.conditions}>
            {(props) => <Textarea {...props} mono rows={2} value={conditions} onChange={(event) => setConditions(event.target.value)} />}
          </Field>
          <ScopeFieldset account={account} scopes={scopes} onChange={setScopes} error={errors.scopes} />
          <button type="submit" hidden />
        </form>
      )}
    </Dialog>
  );
}
