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
import type { IdentityProof } from "@/lib/types";

export const DOCUMENTS: Record<Exclude<IdentityProof["document"], "">, string> = {
  passport: "Passport",
  "national-id": "National identity card",
  "driving-licence": "Driving licence",
  "residence-permit": "Residence permit",
  other: "Another official document",
};

export const PROOF_STATUS: Record<IdentityProof["status"], { label: string; tone: Tone }> = {
  verified: { label: "Verified", tone: "success" },
  pending: { label: "Waiting for the provider", tone: "warning" },
  failed: { label: "Not verified", tone: "danger" },
};

export function IdentityChecks({ proofs, providerName, empty }: { proofs: IdentityProof[]; providerName: string; empty: string }) {
  if (proofs.length === 0) return <EmptyState icon={IdCard} title="No identity checks" description={empty} />;
  return (
    <SimpleTable
      caption="Identity checks"
      head={["Result", "Method", "Details", "Date"]}
      rows={proofs.map((p) => [
        <Badge key="s" tone={PROOF_STATUS[p.status].tone}>
          {PROOF_STATUS[p.status].label}
        </Badge>,
        p.method === "in-person" ? `In person by ${p.checkedBy?.name ?? "a deleted administrator"}` : p.provider === "stripe" ? "Stripe Identity" : providerName || "Verification provider",
        p.method === "in-person" ? (
          <span key="d" className="flex flex-col">
            <span className="text-fg">{p.document ? DOCUMENTS[p.document] : "—"}</span>
            <span className="text-caption text-fg-3">{p.note}</span>
          </span>
        ) : (
          <span key="d" className="font-mono text-caption text-fg-3">
            {p.reference}
          </span>
        ),
        formatDateTime(p.completedAt ?? p.createdAt),
      ])}
    />
  );
}

export function CheckIdentityButton({ name, endpoint }: { name: string; endpoint: string }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const formId = useId();
  const [open, setOpen] = useState(false);
  const [document, setDocument] = useState<keyof typeof DOCUMENTS>("passport");
  const [note, setNote] = useState("");
  const [attempted, setAttempted] = useState(false);
  const noteError = !note.trim()
    ? "Describe how you checked them, for example where you met and what you compared."
    : /[0-9]{5,}/.test(note)
      ? "Leave the document number out. Halo records which kind of document you checked, not its number."
      : note.trim().length > 500
        ? "Keep the note to 500 characters."
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
        Check identity
      </Button>
      <Dialog
        open={open}
        onOpenChange={close}
        error={errorFor("check")}
        title={`Check ${name}'s identity`}
        description={`Confirm that you checked ${name} in person or on a video call against an official document with their photo. Halo records your name, the kind of document and your note, and writes it to the audit log.`}
        footer={
          <>
            <Button variant="secondary" onClick={close}>
              Cancel
            </Button>
            <Button variant="primary" type="submit" form={formId} loading={pending === "check"}>
              Confirm identity
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
                toast({ title: `${name}'s identity confirmed`, description: "Recorded with your name in the audit log." });
              },
              "inline",
            );
          }}
        >
          <Field label="Document you checked">
            {(props) => (
              <Select {...props} value={document} onChange={(event) => setDocument(event.target.value as keyof typeof DOCUMENTS)}>
                {(Object.keys(DOCUMENTS) as (keyof typeof DOCUMENTS)[]).map((key) => (
                  <option key={key} value={key}>
                    {DOCUMENTS[key]}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label="How you checked" hint="Where you met and what you compared. Leave out the document number." error={attempted ? noteError : undefined}>
            {(props) => <Textarea {...props} rows={3} value={note} onChange={(event) => setNote(event.target.value)} placeholder="Met at the Lisbon office and compared the photo" />}
          </Field>
        </form>
      </Dialog>
    </>
  );
}
