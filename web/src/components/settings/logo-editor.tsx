"use client";

import { ImageUp, Trash2 } from "lucide-react";
import { useRef } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { HaloLogo } from "@/components/halo-mark";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { api } from "@/lib/api/client";
import { toApiError } from "@/lib/api/error";
import type { OrganizationProfile } from "@/lib/settings-types";
import { ReadOnlyNote } from "./shared";

const TYPES = ["image/png", "image/jpeg", "image/webp"];
const MAX_BYTES = 256 * 1024;

export function LogoEditor({ org, canEdit }: { org: OrganizationProfile; canEdit: boolean }) {
  const { pending, run, toast } = useMutation();
  const input = useRef<HTMLInputElement>(null);

  function upload(file: File) {
    if (!TYPES.includes(file.type)) {
      toast({ title: "That file can't be used", description: "Choose a PNG, JPEG or WebP image. SVG is not accepted.", tone: "danger" });
      return;
    }
    if (file.size > MAX_BYTES) {
      toast({ title: "That image is too large", description: `It is ${Math.ceil(file.size / 1024)} KB. Export a version of 256 KB or less.`, tone: "danger" });
      return;
    }
    void run(
      "upload",
      async () => {
        const res = await fetch("/api/v1/branding/logo", { method: "PUT", body: file, credentials: "same-origin" });
        if (!res.ok) throw await toApiError(res, "Halo could not store the logo. Try again.");
      },
      () => toast({ title: "Logo uploaded", description: "The sign-in and setup pages show it from now on." }),
    );
  }

  return (
    <Card>
      <CardHeader title="Logo" description="Shown with your organization name above the sign-in and setup forms. Halo's own mark stays in the footer." />
      <CardBody className="flex flex-col gap-6">
        <div className="grid h-40 place-items-center rounded-md border border-border bg-sunken px-6">
          {org.logoUrl ? (
            <span className="flex flex-col items-center gap-3">
              <img src={org.logoUrl} alt={`${org.name} logo`} className="h-12 w-auto max-w-[240px] object-contain" />
              <span className="text-h4 text-fg">{org.name}</span>
            </span>
          ) : (
            <span className="flex flex-col items-center gap-3">
              <HaloLogo size="lg" />
              <span className="text-caption text-fg-3">No logo yet. The sign-in page shows the Halo logo.</span>
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
              {org.logoUrl ? "Replace logo" : "Upload logo"}
            </Button>
            {org.logoUrl ? (
              <Button
                variant="secondary"
                loading={pending === "remove"}
                onClick={() => void run("remove", () => api("/branding/logo", { method: "DELETE" }), () => toast({ title: "Logo removed", description: "The sign-in page shows the Halo logo again." }))}
              >
                <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
                Remove logo
              </Button>
            ) : null}
            <span className="text-caption text-fg-3">PNG, JPEG or WebP, up to 256 KB. Shown 48 pixels tall.</span>
          </div>
        ) : (
          <ReadOnlyNote>Only a global administrator can change the logo.</ReadOnlyNote>
        )}
      </CardBody>
    </Card>
  );
}
