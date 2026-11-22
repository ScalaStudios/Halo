"use client";

import { IdCard, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Field, Input, Select, Textarea } from "@/components/ui/input";
import { SectionTitle } from "@/components/ui/section-title";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { formatDate } from "@/lib/format";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { policyMessages } from "@/lib/i18n/console/policy";
import type { CertificateAuthority } from "@/lib/policy-types";

export function CertificateAuthorities({ authorities }: { authorities: CertificateAuthority[] }) {
  const locale = useLocale();
  const copy = policyMessages[locale];
  const t = copy.authorities;
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [adding, setAdding] = useState(false);
  const [deleting, setDeleting] = useState<CertificateAuthority | null>(null);

  function remove() {
    const ca = deleting;
    if (!ca) return;
    void run("delete", () => api(`/certificate-authorities/${ca.id}`, { method: "DELETE" }), () => {
      setDeleting(null);
      toast({ title: t.untrusted(ca.name), description: t.untrustedDescription });
    }, "inline");
  }

  return (
    <div className="flex flex-col gap-6">
      <SectionTitle
        title={t.title}
        description={t.description}
        action={
          <Button size="sm" variant="primary" onClick={() => setAdding(true)}>
            <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
            {t.add}
          </Button>
        }
      />
      {authorities.length ? (
        <SimpleTable
          caption={t.title}
          head={[copy.name, t.matches, t.certificates, copy.actions]}
          rows={authorities.map((ca) => [
            <span key="name" className="font-medium whitespace-nowrap text-fg">
              {ca.name}
            </span>,
            <span key="mapping" className="whitespace-nowrap">
              {t.mapping[ca.mapping].label}
            </span>,
            <ul key="certificates" className="flex flex-col gap-1">
              {ca.certificates.map((c) => (
                <li key={c.subject} className="flex flex-col">
                  <span className="break-all text-fg">{c.subject}</span>
                  <span className="text-caption text-fg-3">{t.expires(formatDate(c.expiresAt, locale))}</span>
                </li>
              ))}
            </ul>,
            <Button key="actions" size="icon-sm" variant="quiet" aria-label={t.stopTrusting(ca.name)} onClick={() => setDeleting(ca)}>
              <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
            </Button>,
          ])}
        />
      ) : (
        <Card>
          <EmptyState icon={IdCard} title={t.empty} description={t.emptyDescription} />
        </Card>
      )}
      {adding ? <AuthorityDialog onClose={() => setAdding(false)} /> : null}
      <Dialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (open) return;
          setDeleting(null);
          clearError();
        }}
        error={errorFor("delete")}
        title={t.confirmStop(deleting?.name ?? t.stopFallback)}
        description={t.stopDescription}
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(null)}>
              {copy.cancel}
            </Button>
            <Button variant="destructive" loading={pending === "delete"} onClick={remove}>
              {t.stop}
            </Button>
          </>
        }
      />
    </div>
  );
}

function AuthorityDialog({ onClose }: { onClose: () => void }) {
  const copy = useMessages(policyMessages);
  const t = copy.authorities;
  const { pending, run, errorFor, toast } = useMutation();
  const [name, setName] = useState("");
  const [mapping, setMapping] = useState<CertificateAuthority["mapping"]>("email");
  const [certificates, setCertificates] = useState("");
  const [errors, setErrors] = useState<{ name?: string; certificates?: string }>({});

  function save() {
    const next = {
      name: name.trim() ? undefined : copy.enterName(t.nameExample),
      certificates: certificates.includes("-----BEGIN CERTIFICATE-----") ? undefined : t.certificatesRequired,
    };
    setErrors(next);
    if (next.name || next.certificates) return;
    void run(
      "save",
      () => api<CertificateAuthority>("/certificate-authorities", { body: { name: name.trim(), mapping, certificates } }),
      (saved) => {
        toast({ title: t.trusted(saved.name), description: t.trustedDescription });
        onClose();
      },
      "inline",
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      error={errorFor("save")}
      title={t.addTitle}
      description={t.addDescription}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {copy.cancel}
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={save}>
            {t.add}
          </Button>
        </>
      }
    >
      <form
        className="flex flex-col gap-6"
        onSubmit={(event) => {
          event.preventDefault();
          save();
        }}
      >
        <Field label={copy.name} error={errors.name}>
          {(props) => <Input {...props} autoFocus autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder={t.nameExample} />}
        </Field>
        <Field label={t.matchBy} hint={t.mapping[mapping].hint}>
          {(props) => (
            <Select {...props} value={mapping} onChange={(event) => setMapping(event.target.value as CertificateAuthority["mapping"])}>
              {(Object.keys(t.mapping) as CertificateAuthority["mapping"][]).map((key) => (
                <option key={key} value={key}>
                  {t.mapping[key].label}
                </option>
              ))}
            </Select>
          )}
        </Field>
        <Field label={t.certificates} error={errors.certificates} hint={t.certificatesHint}>
          {(props) => (
            <Textarea {...props} mono rows={8} spellCheck={false} value={certificates} onChange={(event) => setCertificates(event.target.value)} placeholder="-----BEGIN CERTIFICATE-----" />
          )}
        </Field>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}
