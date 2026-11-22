"use client";

import { Trash2 } from "lucide-react";
import { useRef } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ActionError } from "@/components/ui/action-error";
import { Avatar } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api/client";
import { toApiError } from "@/lib/api/error";
import { useMessages } from "@/lib/i18n/client";
import { frameMessages } from "@/lib/i18n/console/frame";

const TYPES = ["image/png", "image/jpeg", "image/webp"];
const MAX_BYTES = 256 * 1024;

export function OrgLogo({ name, logoUrl, canEdit }: { name: string; logoUrl: string | null; canEdit: boolean }) {
  const { pending, run, errorFor, toast } = useMutation();
  const input = useRef<HTMLInputElement>(null);
  const copy = useMessages(frameMessages).logo;

  function upload(file: File) {
    if (!TYPES.includes(file.type)) {
      toast({ title: copy.badFile, description: copy.chooseFile, tone: "danger" });
      return;
    }
    if (file.size > MAX_BYTES) {
      toast({ title: copy.tooLarge, description: copy.tooLargeHint, tone: "danger" });
      return;
    }
    void run(
      "upload",
      async () => {
        const res = await fetch("/api/v1/branding/logo", { method: "PUT", body: file, credentials: "same-origin" });
        if (!res.ok) throw await toApiError(res, copy.uploadFailed);
      },
      () => toast({ title: copy.updated, description: copy.updatedHint }),
      "inline",
    );
  }

  const mark = logoUrl ? (
    <img src={logoUrl} alt="" className="size-12 shrink-0 rounded-md border border-border bg-n800 object-contain" />
  ) : (
    <Avatar name={name} size="lg" className="rounded-md" />
  );

  return (
    <div className="flex min-w-0 items-center gap-4">
      {canEdit ? (
        <button
          type="button"
          aria-label={logoUrl ? copy.changeLabel : copy.addLabel}
          disabled={pending !== null}
          onClick={() => input.current?.click()}
          className="rounded-md transition-colors duration-fast hover:opacity-80"
        >
          {mark}
        </button>
      ) : (
        mark
      )}
      <div className="flex min-w-0 flex-col gap-1">
        <h2 className="min-w-0 truncate text-h3 text-fg">{name}</h2>
        {canEdit ? (
          <div className="flex items-center gap-1">
            <input
              ref={input}
              type="file"
              accept={TYPES.join(",")}
              className="sr-only"
              tabIndex={-1}
              onChange={(event) => {
                const file = event.target.files?.[0];
                event.target.value = "";
                if (file) upload(file);
              }}
            />
            <Button variant="quiet" size="sm" loading={pending === "upload"} onClick={() => input.current?.click()}>
              {logoUrl ? copy.change : copy.add}
            </Button>
            {logoUrl ? (
              <Button
                variant="quiet"
                size="sm"
                loading={pending === "remove"}
                aria-label={copy.removeLabel}
                onClick={() => void run("remove", () => api("/branding/logo", { method: "DELETE" }), () => toast({ title: copy.removed }), "inline")}
              >
                <Trash2 aria-hidden="true" size={14} strokeWidth={1.75} />
                {copy.remove}
              </Button>
            ) : null}
          </div>
        ) : null}
        <ActionError message={errorFor("upload") ?? errorFor("remove")} />
      </div>
    </div>
  );
}
