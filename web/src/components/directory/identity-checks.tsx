"use client";

import { IdCard } from "lucide-react";
import { useId, useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Badge, type Tone } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Field, Select, Textarea } from "@/components/ui/input";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { formatDateTime } from "@/lib/format";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { directoryMessages } from "@/lib/i18n/console/directory";
import type { IdentityProof } from "@/lib/types";

type Document = Exclude<IdentityProof["document"], "">;

const PROOF_TONE: Record<IdentityProof["status"], Tone> = { verified: "success", pending: "warning", failed: "danger" };

export function IdentityChecks({ proofs, providerName, empty }: { proofs: IdentityProof[]; providerName: string; empty: string }) {
  const locale = useLocale();
  const t = directoryMessages[locale];
  if (proofs.length === 0) return <EmptyState icon={IdCard} title={t.identity.emptyTitle} description={empty} />;
  return (
    <SimpleTable
      caption={t.user.identityChecks}
      head={[t.identity.result, t.identity.method, t.identity.details, t.identity.date]}
      rows={proofs.map((p) => [
        <Badge key="s" tone={PROOF_TONE[p.status]}>
          {t.identity.proofStatus[p.status]}
        </Badge>,
        p.method === "in-person" ? t.identity.inPerson(p.checkedBy?.name) : p.provider === "stripe" ? "Stripe Identity" : providerName || t.identity.provider,
        p.method === "in-person" ? (
          <span key="d" className="flex flex-col">
            <span className="text-fg">{p.document ? t.identity.documents[p.document] : "—"}</span>
            <span className="text-caption text-fg-3">{p.note}</span>
          </span>
        ) : (
          <span key="d" className="font-mono text-caption text-fg-3">
            {p.reference}
          </span>
        ),
        formatDateTime(p.completedAt ?? p.createdAt, locale),
      ])}
    />
  );
}

export function CheckIdentityButton({ name, endpoint }: { name: string; endpoint: string }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const formId = useId();
  const t = useMessages(directoryMessages);
  const [open, setOpen] = useState(false);
  const [document, setDocument] = useState<Document>("passport");
  const [note, setNote] = useState("");
  const [attempted, setAttempted] = useState(false);
  const noteError = !note.trim()
    ? t.identity.noteRequired
    : /[0-9]{5,}/.test(note)
      ? t.identity.noteNumber
      : note.trim().length > 500
        ? t.identity.noteLength
        : undefined;

  function close() {
    setOpen(false);
    setAttempted(false);
    setNote("");
  }

  return (
    <>
      <Button
        size="sm"
        onClick={() => {
          clearError();
          setOpen(true);
        }}
      >
        <IdCard aria-hidden="true" size={16} strokeWidth={1.75} />
        {t.identity.check}
      </Button>
      <Dialog
        open={open}
        onOpenChange={close}
        error={errorFor("check")}
        title={t.identity.checkTitle(name)}
        description={t.identity.checkDescription(name)}
        footer={
          <>
            <Button variant="secondary" onClick={close}>
              {t.cancel}
            </Button>
            <Button variant="primary" type="submit" form={formId} loading={pending === "check"}>
              {t.identity.confirm}
            </Button>
          </>
        }
      >
        <form
          id={formId}
          noValidate
          className="flex flex-col gap-4"
          onSubmit={(event) => {
            event.preventDefault();
            setAttempted(true);
            if (noteError) return;
            void run(
              "check",
              () => api<IdentityProof>(endpoint, { body: { document, note: note.trim() } }),
              () => {
                close();
                toast({ title: t.identity.confirmed(name), description: t.identity.confirmedHint });
              },
              "inline",
            );
          }}
        >
          <Field label={t.identity.document}>
            {(props) => (
              <Select {...props} value={document} onChange={(event) => setDocument(event.target.value as Document)}>
                {(Object.keys(t.identity.documents) as Document[]).map((key) => (
                  <option key={key} value={key}>
                    {t.identity.documents[key]}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label={t.identity.note} hint={t.identity.noteHint} error={attempted ? noteError : undefined}>
            {(props) => <Textarea {...props} rows={3} value={note} onChange={(event) => setNote(event.target.value)} placeholder={t.identity.notePlaceholder} />}
          </Field>
        </form>
      </Dialog>
    </>
  );
}
