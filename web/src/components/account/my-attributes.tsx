"use client";

import { useId, useState, type FormEvent } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { AttributeFields, draftsOf, parseDrafts } from "@/components/directory/attribute-fields";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { api } from "@/lib/api/client";
import type { Attribute, AttributeValue } from "@/lib/types";

export function MyAttributesButton({ attributes, values }: { attributes: Attribute[]; values: Record<string, AttributeValue> }) {
  const formId = useId();
  const [open, setOpen] = useState(false);
  const [drafts, setDrafts] = useState(() => draftsOf(attributes, values));
  const [errors, setErrors] = useState<Record<string, string>>({});
  const { pending, run, errorFor, clearError, toast } = useMutation();

  function submit(event: FormEvent) {
    event.preventDefault();
    const parsed = parseDrafts(attributes, drafts);
    setErrors(parsed.errors);
    if (Object.keys(parsed.errors).length > 0) return;
    void run(
      "attributes",
      () => api("/me/attributes", { method: "PATCH", body: parsed.values }),
      () => {
        setOpen(false);
        toast({ title: "Details saved", description: "Applications receive the new values the next time you sign in to them." });
      },
      "inline",
    );
  }

  return (
    <>
      <Button
        size="sm"
        variant="secondary"
        className="self-start"
        onClick={() => {
          setDrafts(draftsOf(attributes, values));
          setErrors({});
          setOpen(true);
        }}
      >
        Edit your details
      </Button>
      <Dialog
        open={open}
        onOpenChange={(next) => {
          clearError();
          setOpen(next);
        }}
        error={errorFor("attributes")}
        title="Edit your details"
        description="Your administrators let you keep these up to date. Applications that use them see the change the next time you sign in."
        footer={
          <>
            <Button variant="secondary" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" form={formId} variant="primary" loading={pending === "attributes"}>
              Save details
            </Button>
          </>
        }
      >
        <form id={formId} className="flex flex-col gap-6" onSubmit={submit} noValidate>
          <AttributeFields attributes={attributes} drafts={drafts} errors={errors} onChange={setDrafts} />
        </form>
      </Dialog>
    </>
  );
}
