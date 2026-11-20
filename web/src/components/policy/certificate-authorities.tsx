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
import type { CertificateAuthority } from "@/lib/policy-types";

const MAPPING: Record<CertificateAuthority["mapping"], { label: string; hint: string }> = {
  email: { label: "Email address", hint: "The first email address in the certificate's subject alternative name." },
  upn: { label: "User principal name", hint: "The UPN in the subject alternative name, as on Active Directory smart cards." },
  subject: { label: "Subject common name", hint: "The certificate's common name (CN), when your authority puts the person's email address there." },
};

export function CertificateAuthorities({ authorities }: { authorities: CertificateAuthority[] }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [adding, setAdding] = useState(false);
  const [deleting, setDeleting] = useState<CertificateAuthority | null>(null);

  function remove() {
    const ca = deleting;
    if (!ca) return;
    void run("delete", () => api(`/certificate-authorities/${ca.id}`, { method: "DELETE" }), () => {
      setDeleting(null);
      toast({ title: `${ca.name} no longer trusted`, description: "Its certificates can't sign anyone in. Recorded in the audit log." });
    }, "inline");
  }

  return (
    <div className="flex flex-col gap-6">
      <SectionTitle
        title="Trusted certificate authorities"
        description="Client certificates must chain to one of these. Your reverse proxy checks the certificate and passes it in the header HALO_CLIENT_CERT_HEADER names, replacing that header when the browser sends it. Halo asks the OCSP responder or CRL each certificate names over HTTP whether it was revoked, and refuses the certificate when it can't."
        action={
          <Button size="sm" variant="primary" onClick={() => setAdding(true)}>
            <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
            Add authority
          </Button>
        }
      />
      {authorities.length ? (
        <SimpleTable
          caption="Trusted certificate authorities"
          head={["Name", "Matches people by", "Certificates", "Actions"]}
          rows={authorities.map((ca) => [
            <span key="name" className="font-medium whitespace-nowrap text-fg">
              {ca.name}
            </span>,
            <span key="mapping" className="whitespace-nowrap">
              {MAPPING[ca.mapping].label}
            </span>,
            <ul key="certificates" className="flex flex-col gap-1">
              {ca.certificates.map((c) => (
                <li key={c.subject} className="flex flex-col">
                  <span className="break-all text-fg">{c.subject}</span>
                  <span className="text-caption text-fg-3">Expires {formatDate(c.expiresAt)}</span>
                </li>
              ))}
            </ul>,
            <Button key="actions" size="icon-sm" variant="quiet" aria-label={`Stop trusting ${ca.name}`} onClick={() => setDeleting(ca)}>
              <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
            </Button>,
          ])}
        />
      ) : (
        <Card>
          <EmptyState icon={IdCard} title="No certificate authorities yet" description="Add the authority that issues your people's client certificates before you allow client certificates above." />
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
        title={`Stop trusting ${deleting?.name ?? "this authority"}?`}
        description="Certificates it issued stop signing anyone in from the next attempt. People who are already signed in stay signed in."
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(null)}>
              Cancel
            </Button>
            <Button variant="destructive" loading={pending === "delete"} onClick={remove}>
              Stop trusting
            </Button>
          </>
        }
      />
    </div>
  );
}

function AuthorityDialog({ onClose }: { onClose: () => void }) {
  const { pending, run, errorFor, toast } = useMutation();
  const [name, setName] = useState("");
  const [mapping, setMapping] = useState<CertificateAuthority["mapping"]>("email");
  const [certificates, setCertificates] = useState("");
  const [errors, setErrors] = useState<{ name?: string; certificates?: string }>({});

  function save() {
    const next = {
      name: name.trim() ? undefined : "Enter a name, for example “Corporate PKI”.",
      certificates: certificates.includes("-----BEGIN CERTIFICATE-----") ? undefined : "Paste the authority's certificate in PEM format, starting with -----BEGIN CERTIFICATE-----.",
    };
    setErrors(next);
    if (next.name || next.certificates) return;
    void run(
      "save",
      () => api<CertificateAuthority>("/certificate-authorities", { body: { name: name.trim(), mapping, certificates } }),
      (saved) => {
        toast({ title: `${saved.name} trusted`, description: "Its certificates can sign people in from the next attempt. Recorded in the audit log." });
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
      title="Add certificate authority"
      description="A certificate that chains to it signs in the person whose email address matches the field you choose."
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={save}>
            Add authority
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
        <Field label="Name" error={errors.name}>
          {(props) => <Input {...props} autoFocus autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder="Corporate PKI" />}
        </Field>
        <Field label="Match people by" hint={MAPPING[mapping].hint}>
          {(props) => (
            <Select {...props} value={mapping} onChange={(event) => setMapping(event.target.value as CertificateAuthority["mapping"])}>
              {(Object.keys(MAPPING) as CertificateAuthority["mapping"][]).map((key) => (
                <option key={key} value={key}>
                  {MAPPING[key].label}
                </option>
              ))}
            </Select>
          )}
        </Field>
        <Field label="Certificates" error={errors.certificates} hint="The PEM certificate of the authority that signs your client certificates, not only its root. Paste more issuing authorities one after another.">
          {(props) => (
            <Textarea {...props} mono rows={8} spellCheck={false} value={certificates} onChange={(event) => setCertificates(event.target.value)} placeholder="-----BEGIN CERTIFICATE-----" />
          )}
        </Field>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}
