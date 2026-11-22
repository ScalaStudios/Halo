"use client";

import { useId, useState, type FormEvent } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { AttributeFields, draftsOf, parseDrafts } from "@/components/directory/attribute-fields";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { api } from "@/lib/api/client";
import { accountMessages } from "@/lib/i18n/account";
import { authMessages } from "@/lib/i18n/auth";
import type { Locale } from "@/lib/i18n/locale";
import type { Attribute, AttributeValue } from "@/lib/types";

export function MyAttributesButton({ attributes, values, locale }: { attributes: Attribute[]; values: Record<string, AttributeValue>; locale: Locale }) {
  const copy = accountMessages(locale);
  const formId = useId();
  const [open, setOpen] = useState(false);
  const [drafts, setDrafts] = useState(() => draftsOf(attributes, values));
  const [errors, setErrors] = useState<Record<string, string>>({});
  const { pending, run, errorFor, clearError, toast } = useMutation();

  function submit(event: FormEvent) {
    event.preventDefault();
    const parsed = parseDrafts(attributes, drafts, locale);
    setErrors(parsed.errors);
    if (Object.keys(parsed.errors).length > 0) return;
    void run(
      "attributes",
      () => api("/me/attributes", { method: "PATCH", body: parsed.values }),
      () => {
        setOpen(false);
        toast({ title: copy.detailsSaved, description: copy.detailsSavedHint });
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
        {copy.editDetails}
      </Button>
      <Dialog
        open={open}
        onOpenChange={(next) => {
          clearError();
          setOpen(next);
        }}
        error={errorFor("attributes")}
        lang={locale}
        closeLabel={copy.close}
        title={copy.editDetails}
        description={copy.editDetailsHint}
        footer={
          <>
            <Button variant="secondary" onClick={() => setOpen(false)}>
              {authMessages(locale).cancel}
            </Button>
            <Button type="submit" form={formId} variant="primary" loading={pending === "attributes"}>
              {copy.saveDetails}
            </Button>
          </>
        }
      >
        <form lang="en" id={formId} className="flex flex-col gap-6" onSubmit={submit} noValidate>
          <AttributeFields attributes={attributes} drafts={drafts} errors={errors} onChange={setDrafts} />
        </form>
      </Dialog>
    </>
  );
}
