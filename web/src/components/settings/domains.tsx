"use client";

import { Globe, Plus, ShieldCheck, Trash2 } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Field, Input } from "@/components/ui/input";
import { SectionTitle } from "@/components/ui/section-title";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { formatDate } from "@/lib/format";
import type { Domain } from "@/lib/settings-types";
import { ReadOnlyNote } from "./shared";

export function DomainsManager({ domains, canEdit }: { domains: Domain[]; canEdit: boolean }) {
  const { pending, run, toast } = useMutation();
  const [adding, setAdding] = useState(false);
  const [removing, setRemoving] = useState<Domain | null>(null);
  const waiting = domains.filter((d) => !d.verifiedAt);
  const verified = domains.filter((d) => d.verifiedAt);

  function verify(domain: Domain) {
    void run(`verify-${domain.id}`, () => api<Domain>(`/domains/${domain.id}/verify`, { method: "POST" }), () =>
      toast({ title: `${domain.name} verified`, description: "You can remove the TXT record now, or keep it to verify again later." }),
    );
  }

  function remove() {
    const domain = removing;
    if (!domain) return;
    void run("remove", () => api(`/domains/${domain.id}`, { method: "DELETE" }), () => {
      setRemoving(null);
      toast({ title: `${domain.name} removed`, description: "Recorded in the audit log." });
    });
  }

  const add = canEdit ? (
    <Button size="sm" variant="primary" onClick={() => setAdding(true)}>
      <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
      Add domain
    </Button>
  ) : null;

  const removeButton = (domain: Domain) =>
    canEdit ? (
      <Button size="icon-sm" variant="quiet" aria-label={`Remove ${domain.name}`} onClick={() => setRemoving(domain)}>
        <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
      </Button>
    ) : null;

  return (
    <>
      {domains.length === 0 ? (
        <Card>
          <EmptyState
            icon={Globe}
            title="No domains yet"
            description="Add the domains in your people's email addresses, such as example.com. You prove ownership with a DNS TXT record."
            action={add}
          />
        </Card>
      ) : (
        <>
          {waiting.length ? (
            <section className="flex flex-col gap-6">
              <SectionTitle title="Waiting for verification" description="Add this TXT record at the domain's DNS provider, then select Verify." action={add} />
              {waiting.map((domain) => (
                <Card key={domain.id}>
                  <CardHeader
                    title={domain.name}
                    description={`Added ${formatDate(domain.createdAt)}`}
                    actions={
                      <>
                        <Badge tone="warning" dot>
                          Not verified
                        </Badge>
                        {removeButton(domain)}
                      </>
                    }
                  />
                  <CardBody className="flex flex-col gap-6">
                    <div className="grid grid-cols-1 gap-4 md:grid-cols-[1fr_2fr]">
                      <CopyField label="Host" value={domain.name} hint="Some DNS providers write the apex as @." />
                      <CopyField label="TXT value" value={`halo-verification=${domain.token}`} />
                    </div>
                    {canEdit ? (
                      <div>
                        <Button variant="secondary" loading={pending === `verify-${domain.id}`} onClick={() => verify(domain)}>
                          <ShieldCheck aria-hidden="true" size={16} strokeWidth={1.75} />
                          Verify {domain.name}
                        </Button>
                      </div>
                    ) : null}
                  </CardBody>
                </Card>
              ))}
            </section>
          ) : null}
          <section className="flex flex-col gap-6">
            <SectionTitle title="Verified domains" description="Halo confirmed a TXT record at each of these domains." action={waiting.length ? null : add} />
            {verified.length ? (
              <SimpleTable
                caption="Verified domains"
                head={["Domain", "Status", "Added", "Verified", "Actions"]}
                rows={verified.map((domain) => [
                  <span key="name" className="font-medium whitespace-nowrap text-fg">
                    {domain.name}
                  </span>,
                  <Badge key="status" tone="success" dot>
                    Verified
                  </Badge>,
                  <span key="added" className="tnum whitespace-nowrap">
                    {formatDate(domain.createdAt)}
                  </span>,
                  <span key="verified" className="tnum whitespace-nowrap">
                    {formatDate(domain.verifiedAt!)}
                  </span>,
                  <span key="actions" className="flex justify-end">
                    {removeButton(domain)}
                  </span>,
                ])}
              />
            ) : (
              <Card>
                <CardBody className="pt-6">
                  <p className="text-body-sm text-fg-3">No domain is verified yet.</p>
                </CardBody>
              </Card>
            )}
          </section>
        </>
      )}
      {canEdit ? null : <ReadOnlyNote>Only a global administrator can add, verify or remove domains.</ReadOnlyNote>}
      {adding ? <AddDomainDialog onClose={() => setAdding(false)} /> : null}
      <Dialog
        open={removing !== null}
        onOpenChange={(open) => {
          if (!open) setRemoving(null);
        }}
        title={`Remove ${removing?.name ?? "domain"}?`}
        description="Halo forgets the domain and its verification. To use it again, add it and publish a new TXT record."
        footer={
          <>
            <Button variant="secondary" onClick={() => setRemoving(null)}>
              Cancel
            </Button>
            <Button variant="destructive" loading={pending === "remove"} onClick={remove}>
              Remove domain
            </Button>
          </>
        }
      />
    </>
  );
}

function AddDomainDialog({ onClose }: { onClose: () => void }) {
  const { pending, run, toast } = useMutation();
  const [name, setName] = useState("");
  const [error, setError] = useState<string>();

  function submit() {
    const value = name.trim().toLowerCase().replace(/\.$/, "");
    if (!/^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,}$/.test(value)) {
      setError("Enter a domain name such as example.com, without https:// or a path.");
      return;
    }
    void run("add", () => api<Domain>("/domains", { body: { name: value } }), (domain) => {
      toast({ title: `${domain.name} added`, description: "Publish the TXT record shown on this page, then verify it." });
      onClose();
    });
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title="Add domain"
      description="Halo gives you a TXT record to publish at the domain. Verification proves your organization controls it."
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={pending === "add"} onClick={submit}>
            Add domain
          </Button>
        </>
      }
    >
      <form
        noValidate
        onSubmit={(event) => {
          event.preventDefault();
          submit();
        }}
      >
        <Field label="Domain" error={error}>
          {(props) => <Input {...props} mono autoFocus autoComplete="off" spellCheck={false} value={name} placeholder="example.com" onChange={(event) => setName(event.target.value)} />}
        </Field>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}
