"use client";

import { ImageUp, Trash2 } from "lucide-react";
import { useRef } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { HaloLogo } from "@/components/halo-mark";
import { ActionError } from "@/components/ui/action-error";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { api } from "@/lib/api/client";
import { toApiError } from "@/lib/api/error";
import { useMessages } from "@/lib/i18n/client";
import { settingsMessages } from "@/lib/i18n/console/settings";
import type { OrganizationProfile } from "@/lib/settings-types";
import { ReadOnlyNote } from "./shared";

const TYPES = ["image/png", "image/jpeg", "image/webp"];
const MAX_BYTES = 256 * 1024;

export function LogoEditor({ org, canEdit }: { org: OrganizationProfile; canEdit: boolean }) {
  const { pending, run, errorFor, toast } = useMutation();
  const input = useRef<HTMLInputElement>(null);
  const t = useMessages(settingsMessages).logo;

  function upload(file: File) {
    if (!TYPES.includes(file.type)) {
      toast({ title: t.wrongType, description: t.chooseFile, tone: "danger" });
      return;
    }
    if (file.size > MAX_BYTES) {
      toast({ title: t.tooLarge, description: t.tooLargeHint(Math.ceil(file.size / 1024)), tone: "danger" });
      return;
    }
    void run(
      "upload",
      async () => {
        const res = await fetch("/api/v1/branding/logo", { method: "PUT", body: file, credentials: "same-origin" });
        if (!res.ok) throw await toApiError(res, t.storeFailed);
      },
      () => toast({ title: t.uploaded, description: t.uploadedHint }),
      "inline",
    );
  }

  return (
    <Card>
      <CardHeader title={t.title} description={t.description} />
      <CardBody className="flex flex-col gap-6">
        <div className="grid h-40 place-items-center rounded-md border border-border bg-sunken px-6">
          {org.logoUrl ? (
            <span className="flex flex-col items-center gap-3">
              <img src={org.logoUrl} alt={t.alt(org.name)} className="h-12 w-auto max-w-[240px] object-contain" />
              <span className="text-h4 text-fg">{org.name}</span>
            </span>
          ) : (
            <span className="flex flex-col items-center gap-3">
              <HaloLogo size="lg" />
              <span className="text-caption text-fg-3">{t.empty}</span>
            </span>
          )}
        </div>
        {canEdit ? (
          <div className="flex flex-wrap items-center gap-3">
            <input
              ref={input}
              type="file"
              accept={TYPES.join(",")}
              className="sr-only"
              tabIndex={-1}
              aria-hidden="true"
              onChange={(event) => {
                const file = event.target.files?.[0];
                event.target.value = "";
                if (file) upload(file);
              }}
            />
            <Button variant="primary" loading={pending === "upload"} onClick={() => input.current?.click()}>
              <ImageUp aria-hidden="true" size={16} strokeWidth={1.75} />
              {org.logoUrl ? t.replace : t.upload}
            </Button>
            {org.logoUrl ? (
              <Button
                variant="secondary"
                loading={pending === "remove"}
                onClick={() => void run("remove", () => api("/branding/logo", { method: "DELETE" }), () => toast({ title: t.removed, description: t.removedHint }), "inline")}
              >
                <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
                {t.remove}
              </Button>
            ) : null}
            <span className="text-caption text-fg-3">{t.formats}</span>
            <ActionError message={errorFor("upload") ?? errorFor("remove")} className="basis-full" />
          </div>
        ) : (
          <ReadOnlyNote>{t.readOnly}</ReadOnlyNote>
        )}
      </CardBody>
    </Card>
  );
}
