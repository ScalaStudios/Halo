"use client";

import { Globe, Plus, ShieldCheck, Trash2 } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ActionError } from "@/components/ui/action-error";
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
import { useLocale, useMessages } from "@/lib/i18n/client";
import { settingsMessages } from "@/lib/i18n/console/settings";
import type { Domain } from "@/lib/settings-types";
import { ReadOnlyNote } from "./shared";

export function DomainsManager({ domains, canEdit }: { domains: Domain[]; canEdit: boolean }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [adding, setAdding] = useState(false);
  const [removing, setRemoving] = useState<Domain | null>(null);
  const locale = useLocale();
  const { domains: t, common } = settingsMessages[locale];
  const waiting = domains.filter((d) => !d.verifiedAt);
  const verified = domains.filter((d) => d.verifiedAt);

  function verify(domain: Domain) {
    void run(`verify-${domain.id}`, () => api<Domain>(`/domains/${domain.id}/verify`, { method: "POST" }), () =>
      toast({ title: t.verifiedToast(domain.name), description: t.verifiedHint }),
      "inline",
    );
  }

  function remove() {
    const domain = removing;
    if (!domain) return;
    void run("remove", () => api(`/domains/${domain.id}`, { method: "DELETE" }), () => {
      setRemoving(null);
      toast({ title: t.removed(domain.name), description: common.recorded });
    }, "inline");
  }

  const add = canEdit ? (
    <Button size="sm" variant="primary" onClick={() => setAdding(true)}>
      <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
      {t.add}
    </Button>
  ) : null;

  const removeButton = (domain: Domain) =>
    canEdit ? (
      <Button size="icon-sm" variant="quiet" aria-label={t.removeLabel(domain.name)} onClick={() => setRemoving(domain)}>
        <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
      </Button>
    ) : null;

  return (
    <>
      {domains.length === 0 ? (
        <Card>
          <EmptyState
            icon={Globe}
            title={t.emptyTitle}
            description={t.emptyDescription}
            action={add}
          />
        </Card>
      ) : (
        <>
          {waiting.length ? (
            <section className="flex flex-col gap-6">
              <SectionTitle title={t.waitingTitle} description={t.waitingHint} action={add} />
              {waiting.map((domain) => (
                <Card key={domain.id}>
                  <CardHeader
                    title={domain.name}
                    description={t.added(formatDate(domain.createdAt, locale))}
                    actions={
                      <>
                        <Badge tone="warning" dot>
                          {t.notVerified}
                        </Badge>
                        {removeButton(domain)}
                      </>
                    }
                  />
                  <CardBody className="flex flex-col gap-6">
                    <div className="grid grid-cols-1 gap-4 md:grid-cols-[1fr_2fr]">
                      <CopyField label={t.host} value={domain.name} hint={t.hostHint} />
                      <CopyField label={t.txt} value={`halo-verification=${domain.token}`} />
                    </div>
                    {canEdit ? (
                      <div className="flex flex-wrap items-center gap-4">
                        <Button variant="secondary" loading={pending === `verify-${domain.id}`} onClick={() => verify(domain)}>
                          <ShieldCheck aria-hidden="true" size={16} strokeWidth={1.75} />
                          {t.verify(domain.name)}
                        </Button>
                        <ActionError message={errorFor(`verify-${domain.id}`)} />
                      </div>
                    ) : null}
                  </CardBody>
                </Card>
              ))}
            </section>
          ) : null}
          <section className="flex flex-col gap-6">
            <SectionTitle title={t.verifiedTitle} description={t.verifiedDescription} action={waiting.length ? null : add} />
            {verified.length ? (
              <SimpleTable
                caption={t.verifiedTitle}
                head={[t.domain, common.status, t.addedColumn, t.verified, common.actions]}
                rows={verified.map((domain) => [
                  <span key="name" className="font-medium whitespace-nowrap text-fg">
                    {domain.name}
                  </span>,
                  <Badge key="status" tone="success" dot>
                    {t.verified}
                  </Badge>,
                  <span key="added" className="tnum whitespace-nowrap">
                    {formatDate(domain.createdAt, locale)}
                  </span>,
                  <span key="verified" className="tnum whitespace-nowrap">
                    {formatDate(domain.verifiedAt!, locale)}
                  </span>,
                  <span key="actions" className="flex justify-end">
                    {removeButton(domain)}
                  </span>,
                ])}
              />
            ) : (
              <Card>
                <CardBody className="pt-6">
                  <p className="text-body-sm text-fg-3">{t.noneVerified}</p>
                </CardBody>
              </Card>
            )}
          </section>
        </>
      )}
      {canEdit ? null : <ReadOnlyNote>{t.readOnly}</ReadOnlyNote>}
      {adding ? <AddDomainDialog onClose={() => setAdding(false)} /> : null}
      <Dialog
        open={removing !== null}
        onOpenChange={(open) => {
          if (open) return;
          setRemoving(null);
          clearError();
        }}
        error={errorFor("remove")}
        title={t.removeTitle(removing?.name ?? t.fallback)}
        description={t.removeDescription}
        footer={
          <>
            <Button variant="secondary" onClick={() => setRemoving(null)}>
              {common.cancel}
            </Button>
            <Button variant="destructive" loading={pending === "remove"} onClick={remove}>
              {t.removeButton}
            </Button>
          </>
        }
      />
    </>
  );
}

function AddDomainDialog({ onClose }: { onClose: () => void }) {
  const { pending, run, errorFor, toast } = useMutation();
  const [name, setName] = useState("");
  const [error, setError] = useState<string>();
  const { domains: t, common } = useMessages(settingsMessages);

  function submit() {
    const value = name.trim().toLowerCase().replace(/\.$/, "");
    if (!/^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,}$/.test(value)) {
      setError(t.invalid);
      return;
    }
    void run("add", () => api<Domain>("/domains", { body: { name: value } }), (domain) => {
      toast({ title: t.addedToast(domain.name), description: t.addedHint });
      onClose();
    }, "inline");
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      error={errorFor("add")}
      title={t.add}
      description={t.addDescription}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {common.cancel}
          </Button>
          <Button variant="primary" loading={pending === "add"} onClick={submit}>
            {t.add}
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
        <Field label={t.domain} error={error}>
          {(props) => <Input {...props} mono autoFocus autoComplete="off" spellCheck={false} value={name} placeholder="example.com" onChange={(event) => setName(event.target.value)} />}
        </Field>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}
