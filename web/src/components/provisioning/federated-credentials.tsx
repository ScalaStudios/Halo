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
import type { FederatedCredential, KeyScope, ServiceAccount } from "@/lib/provisioning-types";
import { ScopeFieldset } from "./account-actions";
import { ScopeTags } from "./shared";

const ISSUERS = {
  github: { label: "GitHub Actions", issuer: "https://token.actions.githubusercontent.com", subject: "repo:acme/infra:ref:refs/heads/main" },
  gitlab: { label: "GitLab CI/CD", issuer: "https://gitlab.com", subject: "project_path:acme/infra:ref_type:branch:ref:main" },
  kubernetes: { label: "Kubernetes service account", issuer: "", subject: "system:serviceaccount:ci:deployer" },
  other: { label: "Other OpenID Connect issuer", issuer: "", subject: "" },
};

type IssuerKind = keyof typeof ISSUERS;

export function FederatedCredentials({ account, issuer }: { account: ServiceAccount; issuer: string }) {
  const [adding, setAdding] = useState(false);
  const [removing, setRemoving] = useState<FederatedCredential | null>(null);
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const credentials = account.federatedCredentials;

  function remove() {
    const credential = removing;
    if (!credential) return;
    void run(
      "remove",
      () => api(`/service-accounts/${account.id}/federated-credentials/${credential.id}`, { method: "DELETE" }),
      () => {
        setRemoving(null);
        toast({ title: `“${credential.name}” removed`, description: "Tokens issued through it stopped working. Recorded in the audit log." });
      },
      "inline",
    );
  }

  const add = (
    <Button size="sm" variant="secondary" onClick={() => setAdding(true)}>
      <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
      Add federated credential
    </Button>
  );

  return (
    <section className="flex flex-col gap-6">
      <SectionTitle
        title="Federated credentials"
        description="Workloads such as GitHub Actions or Kubernetes pods sign in with a token from their own identity provider instead of an API key, and get a key that expires after 1 hour."
        action={credentials.length ? add : null}
      />
      {credentials.length ? (
        <SimpleTable
          caption="Federated credentials"
          head={["Credential", "Subject", "Audience", "Scopes", "Last used", "Actions"]}
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
              {credential.lastUsedAt ? <RelativeTime iso={credential.lastUsedAt} /> : "Never"}
            </span>,
            <span key="r" className="flex justify-end">
              <Button size="icon-sm" variant="quiet" aria-label={`Remove ${credential.name}`} onClick={() => setRemoving(credential)}>
                <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
            </span>,
          ])}
        />
      ) : (
        <Card>
          <EmptyState
            icon={Workflow}
            title="No federated credentials"
            description={`Trust a CI pipeline or cluster so it signs in as ${account.name} without storing an API key.`}
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
        title={`Remove “${removing?.name ?? ""}”?`}
        description={`Workloads that match it can no longer sign in as ${account.name}, and the keys they got through it stop working immediately.`}
        footer={
          <>
            <Button
              variant="secondary"
              onClick={() => {
                clearError();
                setRemoving(null);
              }}
            >
              Cancel
            </Button>
            <Button variant="destructive" loading={pending === "remove"} onClick={remove}>
              Remove credential
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
      name: name.trim() ? undefined : "Enter a name that says which workload signs in.",
      issuer: trusted.trim() ? undefined : "Enter the issuer URL, such as https://token.actions.githubusercontent.com.",
      subject: subject.trim() ? undefined : "Enter the subject exactly as it appears in the token.",
      conditions: lines.every((line) => line.indexOf("=") > 0) && parsed.every(([claim, value]) => claim && value) ? undefined : "Write each condition on its own line as claim=value.",
      scopes: scopes.length ? undefined : "Choose at least one scope.",
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
      title={created ? "Federated credential added" : "Add federated credential"}
      description={
        created
          ? "Post the workload's token to the token endpoint with grant_type=client_credentials, this client_id, client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer and the token as client_assertion."
          : `Tokens that match every field sign in as ${account.name}, with its roles. Halo checks their signature against the issuer's published keys.`
      }
      footer={
        created ? (
          <Button variant="primary" onClick={close}>
            Done
          </Button>
        ) : (
          <>
            <Button variant="secondary" onClick={close}>
              Cancel
            </Button>
            <Button variant="primary" loading={pending === "create"} onClick={submit}>
              Add federated credential
            </Button>
          </>
        )
      }
    >
      {created ? (
        <div className="flex animate-enter flex-col gap-4">
          <CopyField label="Token endpoint" value={`${issuer}/oauth2/token`} />
          <CopyField label="client_id" value={account.id} />
          <CopyField label="Audience to request from the issuer" value={created.audience} />
        </div>
      ) : (
        <form
          className="flex flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <Field label="Name" error={errors.name}>
            {(props) => <Input {...props} autoFocus autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder="Deploy workflow" />}
          </Field>
          <Field label="Workload">
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
                    {ISSUERS[key].label}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label="Issuer" hint="The iss claim of the workload's tokens, exactly. Halo reads its signing keys from the issuer's discovery document." error={errors.issuer}>
            {(props) => <Input {...props} autoComplete="off" className="font-mono" value={trusted} onChange={(event) => setTrusted(event.target.value)} placeholder="https://kubernetes.example.com" />}
          </Field>
          <Field label="Subject" hint="The sub claim, exactly. Halo compares it character by character." error={errors.subject}>
            {(props) => <Input {...props} autoComplete="off" className="font-mono" value={subject} onChange={(event) => setSubject(event.target.value)} placeholder={ISSUERS[kind].subject || "workload-name"} />}
          </Field>
          <Field label="Audience" hint="The aud the workload asks its issuer for. Halo's own URL unless you change it.">
            {(props) => <Input {...props} autoComplete="off" className="font-mono" value={audience} onChange={(event) => setAudience(event.target.value)} />}
          </Field>
          <Field label="Claim conditions" hint="Optional. One per line as claim=value, such as repository_owner_id=1342004. Each claim must have exactly that value." error={errors.conditions}>
            {(props) => <Textarea {...props} mono rows={2} value={conditions} onChange={(event) => setConditions(event.target.value)} />}
          </Field>
          <ScopeFieldset account={account} scopes={scopes} onChange={setScopes} error={errors.scopes} />
          <button type="submit" hidden />
        </form>
      )}
    </Dialog>
  );
}
